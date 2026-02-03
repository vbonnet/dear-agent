package main

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/vbonnet/ai-tools/claude-session-manager/internal/session"
)

var getSessionNameCmd = &cobra.Command{
	Use:   "get-session-name",
	Short: "Get CSM session name for current context",
	Long: `Returns the CSM session name if running in a CSM-managed tmux session.

This command auto-detects the current tmux session and looks up the corresponding
CSM session name from the manifest.

The session name is output to stdout for easy use in scripts and automation.

Examples:
  # Get session name (must be run inside CSM session)
  csm get-session-name

  # Use in shell script
  SESSION_NAME=$(csm get-session-name)
  echo "Current session: $SESSION_NAME"

  # Check if in CSM session
  if csm get-session-name >/dev/null 2>&1; then
    echo "In CSM session"
  else
    echo "Not in CSM session"
  fi

Exit codes:
  0 - Success (in CSM session)
  1 - Error (not in tmux or not a CSM session)`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		// Get current session name using shared function
		sessionName, err := session.GetCurrentSessionName(cfg.SessionsDir)
		if err != nil {
			return fmt.Errorf("failed to get session name: %w", err)
		}

		// Output session name to stdout
		fmt.Println(sessionName)
		return nil
	},
}

func init() {
	rootCmd.AddCommand(getSessionNameCmd)
}
