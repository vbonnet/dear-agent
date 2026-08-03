// Package mergeloop is the pure, testable core of the Ralph Wiggum persistent
// PR-merge loop (ADR-029). It classifies open pull requests into a state
// machine and drives each toward MERGED — rebasing behind branches, spawning
// agents to fix CI failures and resolve conflicts, and delegating the
// irreversible squash-merge to the vetted safe-merge wrapper.
//
// The package contains no I/O: callers inject a [PRLister], [Rebaser],
// [Merger] and [AgentSpawner] (see [Deps]). The cmd/mergeloop binary supplies
// the real GitHub/AGM-backed implementations.
package mergeloop

import "time"

// CheckVerdict is the normalized outcome of a single status check.
type CheckVerdict int

const (
	// CheckPending means the check has not finished (queued/in-progress).
	CheckPending CheckVerdict = iota
	// CheckPass means the check finished successfully (or neutral/skipped).
	CheckPass
	// CheckFail means the check finished unsuccessfully.
	CheckFail
)

// Check is a normalized status check on a PR head commit. The adapter that
// builds it is responsible for setting Required from safegit's shared effective
// required-check projection; the classifier only ever considers required
// checks, so an advisory red never triggers an agent spawn or blocks the loop.
type Check struct {
	Name     string
	Verdict  CheckVerdict
	Required bool
}

// PR is the minimal normalized view of a pull request the classifier needs.
// It is deliberately decoupled from the gh JSON schema so the state machine
// can be unit-tested without GitHub.
type PR struct {
	Number      int
	Title       string
	HeadRefName string
	HeadSHA     string
	IsDraft     bool

	// Mergeable is GitHub's mergeability: "MERGEABLE", "CONFLICTING", "UNKNOWN".
	Mergeable string
	// MergeStateStatus is GitHub's computed merge state: "BEHIND", "BLOCKED",
	// "CLEAN", "DIRTY", "UNSTABLE", "UNKNOWN", "HAS_HOOKS", "DRAFT".
	MergeStateStatus string
	// ReviewDecision is "APPROVED", "CHANGES_REQUESTED", "REVIEW_REQUIRED", "".
	ReviewDecision string

	Labels       []string
	Checks       []Check
	ChangedFiles []string // best-effort; empty if not fetched

	UpdatedAt time.Time
}

// hasLabel reports whether the PR carries the given label (case-insensitive).
func (pr PR) hasLabel(want string) bool {
	for _, l := range pr.Labels {
		if equalFold(l, want) {
			return true
		}
	}
	return false
}

// requiredChecks returns the subset of checks that are merge-blocking.
func (pr PR) requiredChecks() []Check {
	out := make([]Check, 0, len(pr.Checks))
	for _, c := range pr.Checks {
		if c.Required {
			out = append(out, c)
		}
	}
	return out
}

// requiredVerdict collapses the required checks into a single verdict:
// CheckFail if any required check failed, else CheckPending if any is still
// running, else CheckPass. With no required checks it returns CheckPass
// (nothing is blocking) — merge authority still rests with safe-merge.
func (pr PR) requiredVerdict() CheckVerdict {
	anyPending := false
	for _, c := range pr.requiredChecks() {
		switch c.Verdict {
		case CheckFail:
			return CheckFail
		case CheckPending:
			anyPending = true
		case CheckPass:
			// counts toward neither fail nor pending
		}
	}
	if anyPending {
		return CheckPending
	}
	return CheckPass
}

// equalFold is a tiny ASCII case-insensitive compare to avoid importing
// strings into this otherwise dependency-free file's hot path. It is correct
// for label and state tokens, which are ASCII.
func equalFold(a, b string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := 0; i < len(a); i++ {
		ca, cb := a[i], b[i]
		if 'A' <= ca && ca <= 'Z' {
			ca += 'a' - 'A'
		}
		if 'A' <= cb && cb <= 'Z' {
			cb += 'a' - 'A'
		}
		if ca != cb {
			return false
		}
	}
	return true
}
