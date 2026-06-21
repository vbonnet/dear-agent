// Package prchecks detects open pull requests whose head commit carries zero
// required CI check-runs — the push-then-PR-open race that stranded PRs
// #579/#581/#582 for 8+ hours with no CI (bead ce-np2s).
//
// # Why this matters
//
// A PR whose head SHA has no check-runs never turns green and never turns red.
// Squash auto-merge (armed by safe-pr on create) waits forever for required
// checks that will never report, and the safe-merge babysit loop skips the PR
// as "pending" on every pass — so it ages indefinitely with no signal that
// anything is wrong. The fix is to detect the condition and re-trigger CI.
//
// This package holds only the pure predicate so it is exhaustively unit
// testable with no GitHub access; cmd/scan-no-checks wires it to the gh CLI.
package prchecks

// PR is the minimal view of an open pull request the detector needs.
type PR struct {
	Number      int
	Title       string
	HeadRefName string
	HeadSHA     string
	IsDraft     bool
}

// CheckRun is one entry of a commit's check_runs array. Only the name is
// needed to match a run against the branch-protection required set.
type CheckRun struct {
	Name string
}

// NeedsRetrigger reports whether pr's head SHA is missing CI entirely and
// should have its checks re-triggered.
//
// required is the set of branch-protection required check names. When it is
// non-empty, a PR is stuck only when NONE of the required checks has a run on
// the head SHA: a partial set (some required runs present, others not yet)
// means CI did fire and is merely still in progress, so the PR is left alone.
// When required is empty — the protection API was unreadable, or the branch is
// unprotected — the predicate falls back to "any check-run at all": zero runs
// means stuck. This fallback is conservative in the safe direction; it can
// only ever flag a PR that genuinely has no checks.
//
// Draft PRs are never flagged: CI legitimately may not run on a draft, so a
// re-trigger would be noise.
func NeedsRetrigger(pr PR, runs []CheckRun, required map[string]bool) bool {
	if pr.IsDraft {
		return false
	}
	if len(required) == 0 {
		return len(runs) == 0
	}
	for _, r := range runs {
		if required[r.Name] {
			// At least one required check has a run on the head SHA, so CI
			// fired for this PR. Whether it is pending, green, or red is the
			// merge loop's problem, not ours.
			return false
		}
	}
	return true
}
