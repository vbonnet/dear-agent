package main

import (
	"github.com/spf13/cobra"
)

var testCmd = &cobra.Command{
	Use:   "test",
	Short: "Manage CSM test sessions",
	Long: `Create and manage isolated test sessions for CSM development and automation.

Test sessions use isolated tmux sessions (csm-test-*) and state directories (/tmp/csm-test-*).
They provide a clean environment for testing CSM functionality without affecting production sessions.

Available commands:
  create   - Create a new isolated test session with Claude started
  send     - Send a command to a test session
  capture  - Capture output from a test session
  cleanup  - Clean up test sessions

Examples:
  csm test create my-test                  # Create test session
  csm test send my-test "csm list"         # Send command
  csm test capture my-test                 # Capture output
  csm test cleanup my-test                 # Cleanup session`,
}

func init() {
	rootCmd.AddCommand(testCmd)
}
