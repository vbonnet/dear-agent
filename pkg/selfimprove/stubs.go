package selfimprove

import (
	"context"
	"fmt"
	"time"

	"github.com/vbonnet/dear-agent/pkg/benchmarks"
)

// PatternProposer turns each FailurePattern into a Hypothesis by lifting the
// pattern's Hypothesis text. It is the default proposer used by the CLI when
// no real model integration is wired: it lets the loop go end-to-end with
// real signal but no LLM cost.
type PatternProposer struct{}

// Propose maps insights.Patterns 1:1 to Hypotheses.
func (PatternProposer) Propose(_ context.Context, insights *benchmarks.Insights, model string) ([]Hypothesis, error) {
	if insights == nil {
		return nil, nil
	}
	out := make([]Hypothesis, 0, len(insights.Patterns))
	for i, p := range insights.Patterns {
		if p.Hypothesis == "" {
			continue
		}
		out = append(out, Hypothesis{
			ID:          fmt.Sprintf("%s-pattern-%d", insights.RunID, i),
			Source:      model,
			Description: p.Hypothesis,
			Rationale:   fmt.Sprintf("derived from pattern %q (count=%d)", p.Name, p.Count),
			Confidence:  0.5,
		})
	}
	return out, nil
}

// NoopApplier records hypotheses as patches without modifying anything.
// It is the safe default for dry runs: the loop exercises its plumbing
// (apply / re-run / compare / revert) without touching the working tree.
type NoopApplier struct{}

// Apply records the hypothesis as a Patch with an empty Diff.
func (NoopApplier) Apply(_ context.Context, h Hypothesis) (Patch, error) {
	return Patch{Hypothesis: h, AppliedAt: time.Now().UTC()}, nil
}

// Revert is a no-op; nothing was applied.
func (NoopApplier) Revert(_ context.Context, _ Patch) error { return nil }
