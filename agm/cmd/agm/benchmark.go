package main

import (
	"github.com/spf13/cobra"
)

var benchmarkCmd = &cobra.Command{
	Use:   "benchmark",
	Short: "Run and analyze performance benchmarks",
	Long: `Benchmark commands run Go benchmarks and analyze results against
performance targets.

Examples:
  agm benchmark query                     # Run all benchmarks
  agm benchmark query --filter Lock       # Run only lock benchmarks
  agm benchmark query -o json             # JSON output`,
	Args: cobra.ArbitraryArgs,
	RunE: groupRunE,
}

func init() {
	rootCmd.AddCommand(benchmarkCmd)
}
