package main

import (
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"
	"github.com/vbonnet/dear-agent/agm/internal/tmux"
	"github.com/vbonnet/dear-agent/internal/override"
)

var (
	sendClearForce  bool
	sendClearReason string
)

var sendClearCmd = &cobra.Command{
	Use:   "clear <session-name>",
	Short: "Clear the input prompt contents without submitting",
	Long: `Clear whatever is in the session's input line without submitting it.

Sends C-c (cancel) followed by C-u (kill line) to reliably clear all input.
Captures the pane before and after to verify clearing worked.

Unlike clear-input, this command clears ANY input (AGM or human) without
submitting it. Use --force to bypass safety checks.

Examples:
  agm send clear my-session
  agm send clear worker-42 --force`,
	Args: cobra.ExactArgs(1),
	RunE: runSendClear,
}

func init() {
	sendClearCmd.Flags().BoolVar(&sendClearForce, "force", false, "Bypass safety checks — requires --reason")
	sendClearCmd.Flags().StringVar(&sendClearReason, "reason", "", "Justification for --force, recorded in the override audit log")
	sendGroupCmd.AddCommand(sendClearCmd)
}

func runSendClear(cmd *cobra.Command, args []string) error {
	sessionName := args[0]

	// The --force escalation (extra C-a C-k clear sequence, bypassing the
	// post-clear safety check) requires a recorded justification.
	if sendClearForce {
		if gerr := override.Require(cmd.Context(), override.Guard{
			Tool: "agm send-clear",
			Flag: "--force",
			Gate: "post-clear safety check (escalates to an unconditional C-a C-k clear)",
			Risk: override.RiskP2,
		}, sendClearReason); gerr != nil {
			return gerr
		}
	}

	// Step 1: Capture pane before clearing
	beforeContent, err := tmux.CapturePaneOutput(sessionName, 50)
	if err != nil {
		return fmt.Errorf("failed to capture pane for session '%s': %w", sessionName, err)
	}

	// Check if there's anything to clear
	inputType, _ := tmux.ClassifyQueuedInput(beforeContent)
	hasQueued := inputType != tmux.QueuedInputNone
	hasTyped := tmux.InputLineHasContent(beforeContent)

	if !hasQueued && !hasTyped {
		_, _ = fmt.Fprintf(os.Stdout, "Input already empty in session '%s' — nothing to clear\n", sessionName)
		return nil
	}

	if os.Getenv("AGM_DEBUG") == "1" {
		_, _ = fmt.Fprintf(os.Stdout, "DEBUG: send-clear session=%s queued=%v typed=%v inputType=%d\n",
			sessionName, hasQueued, hasTyped, inputType)
	}

	// Step 2: Send C-c to cancel any pending input, then C-u to kill the line
	// C-c dismisses [Pasted text] overlays and cancels partial input
	// C-u kills the entire line from cursor position backward
	if err := tmux.SendKeys(sessionName, "C-c"); err != nil {
		return fmt.Errorf("failed to send C-c to session '%s': %w", sessionName, err)
	}

	time.Sleep(200 * time.Millisecond)

	if err := tmux.SendKeys(sessionName, "C-u"); err != nil {
		return fmt.Errorf("failed to send C-u to session '%s': %w", sessionName, err)
	}

	// Step 3: Verify clearing worked
	time.Sleep(500 * time.Millisecond)

	afterContent, err := tmux.CapturePaneOutput(sessionName, 50)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: could not verify input was cleared: %v\n", err)
		_, _ = fmt.Fprintf(os.Stdout, "Clear keys sent to session '%s' (verification skipped)\n", sessionName)
		return nil
	}

	afterType, _ := tmux.ClassifyQueuedInput(afterContent)
	afterTyped := tmux.InputLineHasContent(afterContent)

	if afterType != tmux.QueuedInputNone || afterTyped {
		if sendClearForce {
			// Force mode: try additional clear sequence C-a C-k (home + kill to end of line)
			if err := tmux.SendKeys(sessionName, "C-a"); err == nil {
				time.Sleep(100 * time.Millisecond)
				_ = tmux.SendKeys(sessionName, "C-k")
			}
			time.Sleep(300 * time.Millisecond)
			_, _ = fmt.Fprintf(os.Stdout, "Force-cleared input in session '%s' (sent C-c C-u C-a C-k)\n", sessionName)
			return nil
		}
		fmt.Fprintf(os.Stderr, "Warning: input may not be fully cleared in session '%s'\n", sessionName)
		return fmt.Errorf("input not fully cleared — try with --force")
	}

	_, _ = fmt.Fprintf(os.Stdout, "Input cleared in session '%s'\n", sessionName)
	return nil
}
