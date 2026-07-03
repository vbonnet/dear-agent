package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/vbonnet/dear-agent/pkg/vroom/supervisor"
)

func TestSweepMergedWorktrees_InvokesExecuteJSON(t *testing.T) {
	var gotName string
	var gotArgs []string
	cfg := config{
		agmBin: "agm",
		runCommand: func(_ context.Context, name string, args ...string) ([]byte, error) {
			gotName = name
			gotArgs = args
			return []byte(`{"worktrees":[],"removed":["/x/a","/x/b"],"failed":{"/x/c":"boom"}}`), nil
		},
	}
	r, err := sweepMergedWorktrees(context.Background(), cfg)
	if err != nil {
		t.Fatalf("sweepMergedWorktrees: %v", err)
	}
	if gotName != "agm" {
		t.Errorf("binary = %q, want agm", gotName)
	}
	want := []string{"worktree", "sweep", "--execute", "-o", "json"}
	if strings.Join(gotArgs, " ") != strings.Join(want, " ") {
		t.Errorf("args = %v, want %v", gotArgs, want)
	}
	if len(r.Removed) != 2 || len(r.Failed) != 1 {
		t.Errorf("parsed result wrong: %+v", r)
	}
}

func TestSweepMergedWorktrees_ErrorPropagates(t *testing.T) {
	cfg := config{
		agmBin: "agm",
		runCommand: func(_ context.Context, _ string, _ ...string) ([]byte, error) {
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
		runCommand: func(_ context.Context, _ string, _ ...string) ([]byte, error) {
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
		supervisor.PressureNone, nil, nil, cfg)
	if !strings.Contains(healthy.String(), "Status: OK") {
		t.Errorf("healthy report missing OK status:\n%s", healthy.String())
	}

	var alarm bytes.Buffer
	emitReport(&alarm, supervisor.ResourceSnapshot{DiskFreeBytes: 2 * supervisor.GiB, DiskUsedFraction: 0.99},
		supervisor.PressureCritical, []string{"disk free 2.0GiB < 5GiB (critical)"},
		&sweepResult{Removed: []string{"/x/a"}}, cfg)
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
		supervisor.PressureWarn, []string{"disk free 10.0GiB < 20GiB (warn)"}, nil, cfg)
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
