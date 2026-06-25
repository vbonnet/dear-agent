package main

import (
	"fmt"
	"time"

	"github.com/spf13/cobra"
)

var sessionUpdateModeCmd = &cobra.Command{
	Use:   "update-mode <session-id> <mode>",
	Short: "Update permission mode for a session",
	Long: `Update the stored permission mode for a session.

Valid modes:
  default - Default permission level
  plan    - Plan mode
  auto    - Auto-approve mode
  ask     - Legacy alias retained for older hook payloads
  allow   - Legacy alias retained for older hook payloads

This command is typically called by hooks to track mode changes
automatically, but can also be used manually.

Examples:
  agm session update-mode my-session auto
  agm session update-mode abc123 default`,
	Args: cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		sessionID := args[0]
		mode := args[1]

		// Validate mode
		validModes := map[string]bool{
			"default": true,
			"plan":    true,
			"auto":    true,
			"ask":     true,
			"allow":   true,
		}
		if !validModes[mode] {
			return fmt.Errorf("invalid mode: %s (valid modes: default, plan, auto, ask, allow)", mode)
		}
		if mode == "allow" {
			mode = "auto"
		}

		// Get storage adapter
		adapter, err := getStorage()
		if err != nil {
			return fmt.Errorf("failed to connect to Dolt: %w", err)
		}
		defer func() { _ = adapter.Close() }()

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
