package main

import (
	"github.com/spf13/cobra"
)

var sessionCmd = &cobra.Command{
	Use:   "session",
	Short: "Manage AGM session lifecycle",
	Long: `Session commands handle the full lifecycle of AGM-managed harness sessions including
creation, resumption, termination, archiving, and message management.

Examples:
  agm session new my-project      # Create new session
  agm session resume my-project   # Resume existing session
  agm session kill my-project     # Terminate session
  agm session list                # List all sessions
  agm session archive my-project  # Archive session`,
	Args: cobra.ArbitraryArgs,
	RunE: groupRunE,
}

func init() {
	rootCmd.AddCommand(sessionCmd)
}
