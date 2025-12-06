package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/spf13/cobra"
	"github.com/vbonnet/ai-tools/claude-session-manager/internal/claude"
	"github.com/vbonnet/ai-tools/claude-session-manager/internal/discovery"
	"github.com/vbonnet/ai-tools/claude-session-manager/internal/tmux"
	"github.com/vbonnet/ai-tools/claude-session-manager/internal/ui"
)

const (
	// Display column widths and formatting
	uuidDisplayLen   = 8
	projectMaxLen    = 35
	messagesColWidth = 8
	durationColWidth = 10
	tmuxColWidth     = 14 // Accommodate names like "claude-demo ✓"
	recentDays       = 30 // Default filter for recently active sessions
)

var (
	listJSON bool
	listSort string
	listAll  bool
)

var listCmd = &cobra.Command{
	Use:   "list",
	Short: "List Claude sessions from history",
	Long: `List Claude sessions discovered from ~/.claude/history.jsonl.

By default, shows only recently active sessions (last 30 days).
Use --all to show all sessions from history.

Displays tmux session name and active status for sessions with manifests.

Examples:
  csm list              # List recently active sessions
  csm list --all        # List all sessions from history
  csm list --json       # Output as JSON
  csm list --sort messages  # Sort by message count`,
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

		// Show parse warnings if any issues
		if stats.SkippedErrors > 0 {
			ui.PrintWarning(fmt.Sprintf("Skipped %d malformed lines in history", stats.SkippedErrors))
		}

		// Deduplicate to get sessions
		sessions := claude.Deduplicate(entries)

		if len(sessions) == 0 {
			ui.PrintWarning("No Claude sessions found in history")
			fmt.Println("\nHave you used Claude CLI before? Try running: claude")
			return nil
		}

		// Filter by recent activity unless --all is set
		if !listAll {
			sessions = filterRecentSessions(sessions, recentDays)
			if len(sessions) == 0 {
				ui.PrintWarning(fmt.Sprintf("No sessions found in last %d days", recentDays))
				fmt.Println("\nUse --all to see all sessions from history")
				return nil
			}
		}

		// Sort sessions
		sortSessions(sessions, listSort)

		// Get tmux info (mapping and active sessions)
		sessionsDir := filepath.Join(homeDir, "sessions")
		tmuxMapping, err := discovery.GetTmuxMapping(sessionsDir)
		if err != nil {
			// Non-fatal: just won't show tmux info
			tmuxMapping = make(map[string]string)
		}

		activeTmux, err := tmux.ListSessions()
		if err != nil {
			// Non-fatal: just won't show active status
			activeTmux = []string{}
		}
		activeTmuxSet := make(map[string]bool)
		for _, name := range activeTmux {
			activeTmuxSet[name] = true
		}

		// Output
		if listJSON {
			output, err := json.MarshalIndent(sessions, "", "  ")
			if err != nil {
				return fmt.Errorf("failed to format JSON: %w", err)
			}
			fmt.Println(string(output))
		} else {
			output := formatSessionsTable(sessions, tmuxMapping, activeTmuxSet)
			fmt.Print(output)
		}

		return nil
	},
}

// filterRecentSessions filters sessions to those active within the last N days
func filterRecentSessions(sessions []claude.Session, days int) []claude.Session {
	cutoff := time.Now().AddDate(0, 0, -days)
	filtered := make([]claude.Session, 0, len(sessions))

	for _, s := range sessions {
		if s.LastActivity.After(cutoff) {
			filtered = append(filtered, s)
		}
	}

	return filtered
}

// sortSessions sorts sessions according to the specified sort key
func sortSessions(sessions []claude.Session, sortBy string) {
	switch sortBy {
	case "messages":
		sort.Slice(sessions, func(i, j int) bool {
			return sessions[i].MessageCount > sessions[j].MessageCount
		})
	case "duration":
		sort.Slice(sessions, func(i, j int) bool {
			return sessions[i].DurationHours > sessions[j].DurationHours
		})
	case "activity", "": // default: sort by last activity (newest first)
		sort.Slice(sessions, func(i, j int) bool {
			return sessions[i].LastActivity.After(sessions[j].LastActivity)
		})
	default:
		// Invalid sort key - just use default (activity)
		sort.Slice(sessions, func(i, j int) bool {
			return sessions[i].LastActivity.After(sessions[j].LastActivity)
		})
	}
}

func formatSessionsTable(sessions []claude.Session, tmuxMapping map[string]string, activeTmuxSet map[string]bool) string {
	// Header
	uuidCol := fmt.Sprintf("%%-%ds", uuidDisplayLen+2)
	tmuxCol := fmt.Sprintf("%%-%ds", tmuxColWidth+2)
	projectCol := fmt.Sprintf("%%-%ds", projectMaxLen+2)
	messagesCol := fmt.Sprintf("%%-%ds", messagesColWidth+2)

	header := fmt.Sprintf(uuidCol+tmuxCol+projectCol+messagesCol+"%s\n",
		"UUID", "TMUX", "PROJECT", "MESSAGES", "LAST ACTIVITY")
	separator := "──────────────────────────────────────────────────────────────────────────────────────────────\n"

	table := header + separator

	// Rows
	for _, s := range sessions {
		// Truncate UUID to first N chars
		uuid := s.UUID
		if len(uuid) > uuidDisplayLen {
			uuid = uuid[:uuidDisplayLen]
		}

		// Get tmux session name and active status
		tmuxName := tmuxMapping[s.UUID]
		if tmuxName == "" {
			tmuxName = "-"
		} else if activeTmuxSet[tmuxName] {
			tmuxName = tmuxName + " ✓"
		}

		// Truncate project path if too long
		project := s.Project
		if len(project) > projectMaxLen {
			project = "..." + project[len(project)-(projectMaxLen-3):]
		}

		// Format last activity
		lastActivity := s.LastActivity.Format("2006-01-02 15:04")

		table += fmt.Sprintf(uuidCol+tmuxCol+projectCol+messagesCol+"%s\n",
			uuid, tmuxName, project, fmt.Sprintf("%d", s.MessageCount), lastActivity)
	}

	return table
}

func init() {
	listCmd.Flags().BoolVar(&listJSON, "json", false, "Output as JSON")
	listCmd.Flags().StringVar(&listSort, "sort", "activity", "Sort by (activity, messages, duration)")
	listCmd.Flags().BoolVar(&listAll, "all", false, "Show all sessions (default: last 30 days only)")

	rootCmd.AddCommand(listCmd)
}
