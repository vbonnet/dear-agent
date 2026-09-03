package prreviewer

import (
	"strings"
	"testing"

	"github.com/vbonnet/dear-agent/internal/agenticreview"
)

func TestCodexVerdictReadsTheCodexSectionOnly(t *testing.T) {
	body := `Automated external review.

## Codex

Looks fine to me.

VERDICT: APPROVE

## Gemini

I disagree.

VERDICT: CHANGES_REQUESTED
`
	// The codex family's label reports what Codex said. Gemini's opinion is a
	// separate family with its own label, and letting it bleed across would
	// recreate exactly the cross-family masking this schema exists to stop.
	if got := codexVerdict(body); got != agenticreview.PhaseApproved {
		t.Fatalf("verdict = %q, want %q", got, agenticreview.PhaseApproved)
	}
}

func TestCodexVerdictReadsChangesRequested(t *testing.T) {
	body := "## Codex\n\nA nil dereference on line 12.\n\nVERDICT: CHANGES_REQUESTED\n\n## Gemini\n\nnothing\n"
	if got := codexVerdict(body); got != agenticreview.PhaseChangesRequested {
		t.Fatalf("verdict = %q, want %q", got, agenticreview.PhaseChangesRequested)
	}
}

// A review with no verdict line is posted and nothing more. Reading an absent
// verdict as an approval would turn every malformed reviewer response into a
// merge permission.
func TestCodexVerdictWithoutAVerdictLineIsPostedOnly(t *testing.T) {
	body := "## Codex\n\nI have some thoughts but no conclusion.\n\n## Gemini\n\nnothing\n"
	if got := codexVerdict(body); got != agenticreview.PhasePosted {
		t.Fatalf("verdict = %q, want %q", got, agenticreview.PhasePosted)
	}
}

// Prose that merely mentions the token is not a verdict; only the exact line is.
func TestCodexVerdictIgnoresProseMentions(t *testing.T) {
	body := "## Codex\n\nI would write VERDICT: APPROVE if the tests passed, but they do not.\n\n## Gemini\n\n"
	if got := codexVerdict(body); got != agenticreview.PhasePosted {
		t.Fatalf("verdict = %q, want %q", got, agenticreview.PhasePosted)
	}
}

// The last verdict in the Codex section wins, so a model that restates its
// conclusion does not resolve to the first thing it said.
func TestCodexVerdictTakesTheFinalVerdictLine(t *testing.T) {
	body := "## Codex\n\nVERDICT: APPROVE\n\nOn reflection:\n\nVERDICT: CHANGES_REQUESTED\n\n## Gemini\n\n"
	if got := codexVerdict(body); got != agenticreview.PhaseChangesRequested {
		t.Fatalf("verdict = %q, want %q", got, agenticreview.PhaseChangesRequested)
	}
}

// The prompt has to ask for the line the parser reads, or the codex family can
// never publish a verdict and will always age out into a false "down".
func TestReviewPromptRequestsTheVerdictLine(t *testing.T) {
	prompt := reviewPrompt("o/r", PR{Number: 1, Title: "t", HeadRefOID: "abc"}, "diff")
	for _, want := range []string{"VERDICT: APPROVE", "VERDICT: CHANGES_REQUESTED"} {
		if !strings.Contains(prompt, want) {
			t.Errorf("prompt does not request %q", want)
		}
	}
}

func TestLabelArgsBindToTheReviewedHead(t *testing.T) {
	args := addLabelArgs("o/r", 7, agenticreview.Label(agenticreview.FamilyCodex, agenticreview.PhaseStarted))
	joined := strings.Join(args, " ")
	for _, want := range []string{"pr", "edit", "7", "--repo o/r", "--add-label agentic-review:codex:started"} {
		if !strings.Contains(joined, want) {
			t.Errorf("args %q missing %q", joined, want)
		}
	}
}

func TestEnsureLabelArgsAreIdempotent(t *testing.T) {
	joined := strings.Join(ensureLabelArgs("o/r", "agentic-review:codex:posted"), " ")
	if !strings.Contains(joined, "--force") {
		t.Fatalf("args %q would fail on a label that already exists", joined)
	}
}
