package main

import (
	"github.com/spf13/cobra"
)

var testCmd = &cobra.Command{
	Use:   "test",
	Short: "Test session utilities (legacy)",
	Long: `Test session utilities for CSM development and automation.

RECOMMENDED APPROACH:
Use common commands with --test flag for test session isolation:

  csm new --test <name>           # Create test session in ~/sessions-test/
  csm list --test                 # List test sessions
  csm doctor --test               # Check test session health

Test sessions are isolated from production:
- Tmux sessions: csm-test-* (separate from production)
- Sessions directory: ~/sessions-test/ (not ~/sessions/)
- Working directory: configurable per session

LEGACY COMMANDS:
This command group contains backward-compatibility utilities.
New workflows should use --test flag on common commands instead.`,
}

func init() {
	rootCmd.AddCommand(testCmd)
}
