package audit

import (
	"context"
	"fmt"
)

// Strategy names how a Finding's Remediation is proposed to be applied. The
// default for a finding without an explicit Remediation is determined by the
// severity-policy in .dear-agent.yml > audits.severity-policy; the runner
// consults the policy when Strategy is the zero value StrategyUnspecified.
type Strategy string

// Remediation strategies.
const (
	StrategyUnspecified Strategy = ""      // fall through to severity-policy default
	StrategyAuto        Strategy = "auto"  // runner may invoke its configured Remediator
	StrategyPR          Strategy = "pr"    // proposal for an external PR-producing workflow
	StrategyIssue       Strategy = "issue" // proposal for an external issue-producing workflow
	StrategyNoop        Strategy = "noop"  // record the proposal without applying it
)

// IsValid reports whether s names a known strategy. The zero value
// (StrategyUnspecified) is considered valid because it is the
// canonical "use the policy default" sentinel.
func (s Strategy) IsValid() bool {
	switch s {
	case StrategyUnspecified, StrategyAuto, StrategyPR, StrategyIssue, StrategyNoop:
		return true
	}
	return false
}

// Remediation describes a proposed way to fix a Finding. The shape is
// small on purpose: a check's job is to find, the operator's job is to
// fix, and the substrate's job is to record. Concrete fields:
//
//   - Strategy: how to apply (see Strategy enum).
//   - Command: proposed shell command for StrategyAuto. The runner
//     passes it to its configured Remediator; the built-in remediator
//     does not execute it.
//   - Patch: proposed unified diff for StrategyPR. The audit package
//     records it but does not apply it or open a PR.
//   - Title / Body: PR title/body or issue title/body for StrategyPR
//     / StrategyIssue. Required for those strategies.
//
// The runner validates the strategy ↔ field combination before invoking
// a Remediator for StrategyAuto. StrategyPR and StrategyIssue are not
// dispatched by this package.
type Remediation struct {
	Strategy Strategy
	Command  string
	Patch    string
	Title    string
	Body     string
}

// IsZero reports whether r is the empty value. Cheaper than reflect
// for a fast-path check in the runner.
func (r Remediation) IsZero() bool {
	return r.Strategy == StrategyUnspecified &&
		r.Command == "" &&
		r.Patch == "" &&
		r.Title == "" &&
		r.Body == ""
}

// Validate returns a non-nil error when the strategy ↔ field
// combination is incoherent (e.g. StrategyAuto with no Command).
// Called by the runner before dispatching to a Remediator.
func (r Remediation) Validate() error {
	if !r.Strategy.IsValid() {
		return fmt.Errorf("audit: Remediation.Strategy %q invalid", r.Strategy)
	}
	switch r.Strategy {
	case StrategyUnspecified, StrategyNoop:
		// No required fields; both are valid empty-effect strategies.
	case StrategyAuto:
		if r.Command == "" {
			return fmt.Errorf("audit: Remediation.Strategy=auto requires Command")
		}
	case StrategyPR:
		if r.Patch == "" {
			return fmt.Errorf("audit: Remediation.Strategy=pr requires Patch")
		}
		if r.Title == "" {
			return fmt.Errorf("audit: Remediation.Strategy=pr requires Title")
		}
	case StrategyIssue:
		if r.Title == "" {
			return fmt.Errorf("audit: Remediation.Strategy=issue requires Title")
		}
	}
	return nil
}

// Remediator is a dormant compatibility seam for applying a Finding's
// suggested remediation. The runner invokes it only for StrategyAuto, and
// the sole production implementation is the side-effect-free no-op.
//
// The current runner cannot treat Apply's result as durable evidence: it
// ignores ApplyOutcome.Status and ApplyOutcome.Reference, and passes Note to
// the store only when State is valid and differs from the stored state.
//
// Deprecated: Do not add or configure side-effecting implementations. This
// interface remains exported for compatibility until an idempotent remediation
// event, persistence, and legacy-migration contract replaces it.
type Remediator interface {
	Apply(ctx context.Context, finding Finding, env Env) (ApplyOutcome, error)
}

// ApplyOutcome is the legacy result of one Remediator.Apply call. Status and
// Reference are ignored by Runner. State is considered only when valid and
// different from the stored finding state; Note is passed to the store only
// together with such a state change. An ApplyOutcome is therefore not durable
// evidence that remediation happened.
//
// Deprecated: Retained with Remediator for source compatibility pending an
// idempotent remediation event, persistence, and legacy-migration contract.
type ApplyOutcome struct {
	Status string       // compatibility-only label; Runner ignores it
	State  FindingState // candidate lifecycle state for Runner to validate
	Note   string       // passed to Store only with a valid state change
	// Reference is compatibility-only artifact metadata. Runner ignores it.
	Reference string
}

// noopRemediator is the runner's default and performs no side effects.
type noopRemediator struct{}

// NewNoopRemediator returns a Remediator that performs no side effects. Its
// unchanged FindingOpen outcome causes Runner to discard all descriptive
// fields, so the outcome is not evidence that remediation occurred.
func NewNoopRemediator() Remediator { return noopRemediator{} }

func (noopRemediator) Apply(_ context.Context, _ Finding, _ Env) (ApplyOutcome, error) {
	return ApplyOutcome{Status: "no-op", State: FindingOpen, Note: "noop remediator"}, nil
}
