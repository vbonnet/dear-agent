package main

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/vbonnet/dear-agent/agm/internal/ops"
	"github.com/vbonnet/dear-agent/agm/internal/ui"
)

var (
	dashboardJSON         bool
	dashboardAll          bool
	dashboardOutput       string
	dashboardOrchestrator bool
)

var dashboardCmd = &cobra.Command{
	Use:   "dashboard",
	Short: "Show a consolidated view of all session states",
	Long: `Display a real-time dashboard of all active AGM sessions.

Shows for each session:
  - Current state (WORKING, DONE, USER_PROMPT, COMPACTING, OFFLINE)
  - Time in current state
  - Permission mode (default, plan, ask, allow)
  - Interrupt count
  - Model and project

For orchestrators, use --orchestrator to show:
  - Active session counts by state
  - System metrics (load, RAM, disk)
  - Alerts for threshold violations
  - Top 5 and bottom 5 sessions by trust score
  - Throughput metrics (commits/hour, workers/hour)

Examples:
  agm session dashboard                    # Show active sessions
  agm session dashboard --all              # Include archived sessions
  agm session dashboard --json             # Output as JSON
  agm session dashboard --orchestrator     # Orchestrator unified view
  agm session dashboard --orchestrator --output=json`,
	RunE: runDashboard,
}

func init() {
	sessionCmd.AddCommand(dashboardCmd)
	dashboardCmd.Flags().BoolVar(&dashboardJSON, "json", false, "output as JSON (deprecated, use --output)")
	dashboardCmd.Flags().BoolVar(&dashboardAll, "all", false, "include archived sessions")
	dashboardCmd.Flags().StringVar(&dashboardOutput, "output", "text", "output format: text or json")
	dashboardCmd.Flags().BoolVar(&dashboardOrchestrator, "orchestrator", false, "show orchestrator unified view")
}

func runDashboard(cmd *cobra.Command, args []string) error {
	opCtx, cleanup, err := newOpContextWithStorage()
	if err != nil {
		return fmt.Errorf("failed to connect to storage: %w", err)
	}
	defer cleanup()

	// Determine output format (--json for backward compat, --output takes precedence)
	outputFormat := dashboardOutput
	if dashboardJSON && outputFormat == "text" {
		outputFormat = "json"
	}

	if dashboardOrchestrator {
		return runOrchestratorDashboard(cmd, opCtx, outputFormat)
	}

	result, err := ops.Dashboard(opCtx, &ops.DashboardRequest{
		IncludeArchived: dashboardAll,
	})
	if err != nil {
		return handleError(err)
	}

	if len(result.Entries) == 0 {
		ui.PrintWarning("No sessions found")
		fmt.Println("\nCreate a session with: agm session new <name>")
		return nil
	}

	if outputFormat == "json" {
		return printJSON(result)
	}

	printDashboardTable(cmd, result)
	return nil
}

func printDashboardTable(cmd *cobra.Command, result *ops.DashboardResult) {
	out := cmd.OutOrStdout()

	// Header
	_, _ = fmt.Fprintf(out, "%-25s %-18s %-10s %-10s %-5s %-20s\n",
		"NAME", "STATE", "DURATION", "MODE", "INTR", "MODEL")
	_, _ = fmt.Fprintf(out, "%-25s %-18s %-10s %-10s %-5s %-20s\n",
		"----", "-----", "--------", "----", "----", "-----")

	for _, e := range result.Entries {
		name := e.Name
		if len(name) > 24 {
			name = name[:21] + "..."
		}

		state := e.State
		if !e.TmuxAlive && state != "OFFLINE" {
			state = "OFFLINE"
		}

		// Add state indicator
		stateDisplay := stateWithIndicator(state)

		model := e.Model
		if len(model) > 19 {
			model = model[:16] + "..."
		}

		intrStr := fmt.Sprintf("%d", e.InterruptCount)

		_, _ = fmt.Fprintf(out, "%-25s %-18s %-10s %-10s %-5s %-20s\n",
			name, stateDisplay, e.TimeInState, e.PermissionMode, intrStr, model)
	}

	_, _ = fmt.Fprintf(out, "\n%d session(s) | %s\n", result.Total, result.Timestamp)
}

func stateWithIndicator(state string) string {
	switch state {
	case "WORKING":
		return "* WORKING"
	case "DONE":
		return ". DONE"
	case "USER_PROMPT":
		return "! USER_PROMPT"
	case "COMPACTING":
		return "~ COMPACTING"
	case "OFFLINE":
		return "x OFFLINE"
	default:
		return "? " + state
	}
}

func runOrchestratorDashboard(cmd *cobra.Command, opCtx *ops.OpContext, outputFormat string) error {
	result, err := ops.OrchestratorDashboard(opCtx, &ops.OrchestratorDashboardRequest{})
	if err != nil {
		return handleError(err)
	}

	if outputFormat == "json" {
		return printJSON(result)
	}

	printOrchestratorDashboardTable(cmd, result)
	return nil
}

func printOrchestratorDashboardTable(cmd *cobra.Command, result *ops.OrchestratorDashboardResult) {
	out := cmd.OutOrStdout()

	_, _ = fmt.Fprintf(out, "=== AGM ORCHESTRATOR DASHBOARD ===\n\n")
	_, _ = fmt.Fprintf(out, "Timestamp: %s\n\n", result.Timestamp)

	// Sessions summary
	_, _ = fmt.Fprintf(out, "SESSIONS\n")
	_, _ = fmt.Fprintf(out, "  Total:    %d\n", result.Sessions.Total)
	_, _ = fmt.Fprintf(out, "  Active:   %d\n", result.Sessions.Active)
	_, _ = fmt.Fprintf(out, "  Stopped:  %d\n", result.Sessions.Stopped)
	_, _ = fmt.Fprintf(out, "  Archived: %d\n\n", result.Sessions.Archived)

	// Resources
	_, _ = fmt.Fprintf(out, "RESOURCES\n")
	_, _ = fmt.Fprintf(out, "  Load:  %.2f, %.2f, %.2f (1m, 5m, 15m)\n",
		result.Resources.Load.Load1, result.Resources.Load.Load5, result.Resources.Load.Load15)
	_, _ = fmt.Fprintf(out, "  Memory: %d/%d MB (%.1f%% used)\n",
		result.Resources.Memory.UsedMB, result.Resources.Memory.TotalMB, result.Resources.Memory.UsedPercent)
	if len(result.Resources.Disk) > 0 {
		disk := result.Resources.Disk[0]
		_, _ = fmt.Fprintf(out, "  Disk:  %.1f/%.1f GB (%.1f%% used) at %s\n\n",
			disk.UsedGB, disk.TotalGB, disk.UsedPercent, disk.Mount)
	} else {
		_, _ = fmt.Fprintf(out, "\n")
	}

	// Throughput
	_, _ = fmt.Fprintf(out, "THROUGHPUT\n")
	_, _ = fmt.Fprintf(out, "  Commits/hour:   %d\n", result.Metrics.CommitsPerHour)
	_, _ = fmt.Fprintf(out, "  Workers launched: %d\n\n", result.Metrics.WorkersLaunched)

	// Alerts
	if len(result.Alerts) > 0 {
		_, _ = fmt.Fprintf(out, "ALERTS\n")
		for _, alert := range result.Alerts {
			level := alert.Level
			if level == "critical" {
				level = "🚨 " + level
			} else {
				level = "⚠️  " + level
			}
			_, _ = fmt.Fprintf(out, "  [%s] %s (%s)\n", level, alert.Message, alert.Value)
		}
		_, _ = fmt.Fprintf(out, "\n")
	}

	// Trust leaderboard
	_, _ = fmt.Fprintf(out, "TRUST LEADERBOARD (Total: %d sessions)\n", result.Trust.Total)
	if len(result.Trust.Top) > 0 {
		_, _ = fmt.Fprintf(out, "  Top 5:\n")
		for i, entry := range result.Trust.Top {
			_, _ = fmt.Fprintf(out, "    %d. %s (score: %d, events: %d)\n",
				i+1, entry.SessionName, entry.Score, entry.TotalEvents)
		}
	}
	if len(result.Trust.Bottom) > 0 {
		_, _ = fmt.Fprintf(out, "  Bottom 5:\n")
		for i, entry := range result.Trust.Bottom {
			_, _ = fmt.Fprintf(out, "    %d. %s (score: %d, events: %d)\n",
				i+1, entry.SessionName, entry.Score, entry.TotalEvents)
		}
	}
	_, _ = fmt.Fprintf(out, "\n")

}
