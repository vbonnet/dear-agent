package costtrack

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestLoadBudgetState_NewFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "budget-state.json")
	s, err := LoadBudgetState(path, 70.0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if s.WeeklyLimitUSD != 70.0 {
		t.Errorf("want 70, got %.2f", s.WeeklyLimitUSD)
	}
	if s.WeekStart.IsZero() {
		t.Error("WeekStart should not be zero")
	}
	if len(s.SessionTokens) != 0 {
		t.Error("SessionTokens should be empty")
	}
}

func TestLoadBudgetState_Persistence(t *testing.T) {
	path := filepath.Join(t.TempDir(), "budget-state.json")
	s, err := LoadBudgetState(path, 70.0)
	if err != nil {
		t.Fatalf("load: %v", err)
	}

	now := time.Now().UTC()
	if err := s.RecordSpend(now, 5.0); err != nil {
		t.Fatalf("record spend: %v", err)
	}
	if err := s.RecordSessionTokens("sess-1", "claude-code", 10000); err != nil {
		t.Fatalf("record tokens: %v", err)
	}

	// Reload from same file
	s2, err := LoadBudgetState(path, 70.0)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}

	b := DailyBudget{WeeklyLimit: s2.WeeklyLimitUSD, WeekStart: s2.WeekStart, DailySpend: s2.DailySpend}
	if spent := b.SpentToday(now); spent != 5.0 {
		t.Errorf("want 5.0 today spend after reload, got %.2f", spent)
	}
	if got := s2.SessionTokensUsed("sess-1"); got != 10000 {
		t.Errorf("want 10000 session tokens, got %d", got)
	}
}

func TestLoadBudgetState_WeekReset(t *testing.T) {
	path := filepath.Join(t.TempDir(), "budget-state.json")

	// Simulate a prior week's state by writing it directly
	priorWeek := currentWeekStart(time.Now().UTC()).AddDate(0, 0, -7)
	old := &BudgetState{
		WeeklyLimitUSD: 70.0,
		WeekStart:      priorWeek,
		path:           path,
	}
	old.DailySpend[0] = 15.0
	old.SessionTokens = map[string]int64{"old-sess": 5000}
	old.SessionHarness = map[string]string{"old-sess": "claude-code"}
	if err := old.Save(); err != nil {
		t.Fatalf("save prior week: %v", err)
	}

	s, err := LoadBudgetState(path, 70.0)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}

	// Should have reset — new week, zero spend
	if s.DailySpend != ([7]float64{}) {
		t.Errorf("daily spend should be zero after week reset, got %v", s.DailySpend)
	}
	if len(s.SessionTokens) != 0 {
		t.Errorf("session tokens should be empty after week reset, got %v", s.SessionTokens)
	}
}

func TestLoadBudgetState_LimitChangeResets(t *testing.T) {
	path := filepath.Join(t.TempDir(), "budget-state.json")
	s, err := LoadBudgetState(path, 70.0)
	if err != nil {
		t.Fatalf("first load: %v", err)
	}
	_ = s.RecordSpend(time.Now(), 10.0)

	// Reload with a different limit — should reset spend
	s2, err := LoadBudgetState(path, 140.0)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	if s2.DailySpend != ([7]float64{}) {
		t.Errorf("spend should reset on limit change, got %v", s2.DailySpend)
	}
	if s2.WeeklyLimitUSD != 140.0 {
		t.Errorf("want 140, got %.2f", s2.WeeklyLimitUSD)
	}
}

func TestBudgetState_AtomicSave(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "budget-state.json")
	s, err := LoadBudgetState(path, 50.0)
	if err != nil {
		t.Fatalf("load: %v", err)
	}

	if err := s.Save(); err != nil {
		t.Fatalf("save: %v", err)
	}

	// Tmp file should be cleaned up
	tmp := path + ".tmp"
	if _, err := os.Stat(tmp); !os.IsNotExist(err) {
		t.Error("tmp file should not exist after successful save")
	}

	// State file should exist
	if _, err := os.Stat(path); err != nil {
		t.Errorf("state file should exist: %v", err)
	}
}
