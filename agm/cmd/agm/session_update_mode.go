package main

import (
	"fmt"
	"time"

	"github.com/spf13/cobra"
)

var sessionUpdateModeCmd = &cobra.Command{
	Use:   "update-mode <session-id> <mode>",
	Short: "Update permission mode for a session",
	Long: `Update the Claude Code permission mode for a session.

Valid modes:
  default - Default permission level (prompts for each tool)
  plan    - Plan mode (shows tool plan before execution)
  ask     - Ask mode (prompts for confirmation before each tool)
  allow   - Allow mode (executes tools without prompting)

This command is typically called by the PreToolUse hook to track
mode changes automatically, but can also be used manually.

Examples:
  agm session update-mode my-session plan
  agm session update-mode abc123 default`,
	Args: cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		sessionID := args[0]
		mode := args[1]

		// Validate mode
		validModes := map[string]bool{
			"default": true,
			"plan":    true,
			"ask":     true,
			"allow":   true,
		}
		if !validModes[mode] {
			return fmt.Errorf("invalid mode: %s (valid modes: default, plan, ask, allow)", mode)
		}

		// Get storage adapter
		adapter, err := getStorage()
		if err != nil {
			return fmt.Errorf("failed to connect to Dolt: %w", err)
		}
		defer adapter.Close()

		// Get session manifest
		m, err := adapter.GetSession(sessionID)
		if err != nil {
			return fmt.Errorf("failed to get session: %w", err)
		}

		// Update permission mode fields
		m.PermissionMode = mode
		now := time.Now()
		m.PermissionModeUpdatedAt = &now
		m.PermissionModeSource = "hook"

		// Save updated manifest
		if err := adapter.UpdateSession(m); err != nil {
			return fmt.Errorf("failed to update session: %w", err)
		}

		return nil
	},
}

func init() {
	sessionCmd.AddCommand(sessionUpdateModeCmd)
}
