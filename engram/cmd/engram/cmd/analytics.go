// Package cmd provides cmd-related functionality.
package cmd

import (
	"os"

	"github.com/spf13/cobra"
)

var telemetryPath string

var analyticsCmd = &cobra.Command{
	Use:   "analytics",
	Short: "Analyze Wayfinder session metrics",
	Long: `Analytics commands for viewing Wayfinder session data.

Analyzes telemetry data to show session durations, phase breakdowns,
AI time vs. wait time, and estimated costs.

COMMANDS
  list    - List all Wayfinder sessions
  show    - Show detailed session timeline
  summary - Show aggregate statistics

EXAMPLES
  # List all sessions
  $ engram analytics list

  # Show specific session
  $ engram analytics show abc123-def456

  # Use custom telemetry file
  $ engram analytics list --telemetry-path ~/custom/telemetry.jsonl

  # Export sessions to CSV
  $ engram analytics list --format csv > sessions.csv

  # Show aggregate statistics
  $ engram analytics summary

TELEMETRY FILE
  Priority: --telemetry-path flag > ENGRAM_TELEMETRY_PATH env > ~/.claude/telemetry.jsonl`,
}

func init() {
	rootCmd.AddCommand(analyticsCmd)

	// Add persistent flag for telemetry path (available to all subcommands)
	analyticsCmd.PersistentFlags().StringVar(&telemetryPath, "telemetry-path", "", "Path to telemetry file (default: ~/.claude/telemetry.jsonl)")
}

// getTelemetryPath returns the telemetry path with the following priority:
// 1. --telemetry-path flag
// 2. ENGRAM_TELEMETRY_PATH environment variable
// 3. Default: ~/.claude/telemetry.jsonl
func getTelemetryPath() string {
	if telemetryPath != "" {
		return os.ExpandEnv(telemetryPath)
	}

	if envPath := os.Getenv("ENGRAM_TELEMETRY_PATH"); envPath != "" {
		return envPath
	}

	return os.ExpandEnv("$HOME/.claude/telemetry.jsonl")
}
