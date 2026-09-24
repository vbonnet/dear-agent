// Package nochecks detects open pull requests whose head commit carries zero
// required CI check-runs — the push-then-PR-open race that stranded PRs
// #579/#581/#582 for 8+ hours with no CI (bead ce-np2s).
//
// # Why this matters
//
// safe-pr can create a non-draft PR whose head SHA has no check-runs when a
// push-then-PR-open race drops the CI trigger. The required checks then never
// report, and the safe-merge babysit loop skips the PR as "pending" on every
// pass — there is no red, no green, and no signal. The PR ages indefinitely
// with nothing flagging it. Drafts remain outside this automated recovery
// path: they are advanced by their owner, which is a human only for the
// autonomous-merge carve-out categories.
//
// This package holds the pure detection logic so it is exhaustively unit
// testable with no GitHub access; sources.go wires it to the gh CLI and the
// `agm pr scan-no-checks` command drives it.
//
// It is the inverse-direction sibling of internal/orphanpr: orphanpr flags a PR
// whose tracking bead closed without the PR merging; nochecks flags a PR that
// can never merge because CI never ran. Both surface a stuck-open PR a
// supervisor would otherwise miss.
package nochecks

import (
	"fmt"
	"sort"
	"strings"
)

// PR is the minimal view of an open pull request the detector needs.
type PR struct {
	Number      int    `json:"number"`
	Title       string `json:"title"`
	BaseRefName string `json:"base_ref_name"`
	HeadRefName string `json:"head_ref_name"`
	HeadSHA     string `json:"head_sha"`
	IsDraft     bool   `json:"is_draft"`
}

// CheckRun is one entry of a commit's check_runs array. Only the name is needed
// to match a run against the branch-protection required set.
type CheckRun struct {
	Name string `json:"name"`
}

// StuckPR is an open PR whose head SHA is missing CI and needs a re-trigger.
type StuckPR struct {
	Number      int    `json:"number"`
	Title       string `json:"title"`
	BaseRefName string `json:"base_ref_name"`
	HeadRefName string `json:"head_ref_name"`
	HeadSHA     string `json:"head_sha"`

	// requiredChecks is the complete policy that classified this candidate.
	// It is intentionally unexported so only Scan can produce a triggerable
	// StuckPR; RetriggerCI uses it to prove the candidate has not self-healed.
	requiredChecks map[string]bool
}

// NeedsRetrigger reports whether pr's head SHA is missing CI entirely and should
// have its checks re-triggered.
//
// required is the set of branch-protection required check names. When it is
// non-empty, a PR is stuck only when NONE of the required checks has a run on
// the head SHA: a partial set (some required runs present, others not yet) means
// CI did fire and is merely still in progress, so the PR is left alone. When
// required is empty because the complete effective policy is authoritatively
// known to require no checks, the predicate falls back to "any check-run at
// all": zero runs means stuck. Incomplete policy never reaches this predicate.
// This fallback is conservative in the safe direction; it can only ever flag a
// PR that genuinely has no checks.
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
			// At least one required check has a run on the head SHA, so CI fired
			// for this PR. Whether it is pending, green, or red is the merge
			// loop's problem, not ours.
			return false
		}
	}
	return true
}

// CheckRunsFunc fetches the check-run names reported against a head SHA. It is
// injected so Scan is testable without the gh CLI. A non-nil error means the
// read failed; Scan treats that PR as indeterminate and never flags it, so a
// transient GitHub hiccup can never make a live PR look stuck.
type CheckRunsFunc func(headSHA string) ([]CheckRun, error)

// Scan flags every open, non-draft PR whose head SHA is missing the CI required
// by that PR's provider-observed base (per NeedsRetrigger). Before reading any
// check-runs it validates that required is initialized and contains a complete,
// non-nil policy for every candidate base. This keeps an incomplete policy
// snapshot from producing a partial classification or re-trigger set.
//
// It calls runsOf once per non-draft PR. PRs whose check-runs can't be read are
// skipped, never flagged, and their errors are returned keyed by PR number so
// the caller can surface them without aborting the whole scan.
//
// The returned stuck list is sorted by PR number for stable output.
func Scan(
	prs []PR,
	required RequiredChecksByBase,
	runsOf CheckRunsFunc,
) (stuck []StuckPR, readErrs map[int]error, err error) {
	if required.byBase == nil {
		return nil, nil, fmt.Errorf("required-check policy snapshot is uninitialized")
	}
	bases := make([]string, 0, len(required.byBase))
	for base := range required.byBase {
		bases = append(bases, base)
	}
	sort.Strings(bases)
	for _, base := range bases {
		policy := required.byBase[base]
		if policy == nil {
			return nil, nil, fmt.Errorf("required-check policy for base %q is incomplete", base)
		}
	}
	for _, pr := range prs {
		if pr.IsDraft {
			continue
		}
		if strings.TrimSpace(pr.BaseRefName) == "" {
			return nil, nil, fmt.Errorf("PR #%d has no provider-observed base branch", pr.Number)
		}
		policy, ok := required.byBase[pr.BaseRefName]
		if !ok || policy == nil {
			return nil, nil, fmt.Errorf("required-check policy for PR #%d base %q is incomplete", pr.Number, pr.BaseRefName)
		}
	}

	for _, pr := range prs {
		if pr.IsDraft {
			continue
		}
		runs, err := runsOf(pr.HeadSHA)
		if err != nil {
			if readErrs == nil {
				readErrs = make(map[int]error)
			}
			readErrs[pr.Number] = err
			continue
		}
		if !NeedsRetrigger(pr, runs, required.byBase[pr.BaseRefName]) {
			continue
		}
		stuck = append(stuck, StuckPR{
			Number:      pr.Number,
			Title:       pr.Title,
			BaseRefName: pr.BaseRefName,
			HeadRefName: pr.HeadRefName,
			HeadSHA:     pr.HeadSHA,
			requiredChecks: cloneRequiredChecks(
				required.byBase[pr.BaseRefName],
			),
		})
	}
	sort.Slice(stuck, func(i, j int) bool { return stuck[i].Number < stuck[j].Number })
	return stuck, readErrs, nil
}
