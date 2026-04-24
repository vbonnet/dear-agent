package main

import (
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"
	"github.com/vbonnet/dear-agent/agm/internal/tmux"
)

var sendClearInputCmd = &cobra.Command{
	Use:   "clear-input [session-name]",
	Short: "Clear stuck AGM message from session input buffer",
	Long: `Inspect a session's input buffer and clear it if it contains a stuck AGM message.

This command captures the session's pane, inspects the queued input, and:
  - If input contains an AGM message header ([From: ...]) → sends Enter to submit it
  - If input is freeform human text → refuses with an error (human is typing)

Use this when 'agm send msg' reports a stuck paste-buffer from a previous AGM message.

Examples:
  agm send clear-input my-session
  agm send clear-input worker-42`,
	Args: cobra.ExactArgs(1),
	RunE: runClearInput,
}

func init() {
	sendGroupCmd.AddCommand(sendClearInputCmd)
}

func runClearInput(cmd *cobra.Command, args []string) error {
	sessionName := args[0]

	// Step 1: Capture the pane to inspect queued input
	paneContent, err := tmux.CapturePaneOutput(sessionName, 50)
	if err != nil {
		return fmt.Errorf("failed to capture pane for session '%s': %w", sessionName, err)
	}

	// Step 2: Classify the queued input
	inputType, description := tmux.ClassifyQueuedInput(paneContent)

	switch inputType {
	case tmux.QueuedInputNone:
		fmt.Fprintf(os.Stdout, "No queued input detected in session '%s'\n", sessionName)
		return nil

	case tmux.QueuedInputHuman:
		return fmt.Errorf("human input detected in session '%s' - not clearing. %s", sessionName, description)

	case tmux.QueuedInputAGM:
		// Stuck AGM message — safe to submit by sending Enter
		fmt.Fprintf(os.Stdout, "Detected stuck AGM message in session '%s'. Sending Enter to submit...\n", sessionName)

		if err := tmux.SendKeys(sessionName, "C-m"); err != nil {
			return fmt.Errorf("failed to send Enter to session '%s': %w", sessionName, err)
		}

		// Brief pause then verify it was cleared
		time.Sleep(500 * time.Millisecond)
		verifyContent, err := tmux.CapturePaneOutput(sessionName, 50)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Warning: could not verify input was cleared: %v\n", err)
			return nil
		}

		verifyType, _ := tmux.ClassifyQueuedInput(verifyContent)
		if verifyType != tmux.QueuedInputNone {
			// Try one more Enter
			if err := tmux.SendKeys(sessionName, "C-m"); err != nil {
				fmt.Fprintf(os.Stderr, "Warning: failed to send keys to session %s: %v\n", sessionName, err)
			}
			time.Sleep(300 * time.Millisecond)
			fmt.Fprintf(os.Stdout, "Sent additional Enter (input may still be processing)\n")
		} else {
			fmt.Fprintf(os.Stdout, "Stuck AGM message cleared successfully from session '%s'\n", sessionName)
		}

		// Log the action
		if os.Getenv("AGM_DEBUG") == "1" {
			fmt.Fprintf(os.Stdout, "DEBUG: clear-input action=submit session=%s type=agm description=%q\n", sessionName, description)
		}

		return nil

	default:
		return fmt.Errorf("unexpected input type in session '%s'", sessionName)
	}
}
