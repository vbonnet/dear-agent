package quota_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/vbonnet/dear-agent/pkg/llm/quota"
)

// breakerMeter builds a warmed meter over a hand-written reading.
func breakerMeter(t *testing.T, providers ...quota.ProviderQuota) *quota.Meter {
	t.Helper()
	now := time.Now()
	snapshot := &quota.Snapshot{Source: "test", GeneratedAt: now, Providers: providers}
	meter := quota.New(quota.Options{
		Reader:          &stubReader{snapshot: snapshot},
		RefreshInterval: -1,
		Now:             func() time.Time { return now },
	})
	if _, err := meter.Refresh(context.Background()); err != nil {
		t.Fatalf("refresh: %v", err)
	}
	return meter
}

func provider(family string, remaining float64, pace *quota.Pace) quota.ProviderQuota {
	return quota.ProviderQuota{
		Family:       family,
		SourceID:     family,
		Availability: quota.AvailabilityOK,
		Pace:         pace,
		Windows: []quota.Window{{
			ID:               "weekly",
			Label:            "Weekly",
			RemainingPercent: remaining,
			UsedPercent:      100 - remaining,
			ResetAt:          time.Now().Add(48 * time.Hour),
		}},
	}
}

func overspending() *quota.Pace {
	return &quota.Pace{
		Stage:           "farAhead",
		DeltaPercent:    39,
		WillLastToReset: false,
		ExhaustsIn:      20 * time.Hour,
		Summary:         "39% in deficit | Runs out in 20h 27m",
	}
}

func onTrack() *quota.Pace {
	return &quota.Pace{Stage: "onTrack", DeltaPercent: 1, WillLastToReset: true}
}

func TestBreakerHaltsOnExhaustedHeadroom(t *testing.T) {
	b := quota.NewBreaker(breakerMeter(t, provider("openai", 2, nil)), quota.BreakerPolicy{})
	got := b.Evaluate("openai")
	if got.State != quota.BreakerOpen {
		t.Errorf("State = %q, want open", got.State)
	}
	if got.Allowed {
		t.Error("an exhausted provider must not admit new work")
	}
	if !strings.Contains(got.Reason, "halt floor") {
		t.Errorf("Reason = %q, want it to name the halt floor", got.Reason)
	}
	if got.ResetsAt.IsZero() {
		t.Error("a refusal should say when the window resets")
	}
}

// The spend-spike case: plenty of raw headroom left, but the burn rate
// will not reach the reset. Headroom alone would have allowed this.
func TestBreakerHaltsOnASpendSpikeWithHeadroomRemaining(t *testing.T) {
	b := quota.NewBreaker(breakerMeter(t, provider("openai", 18, overspending())), quota.BreakerPolicy{})
	got := b.Evaluate("openai")
	if got.State != quota.BreakerOpen {
		t.Fatalf("State = %q, want open", got.State)
	}
	if !got.Spike {
		t.Error("want the verdict marked as burn-rate driven")
	}
	if !strings.Contains(got.Reason, "will not reach the reset") {
		t.Errorf("Reason = %q, want it to explain the burn rate", got.Reason)
	}
}

// Above the spike floor there is still room to correct, so a spike
// throttles rather than halts.
func TestBreakerOnlyThrottlesASpikeWithAmpleHeadroom(t *testing.T) {
	b := quota.NewBreaker(breakerMeter(t, provider("openai", 60, overspending())), quota.BreakerPolicy{})
	got := b.Evaluate("openai")
	if got.State != quota.BreakerThrottled {
		t.Errorf("State = %q, want throttled", got.State)
	}
	if !got.Allowed {
		t.Error("a throttled provider still admits the first work")
	}
	if !got.Spike {
		t.Error("want the verdict marked as burn-rate driven")
	}
}

func TestBreakerThrottlesOnLowHeadroom(t *testing.T) {
	b := quota.NewBreaker(breakerMeter(t, provider("openai", 12, onTrack())), quota.BreakerPolicy{})
	got := b.Evaluate("openai")
	if got.State != quota.BreakerThrottled {
		t.Errorf("State = %q, want throttled", got.State)
	}
	if !strings.Contains(got.Reason, "throttle floor") {
		t.Errorf("Reason = %q, want it to name the throttle floor", got.Reason)
	}
}

func TestBreakerStaysClosedWhenHealthy(t *testing.T) {
	b := quota.NewBreaker(breakerMeter(t, provider("openai", 80, onTrack())), quota.BreakerPolicy{})
	got := b.Evaluate("openai")
	if got.State != quota.BreakerClosed || !got.Allowed {
		t.Errorf("verdict = %+v, want closed and allowed", got)
	}
	if got.Spike {
		t.Error("an on-track provider is not a spike")
	}
}

// The guardrail's central safety property: it can only ever stop work on
// positive evidence. Every flavour of not-knowing stays closed.
func TestBreakerNeverHaltsWithoutEvidence(t *testing.T) {
	authFailed := quota.ProviderQuota{
		Family:       "anthropic",
		Availability: quota.AvailabilityAuthRequired,
		Note:         "No Claude session key found in browser cookies.",
	}
	disabled := quota.ProviderQuota{Family: "gemini", Availability: quota.AvailabilityDisabled}

	tests := []struct {
		name   string
		meter  *quota.Meter
		family string
	}{
		{name: "no meter at all", meter: nil, family: "openai"},
		{name: "meter with no reader", meter: quota.New(quota.Options{}), family: "openai"},
		{name: "family absent from the reading", meter: breakerMeter(t, provider("openai", 1, nil)), family: "anthropic"},
		{name: "provider needs credentials", meter: breakerMeter(t, authFailed), family: "anthropic"},
		{name: "provider disabled in the meter", meter: breakerMeter(t, disabled), family: "gemini"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			b := quota.NewBreaker(tc.meter, quota.BreakerPolicy{})
			got := b.Evaluate(tc.family)
			if got.State != quota.BreakerClosed {
				t.Errorf("State = %q, want closed", got.State)
			}
			if !got.Allowed {
				t.Errorf("want the work allowed; reason = %q", got.Reason)
			}
		})
	}
}

func TestBreakerStaleReadingDoesNotHalt(t *testing.T) {
	old := time.Now().Add(-24 * time.Hour)
	snapshot := &quota.Snapshot{
		Source:      "test",
		GeneratedAt: old,
		Providers:   []quota.ProviderQuota{provider("openai", 0, overspending())},
	}
	meter := quota.New(quota.Options{
		Reader:          &stubReader{snapshot: snapshot},
		Policy:          quota.Policy{MaxSnapshotAge: time.Minute},
		RefreshInterval: -1,
	})
	if _, err := meter.Refresh(context.Background()); err != nil {
		t.Fatalf("refresh: %v", err)
	}
	got := quota.NewBreaker(meter, quota.BreakerPolicy{}).Evaluate("openai")
	if got.State != quota.BreakerClosed || !got.Allowed {
		t.Errorf("verdict = %+v, want closed on a stale reading", got)
	}
}

func TestBreakerAdmitSpendsTheThrottleAllowance(t *testing.T) {
	b := quota.NewBreaker(
		breakerMeter(t, provider("openai", 12, onTrack())),
		quota.BreakerPolicy{ThrottledSpawnsPerHour: 2},
	)
	for i := range 2 {
		if got := b.Admit("openai"); !got.Allowed {
			t.Fatalf("admission %d refused: %s", i+1, got.Reason)
		}
	}
	got := b.Admit("openai")
	if got.Allowed {
		t.Error("want the third admission refused once the hourly allowance is spent")
	}
	if got.State != quota.BreakerThrottled {
		t.Errorf("State = %q, want throttled", got.State)
	}
	if !strings.Contains(got.Reason, "per hour") {
		t.Errorf("Reason = %q, want it to explain the throttle", got.Reason)
	}
}

func TestBreakerAdmitDoesNotThrottleAHealthyProvider(t *testing.T) {
	b := quota.NewBreaker(
		breakerMeter(t, provider("openai", 90, onTrack())),
		quota.BreakerPolicy{ThrottledSpawnsPerHour: 1},
	)
	for i := range 5 {
		if got := b.Admit("openai"); !got.Allowed {
			t.Fatalf("admission %d refused on a healthy provider: %s", i+1, got.Reason)
		}
	}
}

func TestBreakerEvaluateDoesNotConsumeAllowance(t *testing.T) {
	b := quota.NewBreaker(
		breakerMeter(t, provider("openai", 12, onTrack())),
		quota.BreakerPolicy{ThrottledSpawnsPerHour: 1},
	)
	for range 10 {
		b.Evaluate("openai")
	}
	if got := b.Admit("openai"); !got.Allowed {
		t.Error("Evaluate must not spend the throttle allowance")
	}
}

func TestBreakerPolicyCanDisableEachRule(t *testing.T) {
	meter := breakerMeter(t, provider("openai", 1, overspending()))
	b := quota.NewBreaker(meter, quota.BreakerPolicy{
		HaltBelowRemainingPercent:      -1,
		SpikeHaltBelowRemainingPercent: -1,
		ThrottleBelowRemainingPercent:  -1,
	})
	got := b.Evaluate("openai")
	if got.State != quota.BreakerClosed {
		t.Errorf("State = %q, want closed when every rule is disabled", got.State)
	}
}

func TestBreakerNegativeThrottleAllowanceBehavesAsClosed(t *testing.T) {
	b := quota.NewBreaker(
		breakerMeter(t, provider("openai", 12, onTrack())),
		quota.BreakerPolicy{ThrottledSpawnsPerHour: -1},
	)
	for range 20 {
		if got := b.Admit("openai"); !got.Allowed {
			t.Fatal("a negative allowance must not refuse work")
		}
	}
}
