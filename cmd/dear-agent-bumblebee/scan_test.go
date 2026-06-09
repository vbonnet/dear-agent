package main

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestDefaultOutputPath_DatedFile(t *testing.T) {
	now := time.Date(2026, 5, 26, 4, 0, 0, 0, time.UTC)
	got, err := defaultOutputPath(now)
	if err != nil {
		t.Fatalf("defaultOutputPath: %v", err)
	}
	if !strings.HasSuffix(got, "2026-05-26.ndjson") {
		t.Fatalf("defaultOutputPath = %q, want suffix 2026-05-26.ndjson", got)
	}
	// Per-platform parent dir layout.
	switch runtime.GOOS {
	case "darwin":
		if !strings.Contains(got, "Library/Application Support/dear-agent/bumblebee") {
			t.Fatalf("darwin path %q missing Application Support segment", got)
		}
	case "linux":
		if !strings.Contains(got, "dear-agent/bumblebee") {
			t.Fatalf("linux path %q missing dear-agent/bumblebee segment", got)
		}
	}
}

func TestDefaultOutputPath_LinuxRespectsXDG(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("XDG_DATA_HOME only honored on linux branch")
	}
	dir := t.TempDir()
	t.Setenv("XDG_DATA_HOME", dir)
	got, err := defaultOutputPath(time.Now())
	if err != nil {
		t.Fatalf("defaultOutputPath: %v", err)
	}
	if !strings.HasPrefix(got, dir) {
		t.Fatalf("defaultOutputPath = %q, want prefix %q", got, dir)
	}
}

// TestOutputDir_AllBranches exercises every GOOS branch directly. This
// replaces what the GOOS-gated TestDefaultOutputPath_LinuxRespectsXDG
// silently skipped on the macOS dev box (2026-05-27 audit) — the XDG
// resolution now has coverage on every platform the package builds on.
func TestOutputDir_AllBranches(t *testing.T) {
	for _, tc := range []struct {
		name string
		goos string
		home string
		xdg  string
		want string
	}{
		{"darwin", "darwin", "/Users/alice", "/ignored", "/Users/alice/Library/Application Support/dear-agent/bumblebee"},
		{"linux-xdg-set", "linux", "/home/alice", "/data/xdg", "/data/xdg/dear-agent/bumblebee"},
		{"linux-xdg-empty-fallback", "linux", "/home/alice", "", "/home/alice/.local/share/dear-agent/bumblebee"},
		{"other-bsd-fallback", "freebsd", "/home/alice", "", "/home/alice/.dear-agent/bumblebee"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := outputDir(tc.goos, tc.home, tc.xdg); got != tc.want {
				t.Errorf("outputDir(%q, %q, %q) = %q, want %q", tc.goos, tc.home, tc.xdg, got, tc.want)
			}
		})
	}
}

func TestResolveBumblebeeBinary_PrefersEnv(t *testing.T) {
	dir := t.TempDir()
	bin := filepath.Join(dir, "bumblebee")
	if err := os.WriteFile(bin, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatalf("seed bin: %v", err)
	}
	t.Setenv("BUMBLEBEE_BIN", bin)
	t.Setenv("PATH", "")

	got, err := resolveBumblebeeBinary()
	if err != nil {
		t.Fatalf("resolveBumblebeeBinary: %v", err)
	}
	if got != bin {
		t.Fatalf("resolveBumblebeeBinary = %q, want %q", got, bin)
	}
}

func TestResolveBumblebeeBinary_NotFound(t *testing.T) {
	t.Setenv("BUMBLEBEE_BIN", "")
	t.Setenv("PATH", "")
	t.Setenv("HOME", t.TempDir())
	_, err := resolveBumblebeeBinary()
	if err == nil {
		t.Fatal("resolveBumblebeeBinary succeeded with no binary present; want error")
	}
	if !strings.Contains(err.Error(), "make bumblebee-install") {
		t.Fatalf("error %q should point at the install path", err)
	}
}

func TestDiscoverCatalog_EnvWins(t *testing.T) {
	dir := t.TempDir()
	cat := filepath.Join(dir, "catalog.json")
	if err := os.WriteFile(cat, []byte("{}"), 0o644); err != nil {
		t.Fatalf("seed: %v", err)
	}
	t.Setenv("BUMBLEBEE_CATALOG", cat)
	if got := discoverCatalog(); got != cat {
		t.Fatalf("discoverCatalog = %q, want %q", got, cat)
	}
}

func TestDiscoverCatalog_MissingEnvIgnored(t *testing.T) {
	t.Setenv("BUMBLEBEE_CATALOG", "/nonexistent/catalog.json")
	// Whatever discoverCatalog returns must not be the bogus env path.
	if got := discoverCatalog(); got == "/nonexistent/catalog.json" {
		t.Fatalf("discoverCatalog returned a non-existent env path: %q", got)
	}
}

// TestRunScan_HappyPathWithStubBinary exercises runScan end-to-end with
// a shell stub standing in for the Bumblebee binary. Covers the
// previously-0% runScan path: flag parse, binary resolve, mkdir, exec,
// atomic rename, and record count. Skip on Windows where the /bin/sh
// shim is not available.
func TestRunScan_HappyPathWithStubBinary(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell-script stub not portable to windows")
	}
	dir := t.TempDir()
	stub := filepath.Join(dir, "bumblebee")
	// Emit two NDJSON lines so countLines has something to count.
	body := "#!/bin/sh\nprintf '{\"a\":1}\\n{\"b\":2}\\n'\n"
	if err := os.WriteFile(stub, []byte(body), 0o755); err != nil {
		t.Fatalf("seed stub: %v", err)
	}
	t.Setenv("BUMBLEBEE_BIN", stub)
	t.Setenv("BUMBLEBEE_CATALOG", "")

	outPath := filepath.Join(dir, "out.ndjson")
	if err := runScan([]string{"--output", outPath}); err != nil {
		t.Fatalf("runScan: %v", err)
	}
	data, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("read out: %v", err)
	}
	if !strings.Contains(string(data), `"a":1`) || !strings.Contains(string(data), `"b":2`) {
		t.Errorf("unexpected scan output: %s", data)
	}
}

// TestRunScan_PropagatesBinaryError ensures a non-zero exit from the
// Bumblebee binary surfaces as an error and does NOT leave behind a
// half-written NDJSON file at the final outPath.
func TestRunScan_PropagatesBinaryError(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell-script stub not portable to windows")
	}
	dir := t.TempDir()
	stub := filepath.Join(dir, "bumblebee")
	if err := os.WriteFile(stub, []byte("#!/bin/sh\nexit 7\n"), 0o755); err != nil {
		t.Fatalf("seed stub: %v", err)
	}
	t.Setenv("BUMBLEBEE_BIN", stub)
	t.Setenv("BUMBLEBEE_CATALOG", "")

	outPath := filepath.Join(dir, "out.ndjson")
	err := runScan([]string{"--output", outPath})
	if err == nil {
		t.Fatal("runScan succeeded despite stub exit 7; want error")
	}
	if _, err := os.Stat(outPath); !os.IsNotExist(err) {
		t.Errorf("outPath should not exist on failure, stat err=%v", err)
	}
}

// TestRunScan_FlagParseError catches malformed args before any side
// effects. Cheap, no stub binary required.
func TestRunScan_FlagParseError(t *testing.T) {
	if err := runScan([]string{"--no-such-flag"}); err == nil {
		t.Fatal("runScan accepted an unknown flag")
	}
}

func TestPrintUsage_MentionsAllSubcommands(t *testing.T) {
	// printUsage drives `--help`; if a subcommand is added without
	// being documented here, this test forces the omission to surface.
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	printUsage(w)
	w.Close()
	buf := make([]byte, 4096)
	n, _ := r.Read(buf)
	got := string(buf[:n])
	for _, want := range []string{"install", "install-launchagent", "scan", "version"} {
		if !strings.Contains(got, want) {
			t.Errorf("usage output missing subcommand %q:\n%s", want, got)
		}
	}
}

func TestCountLines(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "x.ndjson")
	if err := os.WriteFile(path, []byte("a\nb\nc\n"), 0o644); err != nil {
		t.Fatalf("seed: %v", err)
	}
	n, err := countLines(path)
	if err != nil {
		t.Fatalf("countLines: %v", err)
	}
	if n != 3 {
		t.Fatalf("countLines = %d, want 3", n)
	}
}
