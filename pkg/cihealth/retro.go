package cihealth

import (
	"fmt"
	"strings"
)

// Retro is the incident brief the watchdog files when main goes red.
//
// It is NOT a completed DEAR retrospective, and calling it one would be a lie
// the policy can check. DEAR is Define the defect, Execute the fix, Audit the
// outcome, Retro the prevention (docs/policies/dear-retro.ai.md). At detection
// time only Define is knowable: nothing has been executed, so there is no
// outcome to audit, and prevention proposed before a fix exists is a
// hypothesis. The brief therefore renders Define in full, states the
// prevention analysis as provisional, and leaves Execute and Audit as sections
// for whoever fixes the failure to append — at which point the issue becomes
// the retrospective.
type Retro struct {
	Repo         string
	FailingCheck string
	WorkflowName string
	MainSHA      string
	RunURL       string
	Finding      Finding
	ROI          ROI
	Required     []RequiredContext
	// RequiredKnown is false when the ruleset could not be read, so an empty
	// Required list means "not established" rather than "nothing is required".
	RequiredKnown bool
	// WindowDays is the lookback the escape count was measured over.
	WindowDays int
}

// Title is stable per failing check so repeat escapes update one issue instead
// of scattering duplicates — the policy wants repeat incidents to read as a
// frequency signal, not as noise.
// Keyed by workflow AND check. A job name is not unique across workflows —
// `govulncheck` is a job in both CI and Security Audit — so keying on the check
// alone coalesces two workflows' incidents into one issue. The sweep promises
// one brief per red workflow, and the second workflow would silently inherit
// the first's issue: already open, already dequeued, never dispatched.
func (r Retro) Title() string {
	if r.WorkflowName == "" {
		return fmt.Sprintf("main red — %s", r.FailingCheck)
	}
	return fmt.Sprintf("main red — %s / %s", r.WorkflowName, r.FailingCheck)
}

// Body renders the brief. It leads with how the failure got past pre-merge,
// because that is the question the brief exists to answer; the fix for the
// individual failure is secondary and usually obvious.
func (r Retro) Body() string {
	var b strings.Builder

	fmt.Fprintf(&b, "> **Incident brief — not yet a DEAR retrospective.** `Define` below is complete. ")
	fmt.Fprintf(&b, "`Execute` and `Audit` are empty because nothing has been fixed yet, and `Retro` is provisional until it has. ")
	fmt.Fprintf(&b, "Whoever fixes this completes the remaining sections; the issue becomes the retrospective at that point.\n\n")

	fmt.Fprintf(&b, "## Define — what broke\n\n")
	fmt.Fprintf(&b, "`%s` is failing on `main` at [`%s`](%s).\n\n", r.FailingCheck, shortSHA(r.MainSHA), r.RunURL)
	if r.WorkflowName != "" {
		fmt.Fprintf(&b, "Workflow: **%s**\n\n", r.WorkflowName)
	}

	fmt.Fprintf(&b, "### How it got past pre-merge\n\n")
	fmt.Fprintf(&b, "**Classification: `%s`**\n\n", r.Finding.Class)
	fmt.Fprintf(&b, "%s\n\n", r.Finding.Summary)

	fmt.Fprintf(&b, "### Can filter selection be refined?\n\n")
	if r.Finding.FilterRefinable {
		fmt.Fprintf(&b, "**Yes.** This failure reached main because the pre-merge selection did not run a check that was relevant to the change. That is a filter bug, and it is fixable in the producing workflow's own `on.pull_request.paths` block or job-level `if:` condition.\n\n")
	} else {
		fmt.Fprintf(&b, "**No.** Refining path filters will not prevent this class. Selection did what it was told; the gap is elsewhere. Editing filters in response to this would add cost without removing risk.\n\n")
	}

	fmt.Fprintf(&b, "## Execute — the fix\n\n")
	fmt.Fprintf(&b, "_Not started. Record what was changed and link the PR._\n\n")

	fmt.Fprintf(&b, "## Audit — did the fix hold?\n\n")
	fmt.Fprintf(&b, "_Nothing to audit yet. Record whether `main` went green and whether the prevention below was actually landed._\n\n")

	fmt.Fprintf(&b, "## Retro — prevention (provisional)\n\n")
	for _, action := range r.Finding.SuggestedActions {
		fmt.Fprintf(&b, "- %s\n", action)
	}
	fmt.Fprintf(&b, "\n")

	if !r.Finding.PricesPlacement() {
		fmt.Fprintf(&b, "### Should this move pre-merge? (prevention-vs-cure)\n\n")
		fmt.Fprintf(&b, "**Not applicable — `%s` is not a placement decision.** %s\n\n", r.Finding.Class, placementNotApplicable(r.Finding.Class))
		return r.footer(&b)
	}

	fmt.Fprintf(&b, "### Should this move pre-merge? (prevention-vs-cure)\n\n")
	fmt.Fprintf(&b, "Formula: `ROI = (Cure Cost x Frequency) / Prevention Cost`. ")
	fmt.Fprintf(&b, "Bands: >10:1 always prevent, >3:1 usually prevent, <3:1 case-by-case.\n\n")
	fmt.Fprintf(&b, "```\n%s```\n\n", r.ROI.Explain())
	fmt.Fprintf(&b, "Lookback window: %d days. ", r.WindowDays)
	fmt.Fprintf(&b, "Every term's provenance is stated in the block above; a term marked ASSUMED or LOWER BOUND is not evidence, and a verdict resting on one is marked `PROVISIONAL`.\n\n")
	fmt.Fprintf(&b, "Read the verdict as an input, not an instruction — a check that is slow **and** flaky belongs post-merge whatever the ratio says, because the flake tax is paid by everyone and does not appear in the numerator.\n\n")

	return r.footer(&b)
}

// footer closes the retro with the evidence caveat and the provenance line.
// Shared so a retro that skips the pricing section still carries both.
func (r Retro) footer(b *strings.Builder) string {
	switch {
	case !r.RequiredKnown:
		fmt.Fprintf(b, "> **Required status checks: not established.** Reading the branch ruleset needs Administration (read), which the watchdog's token does not have. Any statement above about whether this check gates merges is unresolved, not a finding.\n\n")
	case len(r.Required) > 0:
		fmt.Fprintf(b, "<details><summary>Required status checks (current ruleset, read at analysis time)</summary>\n\n")
		for _, context := range SortedContexts(r.Required) {
			fmt.Fprintf(b, "- `%s`\n", context)
		}
		fmt.Fprintf(b, "\nThis is the ruleset as it stands now, not as it stood at the merge. If a requirement was added or removed in between, the gating classification above is about today's rules applied to an older merge.\n")
		fmt.Fprintf(b, "\n</details>\n\n")
	}

	fmt.Fprintf(b, "---\n")
	fmt.Fprintf(b, "Filed automatically by `.github/workflows/main-health-watchdog.yml`. ")
	fmt.Fprintf(b, "Analysis: `tools/ci-escape-analysis`. Contract: `pkg/cihealth/SPEC.md`. Policy: `docs/policies/dear-retro.ai.md`.\n")

	return b.String()
}

// placementNotApplicable says why the prevention-vs-cure ratio is the wrong
// question for a class, rather than printing one generic disclaimer that is
// false for half of them.
func placementNotApplicable(class Class) string {
	switch class {
	case ClassMergeSkew:
		return "The check already ran pre-merge and passed, at the same scope it runs at on main. Moving it pre-merge is advice it is already following; the open question is flake versus semantic merge skew."
	case ClassInconclusive:
		return "The pre-merge run never concluded. What this needs is for that run to finish, not for the check to be placed somewhere else."
	case ClassBypassed:
		return "No pull request ran the check, so no placement would have changed the outcome. This is a branch-protection question."
	case ClassGatingGap:
		return "The check ran pre-merge and reported. Enforcement let the merge through anyway, which is a ruleset decision, not a placement one."
	case ClassPostMergeOnly:
		return "This is not an escape. The check either cannot run on a pull request, or the failure was a scheduled detection no pull request caused."
	case ClassUnknown:
		return "The evidence needed to classify this was never gathered, so there is nothing to price."
	case ClassNeverRan, ClassSelectionGap, ClassScopeGap:
		return "" // priced; never reached
	default:
		return "The remedy for this class is not to move the check pre-merge."
	}
}
