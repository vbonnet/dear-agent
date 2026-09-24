package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/vbonnet/dear-agent/internal/gcloghealth"
)

func fixedTime() time.Time {
	return time.Date(2026, 9, 5, 12, 0, 0, 0, time.UTC)
}

func captureStdout(fn func()) string {
	r, w, err := os.Pipe()
	if err != nil {
		panic(err)
	}
	old := os.Stdout
	os.Stdout = w
	defer func() { os.Stdout = old }()

	outC := make(chan string)
	go func() {
		var buf bytes.Buffer
		_, _ = io.Copy(&buf, r)
		outC <- buf.String()
	}()

	defer func() {
		if rec := recover(); rec != nil {
			_ = w.Close()
			panic(rec)
		}
	}()

	fn()
	_ = w.Close()
	return <-outC
}

func writeLog(t *testing.T, lines ...string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "gc.jsonl")
	content := strings.Join(lines, "\n") + "\n"
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatalf("writeLog: %v", err)
	}
	return path
}

func TestRun_HealthyInsideLookback(t *testing.T) {
	now := fixedTime()
	logPath := writeLog(t,
		fmt.Sprintf(`{"timestamp":"%s","operation":"sandbox_gc_completed","reason":"scanned=3 reaped=1 kept=2 errors=0 probe_failures=0"}`,
			now.Add(-2*time.Hour).Format(time.RFC3339)),
	)

	d := defaultDeps()
	d.now = fixedTime

	out := captureStdout(func() {
		code := run([]string{"--log", logPath, "--lookback", "6h"}, d)
		if code != 0 {
			t.Fatalf("run() = %d, want 0", code)
		}
	})

	if !strings.Contains(out, "HEALTHY") {
		t.Errorf("stdout = %q, want HEALTHY", out)
	}
}

func TestRun_DegradedOlderThanLookback(t *testing.T) {
	now := fixedTime()
	logPath := writeLog(t,
		fmt.Sprintf(`{"timestamp":"%s","operation":"sandbox_gc_completed","reason":"scanned=3 reaped=1 kept=2 errors=0 probe_failures=0"}`,
			now.Add(-10*time.Hour).Format(time.RFC3339)),
	)

	d := defaultDeps()
	d.now = fixedTime

	out := captureStdout(func() {
		code := run([]string{"--log", logPath, "--lookback", "6h"}, d)
		if code != 1 {
			t.Fatalf("run() = %d, want 1", code)
		}
	})

	if !strings.Contains(out, "DEGRADED") {
		t.Errorf("stdout = %q, want DEGRADED", out)
	}
}

func TestRun_DegradedZeroCompletedSweeps(t *testing.T) {
	now := fixedTime()
	logPath := writeLog(t,
		fmt.Sprintf(`{"timestamp":"%s","operation":"gc_archive","reason":"eligible"}`,
			now.Add(-1*time.Hour).Format(time.RFC3339)),
	)

	d := defaultDeps()
	d.now = fixedTime

	out := captureStdout(func() {
		code := run([]string{"--log", logPath, "--lookback", "6h"}, d)
		if code != 1 {
			t.Fatalf("run() = %d, want 1", code)
		}
	})

	if !strings.Contains(out, "DEGRADED: no completed sandbox sweeps found") {
		t.Errorf("stdout = %q, want no completed sandbox sweeps found", out)
	}
}

func TestRun_DegradedRejectedDryRun(t *testing.T) {
	now := fixedTime()
	logPath := writeLog(t,
		fmt.Sprintf(`{"timestamp":"%s","operation":"sandbox_gc_completed","dry_run":true,"reason":"scanned=3 reaped=0 kept=3 errors=0"}`,
			now.Add(-1*time.Hour).Format(time.RFC3339)),
	)

	d := defaultDeps()
	d.now = fixedTime

	out := captureStdout(func() {
		code := run([]string{"--log", logPath, "--lookback", "6h"}, d)
		if code != 1 {
			t.Fatalf("run() = %d, want 1", code)
		}
	})

	if !strings.Contains(out, "dry run reclaimed nothing") {
		t.Errorf("stdout = %q, want dry run explanation", out)
	}
}

func TestRun_DegradedRejectedErrors(t *testing.T) {
	now := fixedTime()
	logPath := writeLog(t,
		fmt.Sprintf(`{"timestamp":"%s","operation":"sandbox_gc_completed","errors":2,"reason":"scanned=3 reaped=0 kept=1 errors=2"}`,
			now.Add(-1*time.Hour).Format(time.RFC3339)),
	)

	d := defaultDeps()
	d.now = fixedTime

	out := captureStdout(func() {
		code := run([]string{"--log", logPath, "--lookback", "6h"}, d)
		if code != 1 {
			t.Fatalf("run() = %d, want 1", code)
		}
	})

	if !strings.Contains(out, "2 deletion error(s)") {
		t.Errorf("stdout = %q, want deletion errors explanation", out)
	}
}

func TestRun_DegradedRejectedProbeFailures(t *testing.T) {
	now := fixedTime()
	logPath := writeLog(t,
		fmt.Sprintf(`{"timestamp":"%s","operation":"sandbox_gc_completed","probe_failures":3,"reason":"scanned=3 reaped=0 kept=3 errors=0 probe_failures=3"}`,
			now.Add(-1*time.Hour).Format(time.RFC3339)),
	)

	d := defaultDeps()
	d.now = fixedTime

	out := captureStdout(func() {
		code := run([]string{"--log", logPath, "--lookback", "6h"}, d)
		if code != 1 {
			t.Fatalf("run() = %d, want 1", code)
		}
	})

	if !strings.Contains(out, "3 safety-probe failure(s)") {
		t.Errorf("stdout = %q, want safety probe failure explanation", out)
	}
}

func TestRun_DownMissingLog(t *testing.T) {
	d := defaultDeps()
	d.now = fixedTime

	out := captureStdout(func() {
		code := run([]string{"--log", "/path/that/does/not/exist/gc.jsonl"}, d)
		if code != 2 {
			t.Fatalf("run() = %d, want 2", code)
		}
	})

	if !strings.Contains(out, "DOWN: cannot access sandbox GC log") {
		t.Errorf("stdout = %q, want DOWN: cannot access sandbox GC log", out)
	}
}

func TestRun_DownLogIsDir(t *testing.T) {
	dir := t.TempDir()
	d := defaultDeps()
	d.now = fixedTime

	out := captureStdout(func() {
		code := run([]string{"--log", dir}, d)
		if code != 2 {
			t.Fatalf("run() = %d, want 2", code)
		}
	})

	if !strings.Contains(out, "is a directory") {
		t.Errorf("stdout = %q, want is a directory", out)
	}
}

func TestRun_DownFutureTimestampClockSkew(t *testing.T) {
	now := fixedTime()
	logPath := writeLog(t,
		fmt.Sprintf(`{"timestamp":"%s","operation":"sandbox_gc_completed","reason":"scanned=1 reaped=0 kept=1"}`,
			now.Add(15*time.Minute).Format(time.RFC3339)),
	)

	d := defaultDeps()
	d.now = fixedTime

	out := captureStdout(func() {
		code := run([]string{"--log", logPath, "--lookback", "6h"}, d)
		if code != 2 {
			t.Fatalf("run() = %d, want 2", code)
		}
	})

	if !strings.Contains(out, "DOWN: latest sweep timestamp") || !strings.Contains(out, "in the future") {
		t.Errorf("stdout = %q, want DOWN future message", out)
	}
}

func TestRun_UsageBadLookback(t *testing.T) {
	d := defaultDeps()

	code := run([]string{"--lookback", "invalid"}, d)
	if code != 3 {
		t.Fatalf("run() = %d, want 3 for invalid lookback", code)
	}

	code = run([]string{"--lookback", "-1h"}, d)
	if code != 3 {
		t.Fatalf("run() = %d, want 3 for negative lookback", code)
	}
}

func TestRun_HomeDirErrorOnTildePath(t *testing.T) {
	d := defaultDeps()
	d.userHomeDir = func() (string, error) {
		return "", errors.New("cannot determine home directory")
	}

	code := run([]string{"--log", "~/test/gc.jsonl"}, d)
	if code != 3 {
		t.Fatalf("run() = %d, want 3", code)
	}
}

func TestRun_JSONReport(t *testing.T) {
	now := fixedTime()
	logPath := writeLog(t,
		fmt.Sprintf(`{"timestamp":"%s","operation":"sandbox_gc_completed","reason":"scanned=5 reaped=2 kept=3 errors=0 probe_failures=0"}`,
			now.Add(-45*time.Minute).Format(time.RFC3339)),
	)

	d := defaultDeps()
	d.now = fixedTime

	var r Report
	out := captureStdout(func() {
		code := run([]string{"--log", logPath, "--lookback", "2h", "--json"}, d)
		if code != 0 {
			t.Fatalf("run() = %d, want 0", code)
		}
	})

	if err := json.Unmarshal([]byte(out), &r); err != nil {
		t.Fatalf("unmarshal json: %v, raw output: %q", err, out)
	}

	if r.Status != "healthy" {
		t.Errorf("r.Status = %q, want healthy", r.Status)
	}
	if r.LatestSweepAge != "45m0s" {
		t.Errorf("r.LatestSweepAge = %q, want 45m0s", r.LatestSweepAge)
	}
	if r.Lookback != "2h" {
		t.Errorf("r.Lookback = %q, want 2h", r.Lookback)
	}
}

func TestRun_CompatibilityReapRecordsOnly(t *testing.T) {
	now := fixedTime()
	logPath := writeLog(t,
		fmt.Sprintf(`{"timestamp":"%s","operation":"sandbox_gc_reap","reason":"reaped old sandbox"}`,
			now.Add(-1*time.Hour).Format(time.RFC3339)),
	)

	d := defaultDeps()
	d.now = fixedTime

	out := captureStdout(func() {
		code := run([]string{"--log", logPath, "--lookback", "6h"}, d)
		if code != 0 {
			t.Fatalf("run() = %d, want 0", code)
		}
	})

	if !strings.Contains(out, "HEALTHY") {
		t.Errorf("stdout = %q, want HEALTHY for compatibility reap", out)
	}
}

func TestRun_BoundedTailScanDiscardsPartialLine(t *testing.T) {
	now := fixedTime()
	prefixLine := `{"timestamp":"2026-09-05T01:00:00Z","operation":"ignored_prefix_line"}`
	sweepLine := fmt.Sprintf(`{"timestamp":"%s","operation":"sandbox_gc_completed","reason":"scanned=2 reaped=1 kept=1 errors=0 probe_failures=0"}`,
		now.Add(-30*time.Minute).Format(time.RFC3339))

	logPath := writeLog(t, prefixLine, sweepLine)

	d := defaultDeps()
	d.now = fixedTime
	d.maxLogScanBytes = int64(len(sweepLine) + 20)

	out := captureStdout(func() {
		code := run([]string{"--log", logPath, "--lookback", "6h"}, d)
		if code != 0 {
			t.Fatalf("run() = %d, want 0", code)
		}
	})

	if !strings.Contains(out, "HEALTHY") {
		t.Errorf("stdout = %q, want HEALTHY", out)
	}
}

func TestRun_WatchdogRemediationCannotCertifySchedule(t *testing.T) {
	now := fixedTime()
	logPath := writeLog(t,
		fmt.Sprintf(`{"timestamp":%q,"operation":"sandbox_gc_completed"}`,
			now.Add(-8*time.Hour).Format(time.RFC3339)),
		fmt.Sprintf(`{"timestamp":%q,"operation":"sandbox_gc_completed","source":"disk-watchdog"}`,
			now.Add(-time.Minute).Format(time.RFC3339)),
	)
	d := defaultDeps()
	d.now = fixedTime
	var report Report
	out := captureStdout(func() {
		if code := run([]string{"--log", logPath, "--json"}, d); code != 1 {
			t.Fatalf("run() = %d, want degraded/1", code)
		}
	})
	if err := json.Unmarshal([]byte(out), &report); err != nil {
		t.Fatal(err)
	}
	if report.Status != "degraded" || report.LatestSweepAt != now.Add(-8*time.Hour).Format(time.RFC3339) {
		t.Fatalf("report = %+v, want stale scheduled proof only", report)
	}
}

func TestRun_ExplicitSandboxErrorIsDiagnostic(t *testing.T) {
	now := fixedTime()
	logPath := writeLog(t,
		fmt.Sprintf(`{"timestamp":%q,"operation":"gc_archive_error","error":"session error"}`,
			now.Add(-2*time.Minute).Format(time.RFC3339)),
		fmt.Sprintf(`{"timestamp":%q,"operation":"sandbox_gc_error","error":"mount table unreadable"}`,
			now.Add(-time.Minute).Format(time.RFC3339)),
	)
	d := defaultDeps()
	d.now = fixedTime
	out := captureStdout(func() {
		if code := run([]string{"--log", logPath}, d); code != 1 {
			t.Fatalf("run() = %d, want degraded/1", code)
		}
	})
	if !strings.Contains(out, "mount table unreadable") || strings.Contains(out, "session error") {
		t.Fatalf("output = %q, want sandbox diagnostic only", out)
	}
}

func TestRun_IndeterminateTailIsNotNeverRan(t *testing.T) {
	now := fixedTime()
	lines := []string{fmt.Sprintf(`{"timestamp":%q,"operation":"sandbox_gc_completed"}`,
		now.Add(-5*time.Minute).Format(time.RFC3339))}
	for range 12 {
		lines = append(lines, fmt.Sprintf(`{"timestamp":%q,"operation":"gc_archive","reason":"chatter"}`,
			now.Add(-time.Minute).Format(time.RFC3339)))
	}
	lines = append(lines, fmt.Sprintf(`{"timestamp":%q,"operation":"gc_archive","reason":"backdated"}`,
		now.Add(-48*time.Hour).Format(time.RFC3339)))
	logPath := writeLog(t, lines...)
	d := defaultDeps()
	d.now = fixedTime
	d.maxLogScanBytes, d.maxLogMaxBytes = 128, 256
	out := captureStdout(func() {
		if code := run([]string{"--log", logPath}, d); code != 1 {
			t.Fatalf("run() = %d, want degraded/1", code)
		}
	})
	if !strings.Contains(out, "undetermined") || strings.Contains(out, "no completed") {
		t.Fatalf("output = %q, want explicit uncertainty", out)
	}
}

func TestRun_StaleCompletionInCappedTailIsNotDefinitiveLatest(t *testing.T) {
	now := fixedTime()
	lines := []string{fmt.Sprintf(`{"timestamp":%q,"operation":"sandbox_gc_completed"}`,
		now.Add(-5*time.Minute).Format(time.RFC3339))}
	for range 12 {
		lines = append(lines, fmt.Sprintf(`{"timestamp":%q,"operation":"gc_archive","reason":"chatter"}`,
			now.Add(-time.Minute).Format(time.RFC3339)))
	}
	lines = append(lines, fmt.Sprintf(`{"timestamp":%q,"operation":"sandbox_gc_completed"}`,
		now.Add(-48*time.Hour).Format(time.RFC3339)))
	logPath := writeLog(t, lines...)
	d := defaultDeps()
	d.now = fixedTime
	d.maxLogScanBytes, d.maxLogMaxBytes = 128, 256
	var report Report
	out := captureStdout(func() {
		if code := run([]string{"--log", logPath, "--json"}, d); code != 1 {
			t.Fatalf("run() = %d, want degraded/1", code)
		}
	})
	if err := json.Unmarshal([]byte(out), &report); err != nil {
		t.Fatal(err)
	}
	if report.Status != "degraded" || report.LatestSweepAt != "" ||
		!strings.Contains(report.Error, "undetermined") {
		t.Fatalf("report = %+v, want uncertainty without latest-sweep claim", report)
	}
}

// sweep-health must order errors against the observed success, the same way
// the watchdog adapter does.
//
// Proof() is zero for an indeterminate scan, so an error the log itself shows
// was followed by a successful sweep would otherwise be reported as the live
// problem and send responders after something already fixed.
func TestSweepHealthOrdersErrorsAgainstObservedSuccess(t *testing.T) {
	now := time.Now()
	superseded := gcloghealth.Summary{
		LastSuccess:   now.Add(-3 * time.Hour),
		LastError:     "superseded: mount table unreadable",
		LastErrorAt:   now.Add(-5 * time.Hour),
		Indeterminate: true,
	}
	var r Report
	line, code := evaluateSweep(superseded, now, time.Hour, "24h", &r)
	if strings.Contains(line, "superseded") || strings.Contains(r.Error, "superseded") {
		t.Errorf("report = %q / %q, want the superseded error left out", line, r.Error)
	}
	if code != 1 {
		t.Errorf("exit code = %d, want 1 for an indeterminate scan", code)
	}

	// An error genuinely newer than the observed success still surfaces.
	live := superseded
	live.LastErrorAt = now.Add(-time.Hour)
	live.LastError = "live: deletion refused"
	r = Report{}
	line, _ = evaluateSweep(live, now, time.Hour, "24h", &r)
	if !strings.Contains(line, "live: deletion refused") {
		t.Errorf("report = %q, want an error newer than the observed success reported", line)
	}
}
