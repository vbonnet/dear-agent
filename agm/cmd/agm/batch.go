package main

import (
	"github.com/spf13/cobra"
)

var batchCmd = &cobra.Command{
	Use:   "batch",
	Short: "Batch operations across multiple sessions",
	Long: `Batch commands operate on multiple sessions at once.

Examples:
  agm batch spawn --manifest workers.yaml  # Launch workers from manifest
  agm batch status                          # Show all active workers
  agm batch verify --sessions "s1,s2"       # Verify session completions
  agm batch verify --all                    # Verify all archived sessions
  agm batch merge --repo-dir /path/to/repo  # Cherry-pick from verified workers`,
	Args: cobra.ArbitraryArgs,
	RunE: groupRunE,
}

func init() {
	rootCmd.AddCommand(batchCmd)
}
