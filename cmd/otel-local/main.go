// Command otel-local runs a local OpenTelemetry trace collector for dear-agent
// development, with no Docker required. It launches the native Jaeger v2
// all-in-one binary, which exposes:
//
//   - OTLP gRPC receiver on :4317  (point OTEL_EXPORTER_OTLP_ENDPOINT here)
//   - OTLP HTTP receiver on :4318
//   - Jaeger UI + query API on :16686
//
// Tracing in dear-agent is opt-in: instrumented binaries (agm, safe-pr,
// safe-merge, vroom-dispatch, wayfinder, engram) install a no-op tracer unless
// OTEL_EXPORTER_OTLP_ENDPOINT is set. This tool stands up the collector that
// makes that env var meaningful, then prints the exact line to export.
//
// Usage:
//
//	otel-local up            # locate Jaeger, run it in the foreground (Ctrl-C to stop)
//	otel-local up --detach   # run Jaeger in the background, then exit
//	otel-local up --fetch    # download the pinned Jaeger release if not present
//	otel-local down          # stop a --detach'd Jaeger started by this tool
//	otel-local env           # print the export line for a running collector
//
// Jaeger binary resolution order:
//
//  1. $JAEGER_BINARY (explicit override)
//  2. <user cache dir>/dear-agent/jaeger/jaeger (where --fetch installs it):
//     ~/Library/Caches on macOS, ~/.cache on Linux
//  3. "jaeger" on $PATH
//
// If none is found, otel-local prints the download command for this platform.
package main

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// jaegerVersion is the pinned Jaeger v2 release that --fetch downloads. It is a
// single, audit-legible constant rather than "latest" so a fetch is
// reproducible and the verified checksum is meaningful.
const jaegerVersion = "v2.19.0"

const (
	otlpGRPCPort = 4317
	jaegerUIPort = 16686
)

// otlpEndpoint is the value instrumented binaries should export. The scheme is
// mandatory: otlptracegrpc maps a scheme-less "localhost:4317" to an empty
// target and silently exports nothing (see pkg/otelsetup).
var otlpEndpoint = fmt.Sprintf("http://localhost:%d", otlpGRPCPort)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintf(os.Stderr, "otel-local: %v\n", err)
		os.Exit(1)
	}
}

func run(argv []string) error {
	cmd := "up"
	if len(argv) > 0 {
		cmd = argv[0]
		argv = argv[1:]
	}
	switch cmd {
	case "up":
		return cmdUp(argv)
	case "down":
		return cmdDown()
	case "env":
		printEnv()
		return nil
	case "-h", "--help", "help":
		fmt.Print(usage)
		return nil
	default:
		return fmt.Errorf("unknown command %q\n\n%s", cmd, usage)
	}
}

// upFlags holds the parsed flags for the "up" command.
type upFlags struct {
	detach   bool
	fetch    bool
	version  string
	showHelp bool
}

func parseUpFlags(argv []string) (upFlags, error) {
	f := upFlags{version: jaegerVersion}
	for i := 0; i < len(argv); i++ {
		switch argv[i] {
		case "--detach", "-d":
			f.detach = true
		case "--fetch":
			f.fetch = true
		case "--version":
			if i+1 >= len(argv) {
				return f, fmt.Errorf("--version requires an argument (e.g. v2.19.0)")
			}
			f.version = argv[i+1]
			i++
		case "-h", "--help":
			f.showHelp = true
			return f, nil
		default:
			return f, fmt.Errorf("unknown flag %q for 'up'", argv[i])
		}
	}
	return f, nil
}

func cmdUp(argv []string) error {
	flags, err := parseUpFlags(argv)
	if err != nil {
		return err
	}
	if flags.showHelp {
		fmt.Print(usage)
		return nil
	}
	detach, fetch, ver := flags.detach, flags.fetch, flags.version

	bin, found := locateJaeger()
	if !found {
		if !fetch {
			return fmt.Errorf("no Jaeger binary found.\n\n%s", fetchHint(ver))
		}
		fmt.Fprintf(os.Stderr, "otel-local: fetching Jaeger %s for %s/%s…\n", ver, runtime.GOOS, runtime.GOARCH)
		path, err := fetchJaeger(context.Background(), ver)
		if err != nil {
			return fmt.Errorf("fetch failed: %w\n\n%s", err, fetchHint(ver))
		}
		bin = path
		fmt.Fprintf(os.Stderr, "otel-local: installed %s\n", bin)
	}

	if alive(context.Background()) {
		fmt.Fprintln(os.Stderr, "otel-local: a collector is already listening on the Jaeger UI port — reusing it.")
		printEnv()
		return nil
	}

	// bin is an operator-controlled path (JAEGER_BINARY, the dear-agent cache,
	// or a "jaeger" on PATH), not external/network input.
	//
	// Pass the managed config explicitly (OTEL-LOCAL-14). Without --config,
	// Jaeger falls back to its upstream defaults, which bind 0.0.0.0 on every
	// listener -- so the quick-start path everyone actually runs would expose
	// an unauthenticated collector and query API on every interface, while only
	// the launch agent got the loopback-pinned config. Same bytes, both paths.
	args := []string{}
	if cfg, ok := managedConfigPath(); ok {
		args = append(args, "--config", cfg)
	} else {
		fmt.Fprintf(os.Stderr, "otel-local: warning: no managed collector config at %s; "+
			"Jaeger will use its upstream defaults, which listen on all interfaces. "+
			"Install it with: make install-jaeger-launchagent\n", managedConfigLocation())
	}
	proc := exec.Command(bin, args...)
	// Jaeger writes its own startup logs to stderr; surface them prefixed so
	// they don't masquerade as otel-local output.
	proc.Stdout = prefixWriter{w: os.Stderr, prefix: "jaeger: "}
	proc.Stderr = prefixWriter{w: os.Stderr, prefix: "jaeger: "}
	// New process group so a detached Jaeger is not killed when this shell exits.
	proc.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	if err := proc.Start(); err != nil {
		return fmt.Errorf("starting Jaeger: %w", err)
	}

	fmt.Fprintln(os.Stderr, "otel-local: waiting for Jaeger to come up…")
	if err := waitHealthy(30 * time.Second); err != nil {
		_ = proc.Process.Kill()
		return fmt.Errorf("collector did not become healthy: %w", err)
	}

	fmt.Fprintln(os.Stderr, "otel-local: ✓ collector ready")
	fmt.Fprintf(os.Stderr, "  Jaeger UI:  http://localhost:%d\n", jaegerUIPort)
	printEnv()

	if detach {
		if err := writePidfile(proc.Process.Pid); err != nil {
			fmt.Fprintf(os.Stderr, "otel-local: warning: could not write pidfile: %v\n", err)
		}
		fmt.Fprintf(os.Stderr, "otel-local: Jaeger running detached (pid %d). Stop with: otel-local down\n", proc.Process.Pid)
		return nil
	}

	// Foreground: wait for Ctrl-C, then shut Jaeger down cleanly.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	fmt.Fprintln(os.Stderr, "otel-local: Ctrl-C to stop.")
	<-ctx.Done()
	fmt.Fprintln(os.Stderr, "\notel-local: stopping Jaeger…")
	_ = proc.Process.Signal(syscall.SIGTERM)
	done := make(chan error, 1)
	go func() {
		defer func() {
			if r := recover(); r != nil {
				fmt.Fprintf(os.Stderr, "otel-local: wait goroutine panic: %v\n", r)
				done <- fmt.Errorf("wait goroutine panic: %v", r)
			}
		}()
		done <- proc.Wait()
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		_ = proc.Process.Kill()
	}
	return nil
}

func cmdDown() error {
	pid, err := readPidfile()
	if err != nil {
		return fmt.Errorf("no detached Jaeger recorded (%w); if you started it in the foreground, use Ctrl-C", err)
	}
	if err := syscall.Kill(pid, syscall.SIGTERM); err != nil {
		_ = os.Remove(pidfilePath())
		return fmt.Errorf("signalling pid %d: %w (already stopped?)", pid, err)
	}
	_ = os.Remove(pidfilePath())
	fmt.Fprintf(os.Stderr, "otel-local: sent SIGTERM to Jaeger (pid %d)\n", pid)
	return nil
}

func printEnv() {
	fmt.Printf("export OTEL_EXPORTER_OTLP_ENDPOINT=%s\n", otlpEndpoint)
}

// --- Jaeger binary resolution ---

func locateJaeger() (string, bool) {
	if p := os.Getenv("JAEGER_BINARY"); p != "" && isExecutable(p) {
		return p, true
	}
	if c := cacheBinPath(); isExecutable(c) {
		return c, true
	}
	if p, err := exec.LookPath("jaeger"); err == nil {
		return p, true
	}
	return "", false
}

func isExecutable(p string) bool {
	fi, err := os.Stat(p)
	return err == nil && !fi.IsDir() && fi.Mode()&0o111 != 0
}

func cacheDir() string {
	base, err := os.UserCacheDir()
	if err != nil {
		base = filepath.Join(os.Getenv("HOME"), ".cache")
	}
	return filepath.Join(base, "dear-agent", "jaeger")
}

func cacheBinPath() string { return filepath.Join(cacheDir(), "jaeger") }

// managedConfigLocation is where dear-deploy stages deploy/jaeger/config.yaml.
// It is the same path the launch agent's plist passes to --config, so the
// foreground and launchd collectors run identical configuration.
func managedConfigLocation() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, "Library", "Application Support", "Jaeger", "config.yaml")
}

// managedConfigPath returns the staged collector config when it is readable.
func managedConfigPath() (string, bool) {
	path := managedConfigLocation()
	if path == "" {
		return "", false
	}
	if info, err := os.Stat(path); err != nil || info.IsDir() {
		return "", false
	}
	return path, true
}
func pidfilePath() string { return filepath.Join(cacheDir(), "jaeger.pid") }

func fetchHint(ver string) string {
	asset := assetName(ver, runtime.GOOS, runtime.GOARCH)
	return fmt.Sprintf(`To install a local Jaeger collector, either:

  otel-local up --fetch          # download + verify %s and run it

or fetch it manually:

  curl -fsSLO https://github.com/jaegertracing/jaeger/releases/download/%s/%s
  tar xzf %s
  mkdir -p %s
  cp %s/jaeger %s

Then re-run: otel-local up`,
		asset, ver, asset, asset, cacheDir(), strings.TrimSuffix(asset, ".tar.gz"), cacheBinPath())
}

// --- health checks ---

func alive(ctx context.Context) bool {
	return queryServices(ctx) == nil
}

func waitHealthy(timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	var last error
	for time.Now().Before(deadline) {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		last = queryServices(ctx)
		cancel()
		if last == nil {
			return nil
		}
		time.Sleep(500 * time.Millisecond)
	}
	return fmt.Errorf("timed out after %s: %w", timeout, last)
}

func queryServices(ctx context.Context) error {
	// Fixed localhost collector URL — not user input.
	url := fmt.Sprintf("http://localhost:%d/api/services", jaegerUIPort)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("GET /api/services returned %d", resp.StatusCode)
	}
	return nil
}

// --- pidfile ---

func writePidfile(pid int) error {
	if err := os.MkdirAll(cacheDir(), 0o755); err != nil {
		return err
	}
	return os.WriteFile(pidfilePath(), []byte(strconv.Itoa(pid)), 0o600)
}

func readPidfile() (int, error) {
	b, err := os.ReadFile(pidfilePath())
	if err != nil {
		return 0, err
	}
	return strconv.Atoi(strings.TrimSpace(string(b)))
}

// --- fetch + verify + extract ---

// assetName returns the Jaeger release tarball name for a version + platform.
// Jaeger uses GOOS-GOARCH naming that matches Go's runtime values directly
// (e.g. darwin-arm64, linux-amd64).
func assetName(version, goos, goarch string) string {
	return fmt.Sprintf("jaeger-%s-%s-%s.tar.gz", strings.TrimPrefix(version, "v"), goos, goarch)
}

// checksumAssetName returns the published SHA-256 manifest name for a version +
// platform. Jaeger names it after the platform tuple with no archive extension
// (jaeger-2.19.0-darwin-arm64.sha256sum.txt); appending ".sha256sum.txt" to the
// tarball name instead yields a 404.
func checksumAssetName(version, goos, goarch string) string {
	return fmt.Sprintf("jaeger-%s-%s-%s.sha256sum.txt", strings.TrimPrefix(version, "v"), goos, goarch)
}

func fetchJaeger(ctx context.Context, version string) (string, error) {
	asset := assetName(version, runtime.GOOS, runtime.GOARCH)
	base := fmt.Sprintf("https://github.com/jaegertracing/jaeger/releases/download/%s/", version)

	tarball, err := download(ctx, base+asset)
	if err != nil {
		return "", fmt.Errorf("downloading %s: %w", asset, err)
	}
	sumAsset := checksumAssetName(version, runtime.GOOS, runtime.GOARCH)
	sumFile, err := download(ctx, base+sumAsset)
	if err != nil {
		return "", fmt.Errorf("downloading checksum: %w", err)
	}

	// The published manifest digests the files INSIDE the archive (entries look
	// like "<hex> *jaeger-2.19.0-darwin-arm64/jaeger"), not the archive itself.
	// So verify the extracted executable, which is also what we actually run.
	want, err := parseChecksum(string(sumFile), "jaeger")
	if err != nil {
		return "", err
	}

	dst := cacheBinPath()
	if err := installVerified(tarball, want, dst); err != nil {
		return "", fmt.Errorf("installing jaeger from %s: %w", asset, err)
	}
	return dst, nil
}

// installVerified extracts the `jaeger` binary from tarball, checks it against
// the want digest, and only then puts it at dst as an executable.
//
// Extraction goes to a sibling temp path first so a checksum failure can never
// leave an unverified binary at the path otel-local resolves and launches
// (OTEL-LOCAL-12).
//
// The staging path is unique per invocation (OTEL-LOCAL-13). A fixed
// "<dst>.incoming" name is shared state: two concurrent `otel-local up --fetch`
// runs would extract into the same inode, so one could verify its bytes and
// then rename what the other had since rewritten -- reporting success for
// content it never checked against its own manifest. Even two runs of the same
// version race, because whichever renames first removes the file the other is
// still reading.
func installVerified(tarball []byte, want, dst string) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	staged, err := os.CreateTemp(filepath.Dir(dst), filepath.Base(dst)+".incoming-*")
	if err != nil {
		return err
	}
	tmp := staged.Name()
	// extractBinary owns writing tmp; close our handle so it can recreate the
	// path without leaking a descriptor.
	if err := staged.Close(); err != nil {
		return err
	}
	defer func() { _ = os.Remove(tmp) }()
	if err := extractBinary(tarball, "jaeger", tmp); err != nil {
		return err
	}
	// tmp is a path only this invocation knows, inside our own cache dir.
	extracted, err := os.ReadFile(tmp)
	if err != nil {
		return err
	}
	got := sha256.Sum256(extracted)
	if gotHex := hex.EncodeToString(got[:]); gotHex != want {
		return fmt.Errorf("checksum mismatch: want %s, got %s", want, gotHex)
	}
	if err := os.Rename(tmp, dst); err != nil {
		return err
	}
	// The launcher binary must be executable.
	if err := os.Chmod(dst, 0o755); err != nil { //nolint:gosec // G302: an executable must be 0755
		return err
	}
	return nil
}

func download(ctx context.Context, url string) ([]byte, error) {
	// url is built from a pinned version + computed asset name against the
	// official Jaeger GitHub releases host — not arbitrary user input.
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	client := &http.Client{Timeout: 5 * time.Minute}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GET %s returned %d", url, resp.StatusCode)
	}
	return io.ReadAll(resp.Body)
}

// parseChecksum extracts the hex digest for asset from an sha256sum-format file
// ("<hex>  <filename>" lines). It matches on the basename so a path-qualified
// entry still resolves.
func parseChecksum(contents, asset string) (string, error) {
	for line := range strings.SplitSeq(contents, "\n") {
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		// sha256sum marks binary-mode entries with a leading "*" on the
		// filename; strip it so a flat "*jaeger" entry still resolves.
		if strings.TrimPrefix(filepath.Base(fields[len(fields)-1]), "*") == asset {
			return fields[0], nil
		}
	}
	return "", fmt.Errorf("no checksum entry for %s", asset)
}

// extractBinary reads a gzip-compressed tar archive and writes the first regular
// file whose basename equals wantBase to dst.
func extractBinary(tarball []byte, wantBase, dst string) error {
	gz, err := gzip.NewReader(strings.NewReader(string(tarball)))
	if err != nil {
		return fmt.Errorf("gzip: %w", err)
	}
	defer gz.Close()
	tr := tar.NewReader(gz)
	for {
		hdr, err := tr.Next()
		if errors.Is(err, io.EOF) {
			return fmt.Errorf("%q not found in archive", wantBase)
		}
		if err != nil {
			return fmt.Errorf("tar: %w", err)
		}
		if hdr.Typeflag != tar.TypeReg || filepath.Base(hdr.Name) != wantBase {
			continue
		}
		out, err := os.Create(dst)
		if err != nil {
			return err
		}
		// Bound the copy to the declared size to satisfy decompression-bomb linters.
		if _, err := io.CopyN(out, tr, hdr.Size); err != nil && !errors.Is(err, io.EOF) {
			_ = out.Close()
			return err
		}
		return out.Close()
	}
}

// prefixWriter prefixes each write with a fixed string so multiplexed child
// output is attributable. It is line-oriented enough for human logs.
type prefixWriter struct {
	w      io.Writer
	prefix string
}

func (p prefixWriter) Write(b []byte) (int, error) {
	if _, err := io.WriteString(p.w, p.prefix); err != nil {
		return 0, err
	}
	return p.w.Write(b)
}

const usage = `otel-local — run a local Jaeger trace collector for dear-agent (no Docker).

Tracing is opt-in: instrumented binaries (agm, safe-pr, safe-merge,
vroom-dispatch, wayfinder, engram) only export spans when
OTEL_EXPORTER_OTLP_ENDPOINT is set. This tool runs the collector and prints
the line to export.

Usage:
  otel-local up [--detach] [--fetch] [--version vX.Y.Z]
  otel-local down
  otel-local env

Commands:
  up      Launch Jaeger (OTLP gRPC :4317, UI :16686). Foreground by default;
          --detach backgrounds it and exits. --fetch downloads + checksum-
          verifies the pinned release (` + jaegerVersion + `) if no binary is found.
  down    Stop a --detach'd Jaeger started by this tool.
  env     Print: export OTEL_EXPORTER_OTLP_ENDPOINT=` + "http://localhost:4317" + `

Binary resolution: $JAEGER_BINARY → ~/.cache/dear-agent/jaeger/jaeger → $PATH.

Then, in the shell where you run an instrumented binary:
  eval "$(otel-local env)"
  agm session new …            # spans now flow to Jaeger
  open http://localhost:16686  # view traces
`
