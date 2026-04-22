// DEPRECATED: This command is for internal testing only.
// Use 'agm capture' and 'agm state' for production capture functionality.
// This test command will be removed in a future release.

package main

import (
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/vbonnet/dear-agent/agm/internal/tmux"
)

var (
	testCaptureJSON  bool
	testCaptureLines int
)

var testCaptureCmd = &cobra.Command{
	Use:   "capture <name>",
	Short: "Capture output from a test session",
	Long: `Capture the tmux pane output from a test session.

This command captures the visible content of the tmux pane, which includes
the Claude conversation, command output, and any other terminal content.

Examples:
  # Capture last 30 lines (default)
  agm test capture my-test

  # Capture last 100 lines
  agm test capture my-test --lines 100

  # Get JSON output for automation
  agm test capture my-test --json`,
	Args: cobra.ExactArgs(1),
	RunE: runTestCapture,
}

func init() {
	testCaptureCmd.Flags().BoolVar(
		&testCaptureJSON,
		"json",
		false,
		"Output as JSON for automation",
	)
	testCaptureCmd.Flags().IntVar(
		&testCaptureLines,
		"lines",
		30,
		"Number of lines to capture from the pane",
	)

	// Mark as hidden - use common commands with --test flag instead
	testCaptureCmd.Hidden = true

	testCmd.AddCommand(testCaptureCmd)
}

// CaptureResult represents captured output
type CaptureResult struct {
	Name       string    `json:"name"`
	Lines      []string  `json:"lines"`
	Count      int       `json:"count"`
	CapturedAt time.Time `json:"captured_at"`
}

func runTestCapture(cmd *cobra.Command, args []string) error {
	name := args[0]
	tmuxName := fmt.Sprintf("agm-test-%s", name)

	// Check session exists
	exists, err := tmux.HasSession(tmuxName)
	if err != nil {
		return fmt.Errorf("failed to check tmux session: %w", err)
	}
	if !exists {
		return fmt.Errorf("session '%s' does not exist.\n\nSuggestions:\n  • Create session: agm test create %s\n  • List sessions: tmux ls", name, name)
	}

	// Capture pane output
	// Use -p to print to stdout, -S to specify start line (negative = from end)
	captureCmd := exec.Command("tmux", "capture-pane", "-t", tmuxName, "-p", "-S", fmt.Sprintf("-%d", testCaptureLines))
	output, err := captureCmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to capture output: %w\n\nSuggestions:\n  • Check session is alive: tmux has-session -t %s", err, tmuxName)
	}

	// Split into lines
	lines := strings.Split(strings.TrimRight(string(output), "\n"), "\n")

	// Format output
	result := &CaptureResult{
		Name:       name,
		Lines:      lines,
		Count:      len(lines),
		CapturedAt: time.Now(),
	}

	if testCaptureJSON {
		jsonBytes, err := json.MarshalIndent(result, "", "  ")
		if err != nil {
			return fmt.Errorf("failed to marshal JSON: %w", err)
		}
		fmt.Println(string(jsonBytes))
	} else {
		// Human-readable output - just print the captured lines
		for _, line := range lines {
			fmt.Println(line)
		}
	}

	return nil
}
