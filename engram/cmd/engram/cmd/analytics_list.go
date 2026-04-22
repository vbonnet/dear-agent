package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/vbonnet/dear-agent/engram/cmd/engram/internal/cli"
	"github.com/vbonnet/dear-agent/engram/internal/analytics"
)

var (
	listFormat string
	listLimit  int
)

var listCmd = &cobra.Command{
	Use:   "list",
	Short: "List all Wayfinder sessions",
	Long: `List all Wayfinder sessions from telemetry data.

Shows session ID, project, start time, duration, phase count, and status
for all Wayfinder sessions found in the telemetry file.

The output can be formatted as Markdown (default), JSON, or CSV for
further processing or export.`,
	RunE: runList,
}

func init() {
	analyticsCmd.AddCommand(listCmd)

	listCmd.Flags().StringVarP(&listFormat, "format", "f", "markdown", "Output format (markdown|json|csv)")
	listCmd.Flags().IntVarP(&listLimit, "limit", "n", 0, "Limit number of sessions (0 = all)")
}

func runList(cmd *cobra.Command, args []string) error {
	// 1. Validate inputs
	if err := cli.ValidateOutputFormat(listFormat, cli.FormatMarkdown, cli.FormatJSON, cli.FormatCSV); err != nil {
		return err
	}

	if err := cli.ValidatePositive("limit", listLimit); err != nil {
		return err
	}

	// 2. Get telemetry path
	telemetryPath := getTelemetryPath()

	// 2. Parse telemetry
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

	// 4. Apply limit
	if listLimit > 0 && len(sessions) > listLimit {
		sessions = sessions[:listLimit]
	}

	// 5. Format output
	var output string
	switch listFormat {
	case "markdown", "md":
		formatter := &analytics.MarkdownFormatter{}
		output, err = formatter.Format(sessions)
	case "json":
		formatter := &analytics.JSONFormatter{Pretty: true}
		output, err = formatter.Format(sessions)
	case "csv":
		formatter := &analytics.CSVFormatter{}
		output, err = formatter.Format(sessions)
	default:
		return fmt.Errorf("invalid format: %s (use markdown|json|csv)", listFormat)
	}

	if err != nil {
		return fmt.Errorf("failed to format output: %w", err)
	}

	fmt.Print(output)
	return nil
}
