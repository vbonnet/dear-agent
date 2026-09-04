package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/vbonnet/dear-agent/pkg/vroom/admission"
	"github.com/vbonnet/dear-agent/pkg/vroom/supervisor"
)

func TestSweepMergedWorktrees_InvokesExecuteJSON(t *testing.T) {
	var calls []string
	cfg := config{
		agmBin: "agm",
		runCommand: func(_ context.Context, name string, args ...string) ([]byte, error) {
			calls = append(calls, name+" "+strings.Join(args, " "))
			if strings.Join(args, " ") == "sandbox gc --reap --json" {
				return []byte(`{"scanned":3,"reaped":1,"kept":2,"errors":0}`), nil
			}
			return []byte(`{"worktrees":[],"removed":["/x/a","/x/b"],"failed":{"/x/c":"boom"}}`), nil
		},
	}
	r, err := sweepMergedWorktrees(context.Background(), cfg)
	if err != nil {
		t.Fatalf("sweepMergedWorktrees: %v", err)
	}
	want := []string{
		"agm sandbox gc --reap --json",
		"agm worktree sweep --execute -o json",
	}
	if strings.Join(calls, "\n") != strings.Join(want, "\n") {
		t.Errorf("calls = %v, want %v", calls, want)
	}
	if len(r.Removed) != 2 || len(r.Failed) != 1 {
		t.Errorf("parsed result wrong: %+v", r)
	}
	if r.SandboxGC == nil || r.SandboxGC.Reaped != 1 {
		t.Errorf("sandbox gc summary = %+v, want reaped=1", r.SandboxGC)
	}
}

func TestSweepMergedWorktrees_ErrorPropagates(t *testing.T) {
	cfg := config{
		agmBin: "agm",
		runCommand: func(_ context.Context, _ string, args ...string) ([]byte, error) {
			if strings.Join(args, " ") == "sandbox gc --reap --json" {
				return []byte(`{"scanned":0,"reaped":0,"kept":0,"errors":0}`), nil
			}
			return nil, errors.New("exit 1")
		},
	}
	if _, err := sweepMergedWorktrees(context.Background(), cfg); err == nil {
		t.Fatal("expected error from failing sweep")
	}
}

func TestSweepMergedWorktrees_GarbageOutputIsError(t *testing.T) {
	cfg := config{
		agmBin: "agm",
		runCommand: func(_ context.Context, _ string, args ...string) ([]byte, error) {
			if strings.Join(args, " ") == "sandbox gc --reap --json" {
				return []byte(`{"scanned":0,"reaped":0,"kept":0,"errors":0}`), nil
			}
			return []byte("not json"), nil
		},
	}
	if _, err := sweepMergedWorktrees(context.Background(), cfg); err == nil {
		t.Fatal("expected error from unparseable sweep output")
	}
}

func TestEmitReport_HealthyAndAlarm(t *testing.T) {
	cfg := config{path: "/", thresholds: supervisor.DefaultDiskAlertThresholds}

	var healthy bytes.Buffer
	emitReport(&healthy, supervisor.ResourceSnapshot{DiskFreeBytes: 500 * supervisor.GiB, DiskUsedFraction: 0.4},
		supervisor.PressureNone, nil, nil, cfg, nil, nil, nil, nil)
	if !strings.Contains(healthy.String(), "Status: OK") {
		t.Errorf("healthy report missing OK status:\n%s", healthy.String())
	}

	var alarm bytes.Buffer
	emitReport(&alarm, supervisor.ResourceSnapshot{DiskFreeBytes: 2 * supervisor.GiB, DiskUsedFraction: 0.99},
		supervisor.PressureCritical, []string{"disk free 2.0GiB < 5GiB (critical)"},
		&sweepResult{Removed: []string{"/x/a"}}, cfg, nil, nil, nil, nil)
	got := alarm.String()
	if !strings.Contains(got, "Status: ALARM (critical)") {
		t.Errorf("alarm report missing status:\n%s", got)
	}
	if !strings.Contains(got, "reaped 1 provably-merged worktree(s)") {
		t.Errorf("alarm report missing remediation summary:\n%s", got)
	}
}

func TestEmitJSON_Shape(t *testing.T) {
	cfg := config{path: "/", thresholds: supervisor.DefaultDiskAlertThresholds}
	var buf bytes.Buffer
	err := emitJSON(&buf, supervisor.ResourceSnapshot{DiskFreeBytes: 10 * supervisor.GiB, DiskUsedFraction: 0.9},
		supervisor.PressureWarn, []string{"disk free 10.0GiB < 20GiB (warn)"}, nil, cfg, nil, nil, nil, nil)
	if err != nil {
		t.Fatalf("emitJSON: %v", err)
	}
	var got map[string]any
	if err := json.Unmarshal(buf.Bytes(), &got); err != nil {
		t.Fatalf("output is not JSON: %v", err)
	}
	if got["level"] != "warn" || got["ok"] != false {
		t.Errorf("unexpected JSON report: %v", got)
	}
	if got["disk_free_gib"].(float64) != 10 {
		t.Errorf("disk_free_gib = %v, want 10", got["disk_free_gib"])
	}
}

// TestRun_HealthyHostExitsZero exercises the full run path against the real
// filesystem. It asserts only invariants that hold on any sane CI host: with
// thresholds forced to the extremes the classifier cannot trip, the exit code
// is 0 and no remediation runs.
func TestRun_HealthyHostExitsZero(t *testing.T) {
	var buf bytes.Buffer
	code, err := run([]string{
		"--dry-run", "--json",
		// The reaper-liveness check reads an absolute default path under $HOME;
		// disable it so this test stays hermetic and asserts only disk state.
		"--gc-max-age", "0",
		// ~1 byte each: a sub-byte value would truncate to 0 and re-inherit the
		// real defaults, which could trip on a genuinely low-disk CI host.
		"--free-warn-gb", "0.000000001",
		"--free-critical-gb", "0.000000001",
		"--inode-warn", "0.999999",
		"--inode-critical", "0.9999999",
	}, &buf)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if code != 0 {
		t.Fatalf("exit code = %d, want 0 (output: %s)", code, buf.String())
	}
	var got map[string]any
	if err := json.Unmarshal(buf.Bytes(), &got); err != nil {
		t.Fatalf("output is not JSON: %v", err)
	}
	if got["ok"] != true {
		t.Errorf("ok = %v, want true", got["ok"])
	}
}

func TestRun_BadFlagIsUsageError(t *testing.T) {
	var buf bytes.Buffer
	if _, err := run([]string{"--no-such-flag"}, &buf); err == nil {
		t.Fatal("expected usage error")
	}
}

// --- admission brake (ce-93lw.18) ---

// TestDecideBrake pins the tick-outcome to brake-transition policy. Row two is
// the 2026-07-18 incident itself: alarmed, with the remediation being SIGKILLed.
func TestDecideBrake(t *testing.T) {
	killed := &sweepResult{Error: "/Users/x/go/bin/agm worktree sweep --execute: signal: killed"}
	swept := &sweepResult{Removed: []string{"/w/a", "/w/b"}}

	tests := []struct {
		name        string
		breached    bool
		rem         *sweepResult
		wantEngage  bool
		wantRelease bool
	}{
		{"healthy tick releases", false, nil, false, true},
		{"breached with killed remediation engages", true, killed, true, false},
		{"breached with successful remediation leaves it alone", true, swept, false, false},
		{"breached with no remediation attempt leaves it alone", true, nil, false, false},
		{"healthy tick releases even if a sweep ran", false, swept, false, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := decideBrake(tt.breached, tt.rem, 0)
			if got.Engage != tt.wantEngage || got.Release != tt.wantRelease {
				t.Errorf("decideBrake(%v, %+v, 0) = {engage:%v release:%v}, want {engage:%v release:%v}",
					tt.breached, tt.rem, got.Engage, got.Release, tt.wantEngage, tt.wantRelease)
			}
		})
	}
}

func TestDecideBrake_ReasonCarriesTheRemediationError(t *testing.T) {
	d := decideBrake(true, &sweepResult{Error: "agm worktree sweep --execute: signal: killed"}, 0)
	if !d.Engage {
		t.Fatal("a failed remediation must engage the brake")
	}
	if !strings.Contains(d.Reason, "signal: killed") {
		t.Errorf("reason = %q, want it to carry the remediation error verbatim", d.Reason)
	}
}

func TestUpdateAdmissionBrake_EngagesAndReleases(t *testing.T) {
	path := filepath.Join(t.TempDir(), "admission-brake.json")
	cfg := config{brakePath: path, brakeTTL: time.Hour}

	updateAdmissionBrake(cfg, true, &sweepResult{Error: "signal: killed"}, 0)
	brake, err := admission.Read(path)
	if err != nil {
		t.Fatalf("Read after engage: %v", err)
	}
	switch {
	case brake == nil:
		t.Fatal("brake not engaged after a failed remediation")
	case brake.Source != brakeSource:
		t.Errorf("Source = %q, want %q", brake.Source, brakeSource)
	case !strings.Contains(brake.Reason, "signal: killed"):
		t.Errorf("Reason = %q, want the remediation error", brake.Reason)
	}

	updateAdmissionBrake(cfg, false, nil, 0)
	brake, err = admission.Read(path)
	if err != nil {
		t.Fatalf("Read after release: %v", err)
	}
	if brake != nil {
		t.Errorf("brake still engaged after a healthy tick: %+v", brake)
	}
}

func TestUpdateAdmissionBrake_SuccessfulRemediationLeavesBrakeInPlace(t *testing.T) {
	path := filepath.Join(t.TempDir(), "admission-brake.json")
	cfg := config{brakePath: path, brakeTTL: time.Hour}

	if err := admission.Engage(path, brakeSource, "earlier failure", time.Hour); err != nil {
		t.Fatalf("Engage: %v", err)
	}
	updateAdmissionBrake(cfg, true, &sweepResult{Removed: []string{"/w/a"}}, 0)

	brake, err := admission.Read(path)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	switch {
	case brake == nil:
		t.Fatal("one successful sweep under an active alarm is not evidence of health; brake should remain")
	case brake.Reason != "earlier failure":
		t.Errorf("Reason = %q, want the original brake preserved", brake.Reason)
	}
}

func TestApplyBrake_DryRunNeverTouchesTheFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "admission-brake.json")
	cfg := config{brakePath: path, brakeTTL: time.Hour, dryRun: true}

	applyBrake(cfg, true, "would engage")
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Errorf("--dry-run wrote a brake file (stat err = %v)", err)
	}

	if err := admission.Engage(path, brakeSource, "pre-existing", time.Hour); err != nil {
		t.Fatalf("Engage: %v", err)
	}
	applyBrake(cfg, false, "")
	if _, err := os.Stat(path); err != nil {
		t.Errorf("--dry-run removed an existing brake file: %v", err)
	}
}

// A brake write failure is a warning, never a reason to suppress the alarm.
func TestApplyBrake_WriteFailureDoesNotPanicOrExit(t *testing.T) {
	// A path whose parent is a regular file: MkdirAll and rename both fail.
	file := filepath.Join(t.TempDir(), "not-a-dir")
	if err := os.WriteFile(file, []byte("x"), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	cfg := config{brakePath: filepath.Join(file, "admission-brake.json"), brakeTTL: time.Hour}

	applyBrake(cfg, true, "boom") // must not panic
	applyBrake(cfg, false, "")    // must not panic
}

func TestRun_HealthyTickReleasesAStaleBrake(t *testing.T) {
	path := filepath.Join(t.TempDir(), "admission-brake.json")
	if err := admission.Engage(path, brakeSource, "stale from an earlier incident", time.Hour); err != nil {
		t.Fatalf("Engage: %v", err)
	}

	var out bytes.Buffer
	// Thresholds a real filesystem cannot breach, so the tick is healthy.
	//
	// They must be tiny-but-POSITIVE: DiskAlertThresholds.withDefaults treats a
	// zero threshold as "unset" and substitutes the 20 GiB default, so passing 0
	// here silently asked for default thresholds instead of disabling them. That
	// made this test pass only while the host happened to have >20 GiB free, and
	// on a fuller disk it breached and ran the real `agm worktree sweep
	// --execute`.
	//
	// --agm points at a path that cannot exist, so if the no-breach premise ever
	// breaks again this test fails loudly instead of shelling out to real
	// worktree remediation.
	code, err := run([]string{
		"--path", t.TempDir(),
		"--brake", path,
		"--trail", filepath.Join(t.TempDir(), "trail.jsonl"),
		"--agm", filepath.Join(t.TempDir(), "no-such-agm"),
		"--e2e-cache-dir", t.TempDir(),
		// Hermetic: do not consult the real ~/.agm/logs/gc.jsonl.
		"--gc-max-age", "0",
		"--free-warn-gb", "0.0001",
		"--free-critical-gb", "0.0001",
		"--inode-warn", "0.999999",
		"--inode-critical", "0.999999",
	}, &out)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if code != 0 {
		t.Fatalf("exit code = %d, want 0 for a healthy tick (output: %s)", code, out.String())
	}

	brake, rerr := admission.Read(path)
	if rerr != nil {
		t.Fatalf("Read: %v", rerr)
	}
	if brake != nil {
		t.Errorf("healthy tick left the brake engaged: %+v", brake)
	}
}

// The mirror of the governor's guard: a healthy disk tick must not clear a
// brake vroom-governor engaged because its own probes had gone unreadable.
func TestApplyBrake_HealthyTickDoesNotClearAnotherWatchdogsBrake(t *testing.T) {
	path := filepath.Join(t.TempDir(), "admission-brake.json")
	cfg := config{brakePath: path, brakeTTL: time.Hour}
	if err := admission.Engage(path, "vroom-governor", "load probe unreadable", time.Hour); err != nil {
		t.Fatalf("Engage: %v", err)
	}

	applyBrake(cfg, false, "")

	brake, err := admission.Read(path)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	switch {
	case brake == nil:
		t.Fatal("a healthy disk tick cleared the vroom-governor brake")
	case brake.Source != "vroom-governor":
		t.Errorf("Source = %q, want the vroom-governor brake preserved", brake.Source)
	}
}
