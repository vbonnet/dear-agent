package main

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/vbonnet/dear-agent/agm/internal/dolt"
	"github.com/vbonnet/dear-agent/agm/internal/history"
	"github.com/vbonnet/dear-agent/agm/internal/manifest"
	"github.com/vbonnet/dear-agent/agm/internal/tmux"
	"github.com/vbonnet/dear-agent/agm/internal/uuid"
)

var (
	verboseFlag bool
)

var getUUIDCmd = &cobra.Command{
	Use:   "get-uuid [session-name]",
	Short: "Get Claude UUID for a session",
	Long: `Returns the Claude session UUID for a given session.

If session-name is not provided:
  - If running inside tmux: uses the current tmux session
  - If not in tmux: auto-detects the current Claude session from history.jsonl

The UUID is output to stdout for easy use in scripts.

This command uses a 3-level fallback system to find UUIDs (when session-name is provided):
  1. AGM manifest lookup (for AGM-managed sessions), verified against /rename
  2. Claude history search (by /rename command only - strong signal)
  3. JSONL filename fallback (scans ~/.claude/projects/)

Note: Timestamp-based search has been removed to prevent returning wrong UUIDs.

Use --verbose to see which discovery level succeeded.

Examples:
  # Get UUID for current session (auto-detect from history if not in tmux)
  agm get-uuid

  # Get UUID for specific session by tmux name
  agm get-uuid csm-resilience

  # Get UUID for specific session by AGM name
  agm get-uuid my-project

  # Show discovery path (verbose mode)
  agm get-uuid --verbose my-legacy-session

  # Use in shell script
  UUID=$(agm get-uuid)
  echo "Current session: $UUID"`,
	Args: cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		var sessionName string

		// Determine which session to look up
		if len(args) > 0 {
			// User provided session name
			sessionName = args[0]
		} else {
			// No argument - try to get current tmux session first
			tmuxSessionName, err := tmux.GetCurrentSessionName()
			if err == nil {
				// In tmux, use tmux session name
				sessionName = tmuxSessionName
			} else {
				// Not in tmux - auto-detect current Claude session from history
				parser := history.NewParser("")
				sessions, err := parser.ReadConversations(1) // Get most recent session
				if err != nil {
					return fmt.Errorf("not in tmux and failed to auto-detect Claude session: %w", err)
				}
				if len(sessions) == 0 || sessions[0].SessionID == "" {
					return fmt.Errorf("not in tmux and no recent Claude sessions found in history")
				}

				// Return the auto-detected UUID directly
				fmt.Println(sessions[0].SessionID)
				return nil
			}
		}

		// Get Dolt storage adapter
		adapter, err := getStorage()
		if err != nil {
			return fmt.Errorf("failed to connect to Dolt storage: %w", err)
		}
		defer adapter.Close()

		// Create manifest search function for uuid.Discover
		findInManifests := func(name string) (*manifest.Manifest, error) {
			manifests, err := adapter.ListSessions(&dolt.SessionFilter{})
			if err != nil {
				return nil, fmt.Errorf("failed to list sessions: %w", err)
			}

			for _, m := range manifests {
				if m.Tmux.SessionName == name || m.Name == name {
					return m, nil
				}
			}
			return nil, fmt.Errorf("no AGM session found for: %s", name)
		}

		// Use 3-level fallback to discover UUID
		discoveredUUID, err := uuid.Discover(sessionName, findInManifests, verboseFlag)
		if err != nil {
			return err
		}

		// Output UUID to stdout
		fmt.Println(discoveredUUID)
		return nil
	},
}

func init() {
	rootCmd.AddCommand(getUUIDCmd)
	getUUIDCmd.Flags().BoolVarP(&verboseFlag, "verbose", "v", false, "Show UUID discovery path")
}
