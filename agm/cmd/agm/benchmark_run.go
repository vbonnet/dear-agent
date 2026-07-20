package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/vbonnet/dear-agent/pkg/benchmarks"
)

var (
	benchRunSuiteFlag         string
	benchRunModeFlag          string
	benchRunModelFlag         string
	benchRunLimitFlag         int
	benchRunBudgetFlag        float64
	benchRunResultsDirFlag    string
	benchRunTasksFileFlag     string
	benchRunFilterBySuiteFlag bool
)

var benchmarkRunCmd = &cobra.Command{
	Use:   "run",
	Short: "Run a coding benchmark suite (SWE-Bench Lite/Verified, SWE Atlas, Vibe Bench)",
	Long: `Run a registered coding benchmark suite end-to-end.

Suites are discovered from pkg/benchmarks.DefaultRegistry. The mode flag
selects "dear-agent" (DEAR protocol enforced) or "raw" (model+harness only)
so the same suite can be A/B compared.

Without a real TaskExecutor wired in, the scaffold runs against built-in
fixture tasks and reports them as errored — by design, so dry runs cannot
silently report misleading scores.

Examples:
  agm benchmark run --suite swe-bench-lite --mode dear-agent
  agm benchmark run --suite swe-bench-lite --mode raw --model claude-opus-4-7
  agm benchmark run --suite vibe-bench --limit 5 --results-dir .dear-agent/benchmarks/
  agm benchmark run --suite swe-bench-lite --tasks-file ./swe-bench-lite-tasks.json`,
	Args: cobra.NoArgs,
	RunE: runBenchmarkRun,
}

func init() {
	benchmarkCmd.AddCommand(benchmarkRunCmd)
	benchmarkRunCmd.Flags().StringVar(&benchRunSuiteFlag, "suite", "", "Benchmark suite to run (required)")
	benchmarkRunCmd.Flags().StringVar(&benchRunModeFlag, "mode", string(benchmarks.ModeDearAgent),
		"Execution mode: dear-agent or raw")
	benchmarkRunCmd.Flags().StringVar(&benchRunModelFlag, "model", "claude-opus-4-7",
		"Model identifier passed to the executor")
	benchmarkRunCmd.Flags().IntVar(&benchRunLimitFlag, "limit", 0, "Maximum number of tasks to run (0 = all)")
	benchmarkRunCmd.Flags().Float64Var(&benchRunBudgetFlag, "budget", 0, "Spending cap in USD (0 = unbounded)")
	benchmarkRunCmd.Flags().StringVar(&benchRunResultsDirFlag, "results-dir", "",
		"Directory to write the JSON results file (e.g. .dear-agent/benchmarks/)")
	benchmarkRunCmd.Flags().StringVar(&benchRunTasksFileFlag, "tasks-file", "",
		"Path to a JSON or NDJSON file of TaskSpecs (overrides the suite's built-in fixture loader)")
	benchmarkRunCmd.Flags().BoolVar(&benchRunFilterBySuiteFlag, "filter-by-suite", false,
		"With --tasks-file: only run tasks whose suite field matches --suite")
	_ = benchmarkRunCmd.MarkFlagRequired("suite")
}

func runBenchmarkRun(cmd *cobra.Command, _ []string) error {
	suite := benchmarks.Suite(benchRunSuiteFlag)
	bench, err := benchmarks.DefaultRegistry.Get(suite)
	if err != nil {
		return fmt.Errorf("%w\nregistered suites: %s", err, suiteList())
	}

	mode := benchmarks.Mode(benchRunModeFlag)
	if mode != benchmarks.ModeDearAgent && mode != benchmarks.ModeRaw {
		return fmt.Errorf("invalid --mode %q (want dear-agent or raw)", benchRunModeFlag)
	}

	cfg := benchmarks.RunConfig{
		Mode:       mode,
		Model:      benchRunModelFlag,
		Limit:      benchRunLimitFlag,
		BudgetUSD:  benchRunBudgetFlag,
		ResultsDir: benchRunResultsDirFlag,
	}
	if benchRunTasksFileFlag != "" {
		cfg.Loader = &benchmarks.JSONFileLoader{
			Path:          benchRunTasksFileFlag,
			FilterBySuite: benchRunFilterBySuiteFlag,
		}
	}

	results, err := bench.Run(cmd.Context(), cfg)
	if err != nil {
		return fmt.Errorf("run %s: %w", suite, err)
	}

	if benchRunResultsDirFlag != "" {
		path, werr := benchmarks.WriteResults(results, benchRunResultsDirFlag)
		if werr != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to persist results: %v\n", werr)
		} else {
			fmt.Fprintf(os.Stderr, "Results written to %s\n", path)
		}
	}

	return printResult(results, func() { printBenchmarkResults(results) })
}

func suiteList() string {
	suites := benchmarks.DefaultRegistry.List()
	parts := make([]string, len(suites))
	for i, s := range suites {
		parts[i] = string(s)
	}
	return strings.Join(parts, ", ")
}

func printBenchmarkResults(r *benchmarks.Results) {
	fmt.Printf("Benchmark: %s  (mode=%s model=%s)\n", r.Suite, r.Mode, r.Model)
	fmt.Printf("Run ID: %s\n", r.RunID)
	fmt.Printf("Started: %s    Finished: %s\n", r.StartedAt.Format("15:04:05"), r.FinishedAt.Format("15:04:05"))
	fmt.Println()

	for _, t := range r.Tasks {
		status := "FAIL"
		if t.Solved {
			status = "PASS"
		}
		if t.Error != "" {
			status = "ERR "
		}
		errMsg := t.Error
		if len(errMsg) > 60 {
			errMsg = errMsg[:60] + "..."
		}
		fmt.Printf("  %-40s  %s  $%.4f  %dt/%dt  %s\n",
			t.TaskID, status, t.CostUSD, t.TokensIn, t.TokensOut, errMsg)
	}

	s := r.Summary
	fmt.Printf("\nSummary: %d/%d solved (%.1f%%) | failed=%d errored=%d\n",
		s.Solved, s.Total, s.SolveRate*100, s.Failed, s.Errored)
	fmt.Printf("Cost: $%.4f total | $%.4f per solved | %d/%d tokens (in/out)\n",
		s.TotalCostUSD, s.CostPerSolved, s.TotalTokensIn, s.TotalTokensOut)
	fmt.Printf("Wall: %s avg per task\n", s.AvgDuration)
}
