package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// buildBinary compiles agm-bus once per test run and returns the path.
// Uses a short /tmp dir to keep the eventual socket path under macOS's
// ~104-byte unix-socket limit.
func buildBinary(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp("/tmp", "agmbus-*") //nolint:usetesting // socket path-length constraint
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	bin := filepath.Join(dir, "agm-bus")
	cmd := exec.Command("go", "build", "-o", bin, ".")
	cmd.Env = append(os.Environ(), "GOWORK=off")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("go build: %v\n%s", err, out)
	}
	return bin
}

// TestCLISocket verifies `agm-bus socket` prints a sane path.
func TestCLISocket(t *testing.T) {
	bin := buildBinary(t)
	out, err := exec.Command(bin, "socket").Output()
	if err != nil {
		t.Fatalf("exec: %v", err)
	}
	got := strings.TrimSpace(string(out))
	if !strings.HasSuffix(got, ".agm/bus.sock") && !strings.HasSuffix(got, "/bus.sock") {
		t.Errorf("socket path = %q, want suffix /bus.sock", got)
	}
}

// TestCLIStatusNotRunning verifies status reports not-running when the
// socket file is absent.
func TestCLIStatusNotRunning(t *testing.T) {
	bin := buildBinary(t)
	nonexistent, err := os.MkdirTemp("/tmp", "agmbus-notrun-*") //nolint:usetesting // socket path-length constraint
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(nonexistent) })
	sock := filepath.Join(nonexistent, "nope")
	out, err := exec.Command(bin, "status", "-socket", sock).Output()
	if err != nil {
		t.Fatalf("exec: %v (%s)", err, out)
	}
	if !strings.Contains(string(out), "not running") {
		t.Errorf("status output = %q, want mention of 'not running'", string(out))
	}
}

// TestCLIServeShutdown starts the daemon, verifies it binds the socket, then
// SIGINTs it and confirms a clean shutdown with the socket file removed.
func TestCLIServeShutdown(t *testing.T) {
	bin := buildBinary(t)
	sockDir, err := os.MkdirTemp("/tmp", "agmbus-serve-*") //nolint:usetesting // socket path-length constraint
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(sockDir) })
	sock := filepath.Join(sockDir, "s")

	cmd := exec.Command(bin, "serve", "-socket", sock)
	cmd.Env = append(os.Environ(), "AGM_BUS_SOCKET="+sock)
	if err := cmd.Start(); err != nil {
		t.Fatalf("start: %v", err)
	}

	// Wait for the socket to show up.
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(sock); err == nil {
			break
		}
		time.Sleep(25 * time.Millisecond)
	}
	if _, err := os.Stat(sock); err != nil {
		_ = cmd.Process.Kill()
		t.Fatalf("socket never appeared: %v", err)
	}

	// Verify status subcommand sees it.
	out, _ := exec.Command(bin, "status", "-socket", sock).Output()
	if !strings.Contains(string(out), "listening on") {
		t.Errorf("status during serve = %q, want mention of 'listening on'", string(out))
	}

	// Send SIGINT and wait for clean exit.
	if err := cmd.Process.Signal(os.Interrupt); err != nil {
		t.Fatalf("signal: %v", err)
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case err := <-done:
		// Exit code may be 0 or nonzero depending on how signal-notify
		// context treated the cancel — both fine as long as we exited.
		_ = err
	case <-time.After(4 * time.Second):
		_ = cmd.Process.Kill()
		t.Fatal("serve did not exit within 4s of SIGINT")
	}

	// Socket should be removed.
	if _, err := os.Stat(sock); err == nil {
		t.Errorf("socket file still present after shutdown: %s", sock)
	}
}

// TestCLIUnknownSubcommand is a small ergonomics check.
func TestCLIUnknownSubcommand(t *testing.T) {
	bin := buildBinary(t)
	cmd := exec.Command(bin, "nonsense")
	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Error("expected exit error, got nil")
	}
	if !strings.Contains(string(out), "unknown subcommand") {
		t.Errorf("output = %q, want mention of 'unknown subcommand'", out)
	}
}
