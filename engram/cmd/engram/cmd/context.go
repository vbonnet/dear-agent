package cmd

import (
	"github.com/spf13/cobra"
)

var contextCmd = &cobra.Command{
	Use:   "context",
	Short: "Monitor and manage context window usage",
	Long: `Context window management commands for monitoring token usage and preventing
performance degradation across different LLM providers.

This system provides:
  • Model-specific sweet spot thresholds (Claude, Gemini, GPT families)
  • Multi-CLI support (Claude Code, Gemini CLI, OpenCode, Codex)
  • Smart compaction recommendations (phase-aware)
  • Zone classification (safe, warning, danger, critical)

COMMANDS
  status     - Show current context usage and zone
  check      - Check context usage and get compaction recommendation
  models     - List supported models and their thresholds
  set-usage  - Manually set context usage (for testing)

EXAMPLES
  # Show current context status
  $ engram context status

  # Check if compaction recommended at phase start
  $ engram context check --model claude-sonnet-4.5 --tokens 140000/200000 --phase-state start

  # List all supported models
  $ engram context models --list

  # Show thresholds for specific model
  $ engram context models --model gemini-2.0-flash

ZONES
  ✅ safe     - Below warning threshold, quality maintained
  ⚠️  warning - Light degradation, consider compaction
  🔥 danger   - Significant quality loss, compact recommended
  🚨 critical  - High utilization, compaction recommended

DOCUMENTATION
  See pkg/context/README.md for library API documentation.
  See swarm/context-management/RESEARCH-REPORT.md for benchmark data.`,
}

func init() {
	rootCmd.AddCommand(contextCmd)
}
