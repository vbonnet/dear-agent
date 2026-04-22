package main

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/google/uuid"
	"github.com/spf13/cobra"
	"github.com/vbonnet/dear-agent/agm/internal/detection"
	"github.com/vbonnet/dear-agent/agm/internal/dolt"
	"github.com/vbonnet/dear-agent/agm/internal/git"
	"github.com/vbonnet/dear-agent/agm/internal/history"
	"github.com/vbonnet/dear-agent/agm/internal/manifest"
	"github.com/vbonnet/dear-agent/agm/internal/readiness"
	"github.com/vbonnet/dear-agent/agm/internal/session"
	"github.com/vbonnet/dear-agent/agm/internal/tmux"
	"github.com/vbonnet/dear-agent/agm/internal/ui"
	uuidpkg "github.com/vbonnet/dear-agent/agm/internal/uuid"
)

var (
	claudeUUID          string
	createNew           bool
	updateTimestampOnly bool
	autoDetectOnly      bool
	renameSession       bool
)

var associateCmd = &cobra.Command{
	Use:   "associate <session-name>",
	Short: "Associate a AGM session with the current Claude session UUID (Claude-only)",
	Long: `Associate a AGM session with a Claude session UUID by updating the manifest.

NOTE: This command only works with Claude sessions. OpenCode, Gemini, and other
agents manage session state differently and do not require UUID association.

This command is useful when:
- You started a Claude session outside of tmux and want to track it
- You want to reassign a different Claude UUID to an existing AGM session
- You're reconnecting an existing session after the UUID changed

The command will:
1. Get the current Claude session UUID (from history.jsonl latest entry)
2. Find or create the manifest for the specified AGM session
3. Create a backup of the existing manifest (if one exists)
4. Update the manifest with the new Claude UUID

Examples:
  # Associate current Claude session with AGM session "claude-1"
  agm session associate claude-1

  # Specify a specific Claude UUID instead of auto-detecting
  agm session associate claude-1 --uuid c86ffd41-cbcc-4bfa-8b1f-4da7c83fc3d2

  # Create a new manifest if it doesn't exist
  agm session associate my-new-session --create

  # Combined rename + associate (sends /rename to Claude, then associates)
  agm session associate my-session --rename --create`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		sessionName := args[0]
		sessionsDir := getSessionsDir()

		// BUG-001 Phase 2: Validate session name for problematic characters
		// Warn user but allow association (existing sessions may have unsafe names)
		warnings, suggestedName, hasIssues := tmux.ValidateSessionName(sessionName)
		if hasIssues && !autoDetectOnly {
			// Print warnings about problematic characters
			fmt.Println()
			fmt.Println("⚠️  Warning: Session name contains unsafe characters")
			fmt.Println()
			for _, warning := range warnings {
				fmt.Println(warning)
			}
			fmt.Println()
			fmt.Printf("Note: If this is an existing session, association will continue.\n")
			fmt.Printf("For new sessions, consider using: agm new %s\n", suggestedName)
			fmt.Println()
		}

		// Handle --rename: send /rename command to Claude via tmux before associating
		if renameSession {
			// Detect tmux session that contains the Claude instance
			// Use the session name as the tmux session name
			tmuxSessionName := sessionName
			if err := tmux.SendCommandLiteral(tmuxSessionName, "/rename "+sessionName); err != nil {
				// If tmux session doesn't exist, try current tmux session
				currentTmux := os.Getenv("TMUX")
				if currentTmux != "" {
					// We're inside tmux - get current session name and send there
					fmt.Printf("Sending /rename %s to current tmux session...\n", sessionName)
					// The SendCommandLiteral function handles tmux socket detection
				} else {
					fmt.Fprintf(os.Stderr, "Warning: Could not send /rename to tmux session %q: %v\n", tmuxSessionName, err)
					fmt.Fprintf(os.Stderr, "  You may need to run /rename %s manually in Claude.\n", sessionName)
				}
			} else {
				fmt.Printf("Sent /rename %s to Claude session\n", sessionName)
			}
		}

		// Validate harness compatibility: associate is Claude-only
		// Try to find existing manifest to check harness type
		harnessAdapter, _ := getStorage()
		existingManifest, _, manifestErr := session.ResolveIdentifier(sessionName, sessionsDir, harnessAdapter)
		if manifestErr == nil && existingManifest.Harness != "" && existingManifest.Harness != "claude-code" {
			if !autoDetectOnly {
				ui.PrintError(fmt.Errorf("harness '%s' not supported", existingManifest.Harness),
					"Associate command only supports Claude Code sessions",
					"  • OpenCode sessions don't use UUIDs (state managed by server)\n"+
						"  • Gemini sessions use different authentication\n"+
						"  • This command is Claude-specific (detects UUID from history.jsonl)\n\n"+
						"  To manage this session, use: agm session resume "+sessionName)
			}
			return fmt.Errorf("associate command not applicable to harness: %s", existingManifest.Harness)
		}

		// Handle --auto-detect-only mode (for hooks)
		if autoDetectOnly {
			// Get Dolt storage adapter
			adapter, err := getStorage()
			if err != nil {
				// Silently fail in hook mode (Dolt not available)
				return nil
			}
			defer adapter.Close()

			// Try to find existing manifest
			m, manifestPath, err := session.ResolveIdentifier(sessionName, sessionsDir, adapter)
			if err != nil {
				// No manifest found, exit silently (hook mode)
				return nil
			}

			// Always attempt detection — even if UUID is set, it may have changed
			// due to Plan→Execute transitions where Claude Code silently creates
			// a new session UUID (GitHub issue anthropics/claude-code#26832).
			historyPath := filepath.Join(os.Getenv("HOME"), ".claude", "history.jsonl")
			detector := detection.NewDetector(historyPath, 5*time.Minute, adapter)

			var result *detection.Result
			if m.Claude.UUID == "" {
				// No UUID yet — use standard detection
				result, err = detector.DetectUUID(m)
			} else {
				// UUID already set — use re-detection to catch UUID changes
				result, err = detector.DetectCurrentUUID(m)
			}

			if err == nil && result.UUID != "" && result.Confidence == "high" {
				// Only update if UUID is different from what's stored
				if result.UUID != m.Claude.UUID {
					// Preserve the old UUID for recovery (prevents permanent loss on failed resume)
					if m.Claude.UUID != "" {
						m.Claude.PreviousUUID = m.Claude.UUID
					}
					m.Claude.UUID = result.UUID
					m.UpdatedAt = time.Now()
					if err := adapter.UpdateSession(m); err != nil {
						// Silently fail in hook mode
						return nil
					}

					// Auto-commit manifest change if in git repo (silent mode)
					_ = git.CommitManifest(manifestPath, "associate", sessionName)
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

			// Get Dolt storage adapter for manifest search
			adapter, adapterErr := getStorage()
			if adapterErr != nil {
				return fmt.Errorf("failed to connect to Dolt storage: %w", adapterErr)
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
			var err error
			targetUUID, err = uuidpkg.Discover(sessionName, findInManifests, false)
			if err != nil {
				// Check if user might have just run /rename (timing issue)
				historyPath := filepath.Join(os.Getenv("HOME"), ".claude", "history.jsonl")
				parser := history.NewParser(historyPath)
				sessions, readErr := parser.ReadConversations(100) // Check recent 100 entries

				hasRecentRename := false
				if readErr == nil {
					renameCmd := "/rename " + sessionName
					currentTime := time.Now().UnixMilli()
					// Check last 30 seconds for rename command
					for _, session := range sessions {
						for _, entry := range session.Entries {
							if currentTime-entry.Timestamp < 30000 { // 30 seconds in milliseconds
								if entry.Display == renameCmd || entry.Display == renameCmd+"\n/agm:agm-assoc "+sessionName {
									hasRecentRename = true
									break
								}
							}
						}
						if hasRecentRename {
							break
						}
					}
				}

				if hasRecentRename {
					ui.PrintError(err, "Failed to detect Claude UUID (timing issue)",
						"  • Recent /rename command detected but not yet processed\n"+
							"  • Wait a moment (1-2 seconds) and try again\n"+
							"  • Or run /rename separately before /agm:agm-assoc\n"+
							"  • Or specify UUID manually with --uuid flag")
				} else {
					ui.PrintError(err, "Failed to detect Claude UUID",
						"  • Tried AGM manifest lookup, history search by /rename, and timestamp search\n"+
							"  • Ensure Claude has processed at least one message\n"+
							"  • Or specify UUID manually with --uuid flag")
				}
				return err
			}

			ui.PrintSuccess(fmt.Sprintf("Detected Claude UUID: %s", targetUUID[:8]))
		}

		// Get Dolt adapter (will be reused for manifest resolution and write)
		adapter, doltErr := getStorage()
		if doltErr != nil {
			ui.PrintError(doltErr, "Failed to connect to Dolt storage",
				"  • Ensure Dolt server is running\n"+
					"  • Check WORKSPACE environment variable is set")
			return doltErr
		}
		defer adapter.Close()

		// Try to find existing manifest
		manifestPath := ""
		var m *manifest.Manifest
		var err error

		// Try to resolve identifier to existing manifest
		m, manifestPath, err = session.ResolveIdentifier(sessionName, sessionsDir, adapter)
		if err != nil {
			// Manifest doesn't exist
			if !createNew {
				ui.PrintError(err, "Session not found",
					fmt.Sprintf("  • Use --create to create a new session\n"+
						"  • Or run: agm session new %s", sessionName))
				return err
			}

			// Check for existing session with same name before creating
			existingByName, _ := adapter.GetSessionByName(sessionName)
			if existingByName != nil {
				// Reuse existing session instead of creating a duplicate
				fmt.Printf("Reusing existing AGM session: %s (ID: %s)\n", sessionName, existingByName.SessionID)
				m = existingByName
				manifestPath = filepath.Join(sessionsDir, fmt.Sprintf("session-%s", sessionName), "manifest.yaml")
			} else {
				// Create new manifest
				fmt.Printf("Creating new AGM session: %s\n", sessionName)
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

				cwd := ""
				if wd, err := os.Getwd(); err == nil {
					cwd = wd
				}

				m = &manifest.Manifest{
					SchemaVersion: manifest.SchemaVersion,
					SessionID:     "", // Will be populated below
					Name:          sessionName,
					CreatedAt:     time.Now(),
					UpdatedAt:     time.Now(),
					Lifecycle:     "", // Empty = active
					Context: manifest.Context{
						Project: cwd,
					},
					Claude: manifest.Claude{
						UUID: "", // Will be populated below
					},
					Tmux: manifest.Tmux{
						SessionName: sessionName,
					},
					WorkingDirectory: cwd,
				}
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
			// Preserve the old UUID for recovery
			if m.Claude.UUID != "" && m.Claude.UUID != targetUUID {
				m.Claude.PreviousUUID = m.Claude.UUID
			}
			m.Claude.UUID = targetUUID

			// Update working directory to current directory
			if wd, err := os.Getwd(); err == nil {
				m.WorkingDirectory = wd
			}

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

		// Write to Dolt database (adapter already acquired earlier)
		// Check if session exists in Dolt to decide create vs update
		existing, getErr := adapter.GetSession(m.SessionID)
		if getErr != nil || existing == nil {
			// Session doesn't exist - create it
			if err := adapter.CreateSession(m); err != nil {
				ui.PrintError(err, "Failed to create session in Dolt", "")
				return err
			}
		} else {
			// Session exists - update it
			if err := adapter.UpdateSession(m); err != nil {
				ui.PrintError(err, "Failed to update session in Dolt", "")
				return err
			}
		}

		// Auto-commit manifest change if in git repo
		operation := "associate"
		if oldUUID == "" {
			operation = "create"
		}
		_ = git.CommitManifest(manifestPath, operation, sessionName) // Errors logged internally

		// Create ready-file to signal Claude is initialized
		if err := readiness.CreateReadyFile(sessionName, manifestPath); err != nil {
			// Non-fatal: ready-file creation failed, but association succeeded
			fmt.Printf("Warning: Failed to create ready-file signal: %v\n", err)
		}

		fmt.Printf("\nManifest: %s\n", manifestPath)

		// Show completion with softer language to allow skill continuation
		fmt.Printf("\nSession association complete. You can now proceed to the next step.\n")
		fmt.Printf("To resume this session:\n")
		fmt.Printf("  agm session resume %s\n", sessionName)

		// Skill completion marker for smart detection (Bug fix: prompt interruption)
		// This marker allows new.go to detect when skill output is complete
		// before sending user prompts, preventing interruption of completion messages
		fmt.Printf("[AGM_SKILL_COMPLETE]\n")

		return nil
	},
}

func init() {
	associateCmd.Flags().StringVar(&claudeUUID, "uuid", "", "Claude session UUID (auto-detected if not specified)")
	associateCmd.Flags().BoolVar(&createNew, "create", false, "Create new manifest if it doesn't exist")
	associateCmd.Flags().BoolVar(&updateTimestampOnly, "update-timestamp-only", false, "Only update timestamp, don't change UUID (fast path for same UUID)")
	associateCmd.Flags().BoolVar(&autoDetectOnly, "auto-detect-only", false, "Auto-detect UUID only if high confidence (for hooks, silent mode)")
	associateCmd.Flags().BoolVar(&renameSession, "rename", false, "Also send /rename command to Claude session (combines rename + associate)")
	sessionCmd.AddCommand(associateCmd)
}
