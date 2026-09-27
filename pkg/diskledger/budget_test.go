package diskledger

import (
	"path/filepath"
	"testing"
	"time"
)

func TestEvaluateRootsFlagsOverBudget(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "big.bin"), 1<<20)

	got := EvaluateRoots([]RootBudget{
		{Name: "tiny", Path: dir, BudgetBytes: 1024},
		{Name: "roomy", Path: dir, BudgetBytes: 1 << 30},
	})
	if len(got) != 2 {
		t.Fatalf("got %d verdicts, want 2", len(got))
	}
	if !got[0].OverBudget {
		t.Errorf("tiny root should be over budget: %+v", got[0])
	}
	if got[1].OverBudget {
		t.Errorf("roomy root should be within budget: %+v", got[1])
	}
}

// A budget on a path that does not exist is not a leak. Reporting it as one
// would make every un-provisioned cache alarm on a fresh host.
func TestEvaluateRootsIgnoresMissingPaths(t *testing.T) {
	got := EvaluateRoots([]RootBudget{
		{Name: "absent", Path: filepath.Join(t.TempDir(), "nope"), BudgetBytes: 1},
	})
	if len(got) != 1 {
		t.Fatalf("got %d verdicts, want 1", len(got))
	}
	if got[0].OverBudget {
		t.Error("missing path must not be flagged over budget")
	}
}

// This is the exact defect observed in production: the watchdog reported
// "build caches: 14 found, 0 reaped" and "Status: OK" while the shared GOCACHE
// held 44 GiB. Reclaiming nothing is only healthy if there was nothing to
// reclaim; if a tracked root is over budget and nothing came back, the GC is
// silently doing nothing and that must alarm.
func TestReclaimHealthAlarmsOnOverBudgetWithZeroReclaim(t *testing.T) {
	verdicts := []RootVerdict{{Name: "gocache", OverBudget: true, AllocatedBytes: 44 << 30, BudgetBytes: 10 << 30}}

	h := CheckReclaimHealth(verdicts, 0, time.Hour)
	if !h.Alarming() {
		t.Fatalf("over-budget root with zero reclaim must alarm, got %+v", h)
	}
	if h.Status != StatusSilentGC {
		t.Errorf("Status = %q, want %q", h.Status, StatusSilentGC)
	}
}

func TestReclaimHealthOKWhenNothingOverBudget(t *testing.T) {
	verdicts := []RootVerdict{{Name: "gocache", OverBudget: false, AllocatedBytes: 1 << 30, BudgetBytes: 10 << 30}}
	h := CheckReclaimHealth(verdicts, 0, time.Hour)
	if h.Alarming() {
		t.Errorf("zero reclaim with nothing over budget is healthy, got %+v", h)
	}
}

// Over budget but the GC is visibly working: that is pressure, not a silent
// GC. It should report differently so an operator can tell "reaper is broken"
// from "reaper is running and losing the race".
func TestReclaimHealthDistinguishesWorkingGCFromSilentGC(t *testing.T) {
	verdicts := []RootVerdict{{Name: "gocache", OverBudget: true, AllocatedBytes: 44 << 30, BudgetBytes: 10 << 30}}
	h := CheckReclaimHealth(verdicts, 5<<30, time.Hour)
	if h.Status == StatusSilentGC {
		t.Errorf("reclaiming 5GiB is not a silent GC, got %+v", h)
	}
	if !h.Alarming() {
		t.Errorf("still over budget, should remain alarming: %+v", h)
	}
}

func TestDefaultRootBudgetsCoverTheLeakingSharedCaches(t *testing.T) {
	budgets := DefaultRootBudgets("/home/u")
	byName := map[string]RootBudget{}
	for _, b := range budgets {
		byName[b.Name] = b
	}
	// These are the roots that actually leaked on the host and that the
	// pre-existing watchdog never scanned: it only looked at /tmp and TMPDIR.
	for _, want := range []string{"gocache-darwin", "gocache-xdg", "gomodcache", "golangci-lint", "preflight-runs"} {
		b, ok := byName[want]
		if !ok {
			t.Errorf("missing default budget for %q", want)
			continue
		}
		if b.BudgetBytes <= 0 {
			t.Errorf("%s has non-positive budget %d", want, b.BudgetBytes)
		}
		if b.Path == "" {
			t.Errorf("%s has empty path", want)
		}
	}
}
