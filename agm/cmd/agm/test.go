package main

import (
	"github.com/spf13/cobra"
)

var testCmd = &cobra.Command{
	Use:   "test",
	Short: "Test session utilities (legacy)",
	Long: `Test session utilities for AGM development and automation.

RECOMMENDED APPROACH:
Use common commands with --test flag for test session isolation:

  agm session new --test <name>           # Create test session in ~/sessions-test/
  agm session list --test                 # List test sessions
  agm admin doctor --test               # Check test session health

Test sessions are isolated from production:
- Tmux sessions: agm-test-* (separate from production)
- Sessions directory: ~/sessions-test/ (not ~/sessions/)
- Working directory: configurable per session

LEGACY COMMANDS:
This command group contains backward-compatibility utilities.
New workflows should use --test flag on common commands instead.`,
	Args: cobra.ArbitraryArgs,
	RunE: groupRunE,
}

func init() {
	rootCmd.AddCommand(testCmd)
}
