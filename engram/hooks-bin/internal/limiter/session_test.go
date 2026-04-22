package limiter

import (
	"os"
	"path/filepath"
	"testing"
)

func tempTracker(t *testing.T) *SessionTracker {
	t.Helper()
	dir := t.TempDir()
	return NewSessionTracker(dir)
}

func TestSessionTracker_LoadEmpty(t *testing.T) {
	tracker := tempTracker(t)
	b, err := tracker.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if b.TotalTokens != 0 {
		t.Errorf("TotalTokens = %d, want 0", b.TotalTokens)
	}
	if len(b.DisabledHooks) != 0 {
		t.Error("DisabledHooks should be empty")
	}
}

func TestSessionTracker_SaveAndLoad(t *testing.T) {
	tracker := tempTracker(t)
	b := &SessionBudget{
		TotalTokens:           1234,
		ConsecutiveViolations: map[string]int{"hook-a": 2},
		DisabledHooks:         map[string]bool{},
	}
	if err := tracker.Save(b); err != nil {
		t.Fatalf("Save: %v", err)
	}

	loaded, err := tracker.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if loaded.TotalTokens != 1234 {
		t.Errorf("TotalTokens = %d, want 1234", loaded.TotalTokens)
	}
	if loaded.ConsecutiveViolations["hook-a"] != 2 {
		t.Errorf("ConsecutiveViolations[hook-a] = %d, want 2", loaded.ConsecutiveViolations["hook-a"])
	}
}

func TestSessionTracker_EffectiveBudget_Default(t *testing.T) {
	tracker := tempTracker(t)
	budget, err := tracker.EffectiveBudget("hook-a", DefaultMaxTokens)
	if err != nil {
		t.Fatalf("EffectiveBudget: %v", err)
	}
	if budget != DefaultMaxTokens {
		t.Errorf("budget = %d, want %d", budget, DefaultMaxTokens)
	}
}

func TestSessionTracker_EffectiveBudget_Reduced(t *testing.T) {
	tracker := tempTracker(t)

	// Simulate cumulative usage exceeding the limit
	b := newBudget()
	b.TotalTokens = CumulativeTokenLimit
	if err := tracker.Save(b); err != nil {
		t.Fatalf("Save: %v", err)
	}

	budget, err := tracker.EffectiveBudget("hook-a", DefaultMaxTokens)
	if err != nil {
		t.Fatalf("EffectiveBudget: %v", err)
	}
	if budget != ReducedBudget {
		t.Errorf("budget = %d, want %d (reduced)", budget, ReducedBudget)
	}
}

func TestSessionTracker_EffectiveBudget_Disabled(t *testing.T) {
	tracker := tempTracker(t)

	b := newBudget()
	b.DisabledHooks["hook-a"] = true
	if err := tracker.Save(b); err != nil {
		t.Fatalf("Save: %v", err)
	}

	budget, err := tracker.EffectiveBudget("hook-a", DefaultMaxTokens)
	if err != nil {
		t.Fatalf("EffectiveBudget: %v", err)
	}
	if budget != 0 {
		t.Errorf("budget = %d, want 0 (disabled)", budget)
	}
}

func TestSessionTracker_RecordUsage_CumulativeTracking(t *testing.T) {
	tracker := tempTracker(t)

	if err := tracker.RecordUsage("hook-a", 100, false); err != nil {
		t.Fatalf("RecordUsage: %v", err)
	}
	if err := tracker.RecordUsage("hook-b", 200, false); err != nil {
		t.Fatalf("RecordUsage: %v", err)
	}

	b, _ := tracker.Load()
	if b.TotalTokens != 300 {
		t.Errorf("TotalTokens = %d, want 300", b.TotalTokens)
	}
}

func TestSessionTracker_RecordUsage_ViolationTracking(t *testing.T) {
	tracker := tempTracker(t)

	// Record non-violation, then violation
	tracker.RecordUsage("hook-a", 50, false)
	b, _ := tracker.Load()
	if b.ConsecutiveViolations["hook-a"] != 0 {
		t.Errorf("violations = %d, want 0 after clean write", b.ConsecutiveViolations["hook-a"])
	}

	tracker.RecordUsage("hook-a", 50, true)
	b, _ = tracker.Load()
	if b.ConsecutiveViolations["hook-a"] != 1 {
		t.Errorf("violations = %d, want 1", b.ConsecutiveViolations["hook-a"])
	}

	// Non-violation resets counter
	tracker.RecordUsage("hook-a", 50, false)
	b, _ = tracker.Load()
	if b.ConsecutiveViolations["hook-a"] != 0 {
		t.Errorf("violations = %d, want 0 after reset", b.ConsecutiveViolations["hook-a"])
	}
}

func TestSessionTracker_CircuitBreaker(t *testing.T) {
	tracker := tempTracker(t)

	// 3 consecutive violations should disable the hook
	for i := 0; i < CircuitBreakerThreshold; i++ {
		if err := tracker.RecordUsage("bad-hook", 100, true); err != nil {
			t.Fatalf("RecordUsage %d: %v", i, err)
		}
	}

	disabled, err := tracker.IsDisabled("bad-hook")
	if err != nil {
		t.Fatalf("IsDisabled: %v", err)
	}
	if !disabled {
		t.Error("hook should be disabled after 3 consecutive violations")
	}

	// Effective budget should be 0
	budget, _ := tracker.EffectiveBudget("bad-hook", DefaultMaxTokens)
	if budget != 0 {
		t.Errorf("budget = %d, want 0 for disabled hook", budget)
	}
}

func TestSessionTracker_CircuitBreaker_NotTriggeredBelow(t *testing.T) {
	tracker := tempTracker(t)

	// 2 violations, then clean — should NOT disable
	tracker.RecordUsage("hook-a", 100, true)
	tracker.RecordUsage("hook-a", 100, true)
	tracker.RecordUsage("hook-a", 100, false) // resets

	disabled, _ := tracker.IsDisabled("hook-a")
	if disabled {
		t.Error("hook should not be disabled — violations were reset")
	}
}

func TestSessionTracker_Reset(t *testing.T) {
	tracker := tempTracker(t)

	tracker.RecordUsage("hook-a", 500, true)
	tracker.RecordUsage("hook-a", 500, true)
	tracker.RecordUsage("hook-a", 500, true)

	if err := tracker.Reset(); err != nil {
		t.Fatalf("Reset: %v", err)
	}

	b, _ := tracker.Load()
	if b.TotalTokens != 0 {
		t.Errorf("TotalTokens = %d after reset, want 0", b.TotalTokens)
	}
	if len(b.DisabledHooks) != 0 {
		t.Error("DisabledHooks should be empty after reset")
	}
}

func TestSessionTracker_CorruptFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, budgetFileName)
	os.WriteFile(path, []byte("not json"), 0600)

	tracker := NewSessionTracker(dir)
	b, err := tracker.Load()
	if err != nil {
		t.Fatalf("Load with corrupt file should not error: %v", err)
	}
	if b.TotalTokens != 0 {
		t.Error("corrupt file should reset to zero state")
	}
}
