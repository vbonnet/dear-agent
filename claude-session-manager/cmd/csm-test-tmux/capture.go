package main

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"github.com/vbonnet/ai-tools/claude-session-manager/internal/testutil/session"
	"github.com/vbonnet/ai-tools/claude-session-manager/internal/testutil/tmux"
)

var (
	captureSessionsDir string
	captureLines       int
)

var captureCmd = &cobra.Command{
	Use:   "capture <name>",
	Short: "Capture output from the test session",
	Long: `Capture the last N lines of output from the test session.

Output is returned as text (default) or JSON array of lines.

Examples:
  # Capture last 10 lines (default)
  csm-test-tmux capture my-test

  # Capture last 50 lines
  csm-test-tmux capture my-test --lines 50

  # Get JSON output for parsing
  csm-test-tmux capture my-test --format json`,
	Args: cobra.ExactArgs(1),
	RunE: runCapture,
}

func init() {
	captureCmd.Flags().StringVar(
		&captureSessionsDir,
		"sessions-dir",
		"",
		"Directory for CSM session state (default: /tmp/csm-test-<name>)",
	)
	captureCmd.Flags().IntVar(
		&captureLines,
		"lines",
		10,
		"Number of lines to capture",
	)

	rootCmd.AddCommand(captureCmd)
}

func runCapture(cmd *cobra.Command, args []string) error {
	name := args[0]

	// Set defaults
	if captureSessionsDir == "" {
		captureSessionsDir = fmt.Sprintf("/tmp/csm-test-%s", name)
	}

	// Create session manager
	tmuxClient := tmux.New()
	mgr := session.New(tmuxClient)

	// Capture output
	opts := session.CaptureOptions{
		SessionsDir: captureSessionsDir,
		Lines:       captureLines,
	}

	result, err := mgr.Capture(name, opts)
	if err != nil {
		return formatError(err)
	}

	// For text format, print lines directly (more readable)
	// For JSON format, use the structured CaptureResult
	if formatFlag == "text" {
		fmt.Println(strings.Join(result.Lines, "\n"))
		return nil
	}

	return printOutput(result)
}
