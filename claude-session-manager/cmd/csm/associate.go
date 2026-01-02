package main

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/google/uuid"
	"github.com/spf13/cobra"
	"github.com/vbonnet/ai-tools/claude-session-manager/internal/claude"
	"github.com/vbonnet/ai-tools/claude-session-manager/internal/detection"
	"github.com/vbonnet/ai-tools/claude-session-manager/internal/manifest"
	"github.com/vbonnet/ai-tools/claude-session-manager/internal/session"
	"github.com/vbonnet/ai-tools/claude-session-manager/internal/ui"
)

var (
	claudeUUID            string
	createNew             bool
	updateTimestampOnly   bool
	autoDetectOnly        bool
)

var associateCmd = &cobra.Command{
	Use:   "associate <session-name>",
	Short: "Associate a CSM session with the current Claude session UUID",
	Long: `Associate a CSM session with a Claude session UUID by updating the manifest.

This command is useful when:
- You started a Claude session outside of tmux and want to track it
- You want to reassign a different Claude UUID to an existing CSM session
- You're reconnecting an existing session after the UUID changed

The command will:
1. Get the current Claude session UUID (from history.jsonl latest entry)
2. Find or create the manifest for the specified CSM session
3. Create a backup of the existing manifest (if one exists)
4. Update the manifest with the new Claude UUID

Examples:
  # Associate current Claude session with CSM session "claude-1"
  csm associate claude-1

  # Specify a specific Claude UUID instead of auto-detecting
  csm associate claude-1 --uuid c86ffd41-cbcc-4bfa-8b1f-4da7c83fc3d2

  # Create a new manifest if it doesn't exist
  csm associate my-new-session --create`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		sessionName := args[0]
		sessionsDir := getSessionsDir()

		// Handle --auto-detect-only mode (for hooks)
		if autoDetectOnly {
			// Try to find existing manifest
			m, manifestPath, err := session.ResolveIdentifier(sessionName, sessionsDir)
			if err != nil {
				// No manifest found, exit silently (hook mode)
				return nil
			}

			// If UUID already set, nothing to do
			if m.Claude.UUID != "" {
				return nil
			}

			// Attempt high-confidence auto-detection
			historyPath := filepath.Join(os.Getenv("HOME"), ".claude", "history.jsonl")
			detector := detection.NewDetector(historyPath, 5*time.Minute)
			result, err := detector.DetectUUID(m)

			if err == nil && result.UUID != "" && result.Confidence == "high" {
				// High-confidence match found, update manifest
				m.Claude.UUID = result.UUID
				m.UpdatedAt = time.Now()
				if err := manifest.Write(manifestPath, m); err != nil {
					// Silently fail in hook mode
					return nil
				}
			}

			// Exit silently (hook mode - no user output)
			return nil
		}

		// Get or determine Claude UUID
		var targetUUID string
		if claudeUUID != "" {
			// User provided UUID explicitly
			targetUUID = claudeUUID
			// Validate UUID format
			if _, err := uuid.Parse(targetUUID); err != nil {
				ui.PrintError(err, "Invalid UUID format",
					"  • UUID must be in format: xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx")
				return err
			}
		} else {
			// Auto-detect from history.jsonl
			fmt.Println("Auto-detecting Claude session UUID from history...")
			historyPath := filepath.Join(os.Getenv("HOME"), ".claude", "history.jsonl")
			entries, _, err := claude.ParseHistory(historyPath)
			if err != nil {
				ui.PrintError(err, "Failed to read Claude history",
					"  • Ensure ~/.claude/history.jsonl exists\n"+
						"  • Run this command from within a Claude session\n"+
						"  • Or specify UUID manually with --uuid flag")
				return err
			}

			if len(entries) == 0 {
				return fmt.Errorf("no Claude sessions found in history")
			}

			// Get the most recent entry
			latest := entries[len(entries)-1]
			targetUUID = latest.SessionID

			// Verify it's recent (within last 30 seconds)
			entryTime := time.Unix(0, int64(latest.Timestamp)*int64(time.Millisecond))
			age := time.Since(entryTime)
			if age > 30*time.Second {
				ui.PrintWarning(fmt.Sprintf("Latest Claude session is %v old", age.Round(time.Second)))
				fmt.Println("  This may not be the current session.")
				fmt.Println("  Use --uuid flag to specify explicitly if needed.")
			}

			ui.PrintSuccess(fmt.Sprintf("Detected Claude UUID: %s", targetUUID[:8]))
		}

		// Try to find existing manifest
		manifestPath := ""
		var m *manifest.Manifest
		var err error

		// Try to resolve identifier to existing manifest
		m, manifestPath, err = session.ResolveIdentifier(sessionName, sessionsDir)
		if err != nil {
			// Manifest doesn't exist
			if !createNew {
				ui.PrintError(err, "Session not found",
					fmt.Sprintf("  • Use --create to create a new session\n"+
						"  • Or run: csm new %s", sessionName))
				return err
			}

			// Create new manifest
			fmt.Printf("Creating new CSM session: %s\n", sessionName)
			manifestDir := filepath.Join(sessionsDir, fmt.Sprintf("session-%s", sessionName))
			manifestPath = filepath.Join(manifestDir, "manifest.yaml")

			if err := os.MkdirAll(manifestDir, 0700); err != nil {
				ui.PrintError(err, "Failed to create manifest directory", "")
				return err
			}

			m = &manifest.Manifest{
				SchemaVersion: manifest.SchemaVersion,
				SessionID:     "", // Will be populated below
				Name:          sessionName,
				CreatedAt:     time.Now(),
				UpdatedAt:     time.Now(),
				Lifecycle:     "", // Empty = active
				Context: manifest.Context{
					Project: func() string {
					if wd, err := os.Getwd(); err == nil {
						return wd
					}
					return ""
				}(),
				},
				Claude: manifest.Claude{
					UUID: "", // Will be populated below
				},
				Tmux: manifest.Tmux{
					SessionName: sessionName,
				},
			}
		}

		// Update manifest based on flags
		oldUUID := m.Claude.UUID

		if updateTimestampOnly {
			// Fast path: Only update timestamp, don't change UUID
			// Timestamp is automatically updated by manifest.Write()
			ui.PrintSuccess(fmt.Sprintf("Updated timestamp for session '%s' (UUID unchanged: %s)", sessionName, oldUUID[:8]))
		} else {
			// Normal path: Update UUID
			m.Claude.UUID = targetUUID

			// Generate SessionID if not present
			if m.SessionID == "" {
				m.SessionID = uuid.New().String()
			}

			// Report success
			if oldUUID == "" {
				ui.PrintSuccess(fmt.Sprintf("Associated session '%s' with Claude UUID %s", sessionName, targetUUID[:8]))
			} else {
				ui.PrintSuccess(fmt.Sprintf("Updated session '%s': %s → %s", sessionName, oldUUID[:8], targetUUID[:8]))
			}
		}

		// Write manifest (automatic backup will be created if file exists)
		if err := manifest.Write(manifestPath, m); err != nil {
			ui.PrintError(err, "Failed to write manifest", "")
			return err
		}

		fmt.Printf("\nManifest: %s\n", manifestPath)

		// Show how to resume
		fmt.Printf("\nTo resume this session:\n")
		fmt.Printf("  csm resume %s\n", sessionName)

		return nil
	},
}

func init() {
	associateCmd.Flags().StringVar(&claudeUUID, "uuid", "", "Claude session UUID (auto-detected if not specified)")
	associateCmd.Flags().BoolVar(&createNew, "create", false, "Create new manifest if it doesn't exist")
	associateCmd.Flags().BoolVar(&updateTimestampOnly, "update-timestamp-only", false, "Only update timestamp, don't change UUID (fast path for same UUID)")
	associateCmd.Flags().BoolVar(&autoDetectOnly, "auto-detect-only", false, "Auto-detect UUID only if high confidence (for hooks, silent mode)")
	rootCmd.AddCommand(associateCmd)
}
