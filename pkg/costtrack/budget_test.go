package costtrack

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestLoadBudgetConfig(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "budget.yaml")
	body := `
defaults:
  daily: 5.0
  weekly: 25.0
  monthly: 100.0
projects:
  alpha:
    daily: 10.0
models:
  claude-opus-4-7:
    monthly: 200.0
`
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}

	cfg, err := LoadBudgetConfig(path)
	if err != nil {
		t.Fatalf("LoadBudgetConfig: %v", err)
	}
	if cfg.Defaults.Daily != 5.0 || cfg.Defaults.Monthly != 100.0 {
		t.Fatalf("defaults wrong: %+v", cfg.Defaults)
	}
	if cfg.Projects["alpha"].Daily != 10.0 {
		t.Fatalf("project override missing")
	}
	if cfg.Models["claude-opus-4-7"].Monthly != 200.0 {
		t.Fatalf("model override missing")
	}
}

func TestLoadBudgetConfig_MissingFile(t *testing.T) {
	_, err := LoadBudgetConfig(filepath.Join(t.TempDir(), "nope.yaml"))
	if err == nil {
		t.Fatal("expected error for missing file")
	}
}

func TestLoadBudgetConfig_DefaultsAlwaysPresent(t *testing.T) {
	// YAML with no `defaults:` key should still produce a non-nil Defaults so
	// callers can dereference without a nil check.
	path := filepath.Join(t.TempDir(), "budget.yaml")
	if err := os.WriteFile(path, []byte("projects:\n  alpha:\n    daily: 1.0\n"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	cfg, err := LoadBudgetConfig(path)
	if err != nil {
		t.Fatalf("LoadBudgetConfig: %v", err)
	}
	if cfg.Defaults == nil {
		t.Fatal("Defaults must be non-nil after load")
	}
}

func TestGetLimits_PrecedenceOrder(t *testing.T) {
	cfg := &BudgetConfig{
		Defaults: &BudgetLimits{Daily: 1, Weekly: 7, Monthly: 30},
		Projects: map[string]*BudgetLimits{
			"alpha": {Daily: 5}, // overrides daily only
		},
		Models: map[string]*BudgetLimits{
			"opus": {Monthly: 100}, // overrides monthly only
		},
	}

	got := cfg.GetLimits("alpha", "opus")
	// Daily came from project, Weekly stayed at default, Monthly came from model.
	if got.Daily != 5 {
		t.Errorf("Daily: got %v, want 5 (from project)", got.Daily)
	}
	if got.Weekly != 7 {
		t.Errorf("Weekly: got %v, want 7 (from defaults)", got.Weekly)
	}
	if got.Monthly != 100 {
		t.Errorf("Monthly: got %v, want 100 (from model)", got.Monthly)
	}
}

func TestGetLimits_UnknownProjectAndModel(t *testing.T) {
	cfg := &BudgetConfig{
		Defaults: &BudgetLimits{Daily: 1, Weekly: 7, Monthly: 30},
	}
	got := cfg.GetLimits("does-not-exist", "also-missing")
	if got.Daily != 1 || got.Weekly != 7 || got.Monthly != 30 {
		t.Fatalf("unknown project+model should fall back to defaults, got %+v", got)
	}
}

func TestGetLimits_ZeroLimitDoesNotOverride(t *testing.T) {
	// A project entry that omits a field (so it parses as 0) must not zero out
	// the default — only positive values are "set".
	cfg := &BudgetConfig{
		Defaults: &BudgetLimits{Daily: 1, Weekly: 7, Monthly: 30},
		Projects: map[string]*BudgetLimits{
			"alpha": {Daily: 5}, // Weekly and Monthly are 0, must not zero defaults
		},
	}
	got := cfg.GetLimits("alpha", "")
	if got.Weekly != 7 || got.Monthly != 30 {
		t.Fatalf("zero limits should not clobber defaults: %+v", got)
	}
}

func TestPeriodBounds_Daily(t *testing.T) {
	now := time.Date(2026, 5, 11, 14, 30, 45, 0, time.UTC)
	start, end := PeriodBounds(BudgetDaily, now)
	wantStart := time.Date(2026, 5, 11, 0, 0, 0, 0, time.UTC)
	wantEnd := time.Date(2026, 5, 12, 0, 0, 0, 0, time.UTC)
	if !start.Equal(wantStart) || !end.Equal(wantEnd) {
		t.Fatalf("Daily: got [%v, %v), want [%v, %v)", start, end, wantStart, wantEnd)
	}
}

func TestPeriodBounds_WeeklyStartsMonday(t *testing.T) {
	// Wednesday → week starts the preceding Monday.
	wed := time.Date(2026, 5, 13, 14, 0, 0, 0, time.UTC) // Wednesday
	start, end := PeriodBounds(BudgetWeekly, wed)
	if start.Weekday() != time.Monday {
		t.Fatalf("weekly start should be Monday, got %v (%v)", start.Weekday(), start)
	}
	if end.Sub(start) != 7*24*time.Hour {
		t.Fatalf("weekly span should be 7 days, got %v", end.Sub(start))
	}
}

func TestPeriodBounds_WeeklySundayIsEndOfPriorWeek(t *testing.T) {
	// Sunday is treated as day 7 of the prior week, so the start is the
	// Monday six days before.
	sun := time.Date(2026, 5, 17, 12, 0, 0, 0, time.UTC) // Sunday
	start, _ := PeriodBounds(BudgetWeekly, sun)
	wantStart := time.Date(2026, 5, 11, 0, 0, 0, 0, time.UTC) // Monday before
	if !start.Equal(wantStart) {
		t.Fatalf("Sunday weekly start: got %v, want %v", start, wantStart)
	}
}

func TestPeriodBounds_Monthly(t *testing.T) {
	mid := time.Date(2026, 5, 15, 12, 0, 0, 0, time.UTC)
	start, end := PeriodBounds(BudgetMonthly, mid)
	if start.Day() != 1 || start.Month() != time.May {
		t.Fatalf("monthly start: got %v, want 2026-05-01", start)
	}
	if end.Day() != 1 || end.Month() != time.June {
		t.Fatalf("monthly end: got %v, want 2026-06-01", end)
	}
}

func TestPeriodBounds_UnknownPeriodFallsBackToDaily(t *testing.T) {
	now := time.Date(2026, 5, 11, 10, 0, 0, 0, time.UTC)
	start, end := PeriodBounds(BudgetPeriod("hourly"), now)
	wantStart := time.Date(2026, 5, 11, 0, 0, 0, 0, time.UTC)
	wantEnd := time.Date(2026, 5, 12, 0, 0, 0, 0, time.UTC)
	if !start.Equal(wantStart) || !end.Equal(wantEnd) {
		t.Fatalf("unknown period should fall back to daily: got [%v, %v)", start, end)
	}
}
