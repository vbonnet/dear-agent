// Package quota reads per-provider remaining-quota readings from a local
// meter and reduces them to a routing decision.
//
// The package owns three concerns, in layers:
//
//  1. Reader — pulls a Snapshot from some local source. CodexBarReader is
//     the shipped implementation; it shells out to the codexbar CLI.
//  2. Policy — reduces a Snapshot to a per-family Decision (healthy,
//     deprioritized, avoid) using operator-set thresholds.
//  3. Meter — caches a Snapshot, refreshes it off the request path, and
//     exposes the decision to routers as a candidate ordering.
//
// Every layer is fail-safe in the same direction: a missing, stale, or
// unreadable reading yields "unknown", and unknown always routes exactly
// as the caller would have routed without this package. Quota data can
// only demote a provider, never promote one or block a role outright.
//
// See docs/adr/ADR-038-codexbar-quota-routing.md for the design.
package quota

import "time"

// Availability records whether a provider produced a usable reading and,
// when it did not, why. It exists so that an operator (and the router)
// can tell a credential problem apart from genuine exhaustion: a provider
// nobody can read is not a provider that is out of quota.
type Availability string

const (
	// AvailabilityOK means the provider reported at least one usage window.
	AvailabilityOK Availability = "ok"

	// AvailabilityDisabled means the meter knows the provider but the
	// operator has switched it off, so no reading is expected.
	AvailabilityDisabled Availability = "disabled"

	// AvailabilityAuthRequired means the meter could not authenticate to
	// the provider. Remaining quota is unknown, not zero.
	AvailabilityAuthRequired Availability = "auth_required"

	// AvailabilityUnavailable means the meter failed to produce a reading
	// for some other reason (timeout, unsupported provider, parse gap).
	AvailabilityUnavailable Availability = "unavailable"
)

// Known reports whether the availability carries a usable quota reading.
func (a Availability) Known() bool { return a == AvailabilityOK }

// Snapshot is one reading of every provider the meter knows about.
type Snapshot struct {
	// Source names the reader that produced this snapshot, e.g. "codexbar".
	Source string

	// SourceVersion is the reader-reported version of the underlying tool,
	// recorded so a schema regression is attributable.
	SourceVersion string

	// GeneratedAt is when the source produced the reading, not when this
	// process parsed it. Freshness is judged against this.
	GeneratedAt time.Time

	// StaleAfter is the source's own opinion of how long the reading
	// stays good. It is advisory: Policy.MaxSnapshotAge decides. It is
	// reported so an operator can tune the policy against the number the
	// meter itself publishes rather than guessing.
	StaleAfter time.Duration

	// Providers is one entry per provider family the source reported.
	Providers []ProviderQuota
}

// ProviderQuota is the reading for one provider family, such as
// "anthropic", "openai", or "gemini".
type ProviderQuota struct {
	// Family is the dear-agent provider family, as returned by
	// provider.Resolver. This is the key routing joins on.
	Family string

	// SourceID is the meter's own name for the provider ("claude",
	// "codex", "antigravity"). Kept for diagnostics: one family can be
	// fed by more than one source id.
	SourceID string

	// Account identifies which account the reading covers. Redacted by
	// the reader — never a full address.
	Account string

	// Plan is the subscription tier the source reported, e.g. "Pro 20x".
	Plan string

	// Availability says whether Windows is usable and, if not, why.
	Availability Availability

	// Note carries the source's own explanation when Availability is not
	// OK, or a soft warning when it is OK despite a partial failure.
	Note string

	// Windows is every sub-budget the provider reports: rolling session
	// windows, weekly caps, and any provider-specific extras.
	Windows []Window

	// UpdatedAt is when the source last refreshed this provider.
	UpdatedAt time.Time
}

// Window is one sub-budget within a provider — a single rate-limit or
// quota bucket with its own reset clock.
type Window struct {
	// ID is the source's stable identifier for the window.
	ID string

	// Label is the human-readable name, e.g. "Weekly", "Gemini 5-hour".
	Label string

	// RemainingPercent is 0..100 of the window still available.
	RemainingPercent float64

	// UsedPercent is 0..100 of the window consumed.
	UsedPercent float64

	// ResetAt is when the window refills. Zero when the source did not
	// report one.
	ResetAt time.Time
}

// Age reports how old the snapshot is relative to now. A zero
// GeneratedAt yields a zero age so an undated snapshot is never
// discarded for staleness alone.
func (s *Snapshot) Age(now time.Time) time.Duration {
	if s == nil || s.GeneratedAt.IsZero() {
		return 0
	}
	age := now.Sub(s.GeneratedAt)
	if age < 0 {
		return 0
	}
	return age
}

// Provider returns the reading for one family. The second result is
// false when the snapshot does not mention that family at all.
func (s *Snapshot) Provider(family string) (ProviderQuota, bool) {
	if s == nil {
		return ProviderQuota{}, false
	}
	for _, p := range s.Providers {
		if p.Family == family {
			return p, true
		}
	}
	return ProviderQuota{}, false
}

// MostConstrained returns the window with the least remaining quota —
// the one that will actually stop the provider. The second result is
// false when the provider reported no windows.
func (p ProviderQuota) MostConstrained() (Window, bool) {
	if len(p.Windows) == 0 {
		return Window{}, false
	}
	worst := p.Windows[0]
	for _, w := range p.Windows[1:] {
		if w.RemainingPercent < worst.RemainingPercent {
			worst = w
		}
	}
	return worst, true
}
