package cihealth

import (
	"fmt"
	"strings"
)

// Retro is the DEAR retrospective the watchdog files when main goes red.
// Define the defect, Execute the fix, Audit the outcome, Retro the prevention —
// see docs/policies/dear-retro.ai.md.
type Retro struct {
	Repo         string
	FailingCheck string
	WorkflowName string
	MainSHA      string
	RunURL       string
	Finding      Finding
	ROI          ROI
	Required     []string
	// RequiredKnown is false when the ruleset could not be read, so an empty
	// Required list means "not established" rather than "nothing is required".
	RequiredKnown bool
	// WindowDays is the lookback the escape count was measured over.
	WindowDays int
}

// Title is stable per failing check so repeat escapes update one issue instead
// of scattering duplicates — the policy wants repeat incidents to read as a
// frequency signal, not as noise.
func (r Retro) Title() string {
	return fmt.Sprintf("DEAR retro: main red — %s", r.FailingCheck)
}

// Body renders the retro. It leads with how the failure got past pre-merge,
// because that is the question the retro exists to answer; the fix for the
// individual failure is secondary and usually obvious.
func (r Retro) Body() string {
	var b strings.Builder

	fmt.Fprintf(&b, "## Define — what broke\n\n")
	fmt.Fprintf(&b, "`%s` is failing on `main` at [`%s`](%s).\n\n", r.FailingCheck, shortSHA(r.MainSHA), r.RunURL)
	if r.WorkflowName != "" {
		fmt.Fprintf(&b, "Workflow: **%s**\n\n", r.WorkflowName)
	}

	fmt.Fprintf(&b, "## Audit — how it got through pre-merge\n\n")
	fmt.Fprintf(&b, "**Classification: `%s`**\n\n", r.Finding.Class)
	fmt.Fprintf(&b, "%s\n\n", r.Finding.Summary)

	fmt.Fprintf(&b, "### Can filter selection be refined?\n\n")
	if r.Finding.FilterRefinable {
		fmt.Fprintf(&b, "**Yes.** This failure reached main because the pre-merge selection did not run a check that was relevant to the change. That is a filter bug, and it is fixable in the producing workflow's own `on.pull_request.paths` block or job-level `if:` condition.\n\n")
	} else {
		fmt.Fprintf(&b, "**No.** Refining path filters will not prevent this class. Selection did what it was told; the gap is elsewhere. Editing filters in response to this would add cost without removing risk.\n\n")
	}

	fmt.Fprintf(&b, "## Retro — prevention\n\n")
	for _, action := range r.Finding.SuggestedActions {
		fmt.Fprintf(&b, "- %s\n", action)
	}
	fmt.Fprintf(&b, "\n")

	if !r.Finding.PricesPlacement() {
		fmt.Fprintf(&b, "### Should this move pre-merge? (prevention-vs-cure)\n\n")
		fmt.Fprintf(&b, "**Not applicable.** This failure is not an escape, so there is no placement to price. ")
		fmt.Fprintf(&b, "Running the ratio here would answer a question nobody asked, and its verdict would read as an instruction to move a check that either cannot run pre-merge or already does.\n\n")
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
		fmt.Fprintf(b, "<details><summary>Required status checks at time of failure</summary>\n\n")
		for _, context := range SortedContexts(r.Required) {
			fmt.Fprintf(b, "- `%s`\n", context)
		}
		fmt.Fprintf(b, "\n</details>\n\n")
	}

	fmt.Fprintf(b, "---\n")
	fmt.Fprintf(b, "Filed automatically by `.github/workflows/main-health-watchdog.yml`. ")
	fmt.Fprintf(b, "Analysis: `tools/ci-escape-analysis`. Contract: `pkg/cihealth/SPEC.md`. Policy: `docs/policies/dear-retro.ai.md`.\n")

	return b.String()
}
