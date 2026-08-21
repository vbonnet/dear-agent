package main

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/vbonnet/dear-agent/agm/internal/bus"
)

// shortTempDir returns a temp dir under /tmp rather than the Go test temp
// root, because a bound Unix socket path must stay under macOS's ~104-byte
// limit and t.TempDir() paths are long.
func shortTempDir(t *testing.T, prefix string) string {
	t.Helper()
	dir, err := os.MkdirTemp("/tmp", prefix) //nolint:usetesting // socket path-length constraint
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	return dir
}

// syncBuffer is a mutex-guarded log sink. Adapter goroutines outlive the test
// that started them (they stop when their context is cancelled, and some are
// deliberately started with an already-cancelled one), so an unsynchronized
// bytes.Buffer would race between a late log write and the test's read.
type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

// captureLogger returns a logger writing into a buffer the test can assert on,
// which is how the adapter degradation paths are observed: they log and carry
// on rather than returning an error.
func captureLogger(t *testing.T, verbose bool) (*syncBuffer, *slog.Logger) {
	t.Helper()
	buf := &syncBuffer{}
	return buf, newServeLogger(verbose, buf)
}

// offlineOptions returns serve options with every optional subsystem off, so a
// test exercises only what it names.
func offlineOptions(socket string) *serveOptions {
	return &serveOptions{
		socket:              socket,
		queueDir:            "off",
		aclPath:             "off",
		supervisorsDir:      "off",
		heartbeatStaleAfter: 5 * time.Minute,
		heartbeatInterval:   30 * time.Second,
	}
}

// TestParseServeFlagsDefaults pins the defaults every other test relies on.
func TestParseServeFlagsDefaults(t *testing.T) {
	opts, err := parseServeFlags(nil, &bytes.Buffer{})
	if err != nil {
		t.Fatalf("parseServeFlags: %v", err)
	}

	if opts.socket != "" || opts.queueDir != "" || opts.aclPath != "" || opts.supervisorsDir != "" {
		t.Errorf("path flags should default to empty (meaning 'use the default'), got %#v", opts)
	}
	if opts.verbose || opts.discordEnabled || opts.discordMultibot || opts.matrixEnabled {
		t.Errorf("every opt-in flag should default to false, got %#v", opts)
	}
	if opts.heartbeatStaleAfter != 5*time.Minute {
		t.Errorf("heartbeatStaleAfter = %v, want 5m", opts.heartbeatStaleAfter)
	}
	if opts.heartbeatInterval != 30*time.Second {
		t.Errorf("heartbeatInterval = %v, want 30s", opts.heartbeatInterval)
	}
}

// TestParseServeFlagsAll pins that every flag reaches its field, so a rename
// cannot silently drop one.
func TestParseServeFlagsAll(t *testing.T) {
	opts, err := parseServeFlags([]string{
		"-socket", "/tmp/s.sock",
		"-queue-dir", "off",
		"-acl", "/tmp/acl.yaml",
		"-verbose",
		"-discord", "-discord-token", "tok", "-discord-allowlist", "a,b",
		"-discord-multibot", "-discord-agents", "/tmp/agents.yaml",
		"-matrix", "-matrix-homeserver", "https://hs", "-matrix-token", "mt",
		"-matrix-user-id", "@bot:hs", "-matrix-room", "!room:hs", "-matrix-allowlist", "@a:hs",
		"-supervisors-dir", "/tmp/sup",
		"-heartbeat-stale-after", "90s",
		"-heartbeat-scan-interval", "3s",
	}, &bytes.Buffer{})
	if err != nil {
		t.Fatalf("parseServeFlags: %v", err)
	}

	want := serveOptions{
		socket: "/tmp/s.sock", queueDir: "off", aclPath: "/tmp/acl.yaml", verbose: true,
		discordEnabled: true, discordToken: "tok", discordAllowlist: "a,b",
		discordMultibot: true, discordAgentsPath: "/tmp/agents.yaml",
		matrixEnabled: true, matrixHomeserver: "https://hs", matrixToken: "mt",
		matrixUserID: "@bot:hs", matrixRoomID: "!room:hs", matrixAllowlist: "@a:hs",
		supervisorsDir: "/tmp/sup", heartbeatStaleAfter: 90 * time.Second, heartbeatInterval: 3 * time.Second,
	}
	if *opts != want {
		t.Errorf("parsed options = %#v, want %#v", *opts, want)
	}
}

// TestParseServeFlagsRejectsUnknown covers the usage-error path.
func TestParseServeFlagsRejectsUnknown(t *testing.T) {
	var out bytes.Buffer
	if _, err := parseServeFlags([]string{"-nope"}, &out); err == nil {
		t.Fatal("expected an error for an unknown flag")
	}
	if out.Len() == 0 {
		t.Error("expected usage output on the supplied writer")
	}
}

// TestSplitAllowlist covers the allowlist parsing that gates who may reach the
// bus through a chat adapter. An empty entry must never be admitted.
func TestSplitAllowlist(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want []string
	}{
		{name: "empty", in: "", want: nil},
		{name: "single", in: "abc", want: []string{"abc"}},
		{name: "trims spaces", in: " a , b ", want: []string{"a", "b"}},
		{name: "drops empties", in: "a,,b,", want: []string{"a", "b"}},
		{name: "only separators", in: " , , ", want: nil},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := splitAllowlist(tc.in)
			if len(got) != len(tc.want) {
				t.Fatalf("got %#v, want %#v", got, tc.want)
			}
			for i := range tc.want {
				if got[i] != tc.want[i] {
					t.Errorf("entry %d = %q, want %q", i, got[i], tc.want[i])
				}
			}
		})
	}
}

// TestNewServeLoggerVerbosity pins that -verbose is what admits debug records.
func TestNewServeLoggerVerbosity(t *testing.T) {
	for _, verbose := range []bool{false, true} {
		var buf bytes.Buffer
		newServeLogger(verbose, &buf).Debug("probe")
		got := strings.Contains(buf.String(), "probe")
		if got != verbose {
			t.Errorf("verbose=%v: debug record emitted=%v, want %v", verbose, got, verbose)
		}
	}
}

// TestExpandHome covers the ~/ expansion the CLI applies before handing paths
// to the bus library.
func TestExpandHome(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	got, err := expandHome("~/.agm/bus.sock")
	if err != nil {
		t.Fatalf("expandHome: %v", err)
	}
	if want := filepath.Join(home, ".agm/bus.sock"); got != want {
		t.Errorf("expandHome(~/...) = %q, want %q", got, want)
	}

	for _, in := range []string{"/absolute/path", "relative/path", "~notahome", "~"} {
		got, err := expandHome(in)
		if err != nil {
			t.Fatalf("expandHome(%q): %v", in, err)
		}
		if got != in {
			t.Errorf("expandHome(%q) = %q, want it unchanged", in, got)
		}
	}
}

// TestBuildServerAttachesQueueAndACL covers the default wiring: both optional
// subsystems come up from the configured paths.
func TestBuildServerAttachesQueueAndACL(t *testing.T) {
	dir := t.TempDir()
	buf, logger := captureLogger(t, false)

	opts := &serveOptions{
		socket:   filepath.Join(shortTempDir(t, "agmbus-build-"), "s"),
		queueDir: filepath.Join(dir, "queue"),
		aclPath:  filepath.Join(dir, "acl.yaml"),
	}

	srv, err := buildServer(opts, logger)
	if err != nil {
		t.Fatalf("buildServer: %v", err)
	}
	if srv.Queue == nil {
		t.Error("Queue was not attached")
	}
	if srv.ACL == nil {
		t.Error("ACL was not attached")
	}
	if _, ok := srv.ACL.(*bus.ReloadableACL); !ok {
		t.Errorf("ACL = %T, want *bus.ReloadableACL", srv.ACL)
	}
	if !strings.Contains(buf.String(), "offline queue enabled") {
		t.Errorf("queue attach was not logged: %s", buf.String())
	}
	if !strings.Contains(buf.String(), "ACL loaded") {
		t.Errorf("ACL attach was not logged: %s", buf.String())
	}
}

// TestBuildServerHonorsOff covers the escape hatch both subsystems provide,
// which the existing subprocess test depends on.
func TestBuildServerHonorsOff(t *testing.T) {
	buf, logger := captureLogger(t, false)
	opts := offlineOptions(filepath.Join(shortTempDir(t, "agmbus-off-"), "s"))

	srv, err := buildServer(opts, logger)
	if err != nil {
		t.Fatalf("buildServer: %v", err)
	}
	if srv.Queue != nil {
		t.Error("Queue attached despite -queue-dir off")
	}
	if srv.ACL != nil {
		t.Errorf("ACL attached despite -acl off: %T", srv.ACL)
	}
	for _, want := range []string{"offline queue disabled by flag", "ACL enforcement disabled by flag"} {
		if !strings.Contains(buf.String(), want) {
			t.Errorf("missing log %q in:\n%s", want, buf.String())
		}
	}
}

// TestBuildServerDefaultPathsUseHome covers the unflagged case: both defaults
// resolve under the caller's home rather than the process working directory.
func TestBuildServerDefaultPathsUseHome(t *testing.T) {
	home := shortTempDir(t, "agmbus-home-")
	t.Setenv("HOME", home)
	t.Setenv("AGM_BUS_SOCKET", filepath.Join(home, "s"))

	_, logger := captureLogger(t, false)
	srv, err := buildServer(&serveOptions{}, logger)
	if err != nil {
		t.Fatalf("buildServer: %v", err)
	}
	if srv.Queue == nil || srv.ACL == nil {
		t.Fatal("default paths should attach both the queue and the ACL")
	}
	if _, err := os.Stat(filepath.Join(home, ".agm", "bus-queue")); err != nil {
		t.Errorf("default queue dir was not created under HOME: %v", err)
	}
}

// TestBuildServerRejectsUnusableQueueDir covers the failure path: a queue dir
// that cannot be created must fail the build, not silently run queueless.
func TestBuildServerRejectsUnusableQueueDir(t *testing.T) {
	blocker := filepath.Join(t.TempDir(), "not-a-dir")
	if err := os.WriteFile(blocker, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}

	_, logger := captureLogger(t, false)
	opts := offlineOptions(filepath.Join(shortTempDir(t, "agmbus-badq-"), "s"))
	opts.queueDir = filepath.Join(blocker, "queue")

	if _, err := buildServer(opts, logger); err == nil {
		t.Fatal("expected an error for an uncreatable queue dir")
	} else if !strings.Contains(err.Error(), "init queue") {
		t.Errorf("error = %v, want it to name the queue init step", err)
	}
}

// TestStartDiscordAdapterDegradesWithoutToken covers AGMBUS-10: an enabled
// adapter with no token must disable Discord routing, not stop the broker.
func TestStartDiscordAdapterDegradesWithoutToken(t *testing.T) {
	t.Setenv("DISCORD_BOT_TOKEN", "")

	buf, logger := captureLogger(t, false)
	opts := offlineOptions("")
	opts.discordEnabled = true

	startDiscordAdapter(context.Background(), opts, &bus.Server{}, logger)

	if !strings.Contains(buf.String(), "no token provided") {
		t.Errorf("expected a token warning, got:\n%s", buf.String())
	}
}

// TestStartDiscordAdapterDisabledByDefault pins the default-off behavior.
func TestStartDiscordAdapterDisabledByDefault(t *testing.T) {
	buf, logger := captureLogger(t, false)
	startDiscordAdapter(context.Background(), offlineOptions(""), &bus.Server{}, logger)

	if !strings.Contains(buf.String(), "discord adapter disabled") {
		t.Errorf("expected the disabled notice, got:\n%s", buf.String())
	}
}

// TestStartMatrixAdapterDegradesOnMissingSettings covers AGMBUS-11 for each
// individually-missing connection setting.
func TestStartMatrixAdapterDegradesOnMissingSettings(t *testing.T) {
	tests := []struct {
		name       string
		homeserver string
		token      string
		room       string
		wantLog    string
	}{
		{name: "no homeserver", token: "t", room: "!r:hs", wantLog: "no homeserver"},
		{name: "no token", homeserver: "https://hs", room: "!r:hs", wantLog: "no token"},
		{name: "no room", homeserver: "https://hs", token: "t", wantLog: "no room"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("MATRIX_HOMESERVER", "")
			t.Setenv("MATRIX_ACCESS_TOKEN", "")
			t.Setenv("MATRIX_USER_ID", "")
			t.Setenv("MATRIX_ROOM_ID", "")

			buf, logger := captureLogger(t, false)
			opts := offlineOptions("")
			opts.matrixEnabled = true
			opts.matrixHomeserver = tc.homeserver
			opts.matrixToken = tc.token
			opts.matrixRoomID = tc.room

			startMatrixAdapter(context.Background(), opts, &bus.Server{}, logger)

			if !strings.Contains(buf.String(), tc.wantLog) {
				t.Errorf("expected a %q warning, got:\n%s", tc.wantLog, buf.String())
			}
		})
	}
}

// TestStartMatrixAdapterReadsEnvironment covers the env fallbacks, which are
// how the daemon is configured in the launchd unit.
func TestStartMatrixAdapterReadsEnvironment(t *testing.T) {
	t.Setenv("MATRIX_HOMESERVER", "https://hs.example")
	t.Setenv("MATRIX_ACCESS_TOKEN", "token")
	t.Setenv("MATRIX_USER_ID", "@bot:hs.example")
	t.Setenv("MATRIX_ROOM_ID", "!room:hs.example")

	buf, logger := captureLogger(t, false)
	opts := offlineOptions("")
	opts.matrixEnabled = true

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // the adapter goroutine exits immediately; we only assert on wiring
	startMatrixAdapter(ctx, opts, &bus.Server{}, logger)

	if !strings.Contains(buf.String(), "matrix adapter starting") {
		t.Errorf("expected the adapter to start from env settings, got:\n%s", buf.String())
	}
	if !strings.Contains(buf.String(), "!room:hs.example") {
		t.Errorf("expected the env room id in the start log, got:\n%s", buf.String())
	}
}

// TestStartMatrixAdapterDisabledByDefault pins the default-off behavior.
func TestStartMatrixAdapterDisabledByDefault(t *testing.T) {
	buf, logger := captureLogger(t, false)
	startMatrixAdapter(context.Background(), offlineOptions(""), &bus.Server{}, logger)

	if !strings.Contains(buf.String(), "matrix adapter disabled") {
		t.Errorf("expected the disabled notice, got:\n%s", buf.String())
	}
}

// TestStartMultiBotPortalDegradesOnBadConfig covers the portal's failure
// posture: a config it cannot load disables the portal, not the broker.
func TestStartMultiBotPortalDegradesOnBadConfig(t *testing.T) {
	buf, logger := captureLogger(t, false)
	opts := offlineOptions("")
	opts.discordMultibot = true
	opts.discordAgentsPath = filepath.Join(t.TempDir(), "missing.yaml")

	if err := startMultiBotPortal(context.Background(), opts, &bus.Server{}, logger); err != nil {
		t.Fatalf("a bad portal config must not fail the broker: %v", err)
	}
	if !strings.Contains(buf.String(), "portal disabled") {
		t.Errorf("expected the portal to be disabled, got:\n%s", buf.String())
	}
}

// TestStartMultiBotPortalDisabledByDefault pins the default-off behavior.
func TestStartMultiBotPortalDisabledByDefault(t *testing.T) {
	buf, logger := captureLogger(t, false)
	if err := startMultiBotPortal(context.Background(), offlineOptions(""), &bus.Server{}, logger); err != nil {
		t.Fatalf("startMultiBotPortal: %v", err)
	}
	if !strings.Contains(buf.String(), "discord-multibot portal disabled") {
		t.Errorf("expected the disabled notice, got:\n%s", buf.String())
	}
}

// TestStartHeartbeatWatcher covers AGMBUS-12 in both directions: off means no
// watcher, and a configured dir starts one carrying the flagged thresholds.
func TestStartHeartbeatWatcher(t *testing.T) {
	t.Run("off", func(t *testing.T) {
		buf, logger := captureLogger(t, false)
		if err := startHeartbeatWatcher(context.Background(), offlineOptions(""), &bus.Server{}, logger); err != nil {
			t.Fatalf("startHeartbeatWatcher: %v", err)
		}
		if !strings.Contains(buf.String(), "heartbeat watcher disabled by flag") {
			t.Errorf("expected the disabled notice, got:\n%s", buf.String())
		}
	})

	// The configured case asserts the watcher's own output, not just the
	// synchronous start log. Checking only the start line would still pass if
	// the watcher.Run goroutine were deleted, which is the failure this test
	// exists to catch.
	t.Run("configured", func(t *testing.T) {
		supervisors := shortTempDir(t, "agmbus-sup-")
		// A supervisor directory with no heartbeat file is reported as
		// "never", which is the cheapest way to make the watcher emit on its
		// first scan without waiting out a staleness threshold.
		if err := os.MkdirAll(filepath.Join(supervisors, "vroom-orchestrator"), 0o700); err != nil {
			t.Fatal(err)
		}

		buf, logger := captureLogger(t, false)
		opts := offlineOptions("")
		opts.supervisorsDir = supervisors
		opts.heartbeatStaleAfter = 42 * time.Second
		opts.heartbeatInterval = 7 * time.Second

		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()
		if err := startHeartbeatWatcher(ctx, opts, &bus.Server{}, logger); err != nil {
			t.Fatalf("startHeartbeatWatcher: %v", err)
		}

		if !strings.Contains(buf.String(), "heartbeat watcher started") {
			t.Fatalf("expected the watcher to start, got:\n%s", buf.String())
		}
		if !strings.Contains(buf.String(), "42s") || !strings.Contains(buf.String(), "7s") {
			t.Errorf("watcher did not carry the flagged thresholds:\n%s", buf.String())
		}

		// Run scans once immediately, so the emit shows up without waiting an
		// interval. There is no broker on the socket, so the emit itself
		// fails and the watcher logs that; either the emit line or the
		// emit-failed line proves the goroutine ran and read the directory.
		deadline := time.Now().Add(10 * time.Second)
		for time.Now().Before(deadline) {
			out := buf.String()
			if strings.Contains(out, "heartbeat event emitted") || strings.Contains(out, "emit failed") {
				return
			}
			time.Sleep(10 * time.Millisecond)
		}
		t.Errorf("watcher never scanned the supervisors dir:\n%s", buf.String())
	})
}

// TestStartAdaptersAllDisabled covers the composed path used by a minimal
// broker: every adapter opts out and the call still succeeds.
func TestStartAdaptersAllDisabled(t *testing.T) {
	buf, logger := captureLogger(t, false)
	if err := startAdapters(context.Background(), offlineOptions(""), &bus.Server{}, logger); err != nil {
		t.Fatalf("startAdapters: %v", err)
	}
	for _, want := range []string{
		"discord adapter disabled",
		"discord-multibot portal disabled",
		"matrix adapter disabled",
		"heartbeat watcher disabled by flag",
	} {
		if !strings.Contains(buf.String(), want) {
			t.Errorf("missing log %q in:\n%s", want, buf.String())
		}
	}
}

// TestWatchACLReloadNoOpWithoutReloadableACL covers the branch where SIGHUP
// handling is not installed, so the returned stop function is still safe.
func TestWatchACLReloadNoOpWithoutReloadableACL(t *testing.T) {
	_, logger := captureLogger(t, false)
	stop := watchACLReload(&bus.Server{}, logger)
	if stop == nil {
		t.Fatal("watchACLReload returned a nil stop function")
	}
	stop()
}

// TestWatchACLReloadOnSIGHUP covers AGMBUS-09 end to end: the handler is
// installed, a real SIGHUP reaches it, and the ACL is re-read.
//
// Installing the handler and immediately stopping it would assert nothing:
// that version passed with signal.Notify deleted. This one delivers the
// signal to the test process and waits for the reload to be logged.
func TestWatchACLReloadOnSIGHUP(t *testing.T) {
	aclPath := filepath.Join(t.TempDir(), "acl.yaml")
	rac, err := bus.NewReloadableACL(aclPath)
	if err != nil {
		t.Fatalf("NewReloadableACL: %v", err)
	}

	buf, logger := captureLogger(t, false)
	stop := watchACLReload(&bus.Server{ACL: rac}, logger)
	defer stop()

	proc, err := os.FindProcess(os.Getpid())
	if err != nil {
		t.Fatalf("FindProcess: %v", err)
	}
	if err := proc.Signal(syscall.SIGHUP); err != nil {
		t.Fatalf("delivering SIGHUP: %v", err)
	}

	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if strings.Contains(buf.String(), "acl reloaded") {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Errorf("SIGHUP did not trigger an ACL reload:\n%s", buf.String())
}

// TestServeBindsAndRemovesSocket covers AGMBUS-05 and AGMBUS-06 in-process:
// serve binds the configured socket for the lifetime of the context and
// removes the file on shutdown. The existing subprocess test asserts the same
// contract through a signal; this one asserts it through the context, which is
// what makes it countable as coverage.
func TestServeBindsAndRemovesSocket(t *testing.T) {
	sock := filepath.Join(shortTempDir(t, "agmbus-serve-inproc-"), "s")
	buf, logger := captureLogger(t, false)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- serve(ctx, offlineOptions(sock), logger) }()

	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(sock); err == nil {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if _, err := os.Stat(sock); err != nil {
		cancel()
		if serveErr := <-done; serveErr != nil && strings.Contains(serveErr.Error(), "operation not permitted") {
			t.Skipf("host sandbox denies Unix-socket binds: %v", serveErr)
		}
		t.Fatalf("socket never appeared: %v", err)
	}

	// serve must have run adapter startup, not just bound the socket. Each
	// adapter logs its own opt-out notice, so their absence means serve
	// skipped startAdapters entirely.
	for _, want := range []string{
		"discord adapter disabled",
		"discord-multibot portal disabled",
		"matrix adapter disabled",
		"heartbeat watcher disabled by flag",
	} {
		if !strings.Contains(buf.String(), want) {
			t.Errorf("serve did not bring up adapter startup, missing %q in:\n%s", want, buf.String())
		}
	}

	cancel()
	select {
	case err := <-done:
		if err != nil && !errors.Is(err, context.Canceled) {
			t.Fatalf("serve returned %v, want nil or context.Canceled", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("serve did not return within 10s of cancellation")
	}

	if _, err := os.Stat(sock); err == nil {
		t.Errorf("socket file still present after shutdown: %s", sock)
	}
}

// TestServeInstallsACLReload covers the other half of serve's orchestration:
// the SIGHUP reloader must be installed by the same path production uses, not
// only by a test calling watchACLReload directly. Without this, serve could
// drop the reloader and every test would stay green.
func TestServeInstallsACLReload(t *testing.T) {
	dir := shortTempDir(t, "agmbus-aclreload-")
	sock := filepath.Join(dir, "s")
	aclPath := filepath.Join(t.TempDir(), "acl.yaml")

	buf, logger := captureLogger(t, false)
	opts := offlineOptions(sock)
	opts.aclPath = aclPath

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- serve(ctx, opts, logger) }()

	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(sock); err == nil {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if _, err := os.Stat(sock); err != nil {
		cancel()
		<-done
		t.Skipf("broker never bound its socket on this host: %v", err)
	}

	proc, err := os.FindProcess(os.Getpid())
	if err != nil {
		t.Fatalf("FindProcess: %v", err)
	}
	if err := proc.Signal(syscall.SIGHUP); err != nil {
		t.Fatalf("delivering SIGHUP: %v", err)
	}

	reloaded := false
	deadline = time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if strings.Contains(buf.String(), "acl reloaded") {
			reloaded = true
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	cancel()
	<-done

	if !reloaded {
		t.Errorf("serve did not install the SIGHUP ACL reloader:\n%s", buf.String())
	}
}

// TestServePropagatesBuildFailure covers the path where setup fails before any
// socket is bound: serve must surface the error rather than start a broker
// missing the subsystem the operator asked for.
func TestServePropagatesBuildFailure(t *testing.T) {
	blocker := filepath.Join(t.TempDir(), "not-a-dir")
	if err := os.WriteFile(blocker, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}

	_, logger := captureLogger(t, false)
	opts := offlineOptions(filepath.Join(shortTempDir(t, "agmbus-servefail-"), "s"))
	opts.queueDir = filepath.Join(blocker, "queue")

	if err := serve(context.Background(), opts, logger); err == nil {
		t.Fatal("expected serve to fail when its queue cannot be built")
	}
}
