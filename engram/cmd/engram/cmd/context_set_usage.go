package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

var (
	setUsageSession string
)

var contextSetUsageCmd = &cobra.Command{
	Use:   "set-usage PERCENTAGE",
	Short: "Manually set context usage (for testing)",
	Long: `Manually set context usage percentage for testing purposes.

This command is primarily for testing and development. In production, context
usage is automatically detected from CLI sessions.

FLAGS
  --session ID    Session ID to update (optional)

EXAMPLES
  # Set current session to 75% usage
  $ engram context set-usage 75

  # Set specific session to 85% usage
  $ engram context set-usage 85 --session abc-123-def

NOTE
  This command is for testing only. Actual context monitoring uses auto-detection
  from Claude Code, Gemini CLI, OpenCode, or Codex sessions.`,
	Args: cobra.ExactArgs(1),
	RunE: runContextSetUsage,
}

func init() {
	contextCmd.AddCommand(contextSetUsageCmd)

	contextSetUsageCmd.Flags().StringVar(&setUsageSession, "session", "", "Session ID to update")
}

func runContextSetUsage(cmd *cobra.Command, args []string) error {
	// This is a placeholder for manual testing
	// In production, this would update AGM session manifest or similar

	percentage := args[0]

	if setUsageSession != "" {
		fmt.Printf("Would set session %s to %s%% usage\n", setUsageSession, percentage)
	} else {
		fmt.Printf("Would set current session to %s%% usage\n", percentage)
	}

	fmt.Println("\nNOTE: This is a placeholder for testing.")
	fmt.Println("In production, use AGM integration:")
	fmt.Println("  agm session set-context-usage PERCENTAGE --session NAME")

	return nil
}
