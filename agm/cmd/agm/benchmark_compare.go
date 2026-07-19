package main

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/vbonnet/dear-agent/pkg/benchmarks"
)

var (
	benchCompareBaselineFlag string
	benchCompareTestFlag     string
)

var benchmarkCompareCmd = &cobra.Command{
	Use:   "compare",
	Short: "Compare two benchmark runs and report the delta",
	Long: `Compare two persisted benchmark runs (written by 'agm benchmark run'
with --results-dir) and report the per-task and aggregate delta.

The convention is --baseline raw --test dear-agent so a positive
solve_rate_delta means dear-agent improved over the raw model+harness.

Examples:
  agm benchmark compare --baseline .dear-agent/benchmarks/raw.json --test .dear-agent/benchmarks/dear.json`,
	Args: cobra.NoArgs,
	RunE: runBenchmarkCompare,
}

func init() {
	benchmarkCmd.AddCommand(benchmarkCompareCmd)
	benchmarkCompareCmd.Flags().StringVar(&benchCompareBaselineFlag, "baseline", "",
		"Path to baseline results JSON (required)")
	benchmarkCompareCmd.Flags().StringVar(&benchCompareTestFlag, "test", "",
		"Path to test results JSON (required)")
	_ = benchmarkCompareCmd.MarkFlagRequired("baseline")
	_ = benchmarkCompareCmd.MarkFlagRequired("test")
}

func runBenchmarkCompare(cmd *cobra.Command, _ []string) error {
	baseline, err := benchmarks.ReadResults(benchCompareBaselineFlag)
	if err != nil {
		return fmt.Errorf("read baseline: %w", err)
	}
	test, err := benchmarks.ReadResults(benchCompareTestFlag)
	if err != nil {
		return fmt.Errorf("read test: %w", err)
	}

	bench, err := benchmarks.DefaultRegistry.Get(baseline.Suite)
	if err != nil {
		return err
	}

	delta, err := bench.Compare(baseline, test)
	if err != nil {
		return err
	}

	return printResult(delta, func() { printBenchmarkDelta(baseline, test, delta) })
}

func printBenchmarkDelta(baseline, test *benchmarks.Results, d *benchmarks.Delta) {
	fmt.Printf("Suite: %s\n", d.Suite)
	fmt.Printf("Baseline: %s  (mode=%s model=%s)\n", baseline.RunID, baseline.Mode, baseline.Model)
	fmt.Printf("    %d/%d solved (%.1f%%)  $%.4f total  $%.4f per solved\n",
		baseline.Summary.Solved, baseline.Summary.Total, baseline.Summary.SolveRate*100,
		baseline.Summary.TotalCostUSD, baseline.Summary.CostPerSolved)
	fmt.Printf("Test:     %s  (mode=%s model=%s)\n", test.RunID, test.Mode, test.Model)
	fmt.Printf("    %d/%d solved (%.1f%%)  $%.4f total  $%.4f per solved\n",
		test.Summary.Solved, test.Summary.Total, test.Summary.SolveRate*100,
		test.Summary.TotalCostUSD, test.Summary.CostPerSolved)

	verdict := "improved"
	if d.IsRegression {
		verdict = "REGRESSED"
	} else if d.SolveRateDelta == 0 {
		verdict = "neutral"
	}
	fmt.Printf("\nDelta (%s):\n", verdict)
	fmt.Printf("  solve_rate:      %+.4f\n", d.SolveRateDelta)
	fmt.Printf("  cost_usd:        %+.4f\n", d.CostUSDDelta)
	fmt.Printf("  cost_per_solved: %+.4f\n", d.CostPerSolvedDelta)
	if len(d.Improvements) > 0 {
		fmt.Printf("  newly solved:    %v\n", d.Improvements)
	}
	if len(d.Regressions) > 0 {
		fmt.Printf("  regressed:       %v\n", d.Regressions)
	}
}
