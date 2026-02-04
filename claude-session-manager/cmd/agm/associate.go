package main

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/google/uuid"
	"github.com/spf13/cobra"
	"github.com/vbonnet/ai-tools/claude-session-manager/internal/detection"
	"github.com/vbonnet/ai-tools/claude-session-manager/internal/manifest"
	"github.com/vbonnet/ai-tools/claude-session-manager/internal/readiness"
	"github.com/vbonnet/ai-tools/claude-session-manager/internal/session"
	"github.com/vbonnet/ai-tools/claude-session-manager/internal/ui"
	uuidpkg "github.com/vbonnet/ai-tools/claude-session-manager/internal/uuid"
)

var (
	claudeUUID          string
	createNew           bool
	updateTimestampOnly bool
	autoDetectOnly      bool
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
			// Auto-detect using 3-level fallback discovery system
			fmt.Println("Auto-detecting Claude session UUID...")

			// Create manifest search function for uuid.Discover
			findInManifests := func(name string) (*manifest.Manifest, error) {
				manifests, err := manifest.List(sessionsDir)
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
			var err error
			targetUUID, err = uuidpkg.Discover(sessionName, findInManifests, false)
			if err != nil {
				ui.PrintError(err, "Failed to detect Claude UUID",
					"  • Tried CSM manifest lookup, history search by /rename, and timestamp search\n"+
						"  • Ensure Claude has processed at least one message\n"+
						"  • Or specify UUID manually with --uuid flag")
				return err
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
				ui.PrintError(err,
					"Failed to create manifest directory",
					"  • Check sessions directory: ls -ld "+sessionsDir+"\n"+
						"  • Verify disk space: df -h "+sessionsDir+"\n"+
						"  • Check permissions: ls -ld "+filepath.Dir(manifestDir))
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
			fmt.Printf("Session association in progress: updated timestamp for '%s'\n", sessionName)
		} else {
			// Normal path: Update UUID
			m.Claude.UUID = targetUUID

			// Generate SessionID if not present
			if m.SessionID == "" {
				m.SessionID = uuid.New().String()
			}

			// Report progress (softer message to allow skill continuation)
			if oldUUID == "" {
				fmt.Printf("Session association in progress: '%s' linked to Claude UUID %s\n", sessionName, targetUUID[:8])
			} else {
				fmt.Printf("Session association in progress: '%s' updated %s → %s\n", sessionName, oldUUID[:8], targetUUID[:8])
			}
		}

		// Write manifest (automatic backup will be created if file exists)
		if err := manifest.Write(manifestPath, m); err != nil {
			ui.PrintManifestWriteError(err)
			return err
		}

		// Create ready-file to signal Claude is initialized
		if err := readiness.CreateReadyFile(sessionName, manifestPath); err != nil {
			// Non-fatal: ready-file creation failed, but association succeeded
			fmt.Printf("Warning: Failed to create ready-file signal: %v\n", err)
		}

		fmt.Printf("\nManifest: %s\n", manifestPath)

		// Show completion with softer language to allow skill continuation
		fmt.Printf("\nSession association complete. You can now proceed to the next step.\n")
		fmt.Printf("To resume this session:\n")
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
