package selfimprove

import (
	"context"
	"fmt"
	"time"

	"github.com/vbonnet/dear-agent/pkg/benchmarks"
)

// Loop wires a Benchmark, a Proposer, and an Applier into a recursive
// self-improvement loop. Loop.Run drives one or more cycles up to the
// configured cap.
type Loop struct {
	Bench    benchmarks.Benchmark
	Proposer Proposer
	Applier  Applier
	Notifier Notifier // optional; defaults to NoopNotifier
}

// New returns a Loop with the given dependencies. nil Notifier becomes a
// NoopNotifier so callers can leave it unset in tests and dry runs.
func New(bench benchmarks.Benchmark, proposer Proposer, applier Applier, notifier Notifier) *Loop {
	if notifier == nil {
		notifier = NoopNotifier{}
	}
	return &Loop{Bench: bench, Proposer: proposer, Applier: applier, Notifier: notifier}
}

// Run executes the self-improvement loop. It returns one CycleResult per
// completed cycle, regardless of acceptance — callers inspect Accepted to
// distinguish kept vs reverted cycles.
//
// The loop stops on any of:
//   - context cancellation
//   - cfg.MaxCycles reached (defaults to 1 if zero)
//   - cumulative CostUSD exceeds cfg.BudgetUSD (when > 0)
//   - the Proposer returns no Hypotheses
//   - a cycle is rejected by the regression gate
func (l *Loop) Run(ctx context.Context, cfg LoopConfig) ([]CycleResult, error) {
	if err := l.validate(cfg); err != nil {
		return nil, err
	}
	if cfg.MaxCycles <= 0 {
		cfg.MaxCycles = 1
	}

	var (
		cycles    []CycleResult
		spent     float64
		baseline  *benchmarks.Results
		runConfig = benchmarks.RunConfig{
			Mode:      cfg.Mode,
			Model:     cfg.Model,
			Limit:     cfg.BenchLimit,
			BudgetUSD: cfg.BudgetUSD,
		}
	)

	// Baseline run.
	l.Notifier.Phase("baseline.start", map[string]any{"suite": cfg.Suite, "mode": cfg.Mode})
	baseline, err := l.Bench.Run(ctx, runConfig)
	if err != nil {
		return nil, fmt.Errorf("baseline run: %w", err)
	}
	spent += baseline.Summary.TotalCostUSD
	l.Notifier.Phase("baseline.done", map[string]any{
		"run_id":     baseline.RunID,
		"solve_rate": baseline.Summary.SolveRate,
		"cost_usd":   baseline.Summary.TotalCostUSD,
	})

	for cycle := 1; cycle <= cfg.MaxCycles; cycle++ {
		if err := ctx.Err(); err != nil {
			return cycles, err
		}
		if cfg.BudgetUSD > 0 && spent >= cfg.BudgetUSD {
			cycles = append(cycles, CycleResult{
				Cycle:    cycle,
				Suite:    cfg.Suite,
				Baseline: baseline,
				Reason:   fmt.Sprintf("budget exhausted: spent $%.4f >= cap $%.4f", spent, cfg.BudgetUSD),
				CostUSD:  spent,
			})
			break
		}

		result := l.runCycle(ctx, cycle, cfg, baseline, &spent)
		cycles = append(cycles, result)

		// A non-accepted cycle stops further iteration: the baseline has
		// not moved, so re-deriving the same insights would re-propose the
		// same hypotheses. Continuing would loop without progress.
		if !result.Accepted {
			break
		}
		if result.PostResults != nil {
			baseline = result.PostResults
		}
	}

	return cycles, nil
}

func (l *Loop) runCycle(
	ctx context.Context,
	cycle int,
	cfg LoopConfig,
	baseline *benchmarks.Results,
	spent *float64,
) CycleResult {
	out := CycleResult{Cycle: cycle, Suite: cfg.Suite, Baseline: baseline}

	insights, err := l.Bench.Analyze(baseline)
	if err != nil {
		out.Reason = fmt.Sprintf("analyze: %v", err)
		return out
	}
	l.Notifier.Phase("analyze.done", map[string]any{
		"cycle":    cycle,
		"patterns": len(insights.Patterns),
	})

	hypotheses := l.gatherHypotheses(ctx, cycle, insights, cfg)
	if len(hypotheses) == 0 {
		out.Reason = "no hypotheses proposed"
		return out
	}

	patches := l.applyAll(ctx, cycle, hypotheses)
	out.Patches = patches
	if len(patches) == 0 {
		out.Reason = "all hypotheses failed to apply"
		return out
	}

	runConfig := benchmarks.RunConfig{
		Mode:      cfg.Mode,
		Model:     cfg.Model,
		Limit:     cfg.BenchLimit,
		BudgetUSD: cfg.BudgetUSD,
	}
	post, err := l.Bench.Run(ctx, runConfig)
	if err != nil {
		l.revertAll(ctx, patches)
		out.Reason = fmt.Sprintf("post-run: %v", err)
		return out
	}
	*spent += post.Summary.TotalCostUSD
	out.PostResults = post
	out.CostUSD = post.Summary.TotalCostUSD

	delta, err := l.Bench.Compare(baseline, post)
	if err != nil {
		l.revertAll(ctx, patches)
		out.Reason = fmt.Sprintf("compare: %v", err)
		return out
	}
	out.Delta = delta

	if cfg.RegressionGate && delta.IsRegression {
		l.revertAll(ctx, patches)
		out.Reason = fmt.Sprintf("regression gate: solve rate dropped by %.4f", -delta.SolveRateDelta)
		l.Notifier.Phase("cycle.rejected", map[string]any{"cycle": cycle, "reason": out.Reason})
		return out
	}

	out.Accepted = true
	out.Reason = fmt.Sprintf("accepted: solve_rate %+.4f, cost_per_solved %+.4f",
		delta.SolveRateDelta, delta.CostPerSolvedDelta)
	l.Notifier.Phase("cycle.accepted", map[string]any{
		"cycle":            cycle,
		"solve_rate_delta": delta.SolveRateDelta,
	})
	return out
}

func (l *Loop) gatherHypotheses(ctx context.Context, cycle int, insights *benchmarks.Insights, cfg LoopConfig) []Hypothesis {
	models := cfg.ProposerModels
	if len(models) == 0 {
		models = []string{cfg.Model}
	}
	var all []Hypothesis
	for _, model := range models {
		hs, err := l.Proposer.Propose(ctx, insights, model)
		if err != nil {
			l.Notifier.Phase("propose.error", map[string]any{
				"cycle": cycle, "model": model, "error": err.Error(),
			})
			continue
		}
		l.Notifier.Phase("propose.done", map[string]any{
			"cycle": cycle, "model": model, "count": len(hs),
		})
		all = append(all, hs...)
	}
	return all
}

func (l *Loop) applyAll(ctx context.Context, cycle int, hs []Hypothesis) []Patch {
	var patches []Patch
	for _, h := range hs {
		p, err := l.Applier.Apply(ctx, h)
		if err != nil {
			l.Notifier.Phase("apply.error", map[string]any{
				"cycle": cycle, "hypothesis": h.ID, "error": err.Error(),
			})
			continue
		}
		if p.AppliedAt.IsZero() {
			p.AppliedAt = time.Now().UTC()
		}
		patches = append(patches, p)
	}
	return patches
}

func (l *Loop) revertAll(ctx context.Context, patches []Patch) {
	for i := range patches {
		if err := l.Applier.Revert(ctx, patches[i]); err == nil {
			patches[i].Reverted = true
		}
	}
}

func (l *Loop) validate(cfg LoopConfig) error {
	if l.Bench == nil {
		return fmt.Errorf("selfimprove: Loop.Bench is required")
	}
	if l.Proposer == nil {
		return fmt.Errorf("selfimprove: Loop.Proposer is required")
	}
	if l.Applier == nil {
		return fmt.Errorf("selfimprove: Loop.Applier is required")
	}
	if cfg.Suite == "" {
		return fmt.Errorf("selfimprove: LoopConfig.Suite is required")
	}
	if cfg.Mode == "" {
		return fmt.Errorf("selfimprove: LoopConfig.Mode is required")
	}
	if cfg.Suite != l.Bench.Suite() {
		return fmt.Errorf("selfimprove: bench suite %q does not match config suite %q",
			l.Bench.Suite(), cfg.Suite)
	}
	return nil
}
