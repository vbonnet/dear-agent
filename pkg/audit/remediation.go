package audit

import (
	"context"
	"fmt"
)

// Strategy names how a Finding's Remediation should be applied. See
// ADR-011 §D4. The default for a finding without an explicit
// Remediation is determined by the severity-policy in
// .dear-agent.yml > audits.severity-policy; the runner consults the
// policy when Strategy is the zero value StrategyUnspecified.
type Strategy string

// Remediation strategies.
const (
	StrategyUnspecified Strategy = ""        // fall through to severity-policy default
	StrategyAuto        Strategy = "auto"    // runner executes Command directly
	StrategyPR          Strategy = "pr"      // runner produces a patch + opens a draft PR
	StrategyIssue       Strategy = "issue"   // runner files a tracked issue
	StrategyNoop        Strategy = "noop"    // runner records and stops
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

// Remediation describes how to fix a Finding. The shape is small on
// purpose: a check's job is to find, the operator's job is to fix,
// and the substrate's job is to record. Concrete fields:
//
//   - Strategy: how to apply (see Strategy enum).
//   - Command: shell command for StrategyAuto. The runner runs it
//     in the repo working directory with the same Env as the check.
//   - Patch: unified diff for StrategyPR. The runner applies it to
//     a fresh branch and opens a PR.
//   - Title / Body: PR title/body or issue title/body for StrategyPR
//     / StrategyIssue. Required for those strategies.
//
// The runner validates the strategy ↔ field combination before
// dispatch. A Remediation with no Command and StrategyAuto is a
// runner error, not a check error — the check did its job.
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

// Remediator applies a Remediation. Implementations are pluggable —
// the runner has a built-in no-op remediator that records the
// remediation in the audit_runs row but does not execute. Real
// implementations (shell auto-fix, GitHub PR opener, Beads issue
// filer) live in subpackages so the core has no external deps.
//
// Apply MUST be idempotent: the runner may invoke it more than once
// for the same finding across reruns (e.g. partial-failure resume).
// Apply returns ApplyOutcome describing what the remediator did so
// the runner can record it without re-deriving from side effects.
type Remediator interface {
	Apply(ctx context.Context, finding Finding, env Env) (ApplyOutcome, error)
}

// ApplyOutcome is the structured result of one Remediator.Apply call.
// At minimum the runner records Status; State is the post-apply
// FindingState the runner should write back (typically
// FindingResolved on success, FindingAcknowledged on partial). Note
// is a free-form human-readable summary persisted on the finding row.
type ApplyOutcome struct {
	Status string       // "applied" | "deferred" | "no-op" | "failed"
	State  FindingState // post-apply lifecycle state to write
	Note   string
	// Reference is an optional URL / id of the artifact the remediator
	// produced (PR url, issue id, commit sha). Persisted into the
	// finding's Evidence map under key "remediation_ref".
	Reference string
}

// noopRemediator is the runner's default. It records the proposed
// remediation but does nothing. Useful both as the safe default and
// in tests where we want to assert what the runner *would* have done.
type noopRemediator struct{}

// NewNoopRemediator returns a Remediator that performs no side
// effects. Exported for tests and for operators who want to dry-run
// the audit pipeline.
func NewNoopRemediator() Remediator { return noopRemediator{} }

func (noopRemediator) Apply(_ context.Context, _ Finding, _ Env) (ApplyOutcome, error) {
	return ApplyOutcome{Status: "no-op", State: FindingOpen, Note: "noop remediator"}, nil
}
