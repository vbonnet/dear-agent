package cmd

import (
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"
	"github.com/vbonnet/dear-agent/engram/internal/analytics"
)

var summaryFormat string

var summaryCmd = &cobra.Command{
	Use:   "summary",
	Short: "Show aggregate session statistics",
	Long: `Show aggregate statistics across all Wayfinder sessions.

Displays total sessions, completion rate, total time spent, average duration,
and cost statistics.

EXAMPLES
  # Show summary
  $ engram analytics summary

  # Output as JSON
  $ engram analytics summary --format json`,
	RunE: runSummary,
}

func init() {
	analyticsCmd.AddCommand(summaryCmd)

	summaryCmd.Flags().StringVarP(&summaryFormat, "format", "f", "markdown", "Output format (markdown|json)")
}

func runSummary(cmd *cobra.Command, args []string) error {
	// 1. Get telemetry path
	telemetryPath := getTelemetryPath()

	// 2. Parse all sessions
	parser := analytics.NewParser(telemetryPath)
	eventsBySession, err := parser.ParseAll()
	if err != nil {
		return fmt.Errorf("failed to parse telemetry: %w", err)
	}

	if len(eventsBySession) == 0 {
		fmt.Println("No Wayfinder sessions found in telemetry.")
		return nil
	}

	// 3. Aggregate sessions
	aggregator := analytics.NewAggregator()
	sessions, err := aggregator.AggregateSessions(eventsBySession)
	if err != nil {
		return fmt.Errorf("failed to aggregate sessions: %w", err)
	}

	// 4. Compute summary
	summary := aggregator.ComputeSummary(sessions)

	// 5. Format output
	var output string
	switch summaryFormat {
	case "markdown", "md":
		formatter := &analytics.MarkdownFormatter{}
		output, err = formatter.FormatSummary(summary)
	case "json":
		// Marshal summary to JSON
		data, jsonErr := json.MarshalIndent(summary, "", "  ")
		if jsonErr != nil {
			return fmt.Errorf("failed to marshal summary to JSON: %w", jsonErr)
		}
		output = string(data)
	default:
		return fmt.Errorf("invalid format: %s (use markdown|json)", summaryFormat)
	}

	if err != nil {
		return fmt.Errorf("failed to format output: %w", err)
	}

	fmt.Print(output)
	return nil
}
