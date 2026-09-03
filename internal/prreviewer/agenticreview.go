package prreviewer

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/vbonnet/dear-agent/internal/agenticreview"
)

// This file is the codex family's half of the agentic review gate. The Claude
// and Gemini families publish their lifecycle from GitHub Actions; this
// reviewer runs on the operator's host, so it publishes its own.
//
// A host-side poller is exactly the reviewer most likely to be down — the
// machine sleeps, the CLI needs re-auth, a quota runs out mid-pass — which is
// why the gate's degradation rule exists at all. Publishing an explicit error
// state here is what lets the gate tell "codex is down" from "codex has not
// answered yet", and only the first of those can be quorum'd around.

// verdictMarkers are the exact lines the review prompt asks Codex to end with.
var verdictMarkers = map[string]agenticreview.Phase{
	"VERDICT: APPROVE":           agenticreview.PhaseApproved,
	"VERDICT: CHANGES_REQUESTED": agenticreview.PhaseChangesRequested,
}

// codexVerdict reads the Codex section's final verdict line.
//
// Scoping to the Codex section is not cosmetic: the composed body carries a
// Gemini section too, and that family publishes its own label. Letting one
// family's conclusion resolve another's label would rebuild the cross-family
// masking the per-family schema exists to prevent.
//
// A body with no verdict line resolves to posted, never approved. An absent
// verdict is a reviewer that reached no conclusion, and reading that as
// approval would turn every malformed response into a merge permission.
func codexVerdict(body string) agenticreview.Phase {
	section := body
	if start := strings.Index(section, "## Codex"); start >= 0 {
		section = section[start:]
	}
	if end := strings.Index(section, "\n## Gemini"); end >= 0 {
		section = section[:end]
	}

	phase := agenticreview.PhasePosted
	for line := range strings.SplitSeq(section, "\n") {
		if p, ok := verdictMarkers[strings.TrimSpace(line)]; ok {
			phase = p
		}
	}
	return phase
}

func ensureLabelArgs(repo, label string) []string {
	return []string{"label", "create", label, "--repo", repo, "--force",
		"--color", "BFD4F2", "--description", "Agentic review lifecycle (machine-managed)"}
}

func addLabelArgs(repo string, number int, label string) []string {
	return []string{"pr", "edit", fmt.Sprint(number), "--repo", repo, "--add-label", label}
}

// publishPhase records one codex lifecycle phase on the pull request.
//
// A failure to label is reported to the operator but never fails the review
// pass. The review itself is the valuable, expensive artifact; losing a label
// costs the family its verdict, which the gate already handles by holding the
// merge until the deadline and then treating the family as down. Failing the
// pass instead would lose the review as well.
func publishPhase(ctx context.Context, cfg Config, runner Runner, repo string, number int, phase agenticreview.Phase, stdout io.Writer) {
	if cfg.DryRun {
		fmt.Fprintf(stdout, "external-pr-reviewer: dry-run would label %s PR #%d %s\n",
			repo, number, agenticreview.Label(agenticreview.FamilyCodex, phase))
		return
	}
	label := agenticreview.Label(agenticreview.FamilyCodex, phase)
	if _, err := runGH(ctx, cfg, runner, ensureLabelArgs(repo, label), ""); err != nil {
		fmt.Fprintf(stdout, "external-pr-reviewer: could not provision %s: %v\n", label, err)
		return
	}
	if _, err := runGH(ctx, cfg, runner, addLabelArgs(repo, number, label), ""); err != nil {
		fmt.Fprintf(stdout, "external-pr-reviewer: could not apply %s: %v\n", label, err)
	}
}
