package main

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/vbonnet/dear-agent/pkg/benchmarks"
	"github.com/vbonnet/dear-agent/pkg/selfimprove"
)

var (
	selfImproveSuiteFlag     string
	selfImproveModeFlag      string
	selfImproveModelFlag     string
	selfImproveBudgetFlag    float64
	selfImproveMaxCyclesFlag int
	selfImproveLimitFlag     int
	selfImproveProposersFlag []string
	selfImproveNoGateFlag    bool
)

var selfImproveCmd = &cobra.Command{
	Use:   "selfimprove",
	Short: "Run the recursive self-improvement loop against a benchmark suite",
	Long: `Run the recursive self-improvement loop on a coding benchmark.

Cycle: run → analyze → propose → apply → re-run → compare → accept-or-revert.

Default proposer is PatternProposer (no LLM calls — uses analyzer-derived
hypotheses for end-to-end plumbing). Default applier is NoopApplier (records
hypotheses without touching the working tree). Real proposers / appliers
plug in via the pkg/selfimprove interfaces.

The regression gate is on by default: any cycle that drops solve_rate is
reverted and the loop stops.

Examples:
  agm selfimprove --suite swe-bench-lite --budget 50
  agm selfimprove --suite swe-bench-lite --max-cycles 3 --proposer-models claude,gpt-4`,
	Args: cobra.NoArgs,
	RunE: runSelfImprove,
}

func init() {
	rootCmd.AddCommand(selfImproveCmd)
	selfImproveCmd.Flags().StringVar(&selfImproveSuiteFlag, "suite", "", "Benchmark suite to optimize (required)")
	selfImproveCmd.Flags().StringVar(&selfImproveModeFlag, "mode", string(benchmarks.ModeDearAgent),
		"Execution mode: dear-agent or raw")
	selfImproveCmd.Flags().StringVar(&selfImproveModelFlag, "model", "claude-opus-4-7",
		"Model identifier passed to the executor")
	selfImproveCmd.Flags().Float64Var(&selfImproveBudgetFlag, "budget", 0,
		"Total spending cap in USD across all cycles (0 = unbounded)")
	selfImproveCmd.Flags().IntVar(&selfImproveMaxCyclesFlag, "max-cycles", 1,
		"Maximum number of improvement cycles")
	selfImproveCmd.Flags().IntVar(&selfImproveLimitFlag, "limit", 0,
		"Limit per benchmark run (0 = run all tasks)")
	selfImproveCmd.Flags().StringSliceVar(&selfImproveProposersFlag, "proposer-models", nil,
		"Comma-separated list of models to ask for hypotheses (default: --model)")
	selfImproveCmd.Flags().BoolVar(&selfImproveNoGateFlag, "no-regression-gate", false,
		"Disable the regression gate (a regressing cycle will not be reverted)")
	_ = selfImproveCmd.MarkFlagRequired("suite")
}

func runSelfImprove(cmd *cobra.Command, _ []string) error {
	suite := benchmarks.Suite(selfImproveSuiteFlag)
	bench, err := benchmarks.DefaultRegistry.Get(suite)
	if err != nil {
		return fmt.Errorf("%w\nregistered suites: %s", err, suiteList())
	}

	mode := benchmarks.Mode(selfImproveModeFlag)
	if mode != benchmarks.ModeDearAgent && mode != benchmarks.ModeRaw {
		return fmt.Errorf("invalid --mode %q (want dear-agent or raw)", selfImproveModeFlag)
	}

	loop := selfimprove.New(bench, selfimprove.PatternProposer{}, selfimprove.NoopApplier{}, nil)

	cfg := selfimprove.LoopConfig{
		Suite:          suite,
		Mode:           mode,
		Model:          selfImproveModelFlag,
		MaxCycles:      selfImproveMaxCyclesFlag,
		BudgetUSD:      selfImproveBudgetFlag,
		ProposerModels: selfImproveProposersFlag,
		RegressionGate: !selfImproveNoGateFlag,
		BenchLimit:     selfImproveLimitFlag,
	}

	cycles, err := loop.Run(cmd.Context(), cfg)
	if err != nil {
		return err
	}

	return printResult(cycles, func() { printSelfImproveCycles(cycles) })
}

func printSelfImproveCycles(cycles []selfimprove.CycleResult) {
	fmt.Printf("Self-improvement cycles: %d\n\n", len(cycles))
	for _, c := range cycles {
		verdict := "REJECTED"
		if c.Accepted {
			verdict = "ACCEPTED"
		}
		fmt.Printf("Cycle %d  %s  %s\n", c.Cycle, verdict, c.Reason)
		if c.Delta != nil {
			fmt.Printf("  solve_rate %+.4f  cost_usd %+.4f  cost_per_solved %+.4f\n",
				c.Delta.SolveRateDelta, c.Delta.CostUSDDelta, c.Delta.CostPerSolvedDelta)
			if len(c.Delta.Improvements) > 0 {
				fmt.Printf("  newly solved: %s\n", strings.Join(c.Delta.Improvements, ", "))
			}
			if len(c.Delta.Regressions) > 0 {
				fmt.Printf("  regressed:    %s\n", strings.Join(c.Delta.Regressions, ", "))
			}
		}
		fmt.Println()
	}
}
