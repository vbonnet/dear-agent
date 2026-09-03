package agenticreview

import (
	"fmt"
	"slices"
	"strings"
	"time"
)

// FamilyState is one family's resolved position at evaluation time.
type FamilyState string

const (
	// StateMissing means the family has published nothing and is still inside
	// its dispatch window. It blocks: the review has not started, so there is
	// nothing to have passed.
	StateMissing FamilyState = "MISSING"
	// StatePending means the family started and may still publish a verdict.
	// It blocks, and it is the state that closes the ready-to-merge window.
	StatePending FamilyState = "PENDING"
	// StateApproved means the family published a clean verdict.
	StateApproved FamilyState = "APPROVED"
	// StateBlocking means the family requested changes. No quorum clears it.
	StateBlocking FamilyState = "BLOCKING"
	// StateDown means the family is established as unable to report, either by
	// its own error label or by exhausting a deadline. A down family is
	// excluded from the vote rather than counted against it.
	StateDown FamilyState = "DOWN"
)

// Decision is the gate's overall answer.
type Decision string

const (
	// DecisionPass means the quorum is satisfied and nothing is blocking.
	DecisionPass Decision = "PASS"
	// DecisionPending means the review lifecycle has not resolved yet. It is
	// not mergeable, but it is expected to settle without intervention.
	DecisionPending Decision = "PENDING"
	// DecisionBlock means the review lifecycle resolved against the merge.
	DecisionBlock Decision = "BLOCK"
)

// Config is the gate's policy. Every field is operator-configurable so the
// strictness of the gate is a repository decision rather than a code change.
type Config struct {
	// Families is the reviewer set under the gate, in reporting order.
	Families []Family
	// Quorum is how many families must approve for the gate to pass. It is
	// the degradation allowance: with three families and a quorum of two, one
	// family may be down without wedging the merge queue. Setting it to
	// len(Families) withdraws the allowance and demands unanimity.
	Quorum int
	// VerdictTimeout is how long a family that has started (or posted) has to
	// publish a verdict before it is treated as down.
	VerdictTimeout time.Duration
	// DispatchTimeout is how long after the pull request became reviewable a
	// family has to publish its started label before it is treated as down.
	// Without it a reviewer that never comes up at all blocks every merge
	// forever, which is an outage dressed as a policy.
	DispatchTimeout time.Duration
}

// Input is everything the gate reads. It is deliberately inert data: no
// network, no clock, no GitHub. The caller that fetched the labels is also the
// caller that decides what "now" means, which is what makes the same decision
// reproducible in CI, in the merge loop, and in a test.
type Input struct {
	// Labels are the pull request's current labels. Labels outside this
	// package's namespace are ignored.
	Labels []string `json:"labels"`
	// AppliedAt maps a label to the time it was applied. A label with no
	// recorded time can never be aged out, so its family stays pending: the
	// gate does not convert missing evidence into permission to merge.
	AppliedAt map[string]time.Time `json:"applied_at,omitempty"`
	// ReadyAt is when the pull request became reviewable. A zero value
	// disables the dispatch deadline, leaving undispatched families missing.
	ReadyAt time.Time `json:"ready_at"`
	// Now is the evaluation time.
	Now time.Time `json:"now"`
}

// FamilyVerdict is one family's resolved state with the reason for it.
type FamilyVerdict struct {
	Family Family      `json:"family"`
	State  FamilyState `json:"state"`
	Reason string      `json:"reason"`
}

// Verdict is the gate's full answer: the decision, the reason a human reads on
// a red check, and the per-family detail behind it.
type Verdict struct {
	Decision Decision        `json:"decision"`
	Reason   string          `json:"reason"`
	Families []FamilyVerdict `json:"families"`
	Approved int             `json:"approved"`
	Down     int             `json:"down"`
	Quorum   int             `json:"quorum"`
	// Unconfigured names families that published labels but are not in the
	// configured set. They never vote; they are reported so a reviewer wired
	// up under a name the gate does not know shows up as a misconfiguration
	// instead of vanishing.
	Unconfigured []Family `json:"unconfigured,omitempty"`
}

// Mergeable reports whether the gate permits a merge.
func (v Verdict) Mergeable() bool { return v.Decision == DecisionPass }

// Validate reports why the configuration could never be satisfied. A quorum
// above the family count, or an empty family set, is a permanently red gate
// with no explanation attached, so it is rejected up front as an error the
// operator can act on.
func (c Config) Validate() error {
	if len(c.Families) == 0 {
		return fmt.Errorf("agentic review: no reviewer families configured")
	}
	seen := make(map[Family]bool, len(c.Families))
	for _, f := range c.Families {
		if strings.TrimSpace(string(f)) == "" {
			return fmt.Errorf("agentic review: blank reviewer family in configuration")
		}
		if seen[f] {
			return fmt.Errorf("agentic review: reviewer family %q configured twice", f)
		}
		seen[f] = true
	}
	if c.Quorum < 1 {
		return fmt.Errorf("agentic review: quorum %d must be at least 1", c.Quorum)
	}
	if c.Quorum > len(c.Families) {
		return fmt.Errorf("agentic review: quorum %d exceeds the %d configured families, so it can never be met",
			c.Quorum, len(c.Families))
	}
	if c.VerdictTimeout <= 0 {
		return fmt.Errorf("agentic review: verdict timeout must be positive")
	}
	if c.DispatchTimeout <= 0 {
		return fmt.Errorf("agentic review: dispatch timeout must be positive")
	}
	return nil
}

// Evaluate resolves each configured family and applies the quorum rule.
//
// The order of the top-level checks is the policy, and it is deliberate:
//
//  1. A family that requested changes blocks outright. Quorum is an allowance
//     for reviewers that could not speak, never a vote to overrule one that
//     did.
//  2. A family that is still missing or pending blocks. This is the window the
//     gate exists to close — a pull request must not merge between going ready
//     and its reviews resolving.
//  3. Otherwise every family has either approved or is established as down,
//     and the merge turns on whether the approvals reach the quorum.
func (c Config) Evaluate(in Input) (Verdict, error) {
	if err := c.Validate(); err != nil {
		return Verdict{}, err
	}

	present := make(map[string]bool, len(in.Labels))
	configured := make(map[Family]bool, len(c.Families))
	for _, f := range c.Families {
		configured[f] = true
	}
	strays := make(map[Family]bool)
	for _, name := range in.Labels {
		present[name] = true
		if f, _, ok := ParseLabel(name); ok && !configured[f] {
			strays[f] = true
		}
	}

	v := Verdict{
		Quorum:       c.Quorum,
		Families:     make([]FamilyVerdict, 0, len(c.Families)),
		Unconfigured: sortedFamilies(strays),
	}
	for _, f := range c.Families {
		v.Families = append(v.Families, c.resolve(f, present, in))
	}

	var blocking, unresolved []string
	for _, fv := range v.Families {
		switch fv.State {
		case StateApproved:
			v.Approved++
		case StateDown:
			v.Down++
		case StateBlocking:
			blocking = append(blocking, string(fv.Family))
		case StateMissing, StatePending:
			unresolved = append(unresolved, string(fv.Family)+" "+strings.ToLower(string(fv.State)))
		}
	}

	switch {
	case len(blocking) > 0:
		v.Decision = DecisionBlock
		v.Reason = "reviewer families requested changes: " + strings.Join(blocking, ", ")
	case len(unresolved) > 0:
		v.Decision = DecisionPending
		v.Reason = "review lifecycle unresolved: " + strings.Join(unresolved, ", ")
	case v.Approved >= c.Quorum:
		v.Decision = DecisionPass
		v.Reason = fmt.Sprintf("%d of %d reviewer families approved (quorum %d, %d down)",
			v.Approved, len(c.Families), c.Quorum, v.Down)
	default:
		v.Decision = DecisionBlock
		v.Reason = fmt.Sprintf("only %d of %d reviewer families approved; quorum %d not met with %d down",
			v.Approved, len(c.Families), c.Quorum, v.Down)
	}
	return v, nil
}

// resolve determines a single family's state from its labels and the clock.
func (c Config) resolve(f Family, present map[string]bool, in Input) FamilyVerdict {
	has := func(p Phase) bool { return present[Label(f, p)] }

	switch {
	case has(PhaseChangesRequested):
		return FamilyVerdict{f, StateBlocking, "requested changes"}
	case has(PhaseApproved):
		return FamilyVerdict{f, StateApproved, "approved the reviewed head"}
	case has(PhaseError):
		return FamilyVerdict{f, StateDown, "published an error state and cannot reach a verdict"}
	case has(PhaseStarted) || has(PhasePosted):
		return c.resolveInFlight(f, present, in)
	}

	// Nothing published at all. Inside the dispatch window that is a review
	// still on its way; past it, the family never came up.
	if !in.ReadyAt.IsZero() && in.Now.Sub(in.ReadyAt) > c.DispatchTimeout {
		return FamilyVerdict{f, StateDown, fmt.Sprintf(
			"published no started label within %s of the pull request going ready", c.DispatchTimeout)}
	}
	return FamilyVerdict{f, StateMissing, "has not started reviewing this head"}
}

// resolveInFlight ages a family that started but has not published a verdict.
// The deadline runs from the latest lifecycle evidence, so a family that got
// its review body onto the pull request late still gets a full window to turn
// that body into a verdict.
func (c Config) resolveInFlight(f Family, present map[string]bool, in Input) FamilyVerdict {
	var latest time.Time
	for _, p := range []Phase{PhaseStarted, PhasePosted} {
		if !present[Label(f, p)] {
			continue
		}
		if at, ok := in.AppliedAt[Label(f, p)]; ok && at.After(latest) {
			latest = at
		}
	}
	if latest.IsZero() {
		// No timestamp to age against. Staying pending is the fail-closed
		// answer: an unknown start time must not become a free pass.
		return FamilyVerdict{f, StatePending, "started but its start time is unknown, so it cannot be aged out"}
	}
	if in.Now.Sub(latest) > c.VerdictTimeout {
		return FamilyVerdict{f, StateDown, fmt.Sprintf(
			"started but published no verdict within %s", c.VerdictTimeout)}
	}
	return FamilyVerdict{f, StatePending, "started and may still publish a verdict"}
}

func sortedFamilies(set map[Family]bool) []Family {
	if len(set) == 0 {
		return nil
	}
	out := make([]Family, 0, len(set))
	for f := range set {
		out = append(out, f)
	}
	slices.Sort(out)
	return out
}
