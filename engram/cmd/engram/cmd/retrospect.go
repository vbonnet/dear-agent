package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	"github.com/vbonnet/dear-agent/pkg/autoconfig"
)

var (
	retrospectSession string
	retrospectProject string
	retrospectLast    int
)

var retrospectCmd = &cobra.Command{
	Use:   "retrospect",
	Short: "View session retrospectives and trends",
	Long: `View retrospectives for individual sessions or aggregate trends
across multiple sessions for a project.

EXAMPLES
  # Single session
  $ engram retrospect --session abc123

  # Project trends (last 20 sessions)
  $ engram retrospect --project myproject --last 20`,
	RunE: runRetrospect,
}

func init() {
	rootCmd.AddCommand(retrospectCmd)
	retrospectCmd.Flags().StringVar(&retrospectSession, "session", "", "show retrospective for a single session")
	retrospectCmd.Flags().StringVar(&retrospectProject, "project", "", "show trends for a project")
	retrospectCmd.Flags().IntVar(&retrospectLast, "last", 10, "number of recent sessions to include in trends")
}

func runRetrospect(cmd *cobra.Command, args []string) error {
	if retrospectSession != "" {
		return showSessionRetrospective(retrospectSession)
	}
	if retrospectProject != "" {
		return showProjectTrends(retrospectProject, retrospectLast)
	}
	return fmt.Errorf("specify --session <id> or --project <name>")
}

func showSessionRetrospective(sessionID string) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}

	dir := filepath.Join(home, ".engram", "traces", sessionID)

	// Try retrospective.md first.
	retroPath := filepath.Join(dir, "retrospective.md")
	if data, err := os.ReadFile(retroPath); err == nil {
		fmt.Print(string(data))
		return nil
	}

	// Fall back to summary.json.
	summaryPath := filepath.Join(dir, "summary.json")
	data, err := os.ReadFile(summaryPath)
	if err != nil {
		return fmt.Errorf("no retrospective or summary found for session %q", sessionID)
	}

	var summary autoconfig.SessionSummary
	if err := json.Unmarshal(data, &summary); err != nil {
		return fmt.Errorf("parse summary: %w", err)
	}

	printSummary(summary)
	return nil
}

func showProjectTrends(project string, last int) error {
	projectHash := autoconfig.ProjectHash(project)
	baseline, err := autoconfig.LoadBaseline(projectHash)
	if err != nil {
		return fmt.Errorf("load baseline for %q: %w", project, err)
	}

	if len(baseline.Sessions) == 0 {
		return fmt.Errorf("no sessions found for project %q", project)
	}

	sessions := baseline.Sessions
	if last > 0 && last < len(sessions) {
		sessions = sessions[len(sessions)-last:]
	}

	fmt.Printf("# Project Trends: %s\n\n", project)
	fmt.Printf("Sessions: %d (showing last %d)\n\n", len(baseline.Sessions), len(sessions))

	fmt.Println("## Aggregates")
	fmt.Printf("  Avg Cost:       $%.4f\n", baseline.AvgCostUSD)
	fmt.Printf("  P50 Cost:       $%.4f\n", baseline.P50CostUSD)
	fmt.Printf("  P90 Cost:       $%.4f\n", baseline.P90CostUSD)
	fmt.Printf("  Avg Efficiency: %.2f\n", baseline.AvgTokenEfficiency)
	fmt.Printf("  P50 Efficiency: %.2f\n", baseline.P50Efficiency)
	fmt.Printf("  P90 Efficiency: %.2f\n\n", baseline.P90Efficiency)

	if len(baseline.AvgPhaseScores) > 0 {
		fmt.Println("## Phase Scores (avg)")
		phases := make([]string, 0, len(baseline.AvgPhaseScores))
		for p := range baseline.AvgPhaseScores {
			phases = append(phases, p)
		}
		sort.Strings(phases)
		for _, p := range phases {
			fmt.Printf("  %-20s %.2f\n", p, baseline.AvgPhaseScores[p])
		}
		fmt.Println()
	}

	fmt.Println("## Session History")
	fmt.Printf("  %-20s %10s %12s %8s %s\n", "SESSION", "COST", "EFFICIENCY", "SPANS", "ANOMALIES")
	fmt.Println("  " + strings.Repeat("-", 75))
	for _, s := range sessions {
		anomalies := "-"
		if len(s.Anomalies) > 0 {
			anomalies = strings.Join(s.Anomalies, "; ")
		}
		sid := s.SessionID
		if len(sid) > 20 {
			sid = sid[:17] + "..."
		}
		fmt.Printf("  %-20s $%9.4f %11.2f %8d %s\n", sid, s.TotalCostUSD, s.TokenEfficiency, s.SpanCount, anomalies)
	}

	return nil
}

func printSummary(s autoconfig.SessionSummary) {
	fmt.Printf("# Session Summary: %s\n\n", s.SessionID)
	fmt.Printf("  Cost:       $%.4f\n", s.TotalCostUSD)
	fmt.Printf("  Efficiency: %.2f\n", s.TokenEfficiency)
	fmt.Printf("  Spans:      %d\n", s.SpanCount)
	fmt.Printf("  Duration:   %.1fms\n", s.DurationMs)

	if len(s.PhaseScores) > 0 {
		fmt.Println("\n  Phase Scores:")
		for p, score := range s.PhaseScores {
			fmt.Printf("    %-20s %.2f\n", p, score)
		}
	}

	if len(s.Anomalies) > 0 {
		fmt.Println("\n  Anomalies:")
		for _, a := range s.Anomalies {
			fmt.Printf("    - %s\n", a)
		}
	}
}
