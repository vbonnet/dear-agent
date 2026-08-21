package quota

import (
	"fmt"
	"time"
)

// Class is the routing verdict for one provider family.
type Class string

const (
	// ClassUnknown means no usable reading. Routing must behave exactly
	// as it would without this package.
	ClassUnknown Class = "unknown"

	// ClassHealthy means the provider has room.
	ClassHealthy Class = "healthy"

	// ClassDeprioritized means the provider still works but should be
	// tried after healthier candidates.
	ClassDeprioritized Class = "deprioritized"

	// ClassAvoid means the provider is close enough to its limit that it
	// should be tried only when nothing else is left.
	ClassAvoid Class = "avoid"
)

// Default policy thresholds. They are deliberately conservative: avoid
// triggers only in the last few percent, because a wrong avoid costs a
// working provider while a wrong allow costs one rejected request.
const (
	DefaultAvoidBelowRemainingPercent        = 5.0
	DefaultDeprioritizeBelowRemainingPercent = 25.0
	DefaultMaxSnapshotAge                    = 15 * time.Minute
)

// Policy configures how a reading turns into a routing verdict.
type Policy struct {
	// AvoidBelowRemainingPercent classifies a family as ClassAvoid when
	// its most constrained window is at or below this percentage.
	// Zero uses DefaultAvoidBelowRemainingPercent; negative disables.
	AvoidBelowRemainingPercent float64

	// DeprioritizeBelowRemainingPercent classifies a family as
	// ClassDeprioritized at or below this percentage. Zero uses
	// DefaultDeprioritizeBelowRemainingPercent; negative disables.
	DeprioritizeBelowRemainingPercent float64

	// MaxSnapshotAge rejects readings older than this, which fall back to
	// ClassUnknown. Zero uses DefaultMaxSnapshotAge; negative accepts any
	// age.
	MaxSnapshotAge time.Duration
}

func (p Policy) avoidBelow() float64 {
	if p.AvoidBelowRemainingPercent == 0 {
		return DefaultAvoidBelowRemainingPercent
	}
	return p.AvoidBelowRemainingPercent
}

func (p Policy) deprioritizeBelow() float64 {
	if p.DeprioritizeBelowRemainingPercent == 0 {
		return DefaultDeprioritizeBelowRemainingPercent
	}
	return p.DeprioritizeBelowRemainingPercent
}

func (p Policy) maxAge() time.Duration {
	if p.MaxSnapshotAge == 0 {
		return DefaultMaxSnapshotAge
	}
	return p.MaxSnapshotAge
}

// Decision is the policy verdict for one family, carrying enough context
// for a router to log why it reordered candidates.
type Decision struct {
	// Family is the dear-agent provider family this verdict covers.
	Family string

	// Class is the verdict. ClassUnknown means "route as if unmetered".
	Class Class

	// RemainingPercent is the most constrained window's remaining share,
	// meaningful only when Class is not ClassUnknown.
	RemainingPercent float64

	// ConstrainedWindow labels the window that produced RemainingPercent,
	// so an operator can see which sub-budget is binding.
	ConstrainedWindow string

	// Availability distinguishes a credential problem from exhaustion.
	Availability Availability

	// Stale is true when a reading existed but was older than the policy
	// allows. Class is ClassUnknown in that case.
	Stale bool

	// Reason is a one-line explanation suitable for logs and metadata.
	Reason string
}

// Known reports whether the decision carries a usable quota reading.
func (d Decision) Known() bool { return d.Class != ClassUnknown }

// Evaluate reduces a snapshot to a verdict for one family.
//
// It never returns ClassAvoid on the strength of a missing reading: a
// nil snapshot, a stale snapshot, an unknown family, and an unreadable
// provider all return ClassUnknown, which callers treat as full capacity.
func Evaluate(snapshot *Snapshot, family string, now time.Time, policy Policy) Decision {
	decision := Decision{
		Family:       family,
		Class:        ClassUnknown,
		Availability: AvailabilityUnavailable,
	}

	if snapshot == nil {
		decision.Reason = "no quota reading available"
		return decision
	}
	maxAge := policy.maxAge()
	if maxAge > 0 {
		if snapshot.GeneratedAt.IsZero() {
			// A missing/unparseable generatedAt is an unknown age, not a
			// fresh one: Snapshot.Age reports 0 for display purposes, but
			// treating that as "fresh" here would let an undated reading
			// (and any provider percentage retained under it) route
			// indefinitely (codex review on #1218).
			decision.Stale = true
			decision.Reason = "quota snapshot has no generation time, cannot confirm freshness"
			return decision
		}
		if age := snapshot.Age(now); age > maxAge {
			decision.Stale = true
			decision.Reason = fmt.Sprintf("quota reading is %s old, past the %s limit", age.Round(time.Second), maxAge)
			return decision
		}
	}

	quota, ok := snapshot.Provider(family)
	if !ok {
		decision.Reason = fmt.Sprintf("no quota reading for family %q", family)
		return decision
	}
	if maxAge > 0 && !quota.UpdatedAt.IsZero() {
		// The dashboard as a whole can be freshly generated while one
		// provider's own refresh failed and its windows were carried over
		// from an earlier read; convertProvider still marks that provider
		// AvailabilityOK as long as it has windows. Apply the same age
		// limit to the provider's own UpdatedAt so a stale carried-over
		// reading expires even though the snapshot's GeneratedAt is fresh.
		if age := now.Sub(quota.UpdatedAt); age > maxAge {
			decision.Stale = true
			decision.Reason = fmt.Sprintf("%s reading is %s old, past the %s limit", family, age.Round(time.Second), maxAge)
			return decision
		}
	}
	decision.Availability = quota.Availability
	if !quota.Availability.Known() {
		decision.Reason = availabilityReason(quota)
		return decision
	}

	worst, ok := quota.MostConstrainedActive(now)
	if !ok {
		decision.Reason = fmt.Sprintf("family %q reported no usage windows", family)
		return decision
	}
	decision.RemainingPercent = worst.RemainingPercent
	decision.ConstrainedWindow = windowName(worst)

	avoid := policy.avoidBelow()
	deprioritize := policy.deprioritizeBelow()
	switch {
	case avoid > 0 && worst.RemainingPercent <= avoid:
		decision.Class = ClassAvoid
		decision.Reason = fmt.Sprintf("%s has %.1f%% left, at or below the %.1f%% avoid floor",
			decision.ConstrainedWindow, worst.RemainingPercent, avoid)
	case deprioritize > 0 && worst.RemainingPercent <= deprioritize:
		decision.Class = ClassDeprioritized
		decision.Reason = fmt.Sprintf("%s has %.1f%% left, at or below the %.1f%% deprioritize floor",
			decision.ConstrainedWindow, worst.RemainingPercent, deprioritize)
	default:
		decision.Class = ClassHealthy
		decision.Reason = fmt.Sprintf("%s has %.1f%% left", decision.ConstrainedWindow, worst.RemainingPercent)
	}
	return decision
}

// availabilityReason explains an unusable reading in the terms an
// operator needs: whose fault it is and what it is not (exhaustion).
func availabilityReason(quota ProviderQuota) string {
	note := quota.Note
	if note == "" {
		note = "no detail reported"
	}
	switch quota.Availability {
	case AvailabilityAuthRequired:
		return fmt.Sprintf("quota unreadable, credentials needed (not exhaustion): %s", note)
	case AvailabilityDisabled:
		return fmt.Sprintf("quota not collected, provider disabled in the meter: %s", note)
	case AvailabilityOK, AvailabilityUnavailable:
		return fmt.Sprintf("quota unreadable (not exhaustion): %s", note)
	default:
		return fmt.Sprintf("quota unreadable (not exhaustion): %s", note)
	}
}

func windowName(w Window) string {
	switch {
	case w.Label != "":
		return w.Label
	case w.ID != "":
		return w.ID
	default:
		return "usage window"
	}
}

// Band scores a decision into a coarse routing band; lower sorts first.
//
// Bands are quartiles rather than raw percentages on purpose. Ordering on
// the exact remaining percentage would reshuffle every role's vendor
// assignment on each refresh — a provider one point ahead would capture
// all traffic until it fell one point behind. Quartiles let load spread
// across providers that are genuinely in different shape while leaving
// the operator's per-role vendor choice in roles.yaml intact within a
// band.
//
// ClassUnknown scores 0 so an unmetered provider keeps its configured
// position, which is what makes the whole package fail-safe.
func Band(d Decision) int {
	if !d.Known() {
		return 0
	}
	band := 0
	switch {
	case d.RemainingPercent >= 75:
		band = 0
	case d.RemainingPercent >= 50:
		band = 1
	case d.RemainingPercent >= 25:
		band = 2
	default:
		band = 3
	}

	// An explicit threshold verdict floors the band, so operators who
	// tune the thresholds get the demotion they asked for even when the
	// quartile alone would not have produced it.
	switch d.Class {
	case ClassAvoid:
		if band < 4 {
			band = 4
		}
	case ClassDeprioritized:
		if band < 3 {
			band = 3
		}
	case ClassUnknown, ClassHealthy:
	}
	return band
}
