package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/spf13/cobra"
	"github.com/vbonnet/ai-tools/claude-session-manager/internal/claude"
	"github.com/vbonnet/ai-tools/claude-session-manager/internal/ui"
)

const (
	// Display column widths and formatting
	uuidDisplayLen   = 8
	projectMaxLen    = 40
	messagesColWidth = 8
	durationColWidth = 10
)

var (
	listJSON bool
	listSort string
)

var listCmd = &cobra.Command{
	Use:   "list",
	Short: "List all Claude sessions from history",
	Long: `List all Claude sessions discovered from ~/.claude/history.jsonl.

This command parses your Claude CLI history and displays all unique sessions
with their UUIDs, project directories, message counts, and activity times.

Examples:
  csm list              # List all sessions
  csm list --json       # Output as JSON`,
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

		// Sort sessions
		sortSessions(sessions, listSort)

		// Output
		if listJSON {
			output, err := json.MarshalIndent(sessions, "", "  ")
			if err != nil {
				return fmt.Errorf("failed to format JSON: %w", err)
			}
			fmt.Println(string(output))
		} else {
			output := formatSessionsTable(sessions)
			fmt.Print(output)
		}

		return nil
	},
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

func formatSessionsTable(sessions []claude.Session) string {
	// Header
	uuidCol := fmt.Sprintf("%%-%ds", uuidDisplayLen+2)
	projectCol := fmt.Sprintf("%%-%ds", projectMaxLen+2)
	messagesCol := fmt.Sprintf("%%-%ds", messagesColWidth+2)
	durationCol := fmt.Sprintf("%%-%ds", durationColWidth+2)

	header := fmt.Sprintf(uuidCol+projectCol+messagesCol+durationCol+"%s\n",
		"UUID", "PROJECT", "MESSAGES", "DURATION", "LAST ACTIVITY")
	separator := "─────────────────────────────────────────────────────────────────────────────────────────────\n"

	table := header + separator

	// Rows
	for _, s := range sessions {
		// Truncate UUID to first N chars
		uuid := s.UUID
		if len(uuid) > uuidDisplayLen {
			uuid = uuid[:uuidDisplayLen]
		}

		// Truncate project path if too long
		project := s.Project
		if len(project) > projectMaxLen {
			project = "..." + project[len(project)-(projectMaxLen-3):]
		}

		// Format duration
		duration := fmt.Sprintf("%.1fh", s.DurationHours)

		// Format last activity
		lastActivity := s.LastActivity.Format("2006-01-02 15:04")

		table += fmt.Sprintf(uuidCol+projectCol+messagesCol+durationCol+"%s\n",
			uuid, project, fmt.Sprintf("%d", s.MessageCount), duration, lastActivity)
	}

	return table
}

func init() {
	listCmd.Flags().BoolVar(&listJSON, "json", false, "Output as JSON")
	listCmd.Flags().StringVar(&listSort, "sort", "activity", "Sort by (activity, messages, duration)")

	rootCmd.AddCommand(listCmd)
}
