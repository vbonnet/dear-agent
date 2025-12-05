package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
	"github.com/vbonnet/ai-tools/claude-session-manager/internal/claude"
	"github.com/vbonnet/ai-tools/claude-session-manager/internal/ui"
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

func formatSessionsTable(sessions []claude.Session) string {
	// Header
	table := fmt.Sprintf("%-10s  %-40s  %-8s  %-10s  %s\n",
		"UUID", "PROJECT", "MESSAGES", "DURATION", "LAST ACTIVITY")
	table += fmt.Sprintf("%s\n", "─────────────────────────────────────────────────────────────────────────────────────────────")

	// Rows
	for _, s := range sessions {
		// Truncate UUID to first 8 chars
		uuid := s.UUID
		if len(uuid) > 8 {
			uuid = uuid[:8]
		}

		// Truncate project path if too long
		project := s.Project
		if len(project) > 40 {
			project = "..." + project[len(project)-37:]
		}

		// Format duration
		duration := fmt.Sprintf("%.1fh", s.DurationHours)

		// Format last activity
		lastActivity := s.LastActivity.Format("2006-01-02 15:04")

		table += fmt.Sprintf("%-10s  %-40s  %-8d  %-10s  %s\n",
			uuid, project, s.MessageCount, duration, lastActivity)
	}

	return table
}

func init() {
	listCmd.Flags().BoolVar(&listJSON, "json", false, "Output as JSON")
	listCmd.Flags().StringVar(&listSort, "sort", "activity", "Sort by (activity, messages, duration)")

	rootCmd.AddCommand(listCmd)
}
