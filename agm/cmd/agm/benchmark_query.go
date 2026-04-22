package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"
	"github.com/vbonnet/dear-agent/agm/internal/benchmark"
)

var benchmarkFilterFlag string

var benchmarkQueryCmd = &cobra.Command{
	Use:   "query",
	Short: "Run benchmarks and display results with target comparison",
	Long: `Run Go benchmarks and display results compared against performance targets.

Results are shown in a table with pass/fail status for benchmarks that have
documented performance targets.

Examples:
  agm benchmark query                     # Run all benchmarks
  agm benchmark query --filter Lock       # Run only lock benchmarks
  agm benchmark query -o json             # JSON output`,
	Args: cobra.NoArgs,
	RunE: runBenchmarkQuery,
}

func init() {
	benchmarkCmd.AddCommand(benchmarkQueryCmd)
	benchmarkQueryCmd.Flags().StringVar(&benchmarkFilterFlag, "filter", ".", "Benchmark name filter pattern (passed to go test -bench flag)")
}

// findModuleRoot walks up from dir looking for go.mod with the expected module path.
func findModuleRoot(dir string) (string, error) {
	const modulePath = "github.com/vbonnet/dear-agent/agm"
	for {
		gomod := filepath.Join(dir, "go.mod")
		if data, err := os.ReadFile(gomod); err == nil {
			if strings.Contains(string(data), modulePath) {
				return dir, nil
			}
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("could not find go.mod for %s", modulePath)
		}
		dir = parent
	}
}

// getProjectRoot returns the project root directory for running benchmarks.
func getProjectRoot() (string, error) {
	if envDir := os.Getenv("AGM_SOURCE_DIR"); envDir != "" {
		return envDir, nil
	}

	cwd, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("failed to get working directory: %w", err)
	}

	return findModuleRoot(cwd)
}

func runBenchmarkQuery(cmd *cobra.Command, args []string) error {
	projectRoot, err := getProjectRoot()
	if err != nil {
		return fmt.Errorf("cannot locate project root: %w\nSet AGM_SOURCE_DIR to the agm directory", err)
	}

	goTest := exec.Command("go", "test",
		"-bench="+benchmarkFilterFlag,
		"-benchmem",
		"-run=^$",
		"./test/",
	)
	goTest.Dir = projectRoot
	goTest.Env = append(os.Environ(), "CGO_ENABLED=1")

	output, runErr := goTest.CombinedOutput()
	outputStr := string(output)

	results, parseErr := benchmark.ParseBenchmarkOutput(outputStr)
	if parseErr != nil {
		if runErr != nil {
			return fmt.Errorf("benchmark run failed: %w\n%s", runErr, outputStr)
		}
		return parseErr
	}

	if len(results) == 0 {
		fmt.Fprintln(os.Stderr, "No benchmark results found.")
		if runErr != nil {
			return fmt.Errorf("go test error: %w\n%s", runErr, outputStr)
		}
		return nil
	}

	targets := benchmark.DefaultTargets()
	report := benchmark.Evaluate(results, targets)

	return printResult(report, func() {
		printBenchmarkTable(report)
	})
}

func printBenchmarkTable(report *benchmark.BenchmarkReport) {
	w := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
	fmt.Fprintln(w, "NAME\tDURATION\tTARGET\tSTATUS")
	fmt.Fprintln(w, "----\t--------\t------\t------")

	for _, eval := range report.Evaluations {
		dur := formatDuration(eval.Result.Duration)
		target := "-"
		status := "-"

		if eval.Target != nil {
			target = "<" + formatDuration(eval.Target.MaxDuration)
			if eval.Pass {
				status = "PASS"
			} else {
				status = "FAIL"
			}
		}

		fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", eval.Result.Name, dur, target, status)
	}

	w.Flush()

	fmt.Printf("\nSummary: %d passed, %d failed, %d no target",
		report.Summary.Passed, report.Summary.Failed, report.Summary.NoTarget)
	targeted := report.Summary.Passed + report.Summary.Failed
	if targeted > 0 {
		fmt.Printf(" (%d/%d targets met)", report.Summary.Passed, targeted)
	}
	fmt.Println()
}

func formatDuration(d time.Duration) string {
	switch {
	case d >= time.Second:
		return fmt.Sprintf("%.2fs", d.Seconds())
	case d >= time.Millisecond:
		return fmt.Sprintf("%.2fms", float64(d)/float64(time.Millisecond))
	case d >= time.Microsecond:
		return fmt.Sprintf("%.2fus", float64(d)/float64(time.Microsecond))
	default:
		return fmt.Sprintf("%dns", d.Nanoseconds())
	}
}
