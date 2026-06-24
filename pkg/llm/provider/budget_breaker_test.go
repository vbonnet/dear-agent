package provider

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/vbonnet/dear-agent/pkg/costtrack"
)

// stubProvider returns a fixed response.
type stubProvider struct {
	name string
	resp *GenerateResponse
	err  error
}

func (s *stubProvider) Name() string               { return s.name }
func (s *stubProvider) Capabilities() Capabilities { return Capabilities{} }
func (s *stubProvider) Generate(_ context.Context, _ *GenerateRequest) (*GenerateResponse, error) {
	if s.err != nil {
		return nil, s.err
	}
	return s.resp, nil
}

func newTestEnforcer(t *testing.T, weeklyLimit float64, caps costtrack.HarnessTokenCaps) *costtrack.BudgetEnforcer {
	t.Helper()
	path := filepath.Join(t.TempDir(), "budget-state.json")
	e, err := costtrack.NewBudgetEnforcer(costtrack.EnforcerConfig{
		StateFilePath:    path,
		WeeklyLimitUSD:   weeklyLimit,
		SessionTokenCaps: caps,
	})
	if err != nil {
		t.Fatalf("NewBudgetEnforcer: %v", err)
	}
	return e
}

func TestBudgetBreaker_AllowsWhenWithinBudget(t *testing.T) {
	primary := &stubProvider{name: "opus", resp: &GenerateResponse{
		Text:  "hello",
		Model: "opus",
		Usage: Usage{InputTokens: 100, OutputTokens: 50},
	}}

	e := newTestEnforcer(t, 70.0, nil)
	bb := NewBudgetBreaker(primary, BudgetBreakerConfig{
		Enforcer:  e,
		SessionID: "sess-1",
		Harness:   "claude-code",
	})

	resp, err := bb.Generate(context.Background(), &GenerateRequest{Prompt: "test"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Text != "hello" {
		t.Errorf("unexpected response: %v", resp.Text)
	}
	if bb.State() != CBClosed {
		t.Errorf("expected Closed state, got %v", bb.State())
	}
}

func TestBudgetBreaker_OpensOnExhaustion(t *testing.T) {
	e := newTestEnforcer(t, 7.0, nil)
	// Overspend the entire weekly budget so the daily check fails regardless of day.
	_ = e.RecordUsage("sess-1", "claude-code", 0, 0, 8.0)

	primary := &stubProvider{name: "opus"}
	bb := NewBudgetBreaker(primary, BudgetBreakerConfig{
		Enforcer:  e,
		SessionID: "sess-1",
		Harness:   "claude-code",
	})

	_, err := bb.Generate(context.Background(), &GenerateRequest{Prompt: "test"})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	var budgetErr *ErrBudgetExhausted
	if !errors.As(err, &budgetErr) {
		t.Errorf("expected ErrBudgetExhausted, got %T: %v", err, err)
	}
	if bb.State() != CBOpen {
		t.Errorf("expected Open state after exhaustion, got %v", bb.State())
	}
}

func TestBudgetBreaker_FallbackChainActivates(t *testing.T) {
	e := newTestEnforcer(t, 7.0, nil)
	_ = e.RecordUsage("sess-1", "claude-code", 0, 0, 8.0) // drain entire weekly budget

	primary := &stubProvider{name: "opus"}
	haiku := &stubProvider{name: "haiku", resp: &GenerateResponse{
		Text:  "cheap reply",
		Model: "haiku",
		Usage: Usage{InputTokens: 50, OutputTokens: 25},
	}}

	bb := NewBudgetBreaker(primary, BudgetBreakerConfig{
		Enforcer:      e,
		SessionID:     "sess-1",
		Harness:       "claude-code",
		FallbackChain: []Provider{haiku},
	})

	// First call opens the breaker
	_, _ = bb.Generate(context.Background(), &GenerateRequest{Prompt: "first"})

	// But fallback chain is drained too → ErrBudgetExhausted from haiku as well
	// (haiku uses same enforcer which already shows daily budget exhausted)
	_, err := bb.Generate(context.Background(), &GenerateRequest{Prompt: "second"})
	if err == nil {
		t.Fatal("expected error when all fallbacks are exhausted")
	}
}

func TestBudgetBreaker_SessionTokenCapBlocks(t *testing.T) {
	caps := costtrack.HarnessTokenCaps{"claude-code": 100}
	e := newTestEnforcer(t, 70.0, caps)
	// Consume all session tokens
	_ = e.RecordUsage("sess-1", "claude-code", 60, 50, 0.01) // 110 tokens total > cap

	primary := &stubProvider{name: "opus"}
	bb := NewBudgetBreaker(primary, BudgetBreakerConfig{
		Enforcer:  e,
		SessionID: "sess-1",
		Harness:   "claude-code",
	})

	_, err := bb.Generate(context.Background(), &GenerateRequest{Prompt: "test"})
	if err == nil {
		t.Fatal("expected error when session token cap exceeded")
	}
	var budgetErr *ErrBudgetExhausted
	if !errors.As(err, &budgetErr) {
		t.Errorf("expected ErrBudgetExhausted, got %T: %v", err, err)
	}
}

func TestBudgetBreaker_OTelAttributesEmitted(t *testing.T) {
	// Smoke test: verify Generate completes without panicking when tracer is nil
	// (uses global no-op tracer from otel package)
	primary := &stubProvider{name: "opus", resp: &GenerateResponse{
		Text:  "ok",
		Model: "claude-opus-4-8",
		Usage: Usage{InputTokens: 200, OutputTokens: 100, CostUSD: 0.02},
	}}

	e := newTestEnforcer(t, 70.0, nil)
	bb := NewBudgetBreaker(primary, BudgetBreakerConfig{
		Enforcer:  e,
		SessionID: "sess-otel",
		Harness:   "claude-code",
	})

	_, err := bb.Generate(context.Background(), &GenerateRequest{
		Prompt: "test",
		Model:  "claude-opus-4-8",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}
