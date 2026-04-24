package main

import (
	"fmt"
	"time"

	"github.com/spf13/cobra"
	"github.com/vbonnet/dear-agent/agm/internal/session"
	"github.com/vbonnet/dear-agent/agm/internal/state"
	"github.com/vbonnet/dear-agent/agm/internal/tmux"
)

var escapeUICmd = &cobra.Command{
	Use:   "escape-ui <session-name>",
	Short: "Escape harness UI overlays (Background Tasks, etc.)",
	Long: `Send keystrokes to dismiss UI overlays on a session.

This command is designed to recover sessions stuck in read-only UI overlays
such as the Background Tasks view in Claude Code. It avoids needing raw
tmux send-keys permissions by wrapping the operation in a safe AGM command.

Recovery sequence:
  1. Check if the session has an active overlay
  2. Send Left arrow key to dismiss the overlay
  3. Wait 200ms and re-check
  4. If still blocked, try Escape key as fallback
  5. Report success/failure

This command is safe to call even if no overlay is active — it will detect
the session state first and skip recovery if unnecessary.

Use cases:
  - Meta-orchestrator recovering a stuck session without tmux permissions
  - Automated pipeline recovery for Background Tasks overlay deadlock
  - Manual recovery when a session is stuck in a UI overlay

Examples:
  agm escape-ui my-session
  agm escape-ui orchestrator-v2`,
	Args: cobra.ExactArgs(1),
	RunE: runEscapeUI,
}

func init() {
	rootCmd.AddCommand(escapeUICmd)
}

func runEscapeUI(cmd *cobra.Command, args []string) error {
	sessionName := args[0]

	// Verify tmux session exists
	exists, err := tmux.HasSession(sessionName)
	if err != nil {
		return fmt.Errorf("failed to check tmux session: %w", err)
	}
	if !exists {
		return fmt.Errorf("session '%s' does not exist in tmux", sessionName)
	}

	// Check current delivery state
	canReceive := session.CheckSessionDelivery(sessionName)

	switch canReceive {
	case state.CanReceiveYes:
		fmt.Printf("Session '%s' has no active overlay (state: READY). No action needed.\n", sessionName)
		return nil

	case state.CanReceiveOverlay:
		// Overlay detected — attempt recovery
		fmt.Fprintf(cmd.ErrOrStderr(), "Overlay detected on '%s' — sending Left key to dismiss...\n", sessionName)
		return attemptOverlayDismissal(cmd, sessionName)

	case state.CanReceiveNo:
		fmt.Fprintf(cmd.ErrOrStderr(), "Session '%s' has a permission prompt (not an overlay). Use 'agm send approve' or 'agm send reject' instead.\n", sessionName)
		return fmt.Errorf("session has permission prompt, not a UI overlay")

	case state.CanReceiveQueue:
		// Session is busy — try sending Left anyway in case state detection missed an overlay
		fmt.Fprintf(cmd.ErrOrStderr(), "Session '%s' appears busy. Attempting overlay dismissal anyway...\n", sessionName)
		return attemptOverlayDismissal(cmd, sessionName)

	case state.CanReceiveNotFound:
		return fmt.Errorf("session '%s' tmux session not found", sessionName)

	default:
		fmt.Fprintf(cmd.ErrOrStderr(), "Session '%s' in unknown state '%s'. Attempting overlay dismissal...\n", sessionName, canReceive)
		return attemptOverlayDismissal(cmd, sessionName)
	}
}

// attemptOverlayDismissal tries Left arrow, then Escape to dismiss UI overlays.
func attemptOverlayDismissal(cmd *cobra.Command, sessionName string) error {
	// Step 1: Send Left arrow key
	if err := tmux.SendKeys(sessionName, "Left"); err != nil {
		return fmt.Errorf("failed to send Left key: %w", err)
	}

	// Step 2: Wait for overlay to close
	time.Sleep(200 * time.Millisecond)

	// Step 3: Re-check delivery state
	canReceive := session.CheckSessionDelivery(sessionName)

	if canReceive == state.CanReceiveYes {
		fmt.Fprintf(cmd.OutOrStdout(), "OK: overlay dismissed on '%s' (Left key worked)\n", sessionName)
		return nil
	}

	if canReceive == state.CanReceiveOverlay {
		// Left didn't work — try Escape
		fmt.Fprintf(cmd.ErrOrStderr(), "Left key did not dismiss overlay. Trying Escape...\n")
		if err := tmux.SendKeys(sessionName, "Escape"); err != nil {
			return fmt.Errorf("failed to send Escape key: %w", err)
		}

		time.Sleep(200 * time.Millisecond)

		canReceive = session.CheckSessionDelivery(sessionName)
		if canReceive == state.CanReceiveYes {
			fmt.Fprintf(cmd.OutOrStdout(), "OK: overlay dismissed on '%s' (Escape key worked)\n", sessionName)
			return nil
		}

		return fmt.Errorf("failed to dismiss overlay on '%s' (state after Escape: %s)", sessionName, canReceive)
	}

	// Overlay might have been dismissed but session is in another state
	fmt.Fprintf(cmd.OutOrStdout(), "OK: overlay dismissed on '%s' (session now in state: %s)\n", sessionName, canReceive)
	return nil
}
