package main

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/vbonnet/dear-agent/agm/internal/session"
)

var getSessionNameCmd = &cobra.Command{
	Use:   "get-session-name",
	Short: "Get AGM session name for current context",
	Long: `Returns the AGM session name if running in an AGM-managed tmux session.

This command auto-detects the current tmux session and looks up the corresponding
AGM session name from the manifest.

The session name is output to stdout for easy use in scripts and automation.

Examples:
  # Get session name (must be run inside AGM session)
  agm get-session-name

  # Use in shell script
  SESSION_NAME=$(agm get-session-name)
  echo "Current session: $SESSION_NAME"

  # Check if in AGM session
  if agm get-session-name >/dev/null 2>&1; then
    echo "In AGM session"
  else
    echo "Not in AGM session"
  fi

Exit codes:
  0 - Success (in AGM session)
  1 - Error (not in tmux or not a AGM session)`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		// Get Dolt adapter
		adapter, err := getStorage()
		if err != nil {
			return fmt.Errorf("failed to connect to Dolt: %w", err)
		}
		defer adapter.Close()

		// Get current session name using shared function
		sessionName, err := session.GetCurrentSessionName(cfg.SessionsDir, adapter)
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
