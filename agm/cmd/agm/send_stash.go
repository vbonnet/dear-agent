package main

import (
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"
	"github.com/vbonnet/dear-agent/agm/internal/tmux"
)

var sendStashCmd = &cobra.Command{
	Use:   "stash <session-name>",
	Short: "Stash the current input message in Claude Code",
	Long: `Send Ctrl+S to stash the current message in Claude Code.

This preserves any human-typed text in the input line while clearing it,
allowing AGM message delivery to proceed. The stashed message is automatically
restored (unstashed) on the next user interaction.

Use this before sending a message to a session that has human text in the
input line — it saves the text instead of discarding it.

Note: Unstash happens automatically when the user next interacts with the
session. No manual unstash is needed.

Examples:
  agm send stash my-session`,
	Args: cobra.ExactArgs(1),
	RunE: runSendStash,
}

func init() {
	sendGroupCmd.AddCommand(sendStashCmd)
}

func runSendStash(cmd *cobra.Command, args []string) error {
	sessionName := args[0]

	// Step 1: Capture pane to verify there's content to stash
	paneContent, err := tmux.CapturePaneOutput(sessionName, 50)
	if err != nil {
		return fmt.Errorf("failed to capture pane for session '%s': %w", sessionName, err)
	}

	hasQueued := false
	inputType, _ := tmux.ClassifyQueuedInput(paneContent)
	if inputType != tmux.QueuedInputNone {
		hasQueued = true
	}
	hasTyped := tmux.InputLineHasContent(paneContent)

	if !hasQueued && !hasTyped {
		fmt.Fprintf(os.Stdout, "Input already empty in session '%s' — nothing to stash\n", sessionName)
		return nil
	}

	// Step 2: Send Ctrl+S to stash
	if err := tmux.SendKeys(sessionName, "C-s"); err != nil {
		return fmt.Errorf("failed to send Ctrl+S to session '%s': %w", sessionName, err)
	}

	// Step 3: Verify input was cleared (stash should clear the input line)
	time.Sleep(500 * time.Millisecond)

	afterContent, err := tmux.CapturePaneOutput(sessionName, 50)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: could not verify stash worked: %v\n", err)
		fmt.Fprintf(os.Stdout, "Stash sent to session '%s' (verification skipped)\n", sessionName)
		return nil
	}

	afterType, _ := tmux.ClassifyQueuedInput(afterContent)
	afterTyped := tmux.InputLineHasContent(afterContent)

	if afterType != tmux.QueuedInputNone || afterTyped {
		fmt.Fprintf(os.Stderr, "Warning: input may not have been stashed in session '%s' — Ctrl+S may not be supported in this harness\n", sessionName)
		return nil
	}

	fmt.Fprintf(os.Stdout, "Message stashed in session '%s'\n", sessionName)
	fmt.Fprintf(os.Stdout, "Note: stashed message will be restored automatically on next send\n")

	if os.Getenv("AGM_DEBUG") == "1" {
		fmt.Fprintf(os.Stdout, "DEBUG: send-stash session=%s\n", sessionName)
	}

	return nil
}
