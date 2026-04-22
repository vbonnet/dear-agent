package main

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"github.com/vbonnet/dear-agent/agm/internal/ops"
	"github.com/vbonnet/dear-agent/agm/internal/ui"
)

var sessionHealthAll bool

var sessionHealthCmd = &cobra.Command{
	Use:   "health [name]",
	Short: "Check health of one or all active sessions",
	Long: `Check the health of one or more AGM sessions.

Reports resource usage, responsiveness, error rate, and trust score.
Health levels: HEALTHY, WARNING, CRITICAL.

Examples:
  agm session health my-session           # Check a single session
  agm session health --all                # Check all active sessions
  agm session health my-session -o json   # JSON output`,
	Args: cobra.MaximumNArgs(1),
	RunE: runSessionHealth,
}

func init() {
	sessionHealthCmd.Flags().BoolVar(&sessionHealthAll, "all", false, "Check all active sessions")
	sessionCmd.AddCommand(sessionHealthCmd)
}

func runSessionHealth(_ *cobra.Command, args []string) error {
	if !sessionHealthAll && len(args) == 0 {
		return fmt.Errorf("session name required, or use --all")
	}

	opCtx, cleanup, err := newOpContextWithStorage()
	if err != nil {
		return fmt.Errorf("failed to connect to storage: %w", err)
	}
	defer cleanup()

	req := &ops.SessionHealthRequest{
		All: sessionHealthAll,
	}
	if len(args) > 0 {
		req.Identifier = args[0]
	}

	result, opErr := ops.CheckSessionHealth(opCtx, req)
	if opErr != nil {
		return handleError(opErr)
	}

	return printResult(result, func() {
		printSessionHealthText(result)
	})
}

func printSessionHealthText(r *ops.SessionHealthResult) {
	if r.Total == 0 {
		fmt.Println("No sessions found.")
		return
	}

	for _, s := range r.Sessions {
		healthLabel := colorizeHealth(s.Health)
		fmt.Printf("%s  %s  [%s]\n", ui.Bold(s.Name), healthLabel, s.State)
		fmt.Printf("  ID:       %s\n", s.ID)
		fmt.Printf("  Status:   %s\n", s.Status)
		fmt.Printf("  Duration: %s  (started %s)\n", s.Duration, s.StartedAt)
		fmt.Printf("  Updated:  %s ago\n", s.TimeSinceLastUpdate)

		if s.PanePID > 0 {
			fmt.Printf("  CPU:      %.1f%%  Memory: %.0f MB (%.1f%%)\n",
				s.CPUPct, s.MemoryMB, s.MemoryPct)
		}
		if s.CommitCount > 0 {
			fmt.Printf("  Commits:  %d since session start\n", s.CommitCount)
		}
		if s.TrustScore != nil {
			fmt.Printf("  Trust:    %d/100\n", *s.TrustScore)
		}
		if len(s.Warnings) > 0 {
			fmt.Printf("  Warnings:\n")
			for _, w := range s.Warnings {
				fmt.Printf("    • %s\n", w)
			}
		}
		fmt.Println()
	}

	if r.Total > 1 {
		fmt.Printf("Total: %d session(s)\n", r.Total)
		counts := map[string]int{}
		for _, s := range r.Sessions {
			counts[s.Health]++
		}
		parts := []string{}
		if n := counts["healthy"]; n > 0 {
			parts = append(parts, ui.Green(fmt.Sprintf("%d healthy", n)))
		}
		if n := counts["warning"]; n > 0 {
			parts = append(parts, ui.Yellow(fmt.Sprintf("%d warning", n)))
		}
		if n := counts["critical"]; n > 0 {
			parts = append(parts, ui.Red(fmt.Sprintf("%d critical", n)))
		}
		if len(parts) > 0 {
			fmt.Printf("       %s\n", strings.Join(parts, "  "))
		}
	}
}

// colorizeHealth applies ANSI color to the health label.
func colorizeHealth(health string) string {
	switch health {
	case "healthy":
		return ui.Green("HEALTHY")
	case "warning":
		return ui.Yellow("WARNING")
	case "critical":
		return ui.Red("CRITICAL")
	default:
		return strings.ToUpper(health)
	}
}
