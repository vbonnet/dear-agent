package audit

import "fmt"

// Strategy is a closed hint for how an external system might handle a
// Finding's Remediation. It does not prove that the suggestion is applicable
// or authorize a side effect. The default for a finding without an explicit
// strategy is determined by the severity-policy in .dear-agent.yml >
// audits.severity-policy; the runner consults the policy when Strategy is the
// zero value StrategyUnspecified.
type Strategy string

// Remediation strategies.
const (
	StrategyUnspecified Strategy = ""      // fall through to severity-policy default
	StrategyAuto        Strategy = "auto"  // external automation handling hint
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

// Remediation describes an inert handling suggestion for a Finding. The shape
// is small on purpose: a check's job is to find, the operator's job is to fix,
// and the substrate's job is to record. Concrete fields:
//
//   - Strategy: a handling hint (see Strategy enum).
//   - Command: optional shell-command context for StrategyAuto.
//   - Patch: optional unified-diff context for StrategyPR.
//   - Title / Body: optional operator context for StrategyPR / StrategyIssue.
//
// Payload fields are optional: for example, a patchless StrategyPR can
// recommend investigation or PR-producing work. No strategy is dispatched by
// this package. Applicability, authority, revision binding, idempotency, and
// reconciliation belong in a separately chartered durable consumer.
type Remediation struct {
	Strategy Strategy
	Command  string
	Patch    string
	Title    string
	Body     string
}

// IsZero reports whether r contains neither a handling hint nor operator
// context.
func (r Remediation) IsZero() bool {
	return r.Strategy == StrategyUnspecified &&
		r.Command == "" &&
		r.Patch == "" &&
		r.Title == "" &&
		r.Body == ""
}

// Validate returns a non-nil error when Strategy is not part of the closed
// vocabulary. Payload fields are optional and do not prove applicability.
func (r Remediation) Validate() error {
	if !r.Strategy.IsValid() {
		return fmt.Errorf("audit: Remediation.Strategy %q invalid", r.Strategy)
	}
	return nil
}
