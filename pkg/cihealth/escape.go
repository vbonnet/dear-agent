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
	"sort"
	"strings"
)

// Conclusion values GitHub reports for a check run. The unsuccessful set is
// wider than "failure": a check that timed out, was cancelled, or never started
// did not pass, and treating those as passes is how a red main gets classified
// as merge skew.
const (
	ConclusionSuccess        = "success"
	ConclusionFailure        = "failure"
	ConclusionSkipped        = "skipped"
	ConclusionCancelled      = "cancelled"
	ConclusionNeutral        = "neutral"
	ConclusionTimedOut       = "timed_out"
	ConclusionStartupFailure = "startup_failure"
	ConclusionActionRequired = "action_required"
	ConclusionStale          = "stale"
)

// CheckRun is one check context as it reported on a pull request head.
type CheckRun struct {
	Name       string
	Conclusion string
	// AppID is the GitHub App that produced the check. Zero means the producer
	// could not be determined.
	AppID int64
}

// RequiredContext is one required status check from the repository ruleset.
//
// Carries the app as well as the name because a ruleset pins them together:
// this repository requires every context from GitHub Actions specifically
// (.github/rulesets/main.json). Matching on the name alone lets any App
// publish a check called "Build & Test (ubuntu-latest)" and be mistaken for
// the required one.
type RequiredContext struct {
	Name string
	// IntegrationID is the App the ruleset pins the context to. Zero means the
	// ruleset accepts the context from any producer.
	IntegrationID int64
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
	RequiredContexts []RequiredContext
	// RequiredKnown records whether the ruleset lookup actually succeeded.
	// Reading the branch rules needs Administration (read), which the
	// workflow's GITHUB_TOKEN does not have; without this flag an empty list
	// from a denied request is indistinguishable from a repository that
	// requires nothing, and every gating-gap verdict would be fabricated.
	RequiredKnown bool
	// PreMergeCapable is false when the failing check could not have run on a
	// pull request — either its workflow has no pull_request trigger, or the
	// job producing it is guarded to non-pull-request events. Such a check
	// never had a pre-merge opportunity to miss, so "how did this get past
	// pre-merge" has a trivial answer and filter refinement is not the lever.
	PreMergeCapable bool
	// ScheduledDetection marks a failure observed on a scheduled or manually
	// dispatched run rather than on the run for the merge commit itself. Those
	// detect drift in the world (registries, advisories, live infrastructure),
	// so the commit at the head of main is not evidence of what caused them.
	ScheduledDetection bool
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
	// ClassInconclusive — the check reported on the pull request but never
	// reached a successful conclusion: cancelled, timed out, startup failure,
	// action required, neutral. It neither passed nor failed, so neither the
	// scope-gap nor the merge-skew story applies, and asserting either would
	// send the reader after a scoping or determinism problem that is not there.
	ClassInconclusive Class = "inconclusive"
)

// Finding is the classification plus the prose the retro needs.
type Finding struct {
	Class            Class
	Summary          string
	FilterRefinable  bool
	SuggestedActions []string
}

// PricesPlacement reports whether "should this check move pre-merge?" is even a
// question for this class.
//
// It is not, for a post-merge-only finding: the check either cannot run on a
// pull request, or the failure was a scheduled detection that no pull request
// caused. Rendering the ratio anyway produced the contradiction this exists to
// stop — a retro headed `post-merge-only` closing with "ALWAYS PREVENT — block
// this pre-merge", because the workflow happened to have measurable pre-merge
// runs from its other trigger.
func (f Finding) PricesPlacement() bool {
	return f.Class != ClassPostMergeOnly
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

	// A scheduled or dispatched run compares the repository against a world
	// that moves on its own. Attributing it to whatever commit happens to be at
	// the head of main manufactures a culprit, and every downstream class would
	// be reasoning about a pull request that had nothing to do with the finding.
	if e.ScheduledDetection {
		return Finding{
			Class:   ClassPostMergeOnly,
			Summary: fmt.Sprintf("%q failed on a scheduled or manually dispatched run, not on the run for %s. Scheduled runs detect drift in the outside world — new advisories, registry state, live infrastructure — so the commit at the head of `main` is not evidence of what caused this.", e.FailingCheck, shortSHA(e.MainSHA)),
			SuggestedActions: []string{
				"Fix the finding itself. Do not attribute it to the pull request that happens to be at the head of main.",
				"If the finding is one a pull request could have introduced, re-run the workflow on the merge commit to get an attributable result before treating it as an escape.",
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
				"Widen the workflow-level `on.pull_request.paths` filter in the workflow that produces this check, so the relevant change matches.",
				"Better: move the filter off `on.<event>.paths` and onto a job-level `if:`. A job skipped by `if:` reports `skipped`, which is visible and auditable; a workflow skipped by `paths` reports nothing at all, which is why this failure was invisible pre-merge.",
			},
		}

	case run.Conclusion == ConclusionSkipped:
		return Finding{
			Class:           ClassSelectionGap,
			Summary:         fmt.Sprintf("%q was SKIPPED on PR #%d but fails on main. The change was relevant and the path filter said it was not — this is a selection bug, and it is the one class that filter refinement actually fixes.", e.FailingCheck, e.PRNumber),
			FilterRefinable: true,
			SuggestedActions: []string{
				"Find which changed path should have matched and widen the condition that skipped the job.",
				"If the skip came from the affected-test selector (`cmd/test-affected`, ADR-028), the dependency graph it walks missed an edge — fix the selector, not the job.",
			},
		}

	case run.Conclusion == ConclusionFailure:
		if !e.RequiredKnown {
			return Finding{
				Class:   ClassGatingGap,
				Summary: fmt.Sprintf("%q FAILED on PR #%d and the commit is on main. Whether it is a required context could not be determined — reading the branch ruleset needs Administration (read), which this token does not have — so the reason enforcement let it through is unresolved.", e.FailingCheck, e.PRNumber),
				SuggestedActions: []string{
					"Read the ruleset by hand (`gh api repos/<repo>/rules/branches/main`) to establish whether this context is required.",
					"If it is required, cross-check bypassed-merge-audit.yml and the ruleset history for this window. If it is not, this is an advisory check and its red main runs are not escapes.",
				},
			}
		}
		if !isRequired(e.RequiredContexts, run) {
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

	// Everything below asserts that the check PASSED pre-merge. Only a success
	// conclusion supports that. Cancelled, timed out, startup failure, neutral
	// and action-required runs reach neither verdict, and letting them fall
	// through would tell the reader that a check which never passed is either
	// deliberately narrower pre-merge or non-deterministic.
	case run.Conclusion != ConclusionSuccess:
		return Finding{
			Class:   ClassInconclusive,
			Summary: fmt.Sprintf("%q reported %q on PR #%d — it neither passed nor failed — and fails on main. There is no pre-merge pass to compare against, so neither a scoping nor a determinism story applies.", e.FailingCheck, run.Conclusion, e.PRNumber),
			SuggestedActions: []string{
				"Establish why the pre-merge run did not conclude: a cancelled run usually means a superseding push, a timeout means the job outgrew its budget, a startup failure means the workflow itself is malformed.",
				"An inconclusive required check should not have satisfied the merge gate. If it did, that is an enforcement question — check the ruleset and bypassed-merge-audit.yml.",
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
	// CureAssumed is true when CureMinutes is a standing default rather than a
	// measurement of this incident. Nothing here observes how long main was red
	// or how many people it blocked, so the default is an assumption — and an
	// assumption that can push the ratio across the 3:1 and 10:1 bands must not
	// be rendered as though it were evidence.
	CureAssumed bool
	// Escapes is how many failures were counted in the window.
	Escapes float64
	// EscapesScope names what Escapes actually counted. The sweep counts
	// distinct failing commits for the whole producing workflow, not for the
	// individual check, because a multi-job workflow reports one run per job
	// set; saying so keeps the numerator honest.
	EscapesScope string
	// PreventionMinutes is what running the check pre-merge costs over the same
	// window: check duration x pull requests it would run on x wait cost.
	PreventionMinutes float64
	// PreventionMeasured distinguishes "this check is genuinely free" from "we
	// have no pre-merge runs to measure". Without the distinction, a workflow
	// that has never run on a pull request divides by zero and comes out as
	// infinitely worth blocking pre-merge — the exact opposite of the truth for
	// a schedule-only detector.
	PreventionMeasured bool
	// PreventionTruncated is true when the run history hit the API page limit,
	// so the window was only partially observed. A truncated denominator is
	// systematically too small, which inflates the ratio in exactly the
	// direction that argues for adding a pre-merge gate.
	PreventionTruncated bool
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
//
// The bands are only prescriptive when both terms are evidence. An assumed cure
// cost or a truncated prevention measurement still produces a number, and that
// number still crosses thresholds — so the verdict says what it is worth rather
// than dressing an assumption up as a decision.
func (r ROI) Verdict() string {
	if !r.PreventionMeasured {
		return "INSUFFICIENT DATA — no pre-merge runs to price; do not infer a placement from this"
	}
	band := r.band()
	// With no escapes in the window the ratio is zero whatever the cure cost
	// is, so the soft terms cannot be what produced the answer and flagging
	// them would just be noise on every quiet sweep.
	caveats := r.caveats()
	if len(caveats) == 0 || r.Ratio() == 0 {
		return band
	}
	return fmt.Sprintf("PROVISIONAL — %s. Rests on terms that are not evidence: %s. Establish them before moving this check.",
		band, strings.Join(caveats, "; "))
}

// caveats names every term that is not evidence, so the verdict can say which.
func (r ROI) caveats() []string {
	var out []string
	if r.CureAssumed {
		out = append(out, "cure cost is a standing default, not measured for this incident")
	}
	if r.PreventionTruncated {
		out = append(out, "prevention cost is a lower bound: the run history was truncated at the API page limit")
	}
	return out
}

func (r ROI) band() string {
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
	fmt.Fprintf(&b, "ROI = (Cure x Frequency) / Prevention\n")
	if !r.PreventionMeasured {
		fmt.Fprintf(&b, "    = (%.0f min x %.0f escapes) / <unmeasured>\n", r.CureMinutes, r.Escapes)
	} else {
		fmt.Fprintf(&b, "    = (%.0f min x %.0f escapes) / %.0f min\n", r.CureMinutes, r.Escapes, r.PreventionMinutes)
		if ratio := r.Ratio(); math.IsInf(ratio, 1) {
			fmt.Fprintf(&b, "    = unbounded (prevention is free)\n")
		} else {
			fmt.Fprintf(&b, "    = %.1f:1\n", ratio)
		}
	}

	fmt.Fprintf(&b, "\nwhere:\n")
	if r.CureAssumed {
		fmt.Fprintf(&b, "  Cure       = %.0f min/escape — ASSUMED. Nothing here measures how long\n", r.CureMinutes)
		fmt.Fprintf(&b, "               main sat red or how many people it blocked.\n")
	} else {
		fmt.Fprintf(&b, "  Cure       = %.0f min/escape, measured for this incident\n", r.CureMinutes)
	}
	scope := r.EscapesScope
	if scope == "" {
		scope = "unspecified scope"
	}
	fmt.Fprintf(&b, "  Frequency  = %.0f — counted over %s\n", r.Escapes, scope)
	switch {
	case !r.PreventionMeasured:
		fmt.Fprintf(&b, "  Prevention = unmeasured — no qualifying pre-merge runs observed\n")
	case r.PreventionTruncated:
		fmt.Fprintf(&b, "  Prevention = %.0f min — LOWER BOUND; run history truncated at the API\n", r.PreventionMinutes)
		fmt.Fprintf(&b, "               page limit, so the true denominator is larger and the\n")
		fmt.Fprintf(&b, "               true ratio smaller than shown.\n")
	default:
		fmt.Fprintf(&b, "  Prevention = %.0f min, measured over the full window\n", r.PreventionMinutes)
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

// isRequired reports whether this exact check — name and producing app —
// is what the ruleset requires.
//
// An unknown producer (AppID zero) is allowed to match, because refusing would
// turn every failed identity lookup into a false "not required" and flip the
// gating verdict on evidence we do not have.
func isRequired(required []RequiredContext, run CheckRun) bool {
	for _, want := range required {
		if want.Name != run.Name {
			continue
		}
		if want.IntegrationID == 0 || run.AppID == 0 || want.IntegrationID == run.AppID {
			return true
		}
	}
	return false
}

func shortSHA(sha string) string {
	if len(sha) > 7 {
		return sha[:7]
	}
	return sha
}

// SortedContexts returns required context names in a stable order for
// rendering, so retro bodies do not churn between runs.
func SortedContexts(required []RequiredContext) []string {
	out := make([]string, 0, len(required))
	for _, context := range required {
		out = append(out, context.Name)
	}
	sort.Strings(out)
	return out
}
