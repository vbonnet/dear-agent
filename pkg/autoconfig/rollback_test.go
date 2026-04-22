package autoconfig

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRollbackController_NoRevertWithinBounds(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)

	rc, err := NewRollbackController("test-project", 3)
	if err != nil {
		t.Fatal(err)
	}

	if err := rc.RecordModification("s1", []Proposal{{Key: "a", NewValue: "1"}}); err != nil {
		t.Fatal(err)
	}

	baseline := &Baseline{AvgCostUSD: 1.0, AvgTokenEfficiency: 0.5}
	summary := SessionSummary{
		TotalCostUSD:    1.1, // 10% increase, within 30% threshold
		TokenEfficiency: 0.5,
		PhaseScores:     map[string]float64{"plan": 0.85}, // above 0.70 threshold
	}

	revert, reason := rc.CheckSession(summary, baseline)
	if revert {
		t.Errorf("unexpected revert: %s", reason)
	}
}

func TestRollbackController_RevertAfterConsecutiveFailures(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)

	rc, err := NewRollbackController("test-project", 10)
	if err != nil {
		t.Fatal(err)
	}

	if err := rc.RecordModification("s1", []Proposal{{Key: "a"}}); err != nil {
		t.Fatal(err)
	}

	baseline := &Baseline{AvgCostUSD: 1.0}

	// Three consecutive cost breaches.
	badSummary := SessionSummary{
		TotalCostUSD: 2.0,                             // 100% increase
		PhaseScores:  map[string]float64{"plan": 0.5}, // below 0.70
	}

	for i := 0; i < MaxConsecutiveFails; i++ {
		revert, _ := rc.CheckSession(badSummary, baseline)
		if i < MaxConsecutiveFails-1 && revert {
			t.Fatalf("premature revert at check %d", i)
		}
		if i == MaxConsecutiveFails-1 && !revert {
			t.Fatal("expected revert after 3 consecutive failures")
		}
	}

	if !rc.IsSuspended() {
		t.Error("controller should be suspended")
	}
}

func TestRollbackController_RevertRemovesConfig(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)

	// Create a config file to revert.
	configPath, _ := AutoConfigPath("test-project")
	if err := os.MkdirAll(filepath.Dir(configPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(configPath, []byte("test"), 0o644); err != nil {
		t.Fatal(err)
	}

	rc, err := NewRollbackController("test-project", 3)
	if err != nil {
		t.Fatal(err)
	}

	if err := rc.Revert("test revert"); err != nil {
		t.Fatal(err)
	}

	if _, err := os.Stat(configPath); !os.IsNotExist(err) {
		t.Error("config file should be removed after revert")
	}

	if !rc.State().Reverted {
		t.Error("state should be marked reverted")
	}
}

func TestRollbackController_SkipWhenReverted(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)

	rc, err := NewRollbackController("test-project", 3)
	if err != nil {
		t.Fatal(err)
	}

	rc.RecordModification("s1", nil)
	rc.Revert("done")

	revert, _ := rc.CheckSession(SessionSummary{}, nil)
	if revert {
		t.Error("should not revert when already reverted")
	}
}

func TestRollbackController_ConsecutiveFailsReset(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)

	rc, err := NewRollbackController("test-project", 10)
	if err != nil {
		t.Fatal(err)
	}

	rc.RecordModification("s1", nil)

	baseline := &Baseline{AvgCostUSD: 1.0}

	// Two failures.
	bad := SessionSummary{TotalCostUSD: 2.0, PhaseScores: map[string]float64{"x": 0.5}}
	rc.CheckSession(bad, baseline)
	rc.CheckSession(bad, baseline)

	// One good session resets the counter.
	good := SessionSummary{TotalCostUSD: 1.0, PhaseScores: map[string]float64{"x": 0.9}}
	rc.CheckSession(good, baseline)

	if rc.State().ConsecutiveFails != 0 {
		t.Errorf("consecutive fails = %d, want 0 after good session", rc.State().ConsecutiveFails)
	}
}
