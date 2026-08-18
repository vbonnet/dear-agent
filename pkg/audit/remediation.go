package audit

import (
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
	StrategyAuto        Strategy = "auto"  // suggestion is eligible for external automation
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
//   - Command: proposed shell command for StrategyAuto. The audit package
//     records it but never executes it.
//   - Patch: proposed unified diff for StrategyPR. The audit package
//     records it but does not apply it or open a PR.
//   - Title / Body: PR title/body or issue title/body for StrategyPR
//     / StrategyIssue. Required for those strategies.
//
// No strategy is dispatched by this package. Side-effecting consumers belong
// in a separately chartered durable module.
type Remediation struct {
	Strategy Strategy
	Command  string
	Patch    string
	Title    string
	Body     string
}

// IsZero reports whether r is the empty value.
func (r Remediation) IsZero() bool {
	return r.Strategy == StrategyUnspecified &&
		r.Command == "" &&
		r.Patch == "" &&
		r.Title == "" &&
		r.Body == ""
}

// Validate returns a non-nil error when the strategy ↔ field
// combination is incoherent (e.g. StrategyAuto with no Command).
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
