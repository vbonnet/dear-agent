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
  agm pr scan-orphaned    # Flag open PRs whose tracking bead is already closed
  agm pr scan-no-checks   # Flag open PRs whose head SHA has 0 required CI check-runs`,
	Args: cobra.ArbitraryArgs,
	RunE: groupRunE,
}

func init() {
	rootCmd.AddCommand(prCmd)
}
