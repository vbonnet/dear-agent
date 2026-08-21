package main

import (
	"bytes"
	"context"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// captureStdout runs fn with os.Stdout redirected and returns what it printed.
// The subcommands report through fmt.Printf, so this is the only seam their
// user-visible output can be asserted through in-process.
func captureStdout(t *testing.T, fn func()) string {
	t.Helper()

	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	orig := os.Stdout
	os.Stdout = w
	defer func() { os.Stdout = orig }()

	done := make(chan string, 1)
	go func() {
		var buf bytes.Buffer
		_, _ = io.Copy(&buf, r)
		done <- buf.String()
	}()

	fn()
	_ = w.Close()
	out := <-done
	_ = r.Close()
	return out
}

// TestCmdSocketPrintsEffectivePath covers AGMBUS-02 in-process, including that
// AGM_BUS_SOCKET wins over the default.
func TestCmdSocketPrintsEffectivePath(t *testing.T) {
	t.Run("from environment", func(t *testing.T) {
		t.Setenv("AGM_BUS_SOCKET", "/tmp/agmbus-explicit.sock")

		out := captureStdout(t, func() {
			if err := cmdSocket(nil); err != nil {
				t.Errorf("cmdSocket: %v", err)
			}
		})

		if strings.TrimSpace(out) != "/tmp/agmbus-explicit.sock" {
			t.Errorf("cmdSocket printed %q, want the AGM_BUS_SOCKET value", out)
		}
	})

	t.Run("from home default", func(t *testing.T) {
		home := t.TempDir()
		t.Setenv("HOME", home)
		t.Setenv("AGM_BUS_SOCKET", "")

		out := captureStdout(t, func() {
			if err := cmdSocket(nil); err != nil {
				t.Errorf("cmdSocket: %v", err)
			}
		})

		want := filepath.Join(home, ".agm", "bus.sock")
		if strings.TrimSpace(out) != want {
			t.Errorf("cmdSocket printed %q, want %q", out, want)
		}
	})
}

// TestCmdSocketRejectsUnknownFlag covers the flag-error path.
func TestCmdSocketRejectsUnknownFlag(t *testing.T) {
	if err := cmdSocket([]string{"-nope"}); err == nil {
		t.Fatal("expected an error for an unknown flag")
	}
}

// TestCmdStatusSocketAbsent covers AGMBUS-03: a missing socket reports "not
// running" and returns success, because absence is a normal status outcome
// rather than a command failure.
func TestCmdStatusSocketAbsent(t *testing.T) {
	sock := filepath.Join(t.TempDir(), "absent.sock")

	out := captureStdout(t, func() {
		if err := cmdStatus([]string{"-socket", sock}); err != nil {
			t.Errorf("cmdStatus returned %v, want nil for an absent socket", err)
		}
	})

	if !strings.Contains(out, "not running") {
		t.Errorf("status output = %q, want it to report 'not running'", out)
	}
}

// TestCmdStatusSocketListening covers AGMBUS-04 against a real listener.
func TestCmdStatusSocketListening(t *testing.T) {
	dir := shortTempDir(t, "agmbus-status-")
	sock := filepath.Join(dir, "s")

	ln, err := net.Listen("unix", sock)
	if err != nil {
		t.Skipf("host denies Unix-socket binds: %v", err)
	}
	defer func() { _ = ln.Close() }()
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			_ = conn.Close()
		}
	}()

	out := captureStdout(t, func() {
		if err := cmdStatus([]string{"-socket", sock}); err != nil {
			t.Errorf("cmdStatus: %v", err)
		}
	})

	if !strings.Contains(out, "listening on") {
		t.Errorf("status output = %q, want it to report 'listening on'", out)
	}
}

// TestCmdStatusSocketPresentButDead covers the stale-socket case: a leftover
// socket file with nothing behind it must be reported distinctly from both a
// live broker and an absent one, since that is the state a crashed daemon
// leaves and the operator needs to tell them apart.
func TestCmdStatusSocketPresentButDead(t *testing.T) {
	sock := filepath.Join(shortTempDir(t, "agmbus-dead-"), "s")
	if err := os.WriteFile(sock, nil, 0o600); err != nil {
		t.Fatal(err)
	}

	out := captureStdout(t, func() {
		if err := cmdStatus([]string{"-socket", sock}); err != nil {
			t.Errorf("cmdStatus returned %v, want nil for a dead socket", err)
		}
	})

	if !strings.Contains(out, "not accepting") {
		t.Errorf("status output = %q, want it to report 'not accepting'", out)
	}
}

// TestCmdStatusDefaultsToEnvironmentSocket covers the unflagged path.
func TestCmdStatusDefaultsToEnvironmentSocket(t *testing.T) {
	sock := filepath.Join(t.TempDir(), "env-absent.sock")
	t.Setenv("AGM_BUS_SOCKET", sock)

	out := captureStdout(t, func() {
		if err := cmdStatus(nil); err != nil {
			t.Errorf("cmdStatus: %v", err)
		}
	})

	if !strings.Contains(out, sock) {
		t.Errorf("status output = %q, want it to name the AGM_BUS_SOCKET path", out)
	}
}

// TestCmdStatusRejectsUnknownFlag covers the flag-error path.
func TestCmdStatusRejectsUnknownFlag(t *testing.T) {
	if err := cmdStatus([]string{"-nope"}); err == nil {
		t.Fatal("expected an error for an unknown flag")
	}
}

// writeAgentsConfig writes a minimal multi-bot portal config and returns its
// path. Mode 0600 keeps the loader from warning about loose permissions.
func writeAgentsConfig(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "discord-agents.yaml")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

// TestCmdDiscordResetDryRun covers AGMBUS-13: without -confirm the command
// must describe the purge and delete nothing. This is the destructive
// subcommand, so the dry-run default is the safety property worth pinning.
func TestCmdDiscordResetDryRun(t *testing.T) {
	cfg := writeAgentsConfig(t, "channel: \"12345\"\nagents:\n  - name: primary\n    token: tok-primary\n    bus_session: primary-portal\n")

	out := captureStdout(t, func() {
		if err := cmdDiscordReset([]string{"-agents", cfg}); err != nil {
			t.Errorf("cmdDiscordReset: %v", err)
		}
	})

	if !strings.Contains(out, "DRY RUN") {
		t.Errorf("output = %q, want a DRY RUN notice", out)
	}
	if !strings.Contains(out, "12345") {
		t.Errorf("output = %q, want the configured channel id", out)
	}
	if !strings.Contains(out, "primary") {
		t.Errorf("output = %q, want the acting bot name", out)
	}
	if strings.Contains(out, "deleted") {
		t.Errorf("output = %q, a dry run must not report deletions", out)
	}
}

// TestCmdDiscordResetChannelOverride covers -channel, which is how an operator
// targets a channel other than the configured one.
func TestCmdDiscordResetChannelOverride(t *testing.T) {
	cfg := writeAgentsConfig(t, "channel: \"12345\"\nagents:\n  - name: primary\n    token: tok-primary\n    bus_session: primary-portal\n")

	out := captureStdout(t, func() {
		if err := cmdDiscordReset([]string{"-agents", cfg, "-channel", "99999"}); err != nil {
			t.Errorf("cmdDiscordReset: %v", err)
		}
	})

	if !strings.Contains(out, "99999") {
		t.Errorf("output = %q, want the overridden channel id", out)
	}
	if strings.Contains(out, "12345") {
		t.Errorf("output = %q, the configured channel must not be targeted once overridden", out)
	}
}

// TestCmdDiscordResetSelectsNamedAgent covers -agent selection.
func TestCmdDiscordResetSelectsNamedAgent(t *testing.T) {
	cfg := writeAgentsConfig(t, "channel: \"1\"\nagents:\n  - name: primary\n    token: tok-a\n    bus_session: a\n  - name: secondary\n    token: tok-b\n    bus_session: b\n")

	out := captureStdout(t, func() {
		if err := cmdDiscordReset([]string{"-agents", cfg, "-agent", "secondary"}); err != nil {
			t.Errorf("cmdDiscordReset: %v", err)
		}
	})

	if !strings.Contains(out, "secondary") {
		t.Errorf("output = %q, want the named agent to be the actor", out)
	}
}

// TestCmdDiscordResetUnknownAgent covers the failure path: naming an agent the
// config does not define must be an error rather than a silent fallback to
// whichever bot happens to be first.
func TestCmdDiscordResetUnknownAgent(t *testing.T) {
	cfg := writeAgentsConfig(t, "channel: \"1\"\nagents:\n  - name: primary\n    token: tok-a\n    bus_session: a\n")

	err := cmdDiscordReset([]string{"-agents", cfg, "-agent", "nope"})
	if err == nil {
		t.Fatal("expected an error for an agent missing from the config")
	}
	if !strings.Contains(err.Error(), "nope") {
		t.Errorf("error = %v, want it to name the missing agent", err)
	}
}

// TestCmdDiscordResetMissingConfig covers an unreadable config.
func TestCmdDiscordResetMissingConfig(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "missing.yaml")
	if err := cmdDiscordReset([]string{"-agents", missing}); err == nil {
		t.Fatal("expected an error for a missing portal config")
	}
}

// TestCmdDiscordResetRejectsUnknownFlag covers the flag-error path.
func TestCmdDiscordResetRejectsUnknownFlag(t *testing.T) {
	if err := cmdDiscordReset([]string{"-nope"}); err == nil {
		t.Fatal("expected an error for an unknown flag")
	}
}

// TestUsageListsEverySubcommand pins that the usage text stays in step with
// the dispatch table in main, since an undocumented subcommand is invisible.
func TestUsageListsEverySubcommand(t *testing.T) {
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan string, 1)
	go func() {
		var buf bytes.Buffer
		_, _ = io.Copy(&buf, r)
		done <- buf.String()
	}()

	usage(w)
	_ = w.Close()
	out := <-done
	_ = r.Close()

	for _, want := range []string{"serve", "status", "socket", "discord-reset", "AGM_BUS_SOCKET"} {
		if !strings.Contains(out, want) {
			t.Errorf("usage text is missing %q:\n%s", want, out)
		}
	}
}

// TestServeStatusRoundTrip ties the two halves together in-process: while
// serve holds the socket, status reports listening; once serve stops, status
// reports not running. This is the contract an operator actually depends on.
func TestServeStatusRoundTrip(t *testing.T) {
	sock := filepath.Join(shortTempDir(t, "agmbus-roundtrip-"), "s")
	_, logger := captureLogger(t, false)

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
		<-done
		t.Skipf("broker never bound its socket on this host: %v", err)
	}

	live := captureStdout(t, func() {
		if err := cmdStatus([]string{"-socket", sock}); err != nil {
			t.Errorf("cmdStatus while serving: %v", err)
		}
	})
	if !strings.Contains(live, "listening on") {
		t.Errorf("status while serving = %q, want 'listening on'", live)
	}

	cancel()
	<-done

	stopped := captureStdout(t, func() {
		if err := cmdStatus([]string{"-socket", sock}); err != nil {
			t.Errorf("cmdStatus after shutdown: %v", err)
		}
	})
	if !strings.Contains(stopped, "not running") {
		t.Errorf("status after shutdown = %q, want 'not running'", stopped)
	}
}
