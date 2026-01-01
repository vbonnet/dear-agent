package main

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/vbonnet/ai-tools/claude-session-manager/internal/manifest"
	"github.com/vbonnet/ai-tools/claude-session-manager/internal/tmux"
	"github.com/vbonnet/ai-tools/claude-session-manager/internal/uuid"
)

var (
	verboseFlag bool
)

var getUuidCmd = &cobra.Command{
	Use:   "get-uuid [session-name]",
	Short: "Get Claude UUID for a session",
	Long: `Returns the Claude session UUID for a given session.

If session-name is not provided and running inside tmux, uses the current
tmux session. Otherwise, you must specify a session name (tmux session name
or CSM session name).

The UUID is output to stdout for easy use in scripts.

This command uses a 3-level fallback system to find UUIDs:
  1. CSM manifest lookup (for CSM-managed sessions)
  2. Claude history search (by /rename or timestamp)
  3. JSONL filename fallback (scans ~/.claude/projects/)

Use --verbose to see which discovery level succeeded.

Examples:
  # Get UUID for current tmux session
  csm get-uuid

  # Get UUID for specific session by tmux name
  csm get-uuid csm-resilience

  # Get UUID for specific session by CSM name
  csm get-uuid my-project

  # Show discovery path (verbose mode)
  csm get-uuid --verbose my-legacy-session

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

		// Create manifest search function for uuid.Discover
		findInManifests := func(name string) (*manifest.Manifest, error) {
			manifests, err := manifest.List(cfg.SessionsDir)
			if err != nil {
				return nil, fmt.Errorf("failed to list sessions: %w", err)
			}

			for _, m := range manifests {
				if m.Tmux.SessionName == name || m.Name == name {
					return m, nil
				}
			}
			return nil, fmt.Errorf("no CSM session found for: %s", name)
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
	rootCmd.AddCommand(getUuidCmd)
	getUuidCmd.Flags().BoolVarP(&verboseFlag, "verbose", "v", false, "Show UUID discovery path")
}
