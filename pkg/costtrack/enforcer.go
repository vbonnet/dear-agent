package costtrack

import (
	"fmt"
	"log/slog"
	"maps"
	"sync"
	"time"
)

// HarnessTokenCaps maps harness name → max tokens per session.
// Keys are AGM harness identifiers: "claude-code", "codex-cli", "gemini-cli", "opencode-cli".
type HarnessTokenCaps map[string]int64

// EnforcerConfig configures budget enforcement.
type EnforcerConfig struct {
	// StateFilePath is where BudgetState is persisted (default: ~/.agm/budget-state.json).
	StateFilePath string

	// WeeklyLimitUSD is the total spend cap per 7-day week.
	WeeklyLimitUSD float64

	// SessionTokenCaps maps harness name → per-session token ceiling.
	// 0 means no cap for that harness.
	SessionTokenCaps HarnessTokenCaps

	// Logger receives threshold alerts (defaults to slog.Default).
	Logger *slog.Logger
}

// CheckResult is returned by PreCheck.
type CheckResult struct {
	Allowed bool
	Reason  string
	// DailyBudgetRemaining is how much USD is still available today.
	DailyBudgetRemaining float64
	// SessionTokensRemaining is how many tokens are left for this session (-1 = unlimited).
	SessionTokensRemaining int64
}

// BudgetEnforcer enforces per-session token caps and per-day USD spend limits.
// It is the external governance layer that sits between the harness and API calls.
//
// Usage:
//
//	enforcer.PreCheck(harness, sessionID, estimatedCost) → allowed?
//	// ... make the API call ...
//	enforcer.RecordUsage(sessionID, harness, tokensIn, tokensOut, costUSD)
type BudgetEnforcer struct {
	cfg    EnforcerConfig
	state  *BudgetState
	alerts *AlertManager
	mu     sync.Mutex
}

// NewBudgetEnforcer creates an enforcer, loading or initialising budget state from disk.
func NewBudgetEnforcer(cfg EnforcerConfig) (*BudgetEnforcer, error) {
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}
	state, err := LoadBudgetState(cfg.StateFilePath, cfg.WeeklyLimitUSD)
	if err != nil {
		return nil, fmt.Errorf("load budget state: %w", err)
	}
	return &BudgetEnforcer{
		cfg:    cfg,
		state:  state,
		alerts: NewAlertManager(cfg.Logger),
	}, nil
}

// PreCheck decides whether a request is within budget before making the API call.
// estimatedCostUSD may be 0 if unknown — the call will still be allowed unless the
// daily budget is already exhausted.
func (e *BudgetEnforcer) PreCheck(harness, sessionID string, estimatedCostUSD float64) CheckResult {
	e.mu.Lock()
	defer e.mu.Unlock()

	now := time.Now().UTC()
	available := e.state.TodayBudget(now)

	// Daily budget gate
	if available <= 0 {
		return CheckResult{
			Allowed: false,
			Reason:  fmt.Sprintf("daily budget exhausted (weekly limit $%.2f, available today $0.00)", e.cfg.WeeklyLimitUSD),
		}
	}
	if estimatedCostUSD > 0 && estimatedCostUSD > available {
		return CheckResult{
			Allowed: false,
			Reason: fmt.Sprintf("estimated cost $%.4f exceeds today's remaining budget $%.4f",
				estimatedCostUSD, available),
			DailyBudgetRemaining: available,
		}
	}

	// Per-session token cap gate
	sessionRemaining := int64(-1) // -1 = unlimited
	if tokenCap, ok := e.cfg.SessionTokenCaps[harness]; ok && tokenCap > 0 {
		used := e.state.SessionTokensUsed(sessionID)
		remaining := tokenCap - used
		if remaining <= 0 {
			return CheckResult{
				Allowed: false,
				Reason: fmt.Sprintf("session %q exceeded token cap for harness %q (%d/%d tokens)",
					sessionID, harness, used, tokenCap),
				DailyBudgetRemaining:   available,
				SessionTokensRemaining: 0,
			}
		}
		sessionRemaining = remaining
	}

	return CheckResult{
		Allowed:                true,
		DailyBudgetRemaining:   available,
		SessionTokensRemaining: sessionRemaining,
	}
}

// RecordUsage records actual token usage and cost after an API call.
// It also fires threshold alerts if daily spend crosses 50/75/90/100%.
func (e *BudgetEnforcer) RecordUsage(sessionID, harness string, tokensIn, tokensOut int64, costUSD float64) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	now := time.Now().UTC()

	totalTokens := tokensIn + tokensOut
	if err := e.state.RecordSpendAndTokens(now, costUSD, sessionID, harness, totalTokens); err != nil {
		return fmt.Errorf("record usage: %w", err)
	}

	// Fire threshold alerts based on daily percentage used
	b := e.state.Snapshot()
	pct := b.PercentUsedToday(now)

	// Build a synthetic BudgetStatus for the alert manager
	avail := b.AvailableToday(now)
	todaySpend := b.SpentToday(now)
	fullBudget := avail + todaySpend
	status := BudgetStatus{
		Period:    BudgetDaily,
		Limit:     fullBudget,
		Spent:     todaySpend,
		Remaining: avail,
		Percent:   pct,
		Allowed:   avail > 0,
	}
	e.alerts.CheckAndAlert("agm", harness, []BudgetStatus{status})

	return nil
}

// DailyStatus returns the current budget status for display.
func (e *BudgetEnforcer) DailyStatus(now time.Time) BudgetStatus {
	e.mu.Lock()
	defer e.mu.Unlock()

	b := e.state.Snapshot()
	todaySpend := b.SpentToday(now)
	avail := b.AvailableToday(now)
	fullBudget := avail + todaySpend

	pct := 0.0
	if fullBudget > 0 {
		pct = todaySpend / fullBudget * 100
	}

	return BudgetStatus{
		Period:    BudgetDaily,
		Limit:     fullBudget,
		Spent:     todaySpend,
		Remaining: avail,
		Percent:   pct,
		Allowed:   avail > 0,
	}
}

// WeeklyStatus returns the weekly aggregate budget status.
func (e *BudgetEnforcer) WeeklyStatus() BudgetStatus {
	e.mu.Lock()
	defer e.mu.Unlock()

	b := e.state.Snapshot()
	totalSpent := b.TotalSpent()
	remaining := b.WeeklyLimit - totalSpent
	if remaining < 0 {
		remaining = 0
	}
	pct := 0.0
	if b.WeeklyLimit > 0 {
		pct = totalSpent / b.WeeklyLimit * 100
	}

	return BudgetStatus{
		Period:    BudgetWeekly,
		Limit:     b.WeeklyLimit,
		Spent:     totalSpent,
		Remaining: remaining,
		Percent:   pct,
		Allowed:   remaining > 0,
	}
}

// SessionStatus returns per-session token consumption.
// Returns nil if the session is unknown.
func (e *BudgetEnforcer) SessionStatus(sessionID, harness string) *SessionBudgetStatus {
	e.mu.Lock()
	defer e.mu.Unlock()

	used := e.state.SessionTokensUsed(sessionID)
	tokenCap := int64(-1)
	if c, ok := e.cfg.SessionTokenCaps[harness]; ok {
		tokenCap = c
	}

	pct := 0.0
	if tokenCap > 0 {
		pct = float64(used) / float64(tokenCap) * 100
	}
	return &SessionBudgetStatus{
		SessionID:  sessionID,
		Harness:    harness,
		TokensUsed: used,
		TokenCap:   tokenCap,
		Percent:    pct,
		Allowed:    tokenCap <= 0 || used < tokenCap,
	}
}

// SessionBudgetStatus describes budget consumption for a single session.
type SessionBudgetStatus struct {
	SessionID  string
	Harness    string
	TokensUsed int64
	TokenCap   int64   // -1 = unlimited
	Percent    float64 // 0–100 (0 if no cap)
	Allowed    bool
}

// AllSessions returns budget status for every tracked session this week.
func (e *BudgetEnforcer) AllSessions() []SessionBudgetStatus {
	e.mu.Lock()
	defer e.mu.Unlock()

	e.state.mu.Lock()
	sessions := maps.Clone(e.state.SessionTokens)
	harnesses := maps.Clone(e.state.SessionHarness)
	e.state.mu.Unlock()

	out := make([]SessionBudgetStatus, 0, len(sessions))
	for id, tokens := range sessions {
		harness := harnesses[id]
		if harness == "" {
			harness = "unknown"
		}
		tokenCap := int64(-1)
		if c, ok := e.cfg.SessionTokenCaps[harness]; ok {
			tokenCap = c
		}
		pct := 0.0
		if tokenCap > 0 {
			pct = float64(tokens) / float64(tokenCap) * 100
		}
		out = append(out, SessionBudgetStatus{
			SessionID:  id,
			Harness:    harness,
			TokensUsed: tokens,
			TokenCap:   tokenCap,
			Percent:    pct,
			Allowed:    tokenCap <= 0 || tokens < tokenCap,
		})
	}
	return out
}

// WeeklySnap returns a copy of the underlying daily budget for display.
func (e *BudgetEnforcer) WeeklySnap() DailyBudget {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.state.Snapshot()
}
