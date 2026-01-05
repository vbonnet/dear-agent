package main

import (
	"github.com/spf13/cobra"
)

var (
	formatFlag string // Global output format flag
)

var rootCmd = &cobra.Command{
	Use:   "csm-test-tmux",
	Short: "Automated CSM testing in isolated tmux sessions",
	Long: `csm-test-tmux manages isolated test sessions for Claude Code development.

Creates detached tmux sessions with independent CSM state, enabling:
- Parallel test execution without session pollution
- Automated command injection for CSM testing
- Automated output capture for verification

Designed for AI agents and human developers.`,
	Version:      Version,
	SilenceUsage: true,  // Don't print usage on errors (we handle errors ourselves)
	SilenceErrors: true, // Don't print error messages (we format them ourselves)
	Run: func(cmd *cobra.Command, args []string) {
		// Show help if no subcommand
		cmd.Help()
	},
}

func init() {
	// Global flags
	rootCmd.PersistentFlags().StringVar(
		&formatFlag,
		"format",
		"text",
		"Output format: text|json",
	)
}

func Execute() error {
	return rootCmd.Execute()
}
