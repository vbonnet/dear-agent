package main

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/vbonnet/ai-tools/claude-session-manager/internal/manifest"
	"github.com/vbonnet/ai-tools/claude-session-manager/internal/tmux"
)

var getUuidCmd = &cobra.Command{
	Use:   "get-uuid [session-name]",
	Short: "Get Claude UUID for a session",
	Long: `Returns the Claude session UUID for a given session.

If session-name is not provided and running inside tmux, uses the current
tmux session. Otherwise, you must specify a session name (tmux session name
or CSM session name).

The UUID is output to stdout for easy use in scripts.

Examples:
  # Get UUID for current tmux session
  csm get-uuid

  # Get UUID for specific session by tmux name
  csm get-uuid csm-resilience

  # Get UUID for specific session by CSM name
  csm get-uuid my-project

  # Use in shell script
  UUID=$(csm get-uuid)
  echo "Current session: $UUID"`,
	Args: cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		var sessionName string

		// Determine which session to look up
		if len(args) > 0 {
			// User provided session name
			sessionName = args[0]
		} else {
			// No argument - try to get current tmux session
			var err error
			sessionName, err = tmux.GetCurrentSessionName()
			if err != nil {
				return fmt.Errorf("not running in tmux and no session name provided: %w", err)
			}
		}

		// List all manifests to find matching session
		manifests, err := manifest.List(cfg.SessionsDir)
		if err != nil {
			return fmt.Errorf("failed to list sessions: %w", err)
		}

		// Find manifest by tmux session name or CSM session name
		var found *manifest.Manifest
		for _, m := range manifests {
			if m.Tmux.SessionName == sessionName || m.Name == sessionName {
				found = m
				break
			}
		}

		if found == nil {
			return fmt.Errorf("no CSM session found for: %s", sessionName)
		}

		// Verify UUID exists
		if found.Claude.UUID == "" {
			return fmt.Errorf("session exists but has no Claude UUID (session: %s)", found.SessionID)
		}

		// Output UUID to stdout
		fmt.Println(found.Claude.UUID)
		return nil
	},
}

func init() {
	rootCmd.AddCommand(getUuidCmd)
}
