package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

// --- a refused reap is a failed remediation, not a quiet success ---

// The sandbox sweep downgrades an explicit --reap to a scan whenever the
// live-session inventory is partial. Under disk pressure that is a remediation
// that deleted nothing, and its `reaped` count means "would have reaped". A
// watchdog that reads the reply as a successful sweep reports space it never
// reclaimed and leaves the admission brake open on a host that keeps filling.
func TestSweepMergedWorktrees_RefusedReapIsARemediationFailure(t *testing.T) {
	for _, tt := range []struct {
		name    string
		gcJSON  string
		wantSub string
	}{
		{
			name:    "sweep states the refusal",
			gcJSON:  `{"dry_run":true,"reap_refused":"live-session inventory is partial (1 workspace store(s) skipped)","scanned":9,"reaped":4,"kept":5}`,
			wantSub: "live-session inventory is partial",
		},
		{
			name:    "older sweep only reports dry_run",
			gcJSON:  `{"dry_run":true,"scanned":9,"reaped":4,"kept":5}`,
			wantSub: "downgraded to a scan",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			cfg := config{agmBin: "agm", runCommand: gcThenSweep(tt.gcJSON, `{"removed":[]}`)}

			got, err := sweepMergedWorktrees(context.Background(), cfg)
			if err != nil {
				t.Fatalf("sweepMergedWorktrees: %v", err)
			}
			if !strings.Contains(got.Error, tt.wantSub) {
				t.Fatalf("remediation error = %q, want it to mention %q", got.Error, tt.wantSub)
			}
			if d := decideBrake(true, got); !d.Engage {
				t.Errorf("a breached tick whose reap was refused must latch the brake, got %+v", d)
			}
		})
	}
}

// The mirror: a sweep that actually deleted is not reported as a failure, or
// every healthy tick would latch the brake.
func TestSweepMergedWorktrees_CompletedReapIsNotAFailure(t *testing.T) {
	cfg := config{agmBin: "agm", runCommand: gcThenSweep(
		`{"dry_run":false,"scanned":9,"reaped":4,"kept":5}`, `{"removed":["a"]}`)}

	got, err := sweepMergedWorktrees(context.Background(), cfg)
	if err != nil {
		t.Fatalf("sweepMergedWorktrees: %v", err)
	}
	if got.Error != "" {
		t.Fatalf("a completed reap must not be an error, got %q", got.Error)
	}
	if d := decideBrake(true, got); d.Engage {
		t.Errorf("a successful reap must not latch the brake, got %+v", d)
	}
}

// gcThenSweep returns a runCommand seam that replies to `sandbox gc` with one
// canned payload and to `worktree sweep` with another.
func gcThenSweep(gcJSON, sweepJSON string) func(context.Context, string, ...string) ([]byte, error) {
	return func(_ context.Context, _ string, args ...string) ([]byte, error) {
		if len(args) > 0 && args[0] == "sandbox" {
			return []byte(gcJSON), nil
		}
		return []byte(sweepJSON), nil
	}
}

// --- the watchdog must not answer its own liveness question ---

// Remediation runs `agm sandbox gc --reap` on every breached tick, so the
// records it writes are proof this watchdog ran a sweep — never proof the
// hourly schedule is alive. Counting them means a dead schedule reads as
// healthy for as long as disk pressure lasts, which is exactly when the
// leading indicator matters most.
func TestCheckGCHealth_IgnoresItsOwnRemediationHeartbeats(t *testing.T) {
	now := time.Now()
	cfg := config{gcMaxAge: 6 * time.Hour, gcLogPath: writeGCLog(t,
		gcCompletedAt(now.Add(-72*time.Hour)),
		gcCompletedFrom(now.Add(-2*time.Minute), gcSelfSource),
	)}

	got := checkGCHealth(cfg, now)
	if got == nil || !got.Stale {
		t.Fatalf("a heartbeat this watchdog produced must not prove the schedule is alive, got %+v", got)
	}
}

// An unstamped record is not assumed to be ours: a manual `agm sandbox gc
// --reap` and any agm predating the tag both leave the field empty, and
// discarding those would invent staleness on a working host.
func TestCheckGCHealth_UnstampedHeartbeatStillCounts(t *testing.T) {
	now := time.Now()
	cfg := config{gcMaxAge: 6 * time.Hour, gcLogPath: writeGCLog(t, gcCompletedAt(now.Add(-10*time.Minute)))}

	if got := checkGCHealth(cfg, now); got == nil || got.Stale {
		t.Fatalf("an unstamped heartbeat must still count as proof of life, got %+v", got)
	}
}

// The wiring that makes the tag reach the sweep at all.
func TestRemediationEnv_DeclaresTheProducerTag(t *testing.T) {
	env := remediationEnv()
	if !slices.Contains(env, "AGM_GC_SOURCE="+gcSelfSource) {
		t.Errorf("remediation env %v must declare AGM_GC_SOURCE=%s", env, gcSelfSource)
	}
	if !slices.Contains(env, "GIT_TERMINAL_PROMPT=0") {
		t.Errorf("remediation env %v must still forbid git credential prompts", env)
	}
}

// Ordering, end to end: the tick reads liveness from the log as it stood
// BEFORE remediation. A fake agm stands in for the real one and appends an
// untagged completion record, the shape an older agm would write; grading the
// schedule after that record lands would report a month-dead reaper as healthy.
func TestRun_ReadsReaperLivenessBeforeItsOwnRemediation(t *testing.T) {
	dir := t.TempDir()
	gcLog := writeGCLog(t, gcCompletedAt(time.Now().Add(-72*time.Hour)))
	fakeAgm := writeFakeAgm(t, dir, gcLog)

	var out strings.Builder
	code, err := run([]string{
		"--json",
		"--path", dir,
		"--agm", fakeAgm,
		"--gc-log", gcLog,
		"--gc-max-age", "6h",
		"--trail", filepath.Join(dir, "trail.jsonl"),
		"--brake", filepath.Join(dir, "brake.json"),
		// Force a breach so remediation runs at all.
		"--free-warn-gb", "9999999",
		"--free-critical-gb", "0.0001",
		"--inode-warn", "0.999999",
		"--inode-critical", "0.999999",
	}, &out)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if code != 1 {
		t.Fatalf("exit code = %d, want 1 (breached); output: %s", code, out.String())
	}

	var report struct {
		SandboxGC struct {
			Stale bool `json:"stale"`
		} `json:"sandbox_gc"`
	}
	if err := json.Unmarshal([]byte(out.String()), &report); err != nil {
		t.Fatalf("output is not JSON: %v (%s)", err, out.String())
	}
	if !report.SandboxGC.Stale {
		t.Errorf("the tick graded the reaper on the heartbeat its own remediation wrote: %s", out.String())
	}

	// Premise check: remediation really did run and really did append a fresh
	// record, so the assertion above is about ordering and not about a sweep
	// that never happened.
	body, rerr := os.ReadFile(gcLog)
	if rerr != nil {
		t.Fatalf("read gc log: %v", rerr)
	}
	if !strings.Contains(string(body), "written-by-remediation") {
		t.Fatalf("fake agm never ran; log = %s", body)
	}
}

// writeFakeAgm installs a stand-in `agm` that appends a fresh untagged
// completion record when asked to sweep sandboxes.
func writeFakeAgm(t *testing.T, dir, gcLog string) string {
	t.Helper()
	p := filepath.Join(dir, "fake-agm")
	script := fmt.Sprintf(`#!/bin/sh
if [ "$1" = "sandbox" ]; then
  printf '{"timestamp":"%%s","operation":"sandbox_gc_completed","reason":"written-by-remediation"}\n' \
    "$(date -u +%%Y-%%m-%%dT%%H:%%M:%%SZ)" >> %q
  echo '{"dry_run":false,"scanned":1,"reaped":1,"kept":0}'
  exit 0
fi
echo '{"removed":[]}'
`, gcLog)
	if err := os.WriteFile(p, []byte(script), 0o700); err != nil {
		t.Fatalf("write fake agm: %v", err)
	}
	return p
}

func gcCompletedFrom(ts time.Time, source string) string {
	return `{"timestamp":"` + ts.Format(time.RFC3339Nano) +
		`","operation":"sandbox_gc_completed","source":"` + source + `","reason":"scanned=1 reaped=0 kept=1"}`
}

// --- a negative liveness window is a typo, not a disable switch ---

func TestRun_NegativeGCMaxAgeIsAUsageError(t *testing.T) {
	var out strings.Builder
	code, err := run([]string{"--gc-max-age", "-6h"}, &out)
	if err == nil {
		t.Fatal("a negative reaper-liveness window must be rejected, not silently disable the check")
	}
	if code != 2 {
		t.Errorf("exit code = %d, want 2 (usage error)", code)
	}
}

func TestCheckGCHealth_OnlyZeroDisablesTheCheck(t *testing.T) {
	now := time.Now()
	if got := checkGCHealth(config{gcLogPath: writeGCLog(t, gcCompletedAt(now)), gcMaxAge: 0}, now); got != nil {
		t.Errorf("zero must disable the check, got %+v", got)
	}
}

// --- a heartbeat beyond the byte tail is not "never ran" ---

// Record volume is not bounded by elapsed time: enough session-GC chatter after
// a healthy sandbox sweep pushes that heartbeat out of the first window. The
// scan must widen rather than report a reaper well inside its SLA as one that
// never completed a sweep.
func TestScanGCLog_FindsAHeartbeatPushedBeyondTheFirstWindow(t *testing.T) {
	now := time.Now()
	withScanBounds(t, 64*1024, 1024*1024)

	p := filepath.Join(t.TempDir(), "gc.jsonl")
	var b strings.Builder
	b.WriteString(gcCompletedAt(now.Add(-5*time.Minute)) + "\n")
	filler := `{"timestamp":"` + now.Add(-time.Minute).Format(time.RFC3339Nano) +
		`","operation":"gc_archive","reason":"` + strings.Repeat("y", 2048) + `"}` + "\n"
	for written := int64(0); written < 3*maxGCLogScanBytes; written += int64(len(filler)) {
		b.WriteString(filler)
	}
	if err := os.WriteFile(p, []byte(b.String()), 0o600); err != nil {
		t.Fatalf("write gc log: %v", err)
	}

	got, err := scanGCLog(p, now, 6*time.Hour)
	if err != nil {
		t.Fatalf("scanGCLog: %v", err)
	}
	if got.LastSuccess.IsZero() {
		t.Fatal("a heartbeat inside the liveness window must be found beyond the first byte tail")
	}
	if got.Indeterminate {
		t.Error("liveness was determined; the scan must not report it as undetermined")
	}
}

// Once the window reaches back past the liveness horizon, nothing older can be
// a live heartbeat, so the scan stops rather than reading the whole history.
func TestScanGCLog_StopsWideningPastTheLivenessHorizon(t *testing.T) {
	now := time.Now()
	withScanBounds(t, 64*1024, 64*1024*1024)

	p := filepath.Join(t.TempDir(), "gc.jsonl")
	var b strings.Builder
	filler := `{"timestamp":"` + now.Add(-48*time.Hour).Format(time.RFC3339Nano) +
		`","operation":"gc_archive","reason":"` + strings.Repeat("z", 2048) + `"}` + "\n"
	for written := int64(0); written < 4*maxGCLogScanBytes; written += int64(len(filler)) {
		b.WriteString(filler)
	}
	if err := os.WriteFile(p, []byte(b.String()), 0o600); err != nil {
		t.Fatalf("write gc log: %v", err)
	}

	got, err := scanGCLog(p, now, 6*time.Hour)
	if err != nil {
		t.Fatalf("scanGCLog: %v", err)
	}
	if got.Indeterminate {
		t.Error("history older than the SLA cannot hold a live heartbeat; the answer is determined")
	}
}

// At the hard cap the honest answer is "could not tell", which is a different
// answer from "the reaper never ran" — and it must not be reported as the
// latter, which would send a responder to restart a job whose real problem is
// the size of this log.
func TestCheckGCHealth_UndeterminedIsNotReportedAsNeverRan(t *testing.T) {
	now := time.Now()
	withScanBounds(t, 32*1024, 64*1024)

	p := filepath.Join(t.TempDir(), "gc.jsonl")
	var b strings.Builder
	filler := `{"timestamp":"` + now.Add(-time.Minute).Format(time.RFC3339Nano) +
		`","operation":"gc_archive","reason":"` + strings.Repeat("w", 1024) + `"}` + "\n"
	for written := int64(0); written < 4*maxGCLogTotalScanBytes; written += int64(len(filler)) {
		b.WriteString(filler)
	}
	if err := os.WriteFile(p, []byte(b.String()), 0o600); err != nil {
		t.Fatalf("write gc log: %v", err)
	}

	got := checkGCHealth(config{gcLogPath: p, gcMaxAge: 6 * time.Hour}, now)
	if got == nil || !got.Stale {
		t.Fatalf("an unanswerable liveness question is not a healthy answer, got %+v", got)
	}
	if !got.Indeterminate {
		t.Fatalf("want an undetermined verdict, got %+v", got)
	}
	if strings.Contains(got.Reason, "never recorded") {
		t.Errorf("reason blames a reaper that never ran when the scan simply could not tell: %s", got.Reason)
	}
}

// withScanBounds shrinks the scan window bounds for the duration of a test so
// the widening path can be exercised without writing hundreds of megabytes.
func withScanBounds(t *testing.T, window, total int64) {
	t.Helper()
	oldWindow, oldTotal := maxGCLogScanBytes, maxGCLogTotalScanBytes
	maxGCLogScanBytes, maxGCLogTotalScanBytes = window, total
	t.Cleanup(func() { maxGCLogScanBytes, maxGCLogTotalScanBytes = oldWindow, oldTotal })
}

// --- the wire contract this consumer depends on ---

// These are the exact JSON field names the sweep writes (pinned from the
// producer side in agm/internal/ops and agm/internal/gclog). cmd/ cannot
// import agm/internal/..., so the contract is pinned as literals at both ends
// rather than by a shared type: a rename on either side fails one of them.
func TestSandboxGCSummary_DecodesTheSweepsRefusalWireFormat(t *testing.T) {
	var got sandboxGCSummary
	if err := json.Unmarshal([]byte(
		`{"dry_run":true,"reap_refused":"refusing to reap: live-session inventory is partial","scanned":9,"reaped":4}`,
	), &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !got.DryRun || got.ReapRefused == "" {
		t.Fatalf("the refusal did not decode: %+v", got)
	}
	if got.refusalError() == "" {
		t.Error("a refused reap must read as a remediation failure")
	}
}

func TestGCLogEntry_DecodesTheSweepsProducerTagWireFormat(t *testing.T) {
	var got gcLogEntry
	if err := json.Unmarshal([]byte(
		`{"operation":"sandbox_gc_completed","source":"disk-watchdog"}`,
	), &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got.Source != gcSelfSource {
		t.Errorf("Source = %q, want %q", got.Source, gcSelfSource)
	}
}
