package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// writeGCLog writes a gc.jsonl fixture and returns its path.
func writeGCLog(t *testing.T, lines ...string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "gc.jsonl")
	if err := os.WriteFile(p, []byte(strings.Join(lines, "\n")+"\n"), 0o600); err != nil {
		t.Fatalf("write gc log: %v", err)
	}
	return p
}

func gcCompletedAt(ts time.Time) string {
	return `{"timestamp":"` + ts.Format(time.RFC3339Nano) +
		`","operation":"sandbox_gc_completed","reason":"scanned=3 reaped=0 kept=3 errors=0"}`
}

func gcErrorAt(ts time.Time, msg string) string {
	return `{"timestamp":"` + ts.Format(time.RFC3339Nano) +
		`","operation":"sandbox_gc_error","reason":"live_session_inventory_failed","error":"` + msg + `"}`
}

func TestCheckGCHealth_RecentSweepIsHealthy(t *testing.T) {
	now := time.Now()
	cfg := config{gcLogPath: writeGCLog(t, gcCompletedAt(now.Add(-30*time.Minute))), gcMaxAge: 6 * time.Hour}

	got := checkGCHealth(cfg, now)
	if got == nil || got.Stale {
		t.Fatalf("recent sweep should be healthy, got %+v", got)
	}
	if got.Age < 29*time.Minute || got.Age > 31*time.Minute {
		t.Errorf("Age = %s, want ~30m", got.Age)
	}
}

// A sweep that reaps nothing is still a healthy sweep. This is the case that
// makes a completion heartbeat necessary: counting reap records alone would
// classify a correctly-idle reaper as dead.
func TestCheckGCHealth_ReapNothingSweepCountsAsAlive(t *testing.T) {
	now := time.Now()
	cfg := config{gcLogPath: writeGCLog(t, gcCompletedAt(now.Add(-10*time.Minute))), gcMaxAge: time.Hour}

	if got := checkGCHealth(cfg, now); got == nil || got.Stale {
		t.Fatalf("a reap-nothing sweep must count as alive, got %+v", got)
	}
}

func TestCheckGCHealth_StaleSweepAlarmsWithLastError(t *testing.T) {
	now := time.Now()
	cfg := config{gcMaxAge: 6 * time.Hour, gcLogPath: writeGCLog(t,
		gcCompletedAt(now.Add(-72*time.Hour)),
		gcErrorAt(now.Add(-time.Hour), "multiple enabled workspaces require a port"),
	)}

	got := checkGCHealth(cfg, now)
	if got == nil || !got.Stale {
		t.Fatalf("72h-old sweep should be stale, got %+v", got)
	}
	// The alarm must carry the failure cause, not just "it is old" — that is the
	// difference between a page you can act on and one you must go dig for.
	if !strings.Contains(got.Reason, "multiple enabled workspaces require a port") {
		t.Errorf("stale reason should quote the last GC error, got %q", got.Reason)
	}
}

// An error older than the last successful sweep was already superseded and must
// not be reported as the current cause.
func TestCheckGCHealth_SupersededErrorIsNotReported(t *testing.T) {
	now := time.Now()
	cfg := config{gcMaxAge: 6 * time.Hour, gcLogPath: writeGCLog(t,
		gcErrorAt(now.Add(-48*time.Hour), "transient dolt restart"),
		gcCompletedAt(now.Add(-24*time.Hour)),
	)}

	got := checkGCHealth(cfg, now)
	if got == nil || !got.Stale {
		t.Fatalf("24h-old sweep should be stale, got %+v", got)
	}
	if got.LastError != "" {
		t.Errorf("superseded error should not be reported, got %q", got.LastError)
	}
}

// The check exists for the case where the reaper is not running at all, which
// frequently means there is no log to read. Absence must not read as health.
func TestCheckGCHealth_MissingLogIsStale(t *testing.T) {
	cfg := config{gcLogPath: filepath.Join(t.TempDir(), "absent.jsonl"), gcMaxAge: 6 * time.Hour}

	if got := checkGCHealth(cfg, time.Now()); got == nil || !got.Stale {
		t.Fatalf("missing GC log must be stale, got %+v", got)
	}
}

func TestCheckGCHealth_NeverSweptIsStale(t *testing.T) {
	now := time.Now()
	cfg := config{gcMaxAge: 6 * time.Hour, gcLogPath: writeGCLog(t,
		gcErrorAt(now.Add(-2*time.Hour), "inventory failed"),
	)}

	got := checkGCHealth(cfg, now)
	if got == nil || !got.Stale {
		t.Fatalf("a log with no proof of life must be stale, got %+v", got)
	}
	if !got.LastSuccess.IsZero() {
		t.Errorf("LastSuccess should be zero, got %v", got.LastSuccess)
	}
}

func TestCheckGCHealth_DisabledByZeroMaxAge(t *testing.T) {
	cfg := config{gcLogPath: filepath.Join(t.TempDir(), "absent.jsonl"), gcMaxAge: 0}

	if got := checkGCHealth(cfg, time.Now()); got != nil {
		t.Fatalf("zero max-age must disable the check, got %+v", got)
	}
}

// A malformed line must not abort the scan: the newest good record still wins.
func TestCheckGCHealth_SkipsMalformedLines(t *testing.T) {
	now := time.Now()
	cfg := config{gcMaxAge: 6 * time.Hour, gcLogPath: writeGCLog(t,
		"{not json at all",
		gcCompletedAt(now.Add(-5*time.Minute)),
	)}

	if got := checkGCHealth(cfg, now); got == nil || got.Stale {
		t.Fatalf("malformed lines must be skipped, got %+v", got)
	}
}

// hermeticRunArgs pins every path at a temp dir and forces the disk thresholds
// so low they cannot trip, so the only alarm source under test is the reaper.
func hermeticRunArgs(t *testing.T, gcLog, gcMaxAge string) []string {
	t.Helper()
	return []string{
		"--path", t.TempDir(),
		"--trail", filepath.Join(t.TempDir(), "trail.jsonl"),
		"--brake", filepath.Join(t.TempDir(), "brake.json"),
		// A path that cannot exist: if the no-breach premise ever breaks, the
		// test fails loudly instead of shelling out to real remediation.
		"--agm", filepath.Join(t.TempDir(), "no-such-agm"),
		"--gc-log", gcLog,
		"--gc-max-age", gcMaxAge,
		"--e2e-cache-dir", t.TempDir(),
		"--free-warn-gb", "0.0001",
		"--free-critical-gb", "0.0001",
		"--inode-warn", "0.999999",
		"--inode-critical", "0.999999",
	}
}

// End-to-end: a host with plenty of free disk but a dead reaper must still exit
// 1. This is the regression that let ~/.agm/sandboxes reach 239 GB across 119
// dirs while every disk-watchdog tick reported "Status: OK".
func TestRun_StaleReaperAlarmsOnAHealthyDisk(t *testing.T) {
	var out bytes.Buffer
	gcLog := writeGCLog(t, gcCompletedAt(time.Now().Add(-48*time.Hour)))

	code, err := run(hermeticRunArgs(t, gcLog, "6h"), &out)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if code != 1 {
		t.Errorf("exit code = %d, want 1 for a stale reaper (output: %s)", code, out.String())
	}
	if !strings.Contains(out.String(), "sandbox GC last completed a sweep") {
		t.Errorf("report should name the stale reaper:\n%s", out.String())
	}
}

// The converse: a healthy reaper on a healthy disk must stay exit 0, so the new
// check cannot turn every tick into a permanent alarm.
func TestRun_HealthyReaperKeepsTickGreen(t *testing.T) {
	var out bytes.Buffer
	gcLog := writeGCLog(t, gcCompletedAt(time.Now().Add(-15*time.Minute)))

	code, err := run(hermeticRunArgs(t, gcLog, "6h"), &out)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if code != 0 {
		t.Errorf("exit code = %d, want 0 (output: %s)", code, out.String())
	}
}

// A stale reaper must not latch the admission brake: blocking every spawn
// because a GC is behind would be a worse outage than the leak it warns about.
func TestRun_StaleReaperDoesNotEngageTheBrake(t *testing.T) {
	var out bytes.Buffer
	brake := filepath.Join(t.TempDir(), "brake.json")
	gcLog := writeGCLog(t, gcCompletedAt(time.Now().Add(-48*time.Hour)))

	args := hermeticRunArgs(t, gcLog, "6h")
	for i, a := range args {
		if a == "--brake" {
			args[i+1] = brake
		}
	}
	if _, err := run(args, &out); err != nil {
		t.Fatalf("run: %v", err)
	}

	if _, err := os.Stat(brake); err == nil {
		t.Errorf("stale reaper must not engage the admission brake, but %s exists", brake)
	}
}
