package main

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestWithRetrySucceedsAfterTransientFailures(t *testing.T) {
	origDelay := retryBaseDelay
	t.Cleanup(func() { retryBaseDelay = origDelay })
	retryBaseDelay = time.Millisecond

	calls := 0
	err := withRetry(context.Background(), func() error {
		calls++
		if calls < 2 {
			return stubErr("transient")
		}
		return nil
	})
	if err != nil {
		t.Fatalf("withRetry() = %v, want nil", err)
	}
	if calls != 2 {
		t.Errorf("calls = %d, want 2 (should stop retrying once fn succeeds)", calls)
	}
}

func TestWithRetryExhaustsAttempts(t *testing.T) {
	origDelay := retryBaseDelay
	t.Cleanup(func() { retryBaseDelay = origDelay })
	retryBaseDelay = time.Millisecond

	calls := 0
	err := withRetry(context.Background(), func() error {
		calls++
		return stubErr("permanent")
	})
	if err == nil {
		t.Fatal("withRetry() = nil, want error after exhausting attempts")
	}
	if calls != retryAttempts {
		t.Errorf("calls = %d, want %d", calls, retryAttempts)
	}
}

func TestWithRetryStopsOnCanceledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	calls := 0
	err := withRetry(ctx, func() error {
		calls++
		return stubErr("fails")
	})
	if err == nil {
		t.Fatal("withRetry() = nil, want error")
	}
	if calls != 1 {
		t.Errorf("calls = %d, want 1 — a canceled context must not retry", calls)
	}
}

func TestExecStderrExtractsAndTruncates(t *testing.T) {
	// Output() (not Run()) is what populates ExitError.Stderr — matches how
	// queryOpenPRs/queryReady/listSessions actually invoke their subprocess.
	cmd := exec.Command("sh", "-c", "echo 'auth failed: bad credentials' >&2; exit 1")
	_, err := cmd.Output()
	if err == nil {
		t.Fatal("expected command to fail")
	}
	got := execStderr(err)
	if got != "auth failed: bad credentials" {
		t.Errorf("execStderr() = %q, want %q", got, "auth failed: bad credentials")
	}
}

func TestExecStderrTruncatesLongOutput(t *testing.T) {
	cmd := exec.Command("sh", "-c", "yes x | head -c 1000 >&2; exit 1")
	_, err := cmd.Output()
	if err == nil {
		t.Fatal("expected command to fail")
	}
	got := execStderr(err)
	const maxLen = 300
	if len(got) > maxLen+len("…") { // "…" is 3 bytes in UTF-8
		t.Errorf("execStderr() length = %d, want <= %d", len(got), maxLen+len("…"))
	}
	if !strings.HasSuffix(got, "…") {
		t.Errorf("execStderr() = %q, want truncation ellipsis", got)
	}
}

func TestExecStderrNonExecError(t *testing.T) {
	if got := execStderr(stubErr("not an exec error")); got != "" {
		t.Errorf("execStderr() = %q, want empty for a non-exec error", got)
	}
}

func TestWrapExecErrIncludesDetailWhenPresent(t *testing.T) {
	cmd := exec.Command("sh", "-c", "echo 'bad credentials' >&2; exit 1")
	_, err := cmd.Output()
	wrapped := wrapExecErr("gh pr list", err)
	if !strings.Contains(wrapped.Error(), "bad credentials") {
		t.Errorf("wrapExecErr() = %q, want it to contain stderr detail", wrapped.Error())
	}
	if !errors.Is(wrapped, err) {
		t.Error("wrapExecErr() should wrap the original error for errors.Is/As")
	}
}

func TestWrapExecErrNoDetail(t *testing.T) {
	wrapped := wrapExecErr("gh pr list", stubErr("exit status 1"))
	if wrapped.Error() != "gh pr list: exit status 1" {
		t.Errorf("wrapExecErr() = %q, want %q", wrapped.Error(), "gh pr list: exit status 1")
	}
}

func TestDispatchStateRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "dispatch-direct.json")
	want := dispatchState{
		LastRun:             time.Now().Truncate(time.Second),
		OK:                  false,
		ConsecutiveFailures: 3,
		LastError:           "gh pr list: exit status 1: bad credentials",
	}
	if err := saveDispatchState(path, want); err != nil {
		t.Fatalf("saveDispatchState() = %v", err)
	}
	got := loadDispatchState(path)
	if got.ConsecutiveFailures != want.ConsecutiveFailures || got.LastError != want.LastError || got.OK != want.OK {
		t.Errorf("loadDispatchState() = %+v, want %+v", got, want)
	}
}

func TestLoadDispatchStateMissingFile(t *testing.T) {
	got := loadDispatchState(filepath.Join(t.TempDir(), "does-not-exist.json"))
	if got.ConsecutiveFailures != 0 || got.OK {
		t.Errorf("loadDispatchState() for a missing file = %+v, want zero value", got)
	}
}

func TestLoadDispatchStateCorruptFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "corrupt.json")
	if err := os.WriteFile(path, []byte("not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	got := loadDispatchState(path)
	if got.ConsecutiveFailures != 0 || got.OK {
		t.Errorf("loadDispatchState() for a corrupt file = %+v, want zero value", got)
	}
}

func TestRecordFailureIncrementsStreakAndAlertsAtThreshold(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")

	alerts := 0
	origAlert := sendDesktopAlert
	sendDesktopAlert = func(ctx context.Context, body string) error {
		alerts++
		return nil
	}
	t.Cleanup(func() { sendDesktopAlert = origAlert })

	for i := 1; i <= alertThreshold+1; i++ {
		recordFailure(context.Background(), path, stubErr("gh pr list: exit status 1"))
		s := loadDispatchState(path)
		if s.ConsecutiveFailures != i {
			t.Errorf("after failure %d: ConsecutiveFailures = %d, want %d", i, s.ConsecutiveFailures, i)
		}
		if s.OK {
			t.Errorf("after failure %d: OK = true, want false", i)
		}
	}

	if alerts != 2 { // fires at threshold and again the tick after
		t.Errorf("alerts = %d, want 2 (one at threshold, one past it)", alerts)
	}
}

func TestRecordFailureBelowThresholdDoesNotAlert(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")

	alerts := 0
	origAlert := sendDesktopAlert
	sendDesktopAlert = func(ctx context.Context, body string) error {
		alerts++
		return nil
	}
	t.Cleanup(func() { sendDesktopAlert = origAlert })

	recordFailure(context.Background(), path, stubErr("transient"))

	if alerts != 0 {
		t.Errorf("alerts = %d, want 0 — a single failure should not escalate", alerts)
	}
}

func TestRecordSuccessResetsStreak(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	if err := saveDispatchState(path, dispatchState{ConsecutiveFailures: 5, OK: false, LastError: "boom"}); err != nil {
		t.Fatal(err)
	}

	recordSuccess(path)

	s := loadDispatchState(path)
	if s.ConsecutiveFailures != 0 {
		t.Errorf("ConsecutiveFailures = %d, want 0", s.ConsecutiveFailures)
	}
	if !s.OK {
		t.Error("OK = false, want true")
	}
	if s.LastError != "" {
		t.Errorf("LastError = %q, want empty", s.LastError)
	}
	if s.LastSuccess.IsZero() {
		t.Error("LastSuccess not set")
	}
}
