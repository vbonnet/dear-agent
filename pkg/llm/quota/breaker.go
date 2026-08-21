package quota

import (
	"fmt"
	"sync"
	"time"
)

// BreakerState is what the guardrail permits for one provider family.
type BreakerState string

const (
	// BreakerClosed allows new work. This is also what every unknown
	// reading produces.
	BreakerClosed BreakerState = "closed"

	// BreakerThrottled allows new work but rate-limits it, so a provider
	// heading for its limit slows down instead of stopping dead.
	BreakerThrottled BreakerState = "throttled"

	// BreakerOpen refuses new work on this provider until its window
	// resets or a reading shows recovery.
	BreakerOpen BreakerState = "open"
)

// Allowed reports whether the state permits new work at all.
func (s BreakerState) Allowed() bool { return s != BreakerOpen }

// Breaker guardrail defaults. Halting is deliberately reserved for the
// last few percent, or for a confirmed overspend that will not survive to
// the window reset — a breaker that trips early is a breaker an operator
// switches off.
const (
	DefaultHaltBelowRemainingPercent      = 3.0
	DefaultThrottleBelowRemainingPercent  = 15.0
	DefaultSpikeHaltBelowRemainingPercent = 20.0
	DefaultThrottledSpawnsPerHour         = 4
	DefaultBreakerCooldown                = 10 * time.Minute
)

// BreakerPolicy configures the cost guardrail.
type BreakerPolicy struct {
	// HaltBelowRemainingPercent opens the breaker on remaining quota
	// alone. Zero uses the default; negative disables.
	HaltBelowRemainingPercent float64

	// ThrottleBelowRemainingPercent throttles rather than halts. Zero
	// uses the default; negative disables.
	ThrottleBelowRemainingPercent float64

	// SpikeHaltBelowRemainingPercent is the headroom under which a
	// confirmed overspend — the meter reporting the burn will not last to
	// the window reset — is treated as a halt rather than a throttle.
	// Above this much headroom a spike only throttles, because there is
	// still room to correct. Zero uses the default; negative ignores
	// burn rate altogether, leaving raw headroom as the only signal.
	SpikeHaltBelowRemainingPercent float64

	// ThrottledSpawnsPerHour caps admissions per family while throttled.
	// Zero uses the default; negative means "throttled behaves as closed".
	ThrottledSpawnsPerHour int

	// Cooldown is how long an open breaker waits before re-reading state.
	// The breaker also closes as soon as a fresh reading shows recovery or
	// the constraining window has reset. Zero uses the default.
	Cooldown time.Duration
}

func (p BreakerPolicy) haltBelow() float64 {
	if p.HaltBelowRemainingPercent == 0 {
		return DefaultHaltBelowRemainingPercent
	}
	return p.HaltBelowRemainingPercent
}

func (p BreakerPolicy) throttleBelow() float64 {
	if p.ThrottleBelowRemainingPercent == 0 {
		return DefaultThrottleBelowRemainingPercent
	}
	return p.ThrottleBelowRemainingPercent
}

func (p BreakerPolicy) spikeHaltBelow() float64 {
	if p.SpikeHaltBelowRemainingPercent == 0 {
		return DefaultSpikeHaltBelowRemainingPercent
	}
	return p.SpikeHaltBelowRemainingPercent
}

func (p BreakerPolicy) spawnsPerHour() int {
	if p.ThrottledSpawnsPerHour == 0 {
		return DefaultThrottledSpawnsPerHour
	}
	return p.ThrottledSpawnsPerHour
}

func (p BreakerPolicy) cooldown() time.Duration {
	if p.Cooldown == 0 {
		return DefaultBreakerCooldown
	}
	return p.Cooldown
}

// Verdict is one guardrail decision for one provider family.
type Verdict struct {
	// Family is the provider family the verdict covers.
	Family string

	// State is the guardrail's position.
	State BreakerState

	// Allowed is whether this particular request may proceed. It differs
	// from State.Allowed() only while throttled, where admission depends
	// on how much has already been admitted this hour.
	Allowed bool

	// Reason is a one-line explanation naming the constraint, suitable
	// for an operator-facing error or a warning log.
	Reason string

	// RemainingPercent is the most constrained window's headroom, when a
	// reading was available.
	RemainingPercent float64

	// Window labels the sub-budget that produced the verdict.
	Window string

	// ResetsAt is when the constraining window refills, so a caller can
	// tell an operator when work resumes. Zero when unknown.
	ResetsAt time.Time

	// RetryAfter suggests how long to wait before trying again. Zero when
	// the request was allowed.
	RetryAfter time.Duration

	// Spike is true when a burn-rate reading, not raw headroom, drove the
	// verdict.
	Spike bool
}

// Breaker is the runaway-cost guardrail: it decides whether new work may
// start on a provider family, given the meter's reading.
//
// It is deliberately a *spawn-time* gate rather than a per-request one.
// A single agent session spends for as long as it lives, so refusing to
// start one is the control that actually bounds cost; refusing an
// individual call inside a session that is already running mostly just
// breaks that session.
//
// Fail-safe direction matches the rest of the package: no reading, a
// stale reading, an unauthenticated provider, or no meter at all all
// yield BreakerClosed. The guardrail can only ever stop work on evidence,
// never on the absence of it.
type Breaker struct {
	meter  *Meter
	policy BreakerPolicy
	now    func() time.Time

	mu       sync.Mutex
	admitted map[string][]time.Time
}

// NewBreaker builds a guardrail over a meter. A nil meter yields a
// breaker that allows everything, so callers can hold one
// unconditionally.
func NewBreaker(meter *Meter, policy BreakerPolicy) *Breaker {
	now := time.Now
	if meter != nil && meter.now != nil {
		now = meter.now
	}
	return &Breaker{
		meter:    meter,
		policy:   policy,
		now:      now,
		admitted: make(map[string][]time.Time),
	}
}

// Evaluate reports the guardrail's position for a family without
// consuming throttle budget. Use it for status output and warnings;
// use Admit when actually starting work.
func (b *Breaker) Evaluate(family string) Verdict {
	if b == nil || !b.meter.Enabled() {
		return Verdict{
			Family:  family,
			State:   BreakerClosed,
			Allowed: true,
			Reason:  "quota guardrail disabled",
		}
	}
	decision := b.meter.DecisionFor(family)
	pace, resetsAt := b.burnContext(family)
	verdict := b.verdictFor(decision, pace)
	verdict.ResetsAt = resetsAt
	return verdict
}

// Admit is Evaluate plus throttle accounting: when the breaker is
// throttled, it consumes one admission from the hourly allowance and
// refuses once the allowance is spent.
//
// Callers that are about to start work call this exactly once, and must
// honour a false Allowed.
func (b *Breaker) Admit(family string) Verdict {
	verdict := b.Evaluate(family)
	if verdict.State != BreakerThrottled {
		return verdict
	}

	limit := b.policy.spawnsPerHour()
	if limit < 0 {
		verdict.Allowed = true
		return verdict
	}

	now := b.now()
	cutoff := now.Add(-time.Hour)

	b.mu.Lock()
	defer b.mu.Unlock()
	kept := b.admitted[family][:0]
	for _, at := range b.admitted[family] {
		if at.After(cutoff) {
			kept = append(kept, at)
		}
	}
	b.admitted[family] = kept

	if len(kept) >= limit {
		verdict.Allowed = false
		verdict.RetryAfter = max(time.Until(kept[0].Add(time.Hour)), 0)
		verdict.Reason = fmt.Sprintf("%s; throttled to %d starts per hour and %d have already run",
			verdict.Reason, limit, len(kept))
		return verdict
	}
	b.admitted[family] = append(b.admitted[family], now)
	return verdict
}

// burnContext returns the family's burn-rate reading and the reset time
// of its most constrained window, so a verdict can tell an operator both
// why work stopped and when it resumes on its own.
func (b *Breaker) burnContext(family string) (*Pace, time.Time) {
	snapshot, _ := b.meter.Snapshot()
	provider, ok := snapshot.Provider(family)
	if !ok {
		return nil, time.Time{}
	}
	worst, ok := provider.MostConstrainedActive(b.now())
	if !ok {
		return provider.Pace, time.Time{}
	}
	return provider.Pace, worst.ResetAt
}

// verdictFor is the policy itself, factored out so it is testable without
// a meter and readable as a single ordered set of rules.
func (b *Breaker) verdictFor(d Decision, pace *Pace) Verdict {
	verdict := Verdict{
		Family:           d.Family,
		State:            BreakerClosed,
		Allowed:          true,
		RemainingPercent: d.RemainingPercent,
		Window:           d.ConstrainedWindow,
	}

	// No usable reading is never grounds to stop work.
	if !d.Known() {
		verdict.Reason = d.Reason
		return verdict
	}

	halt := b.policy.haltBelow()
	throttle := b.policy.throttleBelow()
	spikeHalt := b.policy.spikeHaltBelow()
	// A negative spike floor switches burn rate off as a signal, so the
	// softer spike-throttle below must respect it too — an operator who
	// disables every rule gets a breaker that never trips.
	overspending := spikeHalt >= 0 && pace.Overspending()

	switch {
	case halt > 0 && d.RemainingPercent <= halt:
		verdict.State = BreakerOpen
		verdict.Allowed = false
		verdict.Reason = fmt.Sprintf("%s has %.1f%% left, at or below the %.1f%% halt floor",
			d.ConstrainedWindow, d.RemainingPercent, halt)

	case overspending && spikeHalt > 0 && d.RemainingPercent <= spikeHalt:
		verdict.State = BreakerOpen
		verdict.Allowed = false
		verdict.Spike = true
		verdict.Reason = fmt.Sprintf("%s has %.1f%% left and the current burn will not reach the reset (%s)",
			d.ConstrainedWindow, d.RemainingPercent, paceDetail(pace))

	case throttle > 0 && d.RemainingPercent <= throttle:
		verdict.State = BreakerThrottled
		verdict.Reason = fmt.Sprintf("%s has %.1f%% left, at or below the %.1f%% throttle floor",
			d.ConstrainedWindow, d.RemainingPercent, throttle)

	case overspending:
		verdict.State = BreakerThrottled
		verdict.Spike = true
		verdict.Reason = fmt.Sprintf("%s has %.1f%% left but is overspending (%s)",
			d.ConstrainedWindow, d.RemainingPercent, paceDetail(pace))

	default:
		verdict.Reason = fmt.Sprintf("%s has %.1f%% left", d.ConstrainedWindow, d.RemainingPercent)
	}

	if !verdict.Allowed {
		verdict.RetryAfter = b.policy.cooldown()
	}
	return verdict
}

// paceDetail renders the burn-rate evidence behind a spike verdict,
// preferring the meter's own sentence when it has one.
func paceDetail(p *Pace) string {
	if p == nil {
		return "no burn-rate detail"
	}
	if p.Summary != "" {
		return p.Summary
	}
	if p.ExhaustsIn > 0 {
		return fmt.Sprintf("%.0f%% ahead of pace, empty in %s", p.DeltaPercent, p.ExhaustsIn.Round(time.Minute))
	}
	return fmt.Sprintf("%.0f%% ahead of pace", p.DeltaPercent)
}
