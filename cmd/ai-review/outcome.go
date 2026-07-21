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

// outcomeTokens maps the canonical REVIEW.md outcome words to their Outcome.
// Only these exact tokens are recognised; anything else fails closed.
var outcomeTokens = map[string]Outcome{
	"approved":           Approved,
	"needs-work":         NeedsWork,
	"needs_work":         NeedsWork,
	"rejected":           Rejected,
	"needs-human-review": NeedsHumanReview,
	"needs_human_review": NeedsHumanReview,
}

// tokenSplitter normalises a synthesis line into comparable tokens: lowercase,
// with every character that is not a letter, digit, hyphen, or underscore
// turned into a separator. This strips markdown emphasis, backticks, quotes and
// punctuation without letting them glue onto an outcome word.
func outcomeWords(s string) []string {
	lowered := strings.ToLower(s)
	// Fold the space-separated spellings of the *blocking* outcomes onto their
	// canonical hyphenated tokens. This only ever maps onto merge-blocking
	// states, so it cannot widen the approved path.
	lowered = strings.ReplaceAll(lowered, "needs human review", "needs-human-review")
	lowered = strings.ReplaceAll(lowered, "human review required", "needs-human-review")
	lowered = strings.ReplaceAll(lowered, "needs work", "needs-work")
	return strings.FieldsFunc(lowered, func(r rune) bool {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9', r == '-', r == '_':
			return false
		default:
			return true
		}
	})
}

// ParseOutcome maps a synthesis model's first line to an Outcome using exact
// token matching. Substring matching is deliberately NOT used: a line such as
// "not approved due to blocking findings" or "approval cannot be given"
// contains the substring "approve" and would otherwise be classified as
// Approved, punching a fail-open hole straight through the gate.
//
// Rules:
//   - only the exact canonical tokens are recognised;
//   - when several outcome tokens appear, the most severe wins (REVIEW.md §1:
//     "ambiguous findings always resolve down");
//   - `approved` is honoured only when it is the sole outcome token present and
//     is not negated ("not approved", "cannot be approved");
//   - anything else — no token, an unknown token, or a negated approval —
//     resolves to NeedsHumanReview, which blocks the merge (SPEC R7).
func ParseOutcome(s string) Outcome {
	words := outcomeWords(s)

	found := map[Outcome]bool{}
	approvedNegated := false
	for i, w := range words {
		o, ok := outcomeTokens[w]
		if !ok {
			continue
		}
		if o == Approved && negatedBefore(words, i) {
			approvedNegated = true
			continue
		}
		found[o] = true
	}

	// Resolve down: most severe outcome present wins.
	switch {
	case found[NeedsHumanReview]:
		return NeedsHumanReview
	case found[Rejected]:
		return Rejected
	case found[NeedsWork]:
		return NeedsWork
	case found[Approved]:
		return Approved
	case approvedNegated:
		// "not approved" with no explicit follow-up state — block.
		return NeedsHumanReview
	default:
		// No recognisable outcome token — fail closed.
		return NeedsHumanReview
	}
}

// negationWords are the tokens that invert an adjacent "approved".
var negationWords = map[string]bool{
	"not": true, "no": true, "cannot": true, "cant": true, "can": true,
	"never": true, "without": true, "isnt": true, "wasnt": true,
	"dont": true, "doesnt": true, "wont": true, "unable": true, "fails": true,
}

// negatedBefore reports whether an "approved" token at index i is negated by a
// nearby preceding word (e.g. "not approved", "cannot be approved").
func negatedBefore(words []string, i int) bool {
	start := max(i-3, 0)
	for _, w := range words[start:i] {
		if negationWords[w] {
			return true
		}
	}
	return false
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
