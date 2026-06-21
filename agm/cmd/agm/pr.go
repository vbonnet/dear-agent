package main

import (
	"github.com/spf13/cobra"
)

var prCmd = &cobra.Command{
	Use:   "pr",
	Short: "Pull-request hygiene commands",
	Long: `PR commands surface pull-request / bead mismatches that supervisors
would otherwise miss.

Examples:
  agm pr scan-orphaned   # Flag open PRs whose tracking bead is already closed`,
	Args: cobra.ArbitraryArgs,
	RunE: groupRunE,
}

func init() {
	rootCmd.AddCommand(prCmd)
}
