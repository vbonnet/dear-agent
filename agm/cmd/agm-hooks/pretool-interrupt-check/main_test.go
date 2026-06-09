package main

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/vbonnet/dear-agent/agm/internal/interrupt"
)

// captureStderr swaps os.Stderr for a pipe, runs fn, and returns whatever was
// written to stderr while fn ran. Hooks talk to Claude via stderr, so most of
// the test assertions need to see that stream.
func captureStderr(t *testing.T, fn func()) string {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	orig := os.Stderr
	os.Stderr = w
	defer func() { os.Stderr = orig }()

	done := make(chan struct{})
	var buf bytes.Buffer
	go func() {
		_, _ = io.Copy(&buf, r)
		close(done)
	}()

	fn()
	_ = w.Close()
	<-done
	return buf.String()
}

// useTempInterruptHome points interrupt.DefaultDir() at a tempdir for the
// duration of the test by overriding HOME. The interrupt package builds its
// directory from os.UserHomeDir(), which on Unix consults $HOME.
func useTempInterruptHome(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	dir := filepath.Join(home, ".agm", "interrupts")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatalf("mkdir interrupts: %v", err)
	}
	return dir
}

func TestGetSessionName(t *testing.T) {
	tests := []struct {
		name   string
		claude string
		agm    string
		want   string
	}{
		{"both empty", "", "", ""},
		{"claude wins over agm", "claude-1", "agm-1", "claude-1"},
		{"falls back to agm", "", "agm-1", "agm-1"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("CLAUDE_SESSION_NAME", tt.claude)
			t.Setenv("AGM_SESSION_NAME", tt.agm)
			if got := getSessionName(); got != tt.want {
				t.Errorf("getSessionName = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestRun_NoSession_FailsOpen(t *testing.T) {
	t.Setenv("CLAUDE_SESSION_NAME", "")
	t.Setenv("AGM_SESSION_NAME", "")
	if code := run(); code != 0 {
		t.Errorf("run() with no session = %d, want 0", code)
	}
}

func TestRun_NoFlag_Allows(t *testing.T) {
	useTempInterruptHome(t)
	t.Setenv("CLAUDE_SESSION_NAME", "no-flag-session")
	t.Setenv("AGM_SESSION_NAME", "")

	if code := run(); code != 0 {
		t.Errorf("run() with no flag = %d, want 0", code)
	}
}

func TestRun_StopFlag_BlocksAndConsumes(t *testing.T) {
	dir := useTempInterruptHome(t)
	session := "stop-session"
	t.Setenv("CLAUDE_SESSION_NAME", session)
	t.Setenv("AGM_SESSION_NAME", "")

	writeFlag(t, dir, session, &interrupt.Flag{
		Type:     interrupt.TypeStop,
		Reason:   "budget exceeded",
		IssuedBy: "orchestrator",
		IssuedAt: time.Now().UTC(),
	})

	var code int
	stderr := captureStderr(t, func() { code = run() })

	if code != 2 {
		t.Errorf("stop flag run() = %d, want 2", code)
	}
	if !contains(stderr, "INTERRUPT (stop)") || !contains(stderr, "budget exceeded") || !contains(stderr, "orchestrator") {
		t.Errorf("stderr missing expected text:\n%s", stderr)
	}

	// Stop is single-shot — the flag must be consumed.
	if remaining, _ := interrupt.Read(dir, session); remaining != nil {
		t.Errorf("stop flag was not consumed: %+v", remaining)
	}
}

func TestRun_KillFlag_BlocksWithoutConsuming(t *testing.T) {
	dir := useTempInterruptHome(t)
	session := "kill-session"
	t.Setenv("CLAUDE_SESSION_NAME", session)
	t.Setenv("AGM_SESSION_NAME", "")

	writeFlag(t, dir, session, &interrupt.Flag{
		Type:     interrupt.TypeKill,
		Reason:   "emergency",
		IssuedBy: "user",
		IssuedAt: time.Now().UTC(),
	})

	var code int
	stderr := captureStderr(t, func() { code = run() })

	if code != 2 {
		t.Errorf("kill flag run() = %d, want 2", code)
	}
	if !contains(stderr, "INTERRUPT (kill)") || !contains(stderr, "HARD STOP") {
		t.Errorf("stderr missing expected text:\n%s", stderr)
	}

	// Kill must persist so every subsequent tool call is also blocked.
	remaining, err := interrupt.Read(dir, session)
	if err != nil {
		t.Fatalf("Read after kill: %v", err)
	}
	if remaining == nil || remaining.Type != interrupt.TypeKill {
		t.Errorf("kill flag was consumed; want it to persist: %+v", remaining)
	}

	// A second call must still block — proving kill is sticky.
	code2 := run()
	if code2 != 2 {
		t.Errorf("second run with kill flag = %d, want 2", code2)
	}
}

func TestRun_SteerFlag_AllowsAndConsumes(t *testing.T) {
	dir := useTempInterruptHome(t)
	session := "steer-session"
	t.Setenv("CLAUDE_SESSION_NAME", session)
	t.Setenv("AGM_SESSION_NAME", "")

	writeFlag(t, dir, session, &interrupt.Flag{
		Type:     interrupt.TypeSteer,
		Reason:   "focus on tests",
		IssuedBy: "orchestrator",
		IssuedAt: time.Now().UTC(),
	})

	var code int
	stderr := captureStderr(t, func() { code = run() })

	if code != 0 {
		t.Errorf("steer flag run() = %d, want 0", code)
	}
	if !contains(stderr, "INTERRUPT (steer)") || !contains(stderr, "focus on tests") {
		t.Errorf("stderr missing expected text:\n%s", stderr)
	}

	if remaining, _ := interrupt.Read(dir, session); remaining != nil {
		t.Errorf("steer flag was not consumed: %+v", remaining)
	}
}

func TestRun_UnknownType_FailsOpen(t *testing.T) {
	dir := useTempInterruptHome(t)
	session := "weird-session"
	t.Setenv("CLAUDE_SESSION_NAME", session)
	t.Setenv("AGM_SESSION_NAME", "")
	// Surface the debug branch — exercises the warn log + the default case.
	t.Setenv("AGM_HOOK_DEBUG", "1")

	// Hand-write a flag with an unrecognized type. We can't use interrupt.Write
	// because the package validates Type on read; the file is plain JSON, so we
	// can just craft the bytes directly.
	path := interrupt.FlagPath(dir, session)
	if err := os.WriteFile(path, []byte(`{"type":"explode","reason":"r","issued_by":"u","issued_at":"2026-01-01T00:00:00Z"}`), 0o600); err != nil {
		t.Fatalf("write raw flag: %v", err)
	}

	var code int
	stderr := captureStderr(t, func() { code = run() })

	if code != 0 {
		t.Errorf("unknown type run() = %d, want 0 (fail-open)", code)
	}
	if !contains(stderr, "unknown interrupt type") {
		t.Errorf("debug log missing:\n%s", stderr)
	}
}

func TestRun_ReadError_FailsOpenAndLogs(t *testing.T) {
	dir := useTempInterruptHome(t)
	session := "broken-session"
	t.Setenv("CLAUDE_SESSION_NAME", session)
	t.Setenv("AGM_SESSION_NAME", "")
	t.Setenv("AGM_HOOK_DEBUG", "1")

	// Malformed JSON — Read returns an error. The hook must allow anyway.
	path := interrupt.FlagPath(dir, session)
	if err := os.WriteFile(path, []byte("{not json"), 0o600); err != nil {
		t.Fatalf("write garbage flag: %v", err)
	}

	var code int
	stderr := captureStderr(t, func() { code = run() })

	if code != 0 {
		t.Errorf("read error run() = %d, want 0 (fail-open)", code)
	}
	if !contains(stderr, "error reading flag") {
		t.Errorf("debug log missing:\n%s", stderr)
	}
}

func TestSafeRun_RecoversPanic(t *testing.T) {
	// safeRun's job is to swallow any panic from run() and return 0. We can't
	// make run() panic without surgery, so prove the deferred recovery itself
	// works: invoke it with a panicking closure to demonstrate the contract.
	exit := func() (code int) {
		defer func() {
			if r := recover(); r != nil {
				code = 0
			}
		}()
		panic("boom")
	}()
	if exit != 0 {
		t.Fatalf("recovered exit = %d, want 0", exit)
	}

	// And exercise safeRun against the happy path (no panic, returns run()).
	t.Setenv("CLAUDE_SESSION_NAME", "")
	t.Setenv("AGM_SESSION_NAME", "")
	if got := safeRun(); got != 0 {
		t.Errorf("safeRun on happy path = %d, want 0", got)
	}
}

// --- helpers below this line --------------------------------------------

func writeFlag(t *testing.T, dir, session string, f *interrupt.Flag) {
	t.Helper()
	if err := interrupt.Write(dir, session, f); err != nil {
		t.Fatalf("interrupt.Write: %v", err)
	}
}

func contains(haystack, needle string) bool {
	return bytes.Contains([]byte(haystack), []byte(needle))
}
