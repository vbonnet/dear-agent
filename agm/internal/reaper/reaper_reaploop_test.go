package reaper

import (
	"errors"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/vbonnet/dear-agent/agm/internal/dolt"
	"github.com/vbonnet/dear-agent/agm/internal/manifest"
	"github.com/vbonnet/dear-agent/agm/internal/safety"
)

// These tests exercise the reap loop (Run, stopProcess, sendExit) for real by
// substituting the tmux + process boundary seams declared in reaper.go with
// fakes. They replace the cargo-cult compile-time "method exists" checks that
// previously stood in for coverage of this destructive code path (ce-6as.110).
//
// The session-killing chain (native exit -> wait -> SIGTERM -> SIGKILL ->
// kill-session -> archive) is what these fakes stand in for: each fake records
// whether/how it was called so the ordering and gating logic can be asserted
// without a live tmux session or process.

// fakeBoundary records calls to the seamed dependencies and lets each test
// program their return values.
type fakeBoundary struct {
	safetyResult *safety.CheckResult

	waitForPromptErr error
	promptCalls      int

	sendPromptErr   error
	sendPromptCalls int
	lastSendPrompt  string
	lastInterrupt   bool

	// paneCloseResults is consumed one entry per waitForPaneClose call; if
	// exhausted the last entry is reused. nil entry == pane closed (success).
	paneCloseResults []error
	paneCloseCalls   int

	// isPaneActive is returned by every isPaneActiveFn call.
	isPaneActive    bool
	isPaneActiveErr error
	isPaneCalls     int

	killSessionCalls int

	panePID    int
	panePIDErr error

	signals []syscall.Signal
}

// install swaps the package seams for the fake and restores them on cleanup.
func (f *fakeBoundary) install(t *testing.T) {
	t.Helper()

	origSafety := checkSafetyFn
	origWaitPrompt := waitForPromptFn
	origSendPrompt := sendPromptSafeFn
	origWaitPaneClose := waitForPaneCloseFn
	origIsPaneActive := isPaneActiveFn
	origKillSession := killSessionFn
	origGetPanePID := getPanePIDFn
	origProcessKill := processKillFn

	t.Cleanup(func() {
		checkSafetyFn = origSafety
		waitForPromptFn = origWaitPrompt
		sendPromptSafeFn = origSendPrompt
		waitForPaneCloseFn = origWaitPaneClose
		isPaneActiveFn = origIsPaneActive
		killSessionFn = origKillSession
		getPanePIDFn = origGetPanePID
		processKillFn = origProcessKill
	})

	checkSafetyFn = func(_ string, _ safety.GuardOptions) *safety.CheckResult {
		if f.safetyResult != nil {
			return f.safetyResult
		}
		return &safety.CheckResult{Safe: true}
	}
	waitForPromptFn = func(_ string, _ time.Duration) error {
		f.promptCalls++
		return f.waitForPromptErr
	}
	sendPromptSafeFn = func(_ string, prompt string, interrupt bool) error {
		f.sendPromptCalls++
		f.lastSendPrompt = prompt
		f.lastInterrupt = interrupt
		return f.sendPromptErr
	}
	waitForPaneCloseFn = func(_ string, _ time.Duration) error {
		idx := f.paneCloseCalls
		f.paneCloseCalls++
		if len(f.paneCloseResults) == 0 {
			return nil
		}
		if idx >= len(f.paneCloseResults) {
			idx = len(f.paneCloseResults) - 1
		}
		return f.paneCloseResults[idx]
	}
	isPaneActiveFn = func(_ string) (bool, error) {
		f.isPaneCalls++
		return f.isPaneActive, f.isPaneActiveErr
	}
	killSessionFn = func(_ string) error {
		f.killSessionCalls++
		return nil
	}
	getPanePIDFn = func(_ string) (int, error) {
		if f.panePID == 0 {
			f.panePID = 4242
		}
		return f.panePID, f.panePIDErr
	}
	processKillFn = func(_ int, sig syscall.Signal) error {
		f.signals = append(f.signals, sig)
		return nil
	}
}

// --- sendExit ---

func TestSendExit_Success(t *testing.T) {
	f := &fakeBoundary{}
	f.install(t)

	r := New("sess", "/tmp/sessions")
	if err := r.sendExit("/exit"); err != nil {
		t.Fatalf("sendExit() returned error: %v", err)
	}

	if f.sendPromptCalls != 1 {
		t.Fatalf("sendExit() called underlying send %d times, want 1", f.sendPromptCalls)
	}
	if f.lastSendPrompt != "/exit" {
		t.Errorf("sendExit() sent %q, want %q", f.lastSendPrompt, "/exit")
	}
	if !f.lastInterrupt {
		t.Error("sendExit() should request interrupt (shouldInterrupt=true)")
	}
}

func TestSendExit_WrapsUnderlyingError(t *testing.T) {
	f := &fakeBoundary{sendPromptErr: errors.New("pane gone")}
	f.install(t)

	r := New("sess", "/tmp/sessions")
	err := r.sendExit("/exit")
	if err == nil {
		t.Fatal("sendExit() should return an error when the send fails")
	}
	if !strings.Contains(err.Error(), "failed to send /exit") {
		t.Errorf("error %q should mention 'failed to send /exit'", err.Error())
	}
	if !strings.Contains(err.Error(), "pane gone") {
		t.Errorf("error %q should wrap the underlying cause", err.Error())
	}
}

func TestGracefulExitCommandUsesNativeHarnessContract(t *testing.T) {
	tests := map[string]string{
		"claude-code":  "/exit",
		"codex-cli":    "/exit",
		"agy":          "/exit",
		"opencode-cli": "/exit",
		"pi-cli":       "/quit",
		"PI":           "/quit",
		"":             "/exit",
	}
	for harness, want := range tests {
		t.Run(harness, func(t *testing.T) {
			if got := GracefulExitCommand(harness); got != want {
				t.Fatalf("GracefulExitCommand(%q) = %q, want %q", harness, got, want)
			}
		})
	}
}

// --- stopProcess ---

// TestStopProcess_ZombieTimeout asserts that an already-expired budget makes
// stopProcess report a zombie and skip graceful exit entirely (never touching the
// pane), per the timeout guard.
func TestStopProcess_ZombieTimeout(t *testing.T) {
	f := &fakeBoundary{}
	f.install(t)

	r := New("sess", "/tmp/sessions")
	zombie := r.stopProcess(time.Now().Add(-ReaperTimeout-time.Minute), "pi-cli")

	if !zombie {
		t.Error("stopProcess() should report zombie when budget already exceeded")
	}
	if f.promptCalls != 0 {
		t.Errorf("expired budget should skip prompt wait, got %d calls", f.promptCalls)
	}
	if f.sendPromptCalls != 0 {
		t.Errorf("expired budget should skip graceful exit, got %d send calls", f.sendPromptCalls)
	}
}

// TestStopProcess_GracefulExit covers the happy path: prompt detected, /exit
// sent, pane closes — no zombie, no force-kill escalation.
func TestStopProcess_GracefulExit(t *testing.T) {
	f := &fakeBoundary{} // all zero values => success
	f.install(t)

	r := New("sess", "/tmp/sessions")
	zombie := r.stopProcess(time.Now(), "claude-code")

	if zombie {
		t.Error("graceful exit should not report a zombie")
	}
	if f.sendPromptCalls != 1 {
		t.Errorf("expected exactly one /exit, got %d", f.sendPromptCalls)
	}
	if f.paneCloseCalls != 1 {
		t.Errorf("expected one pane-close wait, got %d", f.paneCloseCalls)
	}
	if len(f.signals) != 0 {
		t.Errorf("graceful exit should not signal the process, got %v", f.signals)
	}
}

func TestStopProcess_PiUsesQuitAndClosesWithoutSignals(t *testing.T) {
	f := &fakeBoundary{}
	f.install(t)

	r := New("pi-session", "/tmp/sessions")
	zombie := r.stopProcess(time.Now(), "pi-cli")

	if zombie {
		t.Error("graceful Pi exit should not report a zombie")
	}
	if f.lastSendPrompt != "/quit" {
		t.Fatalf("Pi reaper sent %q, want /quit", f.lastSendPrompt)
	}
	if f.paneCloseCalls != 1 {
		t.Fatalf("Pi reaper pane-close waits = %d, want 1", f.paneCloseCalls)
	}
	if len(f.signals) != 0 {
		t.Fatalf("graceful Pi exit should not signal the process, got %v", f.signals)
	}
}

// TestStopProcess_PaneAlreadyClosed covers a failed graceful-exit send when the
// pane is already gone — stopProcess should short-circuit without waiting on
// pane close or escalating.
func TestStopProcess_PaneAlreadyClosed(t *testing.T) {
	f := &fakeBoundary{
		sendPromptErr: errors.New("no such session"),
		isPaneActive:  false, // pane already gone
	}
	f.install(t)

	r := New("sess", "/tmp/sessions")
	zombie := r.stopProcess(time.Now(), "claude-code")

	if zombie {
		t.Error("a closed pane is not a zombie")
	}
	if f.isPaneCalls != 1 {
		t.Errorf("expected one pane-active check after failed graceful exit, got %d", f.isPaneCalls)
	}
	if f.paneCloseCalls != 0 {
		t.Errorf("should not wait for pane close when pane already gone, got %d", f.paneCloseCalls)
	}
	if len(f.signals) != 0 {
		t.Errorf("should not signal a process for an already-closed pane, got %v", f.signals)
	}
}

// TestStopProcess_EscalatesToSIGTERM covers: native exit sent, pane does NOT close,
// escalate to SIGTERM, process exits after SIGTERM (no SIGKILL needed).
func TestStopProcess_EscalatesToSIGTERM(t *testing.T) {
	f := &fakeBoundary{
		// 1st wait (after native exit): stuck. 2nd wait (after SIGTERM): exits.
		paneCloseResults: []error{errors.New("still alive"), nil},
	}
	f.install(t)

	r := New("sess", "/tmp/sessions")
	zombie := r.stopProcess(time.Now(), "claude-code")

	if zombie {
		t.Error("force-kill escalation is not a timeout zombie")
	}
	if len(f.signals) != 1 || f.signals[0] != syscall.SIGTERM {
		t.Errorf("expected a single SIGTERM, got %v", f.signals)
	}
}

// TestStopProcess_EscalatesToSIGKILL covers: pane never closes, so escalation
// must go SIGTERM -> SIGKILL.
func TestStopProcess_EscalatesToSIGKILL(t *testing.T) {
	f := &fakeBoundary{
		// every pane-close wait reports the process is still alive
		paneCloseResults: []error{errors.New("still alive")},
	}
	f.install(t)

	r := New("sess", "/tmp/sessions")
	zombie := r.stopProcess(time.Now(), "claude-code")

	if zombie {
		t.Error("force-kill escalation is not a timeout zombie")
	}
	if len(f.signals) != 2 {
		t.Fatalf("expected SIGTERM then SIGKILL, got %v", f.signals)
	}
	if f.signals[0] != syscall.SIGTERM || f.signals[1] != syscall.SIGKILL {
		t.Errorf("expected [SIGTERM, SIGKILL], got %v", f.signals)
	}
}

// --- Run ---

// TestRun_SafetyGuardBlocks asserts the reaper aborts before touching tmux
// when a human is present (safety guard not satisfied).
func TestRun_SafetyGuardBlocks(t *testing.T) {
	f := &fakeBoundary{
		safetyResult: &safety.CheckResult{
			Safe: false,
			Violations: []safety.Violation{{
				Guard:   safety.ViolationHumanTyping,
				Message: "human is typing",
			}},
		},
	}
	f.install(t)

	r := New("sess", "/tmp/sessions")
	err := r.Run()
	if err == nil {
		t.Fatal("Run() should fail when the safety guard blocks")
	}
	if !strings.Contains(err.Error(), "safety guard blocked") {
		t.Errorf("error %q should mention the safety guard", err.Error())
	}
	if f.killSessionCalls != 0 || f.sendPromptCalls != 0 {
		t.Error("Run() must not touch tmux after a safety block")
	}
}

// TestRun_RefusesToArchiveWhenPaneAlive is the core zombie-prevention
// invariant: if the pane is still alive after all kill attempts, Run must
// refuse to archive (returning an error) rather than mark a live session dead.
func TestRun_RefusesToArchiveWhenPaneAlive(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "agm.db")
	t.Setenv("AGM_DB_PATH", dbPath)
	adapter, err := dolt.NewSQLiteAdapter(dbPath)
	if err != nil {
		t.Fatalf("NewSQLiteAdapter() error: %v", err)
	}
	t.Cleanup(func() { _ = adapter.Close() })
	m := &manifest.Manifest{
		SchemaVersion: manifest.SchemaVersion,
		SessionID:     "pane-alive-id",
		Name:          "sess",
		Harness:       "agy",
		CreatedAt:     time.Now().Add(-time.Hour),
		UpdatedAt:     time.Now().Add(-time.Hour),
		Context:       manifest.Context{Project: t.TempDir()},
		Tmux:          manifest.Tmux{SessionName: "sess"},
	}
	if err := adapter.CreateSession(m); err != nil {
		t.Fatalf("CreateSession() error: %v", err)
	}

	f := &fakeBoundary{
		isPaneActive: true, // pane survives kill-session
	}
	f.install(t)

	r := New("sess", "/tmp/sessions")
	err = r.Run()
	if err == nil {
		t.Fatal("Run() must not archive while the pane is still alive")
	}
	if !strings.Contains(err.Error(), "still alive") {
		t.Errorf("error %q should explain the pane is still alive", err.Error())
	}
	if f.killSessionCalls != 1 {
		t.Errorf("Run() should attempt kill-session once, got %d", f.killSessionCalls)
	}
	stored, getErr := adapter.GetSession(m.SessionID)
	if getErr != nil {
		t.Fatalf("GetSession() error: %v", getErr)
	}
	if stored.Lifecycle != manifest.LifecycleReaping {
		t.Fatalf("Lifecycle = %q, want reaping tombstone", stored.Lifecycle)
	}
}

func TestRun_ReusesOneLifecycleStorageConnection(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "agm.db")
	t.Setenv("AGM_DB_PATH", dbPath)
	t.Setenv("HOME", t.TempDir())
	adapter, err := dolt.NewSQLiteAdapter(dbPath)
	if err != nil {
		t.Fatalf("NewSQLiteAdapter() error: %v", err)
	}
	t.Cleanup(func() { _ = adapter.Close() })
	m := &manifest.Manifest{
		SchemaVersion: manifest.SchemaVersion,
		SessionID:     "single-connection-id",
		Name:          "single-connection",
		Harness:       "agy",
		IsTest:        true,
		CreatedAt:     time.Now().Add(-time.Hour),
		UpdatedAt:     time.Now().Add(-time.Hour),
		Context:       manifest.Context{Project: t.TempDir()},
		Tmux:          manifest.Tmux{SessionName: "single-connection"},
	}
	if err := adapter.CreateSession(m); err != nil {
		t.Fatalf("CreateSession() error: %v", err)
	}

	originalOpenStorage := openStorageFn
	openCalls := 0
	openStorageFn = func() (*dolt.Adapter, error) {
		openCalls++
		return originalOpenStorage()
	}
	t.Cleanup(func() { openStorageFn = originalOpenStorage })
	(&fakeBoundary{}).install(t)

	if err := NewWithOptions(m.Name, t.TempDir(), ArchiveOptions{Force: true}).Run(); err != nil {
		t.Fatalf("Run() error: %v", err)
	}
	if openCalls != 1 {
		t.Fatalf("lifecycle storage opened %d times, want 1", openCalls)
	}
}
