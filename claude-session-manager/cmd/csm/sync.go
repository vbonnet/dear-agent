package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
	"github.com/vbonnet/ai-tools/claude-session-manager/internal/claude"
	"github.com/vbonnet/ai-tools/claude-session-manager/internal/discovery"
	"github.com/vbonnet/ai-tools/claude-session-manager/internal/manifest"
	"github.com/vbonnet/ai-tools/claude-session-manager/internal/ui"
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
  csm sync        # Discover recently active sessions
  csm sync --all  # Discover all sessions from history`,
	RunE: func(cmd *cobra.Command, args []string) error {
		// Parse history.jsonl
		homeDir, _ := os.UserHomeDir()
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

		// List existing manifests
		manifests, err := manifest.List(cfg.SessionsDir)
		if err != nil {
			// Sessions dir might not exist, create it
			if os.IsNotExist(err) {
				if err := os.MkdirAll(cfg.SessionsDir, 0700); err != nil {
					return fmt.Errorf("failed to create sessions directory: %w", err)
				}
				manifests = []*manifest.Manifest{}
			} else {
				return fmt.Errorf("failed to list manifests: %w", err)
			}
		}

		// Match sessions to manifests
		result := discovery.MatchToManifests(sessionPtrs, manifests)

		ui.PrintSuccess(fmt.Sprintf("Matched %d sessions to existing manifests", len(result.Matched)))

		if len(result.OrphanedClaude) > 0 {
			ui.PrintWarning(fmt.Sprintf("Found %d orphaned Claude sessions", len(result.OrphanedClaude)))
			fmt.Println("\nOrphaned sessions (in history.jsonl but no manifest):")
			for i, session := range result.OrphanedClaude {
				fmt.Printf("  %d. UUID: %s\n", i+1, session.UUID)
				fmt.Printf("     Project: %s\n", session.Project)
				fmt.Printf("     Last Activity: %s\n", session.LastActivity.Format("2006-01-02 15:04:05"))

				// Offer to create manifest
				confirm, err := ui.Confirm("Create manifest for this session?")
				if err != nil || !confirm {
					continue
				}

				// Generate tmux name and session ID
				tmuxName := fmt.Sprintf("claude-%d", i+1)
				sessionID := filepath.Base(session.Project)

				m, err := discovery.CreateManifest(session, cfg.SessionsDir, tmuxName, sessionID)
				if err != nil {
					ui.PrintError(err, "Failed to create manifest", "")
					continue
				}

				ui.PrintSuccess(fmt.Sprintf("Created manifest: %s (tmux: %s)", m.SessionID, m.Tmux.SessionName))
			}
		}

		if len(result.OrphanedManifest) > 0 {
			ui.PrintWarning(fmt.Sprintf("Found %d orphaned manifests (not in history.jsonl)", len(result.OrphanedManifest)))
		}

		return nil
	},
}

func init() {
	syncCmd.Flags().BoolVar(&syncAll, "all", false, "Sync all sessions (default: last 30 days only)")
	rootCmd.AddCommand(syncCmd)
}
