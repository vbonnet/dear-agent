// Package cihealth classifies how a failure on main got past pre-merge, and
// prices the fix using the repo's prevention-vs-cure ROI formula.
//
// The classification and the arithmetic are pure functions over plain structs.
// Fetching the facts from GitHub lives in the command, so every judgement this
// package makes is reachable from a table test.
package cihealth

import (
	"fmt"
	"math"
	"slices"
	"sort"
	"strings"
)

// Conclusion values GitHub reports for a check run.
const (
	ConclusionSuccess   = "success"
	ConclusionFailure   = "failure"
	ConclusionSkipped   = "skipped"
	ConclusionCancelled = "cancelled"
	ConclusionNeutral   = "neutral"
)

// CheckRun is one check context as it reported on a pull request head.
type CheckRun struct {
	Name       string
	Conclusion string
}

// Escape is everything known about one red check on main and the pull request
// that introduced the commit.
type Escape struct {
	// FailingCheck is the check context that is red on main, e.g.
	// "Build & Test (ubuntu-latest)".
	FailingCheck string
	// MainSHA is the commit on main where the failure was observed.
	MainSHA string
	// PRNumber is the pull request that introduced MainSHA. Zero means no pull
	// request was found — a direct push or an admin bypass.
	PRNumber int
	// PRChecks is every check that reported on the pull request head.
	PRChecks []CheckRun
	// RequiredContexts is the repository ruleset's required status checks.
	RequiredContexts []string
	// PreMergeCapable is false when the producing workflow has no
	// pull_request trigger at all. Such a workflow never had a pre-merge
	// opportunity to miss, so "how did this get past pre-merge" has a trivial
	// answer and filter refinement is not the lever.
	PreMergeCapable bool
	// DiffScoped marks a check whose pre-merge run is deliberately narrower
	// than its post-merge run (diff-scoped lint, manifest-scoped vuln scan).
	// Such a check passing pre-merge and failing post-merge is the design
	// working, not a hole in it.
	DiffScoped bool
}

// Class is why a failure reached main.
type Class string

const (
	// ClassBypassed — no pull request introduced the commit.
	ClassBypassed Class = "bypassed"
	// ClassNeverRan — the check never reported on the pull request at all.
	// Usually a workflow-level path filter, which reports nothing.
	ClassNeverRan Class = "never-ran"
	// ClassSelectionGap — the check was skipped on the pull request by a
	// job-level condition, but the change did affect it. The path filter is
	// wrong and can be refined.
	ClassSelectionGap Class = "selection-gap"
	// ClassGatingGap — the check ran and failed on the pull request, and the
	// merge happened anyway because it is not a required context.
	ClassGatingGap Class = "gating-gap"
	// ClassScopeGap — the check passed pre-merge and failed post-merge because
	// the post-merge run is wider on purpose. Working as designed; the open
	// question is only whether the wider scope is worth paying pre-merge.
	ClassScopeGap Class = "scope-gap"
	// ClassPostMergeOnly — the producing workflow has no pull_request trigger.
	// This is a scheduled or post-merge detector doing exactly its job; it is
	// not an escape, and no path filter would have changed the outcome.
	ClassPostMergeOnly Class = "post-merge-only"
	// ClassMergeSkew — the check passed on the pull request at the same scope
	// and fails on main. Either non-determinism, or two pull requests that
	// each passed alone and conflict once merged.
	ClassMergeSkew Class = "merge-skew"
)

// Finding is the classification plus the prose the retro needs.
type Finding struct {
	Class            Class
	Summary          string
	FilterRefinable  bool
	SuggestedActions []string
}

// Classify decides how the failure got through pre-merge.
func Classify(e Escape) Finding {
	// Checked before everything else. A schedule-only workflow reports on no
	// pull request by construction, so without this it lands in ClassNeverRan
	// and the retro tells the reader to go widen a path filter that was never
	// involved — advice that costs time and fixes nothing.
	if !e.PreMergeCapable {
		return Finding{
			Class:   ClassPostMergeOnly,
			Summary: fmt.Sprintf("%q is produced by a workflow with no pull_request trigger. It never ran pre-merge because it is not a pre-merge check — this is a scheduled or post-merge detector reporting a real finding, not a failure that escaped a gate.", e.FailingCheck),
			SuggestedActions: []string{
				"Fix the finding itself. There is no selection or gating bug to chase here.",
				"Only ask whether this belongs pre-merge if the finding is one a pull request could have introduced; drift, dependency, and infrastructure detectors usually cannot be.",
			},
		}
	}

	if e.PRNumber == 0 {
		return Finding{
			Class:   ClassBypassed,
			Summary: fmt.Sprintf("No pull request introduced %s — a direct push or an administrative bypass. Pre-merge selection never had a chance to run.", shortSHA(e.MainSHA)),
			SuggestedActions: []string{
				"Confirm against bypassed-merge-audit.yml whether this was an authorised bypass.",
				"If it was not, the gap is in branch protection, not in CI selection.",
			},
		}
	}

	run, reported := findCheck(e.PRChecks, e.FailingCheck)

	switch {
	case !reported:
		return Finding{
			Class:           ClassNeverRan,
			Summary:         fmt.Sprintf("%q never reported on PR #%d. A workflow dropped by a workflow-level path filter creates no check run at all, so nothing was there to be red.", e.FailingCheck, e.PRNumber),
			FilterRefinable: true,
			SuggestedActions: []string{
				"Move the filter from workflow-level `on.<event>.paths` to a job-level `if:` fed by .github/workflows/changed-paths.yml (ADR-038).",
				"A job skipped by `if:` reports `skipped`, which is visible and auditable; a skipped workflow reports nothing.",
			},
		}

	case run.Conclusion == ConclusionSkipped:
		return Finding{
			Class:           ClassSelectionGap,
			Summary:         fmt.Sprintf("%q was SKIPPED on PR #%d but fails on main. The change was relevant and the path filter said it was not — this is a selection bug, and it is the one class that filter refinement actually fixes.", e.FailingCheck, e.PRNumber),
			FilterRefinable: true,
			SuggestedActions: []string{
				"Find which changed path should have matched and widen the pattern in .github/workflows/changed-paths.yml.",
				"If the input is one that invalidates selection itself, add it to the global-inputs list so it forces every consumer on.",
				"Confirm the CI Gateway skip audit did not catch this; if it did not, the gateway's expectation table is out of step with the job conditions.",
			},
		}

	case run.Conclusion == ConclusionFailure:
		if !isRequired(e.RequiredContexts, e.FailingCheck) {
			return Finding{
				Class:   ClassGatingGap,
				Summary: fmt.Sprintf("%q FAILED on PR #%d and the merge went through anyway, because it is not a required status check. Selection worked; enforcement did not.", e.FailingCheck, e.PRNumber),
				SuggestedActions: []string{
					"This is a gating decision, not a filtering one — refining path filters will not help.",
					"Either promote the context to required in the repository ruleset, or accept it as advisory and stop treating its red main runs as escapes.",
				},
			}
		}
		return Finding{
			Class:   ClassGatingGap,
			Summary: fmt.Sprintf("%q FAILED on PR #%d and is a required context, yet the commit is on main. That points at an administrative bypass or a ruleset change.", e.FailingCheck, e.PRNumber),
			SuggestedActions: []string{
				"Cross-check bypassed-merge-audit.yml and the ruleset history for this window.",
			},
		}

	case e.DiffScoped:
		return Finding{
			Class:   ClassScopeGap,
			Summary: fmt.Sprintf("%q passed on PR #%d and fails on main because its post-merge run is deliberately wider than its pre-merge run. This is the diff-scoping trade working as designed, not a hole.", e.FailingCheck, e.PRNumber),
			SuggestedActions: []string{
				"Do not widen the pre-merge scope reflexively — price it with the ROI calculation below first.",
				"If the ratio clears 10:1, widen the pre-merge scope. If it does not, the correct response is to fix the finding on main and leave the scoping alone.",
			},
		}

	default:
		return Finding{
			Class:   ClassMergeSkew,
			Summary: fmt.Sprintf("%q passed on PR #%d at the same scope it runs at on main, and fails on main. Either the check is non-deterministic, or two pull requests each passed alone and conflict once merged.", e.FailingCheck, e.PRNumber),
			SuggestedActions: []string{
				"Re-run the failing job on the same main SHA. Passing on re-run means flake, and the check should be quarantined rather than filtered.",
				"Passing only after a rebase means semantic merge skew, which path filters cannot fix — a merge queue (`merge_group`) is the mechanism that does.",
			},
		}
	}
}

// ROI prices prevention against cure using this repo's formula:
//
//	ROI = (Cure Cost x Frequency) / Prevention Cost
//
// Thresholds: >10:1 always prevent, >3:1 usually prevent, <3:1 case-by-case.
// Source: the prevention-vs-cure pattern in engram-research.
//
// Costs are engineer-minutes. Both sides are expressed over the same window so
// the ratio is dimensionless.
type ROI struct {
	// CureMinutes is what one escape costs: time main sits red, multiplied by
	// how many people that blocks, plus triage and revert.
	CureMinutes float64
	// Escapes is how many times this check let a failure through in the window.
	Escapes float64
	// PreventionMinutes is what running the check pre-merge costs over the same
	// window: check duration x pull requests it would run on x wait cost.
	PreventionMinutes float64
	// PreventionMeasured distinguishes "this check is genuinely free" from "we
	// have no pre-merge runs to measure". Without the distinction, a workflow
	// that has never run on a pull request divides by zero and comes out as
	// infinitely worth blocking pre-merge — the exact opposite of the truth for
	// a schedule-only detector.
	PreventionMeasured bool
}

// Ratio is (cure x frequency) / prevention. Zero prevention cost is treated as
// infinitely worthwhile, which is the right answer: a free check should always
// run.
func (r ROI) Ratio() float64 {
	if !r.PreventionMeasured {
		return 0
	}
	if r.PreventionMinutes <= 0 {
		if r.CureMinutes*r.Escapes == 0 {
			return 0
		}
		return math.Inf(1)
	}
	return (r.CureMinutes * r.Escapes) / r.PreventionMinutes
}

// Verdict maps a ratio onto the repo's three bands.
func (r ROI) Verdict() string {
	if !r.PreventionMeasured {
		return "INSUFFICIENT DATA — no pre-merge runs to price; do not infer a placement from this"
	}
	switch ratio := r.Ratio(); {
	case ratio > 10:
		return "ALWAYS PREVENT — block this pre-merge (ROI > 10:1)"
	case ratio > 3:
		return "USUALLY PREVENT — lean toward blocking pre-merge (ROI > 3:1)"
	case ratio > 0:
		return "CASE-BY-CASE — post-merge detection is defensible here (ROI < 3:1)"
	default:
		return "NO SIGNAL — no escapes recorded in this window; leave the current placement alone"
	}
}

// Explain renders the arithmetic so the retro shows its work rather than
// asserting a verdict.
func (r ROI) Explain() string {
	var b strings.Builder
	if !r.PreventionMeasured {
		fmt.Fprintf(&b, "ROI = (Cure x Frequency) / Prevention\n")
		fmt.Fprintf(&b, "    = (%.0f min x %.0f escapes) / <unmeasured>\n", r.CureMinutes, r.Escapes)
		fmt.Fprintf(&b, "\nVerdict: %s\n", r.Verdict())
		return b.String()
	}
	fmt.Fprintf(&b, "ROI = (Cure x Frequency) / Prevention\n")
	fmt.Fprintf(&b, "    = (%.0f min x %.0f escapes) / %.0f min\n", r.CureMinutes, r.Escapes, r.PreventionMinutes)
	if ratio := r.Ratio(); math.IsInf(ratio, 1) {
		fmt.Fprintf(&b, "    = unbounded (prevention is free)\n")
	} else {
		fmt.Fprintf(&b, "    = %.1f:1\n", ratio)
	}
	fmt.Fprintf(&b, "\nVerdict: %s\n", r.Verdict())
	return b.String()
}

func findCheck(runs []CheckRun, name string) (CheckRun, bool) {
	for _, run := range runs {
		if run.Name == name {
			return run, true
		}
	}
	return CheckRun{}, false
}

func isRequired(required []string, name string) bool {
	return slices.Contains(required, name)
}

func shortSHA(sha string) string {
	if len(sha) > 7 {
		return sha[:7]
	}
	return sha
}

// SortedContexts returns required contexts in a stable order for rendering.
func SortedContexts(required []string) []string {
	out := append([]string(nil), required...)
	sort.Strings(out)
	return out
}
