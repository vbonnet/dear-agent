package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// Regression tests for the PR #1160 review findings. Each case is a way a
// broken reaper could suppress its own alarm, which is the one failure mode
// this whole check exists to prevent.

func gcReapAt(ts time.Time) string {
	return `{"timestamp":"` + ts.Format(time.RFC3339Nano) +
		`","operation":"sandbox_gc_reap","session_id":"abc"}`
}

// `agm sandbox gc` is dry-run by DEFAULT, so previews are the common case. If a
// preview refreshed the heartbeat, anyone running one while the hourly reaper
// was dead would suppress the alarm for another full window — and a recurring
// preview would mask the failure forever.
func TestCheckGCHealth_DryRunCompletionIsNotProofOfLife(t *testing.T) {
	now := time.Now()
	dryRun := `{"timestamp":"` + now.Add(-1*time.Minute).Format(time.RFC3339Nano) +
		`","operation":"sandbox_gc_completed","dry_run":true,"reason":"scanned=3 reaped=0 kept=3 errors=0"}`
	cfg := config{gcMaxAge: 6 * time.Hour, gcLogPath: writeGCLog(t,
		gcCompletedAt(now.Add(-48*time.Hour)),
		dryRun,
	)}

	got := checkGCHealth(cfg, now)
	if got == nil || !got.Stale {
		t.Fatalf("a dry-run completion must not refresh liveness, got %+v", got)
	}
}

// A sweep can return success overall while every individual deletion fails
// (permissions, locked files). Fresh completion records would then suppress the
// alarm indefinitely while sandboxes keep accumulating — the exact outcome the
// check is meant to catch.
func TestCheckGCHealth_CompletionWithReapErrorsIsNotProofOfLife(t *testing.T) {
	now := time.Now()
	failed := `{"timestamp":"` + now.Add(-1*time.Minute).Format(time.RFC3339Nano) +
		`","operation":"sandbox_gc_completed","errors":4,"reason":"scanned=9 reaped=0 kept=5 errors=4"}`
	cfg := config{gcMaxAge: 6 * time.Hour, gcLogPath: writeGCLog(t,
		gcCompletedAt(now.Add(-48*time.Hour)),
		failed,
	)}

	got := checkGCHealth(cfg, now)
	if got == nil || !got.Stale {
		t.Fatalf("a completion with reap errors must not refresh liveness, got %+v", got)
	}
}

// gc.jsonl is shared with the session GC. Attributing one of its failures to a
// stale sandbox reaper sends whoever reads the alarm to the wrong subsystem.
func TestCheckGCHealth_IgnoresUnrelatedSessionGCErrors(t *testing.T) {
	now := time.Now()
	sessionErr := `{"timestamp":"` + now.Add(-1*time.Minute).Format(time.RFC3339Nano) +
		`","operation":"gc_archive_error","error":"archive failed for session xyz"}`
	cfg := config{gcMaxAge: 6 * time.Hour, gcLogPath: writeGCLog(t,
		gcCompletedAt(now.Add(-48*time.Hour)),
		sessionErr,
	)}

	got := checkGCHealth(cfg, now)
	if got == nil || !got.Stale {
		t.Fatalf("expected stale, got %+v", got)
	}
	if strings.Contains(got.Reason, "archive failed") {
		t.Errorf("session-GC failure must not be reported as the sandbox-reaper cause: %q", got.Reason)
	}
}

// An oversized record used to abort the scan (bufio.Scanner ErrTooLong), hiding
// every valid heartbeat appended after it and pinning the watchdog in a false
// stale state forever.
func TestCheckGCHealth_OversizedRecordDoesNotHideLaterHeartbeats(t *testing.T) {
	now := time.Now()
	cfg := config{gcMaxAge: 6 * time.Hour, gcLogPath: writeGCLog(t,
		`{"operation":"sandbox_gc_junk","pad":"`+strings.Repeat("x", 2*1024*1024)+`"}`,
		gcCompletedAt(now.Add(-5*time.Minute)),
	)}

	if got := checkGCHealth(cfg, now); got == nil || got.Stale {
		t.Fatalf("a heartbeat after an oversized record must still be seen, got %+v", got)
	}
}

func TestCheckGCHealth_DiscardsUnterminatedOversizedRecord(t *testing.T) {
	now := time.Now()
	p := filepath.Join(t.TempDir(), "gc.jsonl")
	content := `{"operation":"sandbox_gc_junk","pad":"` + strings.Repeat("x", 2*1024*1024)
	if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
		t.Fatalf("write gc log: %v", err)
	}

	got, err := scanGCLog(p, now)
	if err != nil {
		t.Fatalf("scan oversized unterminated record: %v", err)
	}
	if !got.LastSuccess.IsZero() || got.HasCompletion {
		t.Fatalf("unterminated oversized record should be discarded, got %+v", got)
	}
}

func TestCheckGCHealth_ReapFallbackCannotOverrideFailedCompletion(t *testing.T) {
	now := time.Now()
	failed := `{"timestamp":"` + now.Add(-1*time.Minute).Format(time.RFC3339Nano) +
		`","operation":"sandbox_gc_completed","errors":1,"reason":"scanned=2 reaped=1 kept=0 errors=1"}`
	cfg := config{gcMaxAge: 6 * time.Hour, gcLogPath: writeGCLog(t,
		gcReapAt(now.Add(-1*time.Minute)),
		failed,
	)}

	got := checkGCHealth(cfg, now)
	if got == nil || !got.Stale {
		t.Fatalf("failed modern completion must not be overridden by reap fallback, got %+v", got)
	}
}

// An append-only log keeps the maximum timestamp forever. One heartbeat written
// while the clock ran fast would otherwise yield a negative age — permanently
// "healthy" — that no later correct record could displace.
func TestCheckGCHealth_FutureTimestampIsNotProofOfLife(t *testing.T) {
	now := time.Now()
	cfg := config{gcMaxAge: 6 * time.Hour, gcLogPath: writeGCLog(t,
		gcCompletedAt(now.Add(-48*time.Hour)),
		gcCompletedAt(now.Add(72*time.Hour)),
	)}

	got := checkGCHealth(cfg, now)
	if got == nil || !got.Stale {
		t.Fatalf("a future-dated heartbeat must not mark the reaper healthy, got %+v", got)
	}
	if got.Age < 0 {
		t.Errorf("Age must never be negative, got %s", got.Age)
	}
}

// A watchdog upgraded ahead of agm reads a log with reap records but no
// completion heartbeat. Alarming there would be a false positive on a GC that
// is working fine.
func TestCheckGCHealth_PreHeartbeatAgmReapsCountAsProofOfLife(t *testing.T) {
	now := time.Now()
	cfg := config{gcMaxAge: 6 * time.Hour, gcLogPath: writeGCLog(t,
		gcReapAt(now.Add(-20*time.Minute)),
	)}

	if got := checkGCHealth(cfg, now); got == nil || got.Stale {
		t.Fatalf("a recent reap from a pre-heartbeat agm must count as alive, got %+v", got)
	}
}

// ...but the fallback must not become a way for a dead reaper to look alive:
// once the reaps themselves go stale, the alarm fires and says why.
func TestCheckGCHealth_StalePreHeartbeatReapsStillAlarm(t *testing.T) {
	now := time.Now()
	cfg := config{gcMaxAge: 6 * time.Hour, gcLogPath: writeGCLog(t,
		gcReapAt(now.Add(-48*time.Hour)),
	)}

	got := checkGCHealth(cfg, now)
	if got == nil || !got.Stale {
		t.Fatalf("stale reaps must alarm, got %+v", got)
	}
	if !strings.Contains(got.Reason, "predates the sandbox_gc_completed heartbeat") {
		t.Errorf("reason should explain the version-skew inference: %q", got.Reason)
	}
}

// A GC that is failing writes error records every hour. Those must never be
// mistaken for evidence it is alive — that would mask the original bug.
func TestCheckGCHealth_ErrorRecordsAreNotProofOfLife(t *testing.T) {
	now := time.Now()
	cfg := config{gcMaxAge: 6 * time.Hour, gcLogPath: writeGCLog(t,
		gcErrorAt(now.Add(-1*time.Minute), "multiple enabled workspaces require a port"),
	)}

	got := checkGCHealth(cfg, now)
	if got == nil || !got.Stale {
		t.Fatalf("fresh error records must not count as liveness, got %+v", got)
	}
	if !strings.Contains(got.Reason, "multiple enabled workspaces require a port") {
		t.Errorf("reason should quote the live error: %q", got.Reason)
	}
}

// A real I/O failure must surface as stale rather than be swallowed into a
// zero-value summary that reads as "never swept" with no explanation.
func TestScanGCLog_PropagatesOpenErrors(t *testing.T) {
	if _, err := scanGCLog(t.TempDir()+"/absent.jsonl", time.Now()); err == nil {
		t.Fatal("expected an error opening a missing log")
	}
}
