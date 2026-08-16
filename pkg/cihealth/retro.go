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
		fmt.Fprintf(&b, "**Yes.** This failure reached main because the pre-merge selection did not run a check that was relevant to the change. That is a filter bug and it is fixable in `.github/workflows/changed-paths.yml`.\n\n")
	} else {
		fmt.Fprintf(&b, "**No.** Refining path filters will not prevent this class. Selection did what it was told; the gap is elsewhere. Editing filters in response to this would add cost without removing risk.\n\n")
	}

	fmt.Fprintf(&b, "## Retro — prevention\n\n")
	for _, action := range r.Finding.SuggestedActions {
		fmt.Fprintf(&b, "- %s\n", action)
	}
	fmt.Fprintf(&b, "\n")

	fmt.Fprintf(&b, "### Should this move pre-merge? (prevention-vs-cure)\n\n")
	fmt.Fprintf(&b, "Formula: `ROI = (Cure Cost x Frequency) / Prevention Cost`. ")
	fmt.Fprintf(&b, "Bands: >10:1 always prevent, >3:1 usually prevent, <3:1 case-by-case.\n\n")
	fmt.Fprintf(&b, "```\n%s```\n\n", r.ROI.Explain())
	fmt.Fprintf(&b, "Measured over the last %d days. ", r.WindowDays)
	fmt.Fprintf(&b, "Cure cost is time `main` sat red multiplied by the people it blocked, plus triage. ")
	fmt.Fprintf(&b, "Prevention cost is what running this check pre-merge on every affected PR would have cost over the same window.\n\n")
	fmt.Fprintf(&b, "Read the verdict as an input, not an instruction — a check that is slow **and** flaky belongs post-merge whatever the ratio says, because the flake tax is paid by everyone and does not appear in the numerator.\n\n")

	if len(r.Required) > 0 {
		fmt.Fprintf(&b, "<details><summary>Required status checks at time of failure</summary>\n\n")
		for _, context := range SortedContexts(r.Required) {
			fmt.Fprintf(&b, "- `%s`\n", context)
		}
		fmt.Fprintf(&b, "\n</details>\n\n")
	}

	fmt.Fprintf(&b, "---\n")
	fmt.Fprintf(&b, "Filed automatically by `.github/workflows/main-health-watchdog.yml`. ")
	fmt.Fprintf(&b, "Analysis: `tools/ci-escape-analysis`. Policy: `docs/policies/dear-retro.ai.md`. Design: ADR-038.\n")

	return b.String()
}
