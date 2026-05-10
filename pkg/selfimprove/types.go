package selfimprove

import (
	"context"
	"time"

	"github.com/vbonnet/dear-agent/pkg/benchmarks"
)

// Hypothesis is a proposed improvement to dear-agent generated from failure
// patterns. The Source identifies which model proposed it (Claude, GPT,
// Gemini, …) so the loop can attribute wins per-model.
type Hypothesis struct {
	ID          string  `json:"id"`
	Source      string  `json:"source"`
	Description string  `json:"description"`
	TargetFile  string  `json:"target_file,omitempty"`
	Rationale   string  `json:"rationale,omitempty"`
	Confidence  float64 `json:"confidence,omitempty"`
}

// Patch records a Hypothesis that has been applied to the working tree.
// Diff is the unified diff that was applied; Revert is the recipe an Applier
// uses to undo it.
type Patch struct {
	Hypothesis Hypothesis `json:"hypothesis"`
	Diff       string     `json:"diff,omitempty"`
	AppliedAt  time.Time  `json:"applied_at"`
	Reverted   bool       `json:"reverted"`
}

// CycleResult captures one full iteration of the self-improvement loop.
type CycleResult struct {
	Cycle       int                 `json:"cycle"`
	Suite       benchmarks.Suite    `json:"suite"`
	Baseline    *benchmarks.Results `json:"baseline,omitempty"`
	Patches     []Patch             `json:"patches,omitempty"`
	PostResults *benchmarks.Results `json:"post_results,omitempty"`
	Delta       *benchmarks.Delta   `json:"delta,omitempty"`
	Accepted    bool                `json:"accepted"`
	Reason      string              `json:"reason"`
	CostUSD     float64             `json:"cost_usd"`
}

// LoopConfig configures Loop.Run. The zero value runs a single cycle with
// regression gating on; that is the safe default for an autonomous loop.
type LoopConfig struct {
	Suite          benchmarks.Suite `json:"suite"`
	Mode           benchmarks.Mode  `json:"mode"`
	Model          string           `json:"model"`
	MaxCycles      int              `json:"max_cycles"`
	BudgetUSD      float64          `json:"budget_usd"`
	ProposerModels []string         `json:"proposer_models"`
	RegressionGate bool             `json:"regression_gate"`
	BenchLimit     int              `json:"bench_limit"`
}

// Proposer turns benchmarks.Insights into a list of Hypothesis. The model
// argument lets a single Proposer impl multiplex over Claude, GPT, Gemini.
type Proposer interface {
	Propose(ctx context.Context, insights *benchmarks.Insights, model string) ([]Hypothesis, error)
}

// Applier applies and reverts Hypotheses as Patches on the working tree.
// Implementations are expected to leave the working tree clean on Revert.
type Applier interface {
	Apply(ctx context.Context, h Hypothesis) (Patch, error)
	Revert(ctx context.Context, p Patch) error
}

// Notifier receives VROOM-style lifecycle events during a Run. The default
// implementation is a no-op; production wires it to pkg/vroom/vroom.Emitter.
type Notifier interface {
	Phase(name string, fields map[string]any)
}

// NoopNotifier is the default Notifier — it drops every event.
type NoopNotifier struct{}

// Phase satisfies Notifier with a no-op.
func (NoopNotifier) Phase(string, map[string]any) {}
