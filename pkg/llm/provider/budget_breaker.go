package provider

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"

	"github.com/vbonnet/dear-agent/pkg/costtrack"
)

// BudgetBreakerConfig configures the budget-aware circuit breaker.
type BudgetBreakerConfig struct {
	// Enforcer is required — it provides pre-check and usage recording.
	Enforcer *costtrack.BudgetEnforcer

	// FallbackChain is an ordered list of cheaper providers to try when the
	// primary provider's budget is exhausted: Opus → Sonnet → Haiku → etc.
	// When all fallbacks are exhausted the error is surfaced to the caller.
	FallbackChain []Provider

	// SessionID identifies the session making calls (for per-session token caps).
	SessionID string

	// Harness is the harness type ("claude-code", "codex-cli", etc.).
	Harness string

	// CooldownPeriod is how long the breaker stays Open before transitioning
	// to Half-Open to probe whether the next day's carryover is available.
	// Defaults to 5 minutes.
	CooldownPeriod time.Duration

	// Tracer is used to emit gen_ai semantic convention spans.
	// If nil, the global OTel tracer is used.
	Tracer trace.Tracer
}

// BudgetBreaker wraps a Provider with budget-enforcement circuit-breaking.
//
// States:
//   - Closed:    Budget available — requests flow through to primary provider.
//   - Open:      Daily budget exhausted — requests are rejected (or sent to fallback chain).
//   - Half-Open: Cooldown elapsed — one probe request is allowed to check carryover.
type BudgetBreaker struct {
	primary Provider
	cfg     BudgetBreakerConfig
	tracer  trace.Tracer

	mu          sync.Mutex
	state       CBState
	lastExhaust time.Time
}

// NewBudgetBreaker wraps primary with budget enforcement and fallback routing.
func NewBudgetBreaker(primary Provider, cfg BudgetBreakerConfig) *BudgetBreaker {
	if cfg.Enforcer == nil {
		panic("BudgetBreakerConfig.Enforcer must not be nil")
	}
	if cfg.CooldownPeriod <= 0 {
		cfg.CooldownPeriod = 5 * time.Minute
	}
	t := cfg.Tracer
	if t == nil {
		t = otel.Tracer("dear-agent/llm/provider")
	}
	return &BudgetBreaker{
		primary: primary,
		cfg:     cfg,
		tracer:  t,
		state:   CBClosed,
	}
}

// Name returns the primary provider's name.
func (b *BudgetBreaker) Name() string { return b.primary.Name() }

// Capabilities returns the primary provider's capabilities.
func (b *BudgetBreaker) Capabilities() Capabilities { return b.primary.Capabilities() }

// Generate executes a generation request, enforcing budget limits.
//
// Flow:
//  1. Run PreCheck on the enforcer.
//  2. If budget is exhausted: try fallback chain; if all fail, return ErrBudgetExhausted.
//  3. If allowed: call primary, record usage, emit OTel span with gen_ai attributes.
func (b *BudgetBreaker) Generate(ctx context.Context, req *GenerateRequest) (*GenerateResponse, error) {
	if b.canProceed() {
		resp, err := b.tryGenerate(ctx, req, b.primary)
		if err != nil {
			var budgetErr *ErrBudgetExhausted
			if errors.As(err, &budgetErr) {
				b.openBreaker()
			}
		}
		return resp, err
	}

	// Breaker is Open — try fallback chain
	return b.tryFallbackChain(ctx, req)
}

// canProceed returns true if the breaker is Closed or has transitioned to Half-Open.
func (b *BudgetBreaker) canProceed() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	switch b.state {
	case CBClosed:
		return true
	case CBOpen:
		if time.Since(b.lastExhaust) >= b.cfg.CooldownPeriod {
			b.state = CBHalfOpen
			return true
		}
		return false
	case CBHalfOpen:
		return true
	}
	return false
}

func (b *BudgetBreaker) openBreaker() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.state = CBOpen
	b.lastExhaust = time.Now()
}

func (b *BudgetBreaker) closeBreaker() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.state = CBClosed
}

// State returns the current circuit breaker state.
func (b *BudgetBreaker) State() CBState {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.state
}

// tryGenerate runs the enforcer pre-check, calls provider, records usage, emits OTel.
func (b *BudgetBreaker) tryGenerate(ctx context.Context, req *GenerateRequest, p Provider) (*GenerateResponse, error) {
	check := b.cfg.Enforcer.PreCheck(b.cfg.Harness, b.cfg.SessionID, 0)
	if !check.Allowed {
		return nil, &ErrBudgetExhausted{Reason: check.Reason}
	}

	ctx, span := b.tracer.Start(ctx, "llm.generate",
		trace.WithSpanKind(trace.SpanKindClient),
	)
	defer span.End()

	span.SetAttributes(
		attribute.String("gen_ai.system", p.Name()),
		attribute.String("gen_ai.request.model", req.Model),
	)

	resp, err := p.Generate(ctx, req)
	if err != nil {
		return nil, err
	}
	if resp == nil {
		return nil, fmt.Errorf("provider %q returned nil response", p.Name())
	}

	// Emit gen_ai semantic convention token attributes
	span.SetAttributes(
		attribute.Int("gen_ai.usage.input_tokens", resp.Usage.InputTokens),
		attribute.Int("gen_ai.usage.output_tokens", resp.Usage.OutputTokens),
		attribute.String("gen_ai.response.model", resp.Model),
	)

	// Record usage (best-effort — don't fail the request on a record error)
	_ = b.cfg.Enforcer.RecordUsage(
		b.cfg.SessionID,
		b.cfg.Harness,
		int64(resp.Usage.InputTokens),
		int64(resp.Usage.OutputTokens),
		resp.Usage.CostUSD,
	)

	// Successful call: if we were half-open, close the breaker
	b.closeBreaker()

	return resp, nil
}

// tryFallbackChain walks FallbackChain until a provider succeeds or all are exhausted.
func (b *BudgetBreaker) tryFallbackChain(ctx context.Context, req *GenerateRequest) (*GenerateResponse, error) {
	for _, fallback := range b.cfg.FallbackChain {
		resp, err := b.tryGenerate(ctx, req, fallback)
		if err == nil {
			return resp, nil
		}
		var budgetErr *ErrBudgetExhausted
		if errors.As(err, &budgetErr) {
			// This fallback is also over budget — try the next cheaper one
			continue
		}
		return nil, err
	}

	return nil, fmt.Errorf("all providers exhausted: %w",
		&ErrBudgetExhausted{Reason: "no remaining budget in fallback chain"})
}

// ErrBudgetExhausted is returned when budget enforcement rejects the request.
type ErrBudgetExhausted struct {
	Reason string
}

func (e *ErrBudgetExhausted) Error() string {
	return "budget exhausted: " + e.Reason
}
