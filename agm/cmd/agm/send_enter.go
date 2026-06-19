package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/vbonnet/dear-agent/agm/internal/tmux"
	"github.com/vbonnet/dear-agent/internal/override"
)

var (
	sendEnterForce  bool
	sendEnterReason string
)

var sendEnterCmd = &cobra.Command{
	Use:   "enter <session-name>",
	Short: "Send Enter to submit content in the input line",
	Long: `Send Enter (C-m) to submit whatever is currently in the session's input line.

Before sending, captures the pane to verify the input line has content.
If the input is empty, warns and does not send (prevents accidental blank submissions).

Use --force to skip the content check and send Enter unconditionally.

Examples:
  agm send enter my-session
  agm send enter worker-42 --force`,
	Args: cobra.ExactArgs(1),
	RunE: runSendEnter,
}

func init() {
	sendEnterCmd.Flags().BoolVar(&sendEnterForce, "force", false, "Skip input content check and send Enter unconditionally — requires --reason")
	sendEnterCmd.Flags().StringVar(&sendEnterReason, "reason", "", "Justification for --force, recorded in the override audit log")
	sendGroupCmd.AddCommand(sendEnterCmd)
}

func runSendEnter(cmd *cobra.Command, args []string) (retErr error) {
	sessionName := args[0]

	// Audit trail: log every send-enter occurrence (evidence of ENTER bug tracking)
	defer func() {
		logCommandAudit("send.enter", sessionName, map[string]string{
			"force": fmt.Sprintf("%v", sendEnterForce),
		}, retErr)
	}()

	// The --force bypass of the empty-input check requires a recorded reason.
	if sendEnterForce {
		if gerr := override.Require(cmd.Context(), override.Guard{
			Tool: "agm send-enter",
			Flag: "--force",
			Gate: "empty-input check (refuses to submit a blank line)",
			Risk: override.RiskP2,
		}, sendEnterReason); gerr != nil {
			return gerr
		}
	}

	// Step 1: Capture pane to check input line content
	if !sendEnterForce {
		paneContent, err := tmux.CapturePaneOutput(sessionName, 50)
		if err != nil {
			return fmt.Errorf("failed to capture pane for session '%s': %w", sessionName, err)
		}

		// Check for queued/pasted input (this means there IS content to submit)
		hasQueued := false
		inputType, _ := tmux.ClassifyQueuedInput(paneContent)
		if inputType != tmux.QueuedInputNone {
			hasQueued = true
		}

		// Check if the input line itself has typed content
		hasTyped := tmux.InputLineHasContent(paneContent)

		if !hasQueued && !hasTyped {
			fmt.Fprintf(os.Stderr, "Input line is empty in session '%s' — not sending Enter (use --force to override)\n", sessionName)
			return fmt.Errorf("input line is empty, nothing to submit")
		}
	}

	// Step 2: Send Enter reliably (hex 0x0d with retry)
	if err := tmux.SendEnterReliable(sessionName); err != nil {
		return fmt.Errorf("failed to send Enter to session '%s': %w", sessionName, err)
	}

	_, _ = fmt.Fprintf(os.Stdout, "Enter sent to session '%s'\n", sessionName)

	if os.Getenv("AGM_DEBUG") == "1" {
		_, _ = fmt.Fprintf(os.Stdout, "DEBUG: send-enter session=%s force=%v\n", sessionName, sendEnterForce)
	}

	return nil
}
