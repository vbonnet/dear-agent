// Package agenticreview is the pure, testable core of the per-family agentic
// review gate: the label vocabulary each reviewer family publishes, and the
// quorum rule that turns those labels into a merge decision.
//
// The gate exists because a single global review label cannot distinguish
// "claude approved" from "the review pass as a whole approved". One family's
// approval was able to mask another family's failure, and a pull request could
// go ready and merge in the window before any reviewer had posted at all.
// Every label here is therefore scoped to one family and one lifecycle phase.
//
// The package performs no I/O and reads no clock: callers supply the labels,
// the times they were applied, and the current time (see [Input]). That keeps
// the merge-blocking decision cheap enough to run in a required status check
// with no model call anywhere in its path, and identical whether it is
// evaluated by CI or by the merge loop.
package agenticreview

import "strings"

// LabelPrefix namespaces every label this package owns. A label outside this
// namespace is repository business and is never read or written here.
const LabelPrefix = "agentic-review"

// Family identifies one reviewer family. Families are independent: each runs
// its own model on its own transport and publishes its own lifecycle.
type Family string

// The reviewer families this repository runs.
const (
	// FamilyClaude is the Claude reviewer in .github/workflows/claude-code-review.yml.
	FamilyClaude Family = "claude"
	// FamilyCodex is the Codex reviewer driven by cmd/external-pr-reviewer.
	FamilyCodex Family = "codex"
	// FamilyGemini is the Gemini reviewer in .github/workflows/gemini-review.yml.
	FamilyGemini Family = "gemini"
)

// DefaultFamilies is the shipped family set, in the order verdicts are
// reported.
var DefaultFamilies = []Family{FamilyClaude, FamilyCodex, FamilyGemini}

// Phase is one step of a family's review lifecycle.
type Phase string

const (
	// PhaseStarted is published immediately before the family's model is
	// invoked. Its absence is what closes the ready-to-merge window: a pull
	// request with no started label has no review in flight, so it is not
	// mergeable no matter how green the rest of CI is.
	PhaseStarted Phase = "started"
	// PhasePosted is published once the family's review body reaches the pull
	// request. It is evidence the family ran, never evidence it passed.
	PhasePosted Phase = "posted"
	// PhaseApproved is published only when the family's verdict carries no
	// blocking finding against the reviewed head.
	PhaseApproved Phase = "approved"
	// PhaseChangesRequested is published when the family found something
	// blocking. No quorum overrides it.
	PhaseChangesRequested Phase = "changes-requested"
	// PhaseError is a family declaring itself unable to reach a verdict —
	// quota exhaustion, a transport failure, a cancelled run. It is the
	// explicit, immediate form of the degradation that a timeout otherwise
	// has to infer.
	PhaseError Phase = "error"
)

// AllPhases lists every phase, in lifecycle order.
var AllPhases = []Phase{PhaseStarted, PhasePosted, PhaseApproved, PhaseChangesRequested, PhaseError}

// Label renders the repository label for one family and phase.
func Label(f Family, p Phase) string {
	return LabelPrefix + ":" + string(f) + ":" + string(p)
}

// ParseLabel decomposes a repository label into its family and phase. It
// reports false for any label this package does not own, and for a well-formed
// prefix carrying an unknown phase — an unrecognized phase is not evidence of
// anything, so it must not be silently mapped onto one that is.
//
// An unrecognized family does parse. The gate reports families it was not
// configured for rather than mistaking their labels for unrelated noise.
func ParseLabel(name string) (Family, Phase, bool) {
	parts := strings.Split(name, ":")
	if len(parts) != 3 || parts[0] != LabelPrefix {
		return "", "", false
	}
	family, phase := parts[1], parts[2]
	if family == "" {
		return "", "", false
	}
	for _, p := range AllPhases {
		if phase == string(p) {
			return Family(family), p, true
		}
	}
	return "", "", false
}

// ManagedLabels lists every label the given families can publish. Callers use
// it to provision the labels once and, more importantly, to clear the complete
// set when a push invalidates the review: a stale label that survives a force
// push is an approval of code nobody read.
func ManagedLabels(families []Family) []string {
	out := make([]string, 0, len(families)*len(AllPhases))
	for _, f := range families {
		for _, p := range AllPhases {
			out = append(out, Label(f, p))
		}
	}
	return out
}
