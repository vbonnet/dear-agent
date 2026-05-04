package main

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/charmbracelet/huh"
	"github.com/spf13/cobra"
	"github.com/vbonnet/dear-agent/agm/internal/claude"
	"github.com/vbonnet/dear-agent/agm/internal/detection"
	"github.com/vbonnet/dear-agent/agm/internal/discovery"
	"github.com/vbonnet/dear-agent/agm/internal/dolt"
	"github.com/vbonnet/dear-agent/agm/internal/git"
	"github.com/vbonnet/dear-agent/agm/internal/manifest"
	"github.com/vbonnet/dear-agent/agm/internal/tmux"
	"github.com/vbonnet/dear-agent/agm/internal/ui"
)

var (
	syncAll bool
)

var syncCmd = &cobra.Command{
	Use:   "sync",
	Short: "Discover and sync Claude sessions",
	Long: `Parse ~/.claude/history.jsonl to discover Claude sessions and create
manifests for orphaned sessions.

By default, only shows recently active sessions (last 30 days).
Use --all to discover all sessions from history.

Examples:
  agm admin sync        # Discover recently active sessions
  agm admin sync --all  # Discover all sessions from history`,
	RunE: func(cmd *cobra.Command, args []string) error {
		// Parse history.jsonl
		homeDir, err := os.UserHomeDir()
		if err != nil {
			return fmt.Errorf("failed to get home directory: %w", err)
		}
		historyPath := filepath.Join(homeDir, ".claude", "history.jsonl")

		entries, stats, err := claude.ParseHistory(historyPath)
		if err != nil {
			ui.PrintError(
				err,
				"Failed to parse Claude history.",
				"  • Verify ~/.claude/history.jsonl exists\n"+
					"  • Ensure you have run Claude at least once",
			)
			return err
		}

		// Show parse statistics if any lines were skipped
		if stats.SkippedErrors > 0 || stats.SkippedEmpty > 0 {
			ui.PrintWarning(fmt.Sprintf("Parsed %d lines: %d valid, %d skipped (empty/null), %d errors",
				stats.TotalLines, stats.ValidEntries, stats.SkippedEmpty, stats.SkippedErrors))
		}

		// Deduplicate to get sessions
		sessions := claude.Deduplicate(entries)
		totalSessions := len(sessions)

		// Filter by recent activity unless --all is set
		if !syncAll {
			sessions = filterRecentSessions(sessions, recentDays)
			if len(sessions) == 0 {
				ui.PrintWarning(fmt.Sprintf("No sessions found in last %d days", recentDays))
				fmt.Println("\nUse --all to sync all sessions from history")
				return nil
			}
			ui.PrintSuccess(fmt.Sprintf("Found %d recent Claude sessions (last %d days, %d total in history)",
				len(sessions), recentDays, totalSessions))
		} else {
			ui.PrintSuccess(fmt.Sprintf("Found %d Claude sessions in history", len(sessions)))
		}

		// Convert to pointer slice for discovery
		sessionPtrs := make([]*claude.Session, len(sessions))
		for i := range sessions {
			sessionPtrs[i] = &sessions[i]
		}

		// Get Dolt storage adapter
		adapter, err := getStorage()
		if err != nil {
			return fmt.Errorf("failed to connect to Dolt storage: %w", err)
		}
		defer adapter.Close()

		// List existing manifests from Dolt
		manifests, err := adapter.ListSessions(&dolt.SessionFilter{})
		if err != nil {
			return fmt.Errorf("failed to list manifests: %w", err)
		}

		// Match sessions to manifests
		result := discovery.MatchToManifests(sessionPtrs, manifests)

		ui.PrintSuccess(fmt.Sprintf("Matched %d sessions to existing manifests", len(result.Matched)))

		// PRIORITY 1: Scan active tmux sessions for Claude conversations (automatic, non-interactive)
		if err := syncActiveTmuxSessions(adapter, cfg.SessionsDir, entries); err != nil {
			ui.PrintWarning(fmt.Sprintf("Failed to sync active tmux sessions: %v", err))
		}

		// PRIORITY 2: Optionally handle orphaned sessions from history (interactive)
		if len(result.OrphanedClaude) > 0 {
			ui.PrintWarning(fmt.Sprintf("Found %d orphaned Claude sessions in history", len(result.OrphanedClaude)))

			// Ask if user wants to interactively create manifests for these
			var confirm bool
			err := huh.NewConfirm().
				Title("\nDo you want to interactively create manifests for orphaned sessions?").
				Affirmative("Yes").
				Negative("No").
				Value(&confirm).
				WithTheme(ui.GetTheme()).
				Run()
			if err != nil || !confirm {
				fmt.Println("Skipping orphaned sessions. Run 'agm admin sync' again to handle them later.")
			} else {
				fmt.Println("\nOrphaned sessions (in history.jsonl but no manifest):")

				// Get existing tmux names from manifests to avoid conflicts
				existingTmuxNames := make(map[string]bool)
				for _, m := range manifests {
					if m.Tmux.SessionName != "" {
						existingTmuxNames[m.Tmux.SessionName] = true
					}
				}

				// Get active tmux sessions to avoid conflicts
				activeTmux, err := tmux.ListSessions()
				if err != nil {
					ui.PrintWarning(fmt.Sprintf("Failed to list tmux sessions: %v", err))
					activeTmux = []string{}
				}
				for _, name := range activeTmux {
					existingTmuxNames[name] = true
				}

				for i, session := range result.OrphanedClaude {
					fmt.Printf("  %d. UUID: %s\n", i+1, session.UUID)
					fmt.Printf("     Project: %s\n", session.Project)
					fmt.Printf("     Last Activity: %s\n", session.LastActivity.Format("2006-01-02 15:04:05"))

					// Offer to create manifest
					var confirmSession bool
					err := huh.NewConfirm().
						Title("Create manifest for this session?").
						Affirmative("Yes").
						Negative("No").
						Value(&confirmSession).
						WithTheme(ui.GetTheme()).
						Run()
					if err != nil || !confirmSession {
						continue
					}

					// Generate unique tmux name (check against existing names)
					baseTmuxName := fmt.Sprintf("claude-%d", i+1)
					tmuxName := baseTmuxName
					suffix := 2
					for existingTmuxNames[tmuxName] {
						tmuxName = fmt.Sprintf("%s-%d", baseTmuxName, suffix)
						suffix++
					}
					// Mark as used for next iteration
					existingTmuxNames[tmuxName] = true

					sessionID := filepath.Base(session.Project)

					m, err := discovery.CreateManifest(session, cfg.SessionsDir, tmuxName, sessionID, adapter)
					if err != nil {
						ui.PrintError(err,
							"Failed to create manifest for orphaned session",
							"  • Check sessions directory permissions: ls -ld "+cfg.SessionsDir+"\n"+
								"  • Verify disk space: df -h "+cfg.SessionsDir+"\n"+
								"  • Check session data is valid: UUID "+session.UUID[:8])
						continue
					}

					ui.PrintSuccess(fmt.Sprintf("Created manifest: %s (tmux: %s)", m.SessionID, m.Tmux.SessionName))
				}
			}
		}

		if len(result.OrphanedManifest) > 0 {
			ui.PrintWarning(fmt.Sprintf("Found %d orphaned manifests (not in history.jsonl)", len(result.OrphanedManifest)))
		}

		return nil
	},
}

// syncActiveTmuxSessions scans all active tmux sessions for Claude conversations
// and ensures they have proper manifests (without auto-assigning UUIDs)
func syncActiveTmuxSessions(adapter *dolt.Adapter, sessionsDir string, _ []claude.RawEntry) error {
	// Get all active tmux sessions
	tmuxSessions, err := tmux.ListSessions()
	if err != nil {
		return fmt.Errorf("failed to list tmux sessions: %w", err)
	}

	if len(tmuxSessions) == 0 {
		return nil // No tmux sessions to sync
	}

	var createdCount int
	var claudeSessionCount int
	var needsAssociationCount int

	// First pass: count how many sessions are running Claude
	for _, sessionName := range tmuxSessions {
		isRunning, _ := tmux.IsClaudeRunning(sessionName)
		if isRunning {
			claudeSessionCount++
		}
	}

	if claudeSessionCount == 0 {
		return nil // No Claude sessions running
	}

	fmt.Printf("\nScanning %d active tmux sessions running Claude...\n", claudeSessionCount)

	for _, sessionName := range tmuxSessions {
		created, addAssoc := syncOneTmuxSession(adapter, sessionName, sessionsDir)
		if created {
			createdCount++
		}
		if addAssoc {
			needsAssociationCount++
		}
	}

	if createdCount > 0 {
		fmt.Printf("\nCreated %d manifest(s) for active tmux sessions\n", createdCount)
	}

	if needsAssociationCount > 0 {
		fmt.Printf("\n💡 %d session(s) need Claude UUID association\n", needsAssociationCount)
		fmt.Println("   Use 'agm session associate <session-name>' to link each session to its Claude conversation")
	}

	return nil
}

// syncOneTmuxSession ensures a single tmux session has a manifest in Dolt.
// Returns (created, needsAssociation): created when a new manifest was
// written; needsAssociation when the session lacks a Claude UUID and the
// user must run agm session associate.
func syncOneTmuxSession(adapter *dolt.Adapter, sessionName, sessionsDir string) (bool, bool) {
	isRunning, err := tmux.IsClaudeRunning(sessionName)
	if err != nil || !isRunning {
		return false, false
	}
	manifestDir := filepath.Join(sessionsDir, fmt.Sprintf("session-%s", sessionName))
	manifestPath := filepath.Join(manifestDir, "manifest.yaml")
	sessionID := fmt.Sprintf("session-%s", sessionName)
	m, err := adapter.GetSession(sessionID)
	if err != nil || m == nil {
		return createManifestForTmux(adapter, sessionName, manifestDir, manifestPath)
	}
	if m.Claude.UUID == "" {
		fmt.Printf("  ℹ Session '%s' needs Claude UUID association\n", sessionName)
		fmt.Printf("    → Run 'agm session associate %s' to link\n", sessionName)
		return false, true
	}
	return false, false
}

// createManifestForTmux creates a new manifest record for a tmux session that
// has no Dolt entry yet, attempting to auto-detect the Claude UUID from
// history. Returns (created, needsAssociation): created is true when the
// manifest was successfully written; needsAssociation is true when no UUID
// could be auto-detected and the user must run `agm session associate`.
func createManifestForTmux(adapter *dolt.Adapter, sessionName, manifestDir, manifestPath string) (bool, bool) {
	workDir, err := tmux.GetCurrentWorkingDirectory(sessionName)
	if err != nil {
		workDir = os.Getenv("HOME")
	}
	if err := os.MkdirAll(manifestDir, 0700); err != nil {
		ui.PrintWarning(fmt.Sprintf("Failed to create manifest directory for %s: %v", sessionName, err))
		return false, false
	}
	m := &manifest.Manifest{
		SchemaVersion: manifest.SchemaVersion,
		SessionID:     fmt.Sprintf("session-%s", sessionName),
		Name:          sessionName,
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
		Lifecycle:     "",
		Context:       manifest.Context{Project: workDir},
		Claude:        manifest.Claude{},
		Tmux:          manifest.Tmux{SessionName: sessionName},
	}
	if homeDir, err := os.UserHomeDir(); err == nil {
		historyPath := filepath.Join(homeDir, ".claude", "history.jsonl")
		detector := detection.NewDetector(historyPath, 5*time.Minute, adapter)
		result, err := detector.DetectUUID(m)
		if err == nil && result.UUID != "" && result.Confidence == "high" {
			m.Claude.UUID = result.UUID
		}
	}
	if err := adapter.CreateSession(m); err != nil {
		ui.PrintWarning(fmt.Sprintf("Failed to create session in Dolt for %s: %v", sessionName, err))
		return false, false
	}
	_ = git.CommitManifest(manifestPath, "sync", sessionName)
	if m.Claude.UUID != "" {
		ui.PrintSuccess(fmt.Sprintf("Created manifest for tmux session '%s' (UUID auto-detected)", sessionName))
		return true, false
	}
	ui.PrintSuccess(fmt.Sprintf("Created manifest for tmux session '%s'", sessionName))
	fmt.Printf("  → Run 'agm session associate %s' to link Claude UUID\n", sessionName)
	return true, true
}

func init() {
	syncCmd.Flags().BoolVar(&syncAll, "all", false, "Sync all sessions (default: last 30 days only)")
	adminCmd.AddCommand(syncCmd)
}
