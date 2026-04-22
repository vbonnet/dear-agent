package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/vbonnet/dear-agent/engram/cmd/engram/internal/cli"
	"github.com/vbonnet/dear-agent/engram/internal/analytics"
)

var showFormat string

var showCmd = &cobra.Command{
	Use:   "show <session-id>",
	Short: "Show detailed session timeline",
	Long: `Show detailed information for a specific Wayfinder session.

Displays complete phase timeline, metrics breakdown (AI time vs. wait time),
and session metadata.

EXAMPLES
  # Show session details
  $ engram analytics show abc123-def456

  # Output as JSON
  $ engram analytics show abc123-def456 --format json`,
	Args: cobra.ExactArgs(1),
	RunE: runShow,
}

func init() {
	analyticsCmd.AddCommand(showCmd)

	showCmd.Flags().StringVarP(&showFormat, "format", "f", "markdown", "Output format (markdown|json)")
}

func runShow(cmd *cobra.Command, args []string) error {
	sessionID := args[0]

	// 1. Validate inputs
	if err := cli.ValidateNonEmpty("session-id", sessionID); err != nil {
		return err
	}

	if err := cli.ValidateOutputFormat(showFormat, cli.FormatMarkdown, cli.FormatJSON); err != nil {
		return err
	}

	// 2. Get telemetry path
	telemetryPath := getTelemetryPath()

	// 2. Parse session
	parser := analytics.NewParser(telemetryPath)
	events, err := parser.ParseSession(sessionID)
	if err != nil {
		return fmt.Errorf("failed to parse session: %w", err)
	}

	if len(events) == 0 {
		return fmt.Errorf("session not found: %s", sessionID)
	}

	// 3. Aggregate session
	aggregator := analytics.NewAggregator()
	session, err := aggregator.AggregateSession(sessionID, events)
	if err != nil {
		return fmt.Errorf("failed to aggregate session: %w", err)
	}

	// 4. Format output
	var output string
	switch showFormat {
	case "markdown", "md":
		formatter := &analytics.MarkdownFormatter{}
		output, err = formatter.FormatSession(session)
	case "json":
		formatter := &analytics.JSONFormatter{Pretty: true}
		output, err = formatter.Format([]analytics.Session{*session})
	default:
		return fmt.Errorf("invalid format: %s (use markdown|json)", showFormat)
	}

	if err != nil {
		return fmt.Errorf("failed to format output: %w", err)
	}

	fmt.Print(output)
	return nil
}
