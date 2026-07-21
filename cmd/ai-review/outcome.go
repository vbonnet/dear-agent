// Command ai-review runs the REVIEW.md five-dimension AI review as a
// fail-closed CI merge gate: the process exit code carries the review outcome.
package main

import "strings"

// Outcome is the four-state review result from REVIEW.md §1. The zero value is
// deliberately NeedsHumanReview so that an uninitialized or unparseable result
// fails closed rather than silently approving.
type Outcome int

const (
	// NeedsHumanReview is the fail-closed default: a novel or unresolved
	// decision that the automated protocol must escalate to a human. It is the
	// zero value so that any code path that forgets to set an outcome blocks
	// the merge instead of allowing it.
	NeedsHumanReview Outcome = iota
	// Approved means no blocking findings; the PR is ready to merge.
	Approved
	// NeedsWork means fixable blocking findings exist.
	NeedsWork
	// Rejected means a fundamental design problem the current approach cannot
	// resolve.
	Rejected
)

// String returns the canonical REVIEW.md outcome word.
func (o Outcome) String() string {
	switch o {
	case Approved:
		return "approved"
	case NeedsWork:
		return "needs-work"
	case Rejected:
		return "rejected"
	case NeedsHumanReview:
		return "needs-human-review"
	default:
		return "needs-human-review"
	}
}

// ParseOutcome maps a synthesis model's first line to an Outcome. It is
// deliberately conservative: anything it cannot confidently classify as
// approved becomes NeedsHumanReview, so unparseable or hedged output fails
// closed (SPEC R7). Order matters — the more-severe words are checked first so
// that a synthesis mentioning several states resolves *down* (REVIEW.md §1:
// "ambiguous findings always resolve down").
func ParseOutcome(s string) Outcome {
	line := strings.ToLower(strings.TrimSpace(s))
	switch {
	case strings.Contains(line, "needs-human") || strings.Contains(line, "human-review") || strings.Contains(line, "human review"):
		return NeedsHumanReview
	case strings.Contains(line, "rejected") || strings.Contains(line, "reject"):
		return Rejected
	case strings.Contains(line, "needs-work") || strings.Contains(line, "needs work"):
		return NeedsWork
	case strings.Contains(line, "approved") || strings.Contains(line, "approve"):
		return Approved
	default:
		// Unparseable — fail closed.
		return NeedsHumanReview
	}
}

// ExitFor is the enforcement contract: it maps an outcome (and whether a human
// override is active) to a process exit code. This is the single function that
// decides whether the merge gate blocks. It is pure and table-tested.
//
// Only an Approved outcome, or an explicit human override label, yields 0.
// Every other outcome — needs-work, rejected, needs-human-review — blocks the
// merge (SPEC R1–R3). There is intentionally no path from a non-approved
// outcome to 0 except the audited override.
func ExitFor(o Outcome, override bool) int {
	if o == Approved {
		return 0
	}
	if override {
		// A human has consciously accepted the findings; the override is
		// recorded on the PR as a label (auditable) and requires a human
		// action (verified). This is the "verified human fallback".
		return 0
	}
	return 1
}
