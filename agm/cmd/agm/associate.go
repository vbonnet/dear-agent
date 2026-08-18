package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/spf13/cobra"
	"github.com/vbonnet/dear-agent/agm/internal/agent"
	"github.com/vbonnet/dear-agent/agm/internal/cli"
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
	associateHarness    string
)

var associateCmd = &cobra.Command{
	Use:   "associate <session-name>",
	Short: "Associate a AGM session with the current harness session",
	Long: `Associate a AGM session with the current harness session.

Claude Code sessions are associated with a Claude UUID for resume. Codex,
Gemini, and OpenCode sessions do not use Claude UUIDs; for those harnesses this
command ensures the AGM/Dolt session record exists and is linked to the current
tmux session.

This command is useful when:
- You started a Claude session outside of tmux and want to track it
- You want to reassign a different Claude UUID to an existing AGM session
- You're reconnecting an existing session after the UUID changed
- You are in a non-Claude CLI harness and want to register the current tmux
  session with AGM

The command will:
1. Find or create the AGM session record
2. For Claude Code: get and store the current Claude session UUID
3. For non-Claude harnesses: store harness/tmux/workdir metadata in Dolt
4. Create a ready-file signal for the session

Examples:
  # Associate current Claude session with AGM session "claude-1"
  agm session associate claude-1

  # Specify a specific Claude UUID instead of auto-detecting
  agm session associate claude-1 --uuid c86ffd41-cbcc-4bfa-8b1f-4da7c83fc3d2

  # Create a new manifest if it doesn't exist
  agm session associate my-new-session --create

  # Create or update a Codex session record
  agm session associate my-codex --create --harness codex-cli

  # Infer harness from the current tmux pane where possible
  agm session associate my-session --create --harness auto

  # Create or update an AGY session record
  agm session associate my-agy --create --harness agy

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

		// Determine harness before UUID discovery. Non-Claude harnesses do not
		// have Claude UUIDs, so association means "ensure the Dolt record exists".
		harnessAdapter, harnessErr := getStorage()
		if harnessErr != nil {
			if !autoDetectOnly {
				return fmt.Errorf("failed to connect to Dolt storage: %w", harnessErr)
			}
			return nil
		}
		defer func() { _ = harnessAdapter.Close() }()

		existingManifest, existingManifestPath, manifestErr := session.ResolveIdentifier(sessionName, sessionsDir, harnessAdapter)
		targetHarness, harnessErr := resolveAssociateHarness(sessionName, existingManifest)
		if harnessErr != nil {
			return harnessErr
		}
		if !isClaudeAssociateHarness(targetHarness) {
			return associateNonClaudeSession(sessionName, sessionsDir, targetHarness, existingManifest, existingManifestPath, manifestErr, harnessAdapter)
		}

		// Handle --auto-detect-only mode (for hooks)
		if autoDetectOnly {
			// Get Dolt storage adapter
			adapter, err := getStorage()
			if err != nil {
				// Silently fail in hook mode (Dolt not available)
				return nil
			}
			defer func() { _ = adapter.Close() }()

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
			defer func() { _ = adapter.Close() }()

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
		defer func() { _ = adapter.Close() }()

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
				// Reuse existing session instead of creating a duplicate.
				// manifestPath is resolved canonically below, once the session
				// ID is known (see "Resolve canonical manifest path").
				fmt.Printf("Reusing existing AGM session: %s (ID: %s)\n", sessionName, existingByName.SessionID)
				m = existingByName
			} else {
				// Create a new session. Session data is persisted to Dolt (the
				// source of truth) by adapter.CreateSession below; we no longer
				// mkdir an empty session-<name>/ directory or write a YAML
				// manifest file here. The historical code did both, then printed
				// "Manifest: <empty dir path>" — a path to a file that never
				// existed.
				fmt.Printf("Creating new AGM session: %s\n", sessionName)

				cwd := currentWorkingDirectory()

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

			// Update working directory to the root-resolved command directory.
			if wd := currentWorkingDirectory(); wd != "" {
				m.WorkingDirectory = wd
			}

			// Generate SessionID if not present
			if m.SessionID == "" {
				m.SessionID = uuid.New().String()
			}

			// Resolve canonical manifest path. AGM keys on-disk manifests by
			// session ID (matching session.ResolveIdentifier), not by session
			// name. Compute it here once the ID is known so any downstream
			// consumers (git commit, ready-file) reference a consistent path.
			if manifestPath == "" {
				manifestPath = filepath.Join(sessionsDir, m.SessionID, "manifest.yaml")
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

		// Report where the session is actually stored. Only prints a
		// "Manifest:" path when a manifest file genuinely exists on disk;
		// otherwise reports the Dolt storage location. Avoids the historical
		// bug of printing a path to a manifest file that was never written.
		fmt.Printf("\n%s\n", describeAssociationStorage(m, adapter.Workspace(), manifestPath))

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
	associateCmd.Flags().StringVar(&associateHarness, "harness", "", "Harness for --create when no existing session is found (auto, claude-code, codex-cli, agy, opencode-cli, pi-cli; deprecated: gemini-cli)")
	associateCmd.Flags().BoolVar(&createNew, "create", false, "Create new manifest if it doesn't exist")
	associateCmd.Flags().BoolVar(&updateTimestampOnly, "update-timestamp-only", false, "Only update timestamp, don't change UUID (fast path for same UUID)")
	associateCmd.Flags().BoolVar(&autoDetectOnly, "auto-detect-only", false, "Auto-detect UUID only if high confidence (for hooks, silent mode)")
	associateCmd.Flags().BoolVar(&renameSession, "rename", false, "Also send /rename command to Claude session (combines rename + associate)")
	sessionCmd.AddCommand(associateCmd)
}

func isClaudeAssociateHarness(harness string) bool {
	return harness == "" || harness == "claude-code"
}

func resolveAssociateHarness(sessionName string, existing *manifest.Manifest) (string, error) {
	if existing != nil && existing.Harness != "" {
		return existing.Harness, nil
	}
	switch associateHarness {
	case "", "claude-code":
		return "claude-code", nil
	case "auto":
		h, err := inferHarnessFromTmux(sessionName)
		if err != nil {
			return "", err
		}
		return h, nil
	default:
		if err := agent.ValidateHarnessName(associateHarness); err != nil {
			return "", err
		}
		return agent.NormalizeHarnessName(associateHarness), nil
	}
}

func inferHarnessFromTmux(sessionName string) (string, error) {
	commands, err := tmux.GetPaneCommands(sessionName)
	if err != nil {
		return "", fmt.Errorf("failed to infer harness from tmux session %q: %w", sessionName, err)
	}
	if harness := harnessFromPaneCommands(commands); harness != "" {
		return harness, nil
	}
	return "", fmt.Errorf("could not infer harness from tmux session %q; pass --harness explicitly", sessionName)
}

func harnessFromPaneCommands(commands []string) string {
	for _, cmd := range commands {
		switch commandExecutableName(cmd) {
		case "codex":
			return "codex-cli"
		case "gemini":
			return "gemini-cli"
		case "opencode":
			return "opencode-cli"
		case "agy":
			return "agy"
		case "pi":
			return "pi-cli"
		case "claude":
			return "claude-code"
		}
	}
	return ""
}

func commandExecutableName(cmd string) string {
	cmd = strings.TrimSpace(cmd)
	if len(cmd) > 1 && (cmd[0] == '"' || cmd[0] == '\'') {
		quote := cmd[0]
		if end := strings.IndexByte(cmd[1:], quote); end >= 0 {
			quoted := cmd[1 : end+1]
			rest := strings.TrimSpace(cmd[end+2:])
			if rest != "" {
				return filepath.Base(quoted)
			}
			cmd = quoted
		}
	}
	fields := strings.Fields(cmd)
	if len(fields) > 0 {
		cmd = fields[0]
	}
	cmd = strings.Trim(cmd, `"'`)
	return filepath.Base(cmd)
}

func associateNonClaudeSession(sessionName, sessionsDir, harness string, existing *manifest.Manifest, manifestPath string, manifestErr error, adapter *dolt.Adapter) error {
	if autoDetectOnly {
		if existing != nil {
			_ = readiness.CreateReadyFile(sessionName, manifestPath)
		}
		return nil
	}
	if renameSession {
		fmt.Fprintf(os.Stderr, "Warning: --rename is Claude-only; skipping /rename for harness %s\n", harness)
	}

	m := existing
	if m == nil {
		if !createNew {
			ui.PrintError(manifestErr, "Session not found",
				fmt.Sprintf("  • Use --create to create a new %s session record\n"+
					"  • Or run: agm session new %s --harness %s", harness, sessionName, harness))
			return manifestErr
		}
		m = newNonClaudeAssociationManifest(sessionName, harness, adapter.Workspace())
		manifestPath = filepath.Join(sessionsDir, m.SessionID, "manifest.yaml")
	} else {
		updateNonClaudeAssociationManifest(m, sessionName, harness, adapter.Workspace())
		if manifestPath == "" {
			manifestPath = filepath.Join(sessionsDir, m.SessionID, "manifest.yaml")
		}
	}
	if harness == "agy" {
		enrichAssociatedAgyManifest(adapter, m)
	}

	if err := persistAssociatedManifest(adapter, m); err != nil {
		return err
	}

	if err := readiness.CreateReadyFile(sessionName, manifestPath); err != nil {
		fmt.Printf("Warning: Failed to create ready-file signal: %v\n", err)
	}

	fmt.Printf("Session association in progress: '%s' linked to %s harness\n", sessionName, harness)
	fmt.Printf("\n%s\n", describeAssociationStorage(m, adapter.Workspace(), manifestPath))
	fmt.Printf("\nSession association complete. You can now proceed to the next step.\n")
	fmt.Printf("To resume this session:\n")
	fmt.Printf("  agm session resume %s\n", sessionName)
	fmt.Printf("[AGM_SKILL_COMPLETE]\n")
	return nil
}

func newNonClaudeAssociationManifest(sessionName, harness, workspace string) *manifest.Manifest {
	cwd := currentWorkingDirectory()
	return &manifest.Manifest{
		SchemaVersion: manifest.SchemaVersion,
		SessionID:     uuid.New().String(),
		Name:          sessionName,
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
		Lifecycle:     "",
		Workspace:     workspace,
		Context: manifest.Context{
			Project: cwd,
		},
		Tmux: manifest.Tmux{
			SessionName: sessionName,
		},
		Harness:          harness,
		Model:            associationModel(harness),
		WorkingDirectory: cwd,
	}
}

func updateNonClaudeAssociationManifest(m *manifest.Manifest, sessionName, harness, workspace string) {
	m.UpdatedAt = time.Now()
	if m.Harness == "" {
		m.Harness = harness
	}
	if m.Workspace == "" {
		m.Workspace = workspace
	}
	if m.Tmux.SessionName == "" {
		m.Tmux.SessionName = sessionName
	}
	if wd := currentWorkingDirectory(); wd != "" {
		if m.WorkingDirectory == "" {
			m.WorkingDirectory = wd
		}
		if m.Context.Project == "" {
			m.Context.Project = wd
		}
	}
	if m.Model == "" {
		m.Model = associationModel(harness)
	}
	if m.SessionID == "" {
		m.SessionID = uuid.New().String()
	}
}

// associationModel returns a default only when it describes the process being
// associated. AGY saved conversations retain their own model selection, which
// the public filesystem metadata does not expose, so an empty model is the
// truthful representation until AGM observes an explicit selection.
func associationModel(harness string) string {
	if agent.NormalizeHarnessName(harness) == "agy" {
		return ""
	}
	model, _ := agent.DefaultModelForHarness(harness)
	return model
}

func currentWorkingDirectory() string {
	if directory != "" {
		return cli.GetProjectDirectory()
	}
	wd, err := os.Getwd()
	if err != nil {
		return ""
	}
	return wd
}

func persistAssociatedManifest(adapter *dolt.Adapter, m *manifest.Manifest) error {
	existing, getErr := adapter.GetSession(m.SessionID)
	if getErr != nil || existing == nil {
		if err := adapter.CreateSession(m); err != nil {
			ui.PrintError(err, "Failed to create session in Dolt", "")
			return err
		}
		return nil
	}
	if err := adapter.UpdateSession(m); err != nil {
		ui.PrintError(err, "Failed to update session in Dolt", "")
		return err
	}
	return nil
}
