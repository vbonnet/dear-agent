package main

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"github.com/vbonnet/dear-agent/agm/internal/budget"
	"github.com/vbonnet/dear-agent/agm/internal/ops"
	"github.com/vbonnet/dear-agent/agm/internal/session"
	"github.com/vbonnet/dear-agent/pkg/cliframe"
)

var (
	statusWorkspace string
	statusFormat    string
)

var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show aggregated status across all sessions",
	Long: `Show aggregated status information for all sessions in a workspace.

This command queries all active sessions and displays:
  • Session name
  • Current branch
  • State (READY|THINKING|PERMISSION_PROMPT|COMPACTING|OFFLINE)
  • Uncommitted file count
  • Worktree path
  • Workspace

Output formats:
  • table (default) - Human-readable table
  • json - Machine-readable JSON

Performance:
  • Target: <10 seconds for 10 sessions
  • Uses cached state from hooks (no tmux parsing overhead)

Examples:
  # Show all sessions in default workspace
  agm session status

  # Show sessions in specific workspace
  agm session status --workspace oss

  # Output as JSON for automation
  agm session status --workspace oss --format json`,
	RunE: runStatus,
}

func init() {
	statusCmd.Flags().StringVarP(
		&statusWorkspace,
		"workspace",
		"w",
		"",
		"Filter by workspace (empty = all workspaces)",
	)

	statusCmd.Flags().StringVarP(
		&statusFormat,
		"format",
		"f",
		"table",
		"Output format (table|json)",
	)

	sessionCmd.AddCommand(statusCmd)
}

func runStatus(cmd *cobra.Command, args []string) error {
	// Construct OpContext with storage
	opCtx, cleanup, err := newOpContextWithStorage()
	if err != nil {
		return fmt.Errorf("failed to connect to Dolt storage: %w", err)
	}
	defer cleanup()

	// Call ops layer for session status
	statusResult, err := ops.GetStatus(opCtx, &ops.GetStatusRequest{
		IncludeArchived: false,
	})
	if err != nil {
		return handleError(err)
	}

	// Output using cliframe
	writer := cliframe.NewWriter(cmd.OutOrStdout(), cmd.ErrOrStderr())

	switch statusFormat {
	case "table":
		// Use custom text formatting for tables (hybrid approach)
		// For backward compatibility, also call the session-level workspace status
		// which includes branch/worktree info not available in the ops layer
		workspaceStatus, wsErr := session.AggregateWorkspaceStatus(opCtx.Storage, statusWorkspace)
		if wsErr == nil {
			printTableFormat(workspaceStatus, cmd)
		} else {
			// Fallback: print ops-layer status summary
			printOpsStatusTable(cmd, statusResult)
		}
	case "json":
		// Use cliframe JSON formatter with ops result
		formatter, fmtErr := cliframe.NewFormatter(cliframe.FormatJSON, cliframe.WithPrettyPrint(true))
		if fmtErr != nil {
			return fmtErr
		}
		writer = writer.WithFormatter(formatter)
		return writer.Output(statusResult)
	default:
		return fmt.Errorf("invalid format '%s'. Valid formats: table, json", statusFormat)
	}

	return nil
}

func printTableFormat(ws *session.WorkspaceStatus, cmd *cobra.Command) {
	out := cmd.OutOrStdout()

	// Print header
	if ws.Workspace != "" {
		fmt.Fprintf(out, "Workspace: %s\n", ws.Workspace)
	} else {
		fmt.Fprintln(out, "Workspace: all")
	}

	fmt.Fprintf(out, "Sessions: %d total (%d DONE, %d WORKING)\n\n", ws.TotalSessions, ws.DoneSessions, ws.WorkingSessions)

	if len(ws.Sessions) == 0 {
		fmt.Fprintln(out, "No sessions found.")
		return
	}

	// Print table header
	fmt.Fprintf(out, "%-25s %-20s %-18s %-12s %-12s %-15s\n",
		"Session Name",
		"Branch",
		"State",
		"Uncommitted",
		"Budget",
		"Worktree",
	)
	fmt.Fprintln(out, strings.Repeat("─", 120))

	// Print each session
	for _, s := range ws.Sessions {
		// Truncate long values for display
		sessionName := truncate(s.Name, 24)
		branch := truncate(s.Branch, 19)
		state := formatState(s.State)
		uncommitted := formatUncommitted(s.Uncommitted)
		budgetStr := formatBudget(s.Budget)
		worktree := formatWorktree(s.WorktreePath)

		fmt.Fprintf(out, "%-25s %-20s %-18s %-12s %-12s %-15s\n",
			sessionName,
			branch,
			state,
			uncommitted,
			budgetStr,
			worktree,
		)
	}

	fmt.Fprintln(out)
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}

func formatState(state string) string {
	// Add emoji/symbol for visual clarity
	switch state {
	case "READY":
		return "✓ READY"
	case "THINKING":
		return "● THINKING"
	case "PERMISSION_PROMPT":
		return "? PERMISSION"
	case "COMPACTING":
		return "⟳ COMPACTING"
	case "OFFLINE":
		return "✗ OFFLINE"
	default:
		return state
	}
}

func formatUncommitted(count int) string {
	if count < 0 {
		return "unknown"
	}
	if count == 0 {
		return "clean"
	}
	return fmt.Sprintf("%d files", count)
}

func formatBudget(bs *budget.Status) string {
	if bs == nil {
		return "—"
	}
	//nolint:exhaustive // intentional partial: handles the relevant subset
	switch bs.Level {
	case budget.LevelCritical:
		return fmt.Sprintf("!! %.0f%%", bs.PercentageUsed)
	case budget.LevelWarning:
		return fmt.Sprintf("! %.0f%%", bs.PercentageUsed)
	default:
		return fmt.Sprintf("%.0f%%", bs.PercentageUsed)
	}
}

func formatWorktree(path string) string {
	// Check if path contains "worktree" or "wf"
	if strings.Contains(path, "worktree") || strings.Contains(path, "/wf/") {
		return "worktree"
	}
	return "main"
}

// printOpsStatusTable prints a status table from ops.GetStatusResult.
// Used as a fallback when the richer session.AggregateWorkspaceStatus is unavailable.
func printOpsStatusTable(cmd *cobra.Command, result *ops.GetStatusResult) {
	out := cmd.OutOrStdout()

	fmt.Fprintf(out, "Sessions: %d total (%d active, %d stopped, %d archived)\n\n",
		result.Summary.Total, result.Summary.Active, result.Summary.Stopped, result.Summary.Archived)

	if len(result.Sessions) == 0 {
		fmt.Fprintln(out, "No sessions found.")
		return
	}

	fmt.Fprintf(out, "%-25s %-10s %-15s\n",
		"Session Name", "Status", "Harness")
	fmt.Fprintln(out, strings.Repeat("-", 70))

	for _, s := range result.Sessions {
		name := s.Name
		if len(name) > 24 {
			name = name[:21] + "..."
		}
		fmt.Fprintf(out, "%-25s %-10s %-15s\n",
			name, s.Status, s.Harness)
	}
	fmt.Fprintln(out)
}
