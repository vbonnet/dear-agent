package costtrack

import (
	"log/slog"
	"path/filepath"
	"testing"
	"time"
)

func newTestEnforcer(t *testing.T, weeklyLimit float64, caps HarnessTokenCaps) *BudgetEnforcer {
	t.Helper()
	path := filepath.Join(t.TempDir(), "budget-state.json")
	cfg := EnforcerConfig{
		StateFilePath:    path,
		WeeklyLimitUSD:   weeklyLimit,
		SessionTokenCaps: caps,
		Logger:           slog.Default(),
	}
	e, err := NewBudgetEnforcer(cfg)
	if err != nil {
		t.Fatalf("NewBudgetEnforcer: %v", err)
	}
	return e
}

func TestEnforcer_AllowedWhenWithinBudget(t *testing.T) {
	e := newTestEnforcer(t, 70.0, nil)
	result := e.PreCheck("claude-code", "sess-1", 0)
	if !result.Allowed {
		t.Errorf("expected allowed, got: %s", result.Reason)
	}
}

func TestEnforcer_BlockedWhenDailyBudgetExhausted(t *testing.T) {
	e := newTestEnforcer(t, 7.0, nil) // $1/day
	now := time.Now().UTC()
	// Overspend the entire weekly budget so today's allocation is definitely exhausted,
	// regardless of which day of the week the test runs on.
	_ = e.state.RecordSpend(now, 8.0) // $1 over the $7 weekly cap

	result := e.PreCheck("claude-code", "sess-1", 0)
	if result.Allowed {
		t.Error("expected blocked due to exhausted daily budget")
	}
}

func TestEnforcer_BlockedWhenSessionTokenCapExceeded(t *testing.T) {
	caps := HarnessTokenCaps{"claude-code": 10000}
	e := newTestEnforcer(t, 70.0, caps)

	// Record 10001 tokens for this session
	_ = e.state.RecordSessionTokens("sess-1", "claude-code", 10001)

	result := e.PreCheck("claude-code", "sess-1", 0)
	if result.Allowed {
		t.Error("expected blocked due to token cap")
	}
	if result.SessionTokensRemaining != 0 {
		t.Errorf("expected 0 remaining tokens, got %d", result.SessionTokensRemaining)
	}
}

func TestEnforcer_TokenCapDoesNotAffectOtherHarness(t *testing.T) {
	caps := HarnessTokenCaps{"claude-code": 10000}
	e := newTestEnforcer(t, 70.0, caps)
	_ = e.state.RecordSessionTokens("sess-1", "claude-code", 10001)

	// codex-cli has no cap → allowed
	result := e.PreCheck("codex-cli", "sess-1", 0)
	if !result.Allowed {
		t.Errorf("codex-cli has no cap; expected allowed, got: %s", result.Reason)
	}
	if result.SessionTokensRemaining != -1 {
		t.Errorf("expected -1 (unlimited) for uncapped harness, got %d", result.SessionTokensRemaining)
	}
}

func TestEnforcer_RecordUsageUpdatesState(t *testing.T) {
	e := newTestEnforcer(t, 70.0, nil)
	now := time.Now().UTC()

	if err := e.RecordUsage("sess-1", "claude-code", 1000, 500, 0.05); err != nil {
		t.Fatalf("RecordUsage: %v", err)
	}

	b := e.state.Snapshot()
	todaySpend := b.SpentToday(now)
	if todaySpend < 0.049 || todaySpend > 0.051 {
		t.Errorf("expected today spend ~0.05, got %.4f", todaySpend)
	}
	if tokens := e.state.SessionTokensUsed("sess-1"); tokens != 1500 {
		t.Errorf("expected 1500 total tokens, got %d", tokens)
	}
}

func TestEnforcer_DailyStatus(t *testing.T) {
	e := newTestEnforcer(t, 70.0, nil)
	now := time.Now().UTC()
	_ = e.RecordUsage("sess-1", "claude-code", 1000, 500, 2.50)

	status := e.DailyStatus(now)
	if !status.Allowed {
		t.Error("expected allowed")
	}
	if status.Spent < 2.49 || status.Spent > 2.51 {
		t.Errorf("expected spent ~2.50, got %.4f", status.Spent)
	}
}

func TestEnforcer_WeeklyStatus(t *testing.T) {
	e := newTestEnforcer(t, 70.0, nil)
	_ = e.RecordUsage("sess-1", "claude-code", 1000, 500, 5.0)
	_ = e.RecordUsage("sess-2", "claude-code", 2000, 1000, 10.0)

	status := e.WeeklyStatus()
	if status.Limit != 70.0 {
		t.Errorf("expected limit 70, got %.2f", status.Limit)
	}
	if status.Spent < 14.99 || status.Spent > 15.01 {
		t.Errorf("expected spent ~15, got %.2f", status.Spent)
	}
}

func TestEnforcer_SessionStatus_WithCap(t *testing.T) {
	caps := HarnessTokenCaps{"claude-code": 50000}
	e := newTestEnforcer(t, 70.0, caps)
	_ = e.RecordUsage("sess-1", "claude-code", 10000, 5000, 1.0)

	ss := e.SessionStatus("sess-1", "claude-code")
	if ss == nil {
		t.Fatal("expected status, got nil")
	}
	if ss.TokensUsed != 15000 {
		t.Errorf("expected 15000 tokens used, got %d", ss.TokensUsed)
	}
	if ss.TokenCap != 50000 {
		t.Errorf("expected cap 50000, got %d", ss.TokenCap)
	}
	if !ss.Allowed {
		t.Error("expected allowed (within cap)")
	}
}

func TestEnforcer_SessionStatus_Unlimited(t *testing.T) {
	e := newTestEnforcer(t, 70.0, nil)
	_ = e.RecordUsage("sess-1", "gemini-cli", 10000, 5000, 1.0)

	ss := e.SessionStatus("sess-1", "gemini-cli")
	if ss.TokenCap != -1 {
		t.Errorf("expected -1 (unlimited), got %d", ss.TokenCap)
	}
	if !ss.Allowed {
		t.Error("expected allowed for unlimited session")
	}
}
