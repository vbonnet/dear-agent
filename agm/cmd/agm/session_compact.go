package main

import (
	"fmt"
	"time"

	"github.com/spf13/cobra"
	"github.com/vbonnet/dear-agent/agm/internal/manifest"
	"github.com/vbonnet/dear-agent/agm/internal/ops"
	"github.com/vbonnet/dear-agent/agm/internal/session"
	"github.com/vbonnet/dear-agent/agm/internal/tmux"
	"github.com/vbonnet/dear-agent/agm/internal/ui"
)

var (
	sessionCompactArgs    string
	sessionCompactMonitor bool
	sessionCompactTimeout time.Duration
)

var sessionCompactCmd = &cobra.Command{
	Use:   "compact <identifier>",
	Short: "Trigger context compaction and monitor for completion",
	Long: `Trigger /compact in a running session with state detection and optional monitoring.

This is a higher-level wrapper around 'agm send compact' that:
  1. Resolves the session by name, ID, or UUID
  2. Checks session state before sending (warns if busy)
  3. Sends /compact with optional preservation instructions
  4. Monitors for completion (polls until session returns to ready state)

Examples:
  # Trigger compaction and wait for completion
  agm session compact my-session

  # Compact with preservation instructions
  agm session compact my-session --compact-args "preserve context about auth refactor"

  # Fire and forget (don't wait for completion)
  agm session compact my-session --monitor=false

See Also:
  • agm send compact    - Low-level compact command (no state detection)
  • agm session context - Show current context usage`,
	Args: cobra.ExactArgs(1),
	RunE: runSessionCompact,
}

func init() {
	sessionCompactCmd.Flags().StringVar(&sessionCompactArgs, "compact-args", "", "Compaction instructions (text appended after /compact)")
	sessionCompactCmd.Flags().BoolVar(&sessionCompactMonitor, "monitor", true, "Wait for compaction to complete")
	sessionCompactCmd.Flags().DurationVar(&sessionCompactTimeout, "timeout", 5*time.Minute, "Maximum time to wait for compaction")
	sessionCmd.AddCommand(sessionCompactCmd)
}

func runSessionCompact(_ *cobra.Command, args []string) error {
	identifier := args[0]

	// Resolve session via Dolt
	opCtx, cleanup, err := newOpContextWithStorage()
	if err != nil {
		return fmt.Errorf("failed to connect to Dolt storage: %w", err)
	}
	defer cleanup()

	getResult, opErr := ops.GetSession(opCtx, &ops.GetSessionRequest{
		Identifier: identifier,
	})
	if opErr != nil {
		return handleError(opErr)
	}

	s := getResult.Session
	tmuxName := s.TmuxSession
	if tmuxName == "" {
		tmuxName = s.Name
	}

	// Check tmux session exists
	exists, err := tmux.HasSession(tmuxName)
	if err != nil {
		return fmt.Errorf("failed to check tmux session: %w", err)
	}
	if !exists {
		return fmt.Errorf("session '%s' is not running (tmux session '%s' not found).\n\nSuggestions:\n  - Resume session: agm session resume %s\n  - Check status: agm session get %s", s.Name, tmuxName, s.Name, s.Name)
	}

	// Detect current state
	currentState, err := session.DetectState(tmuxName)
	if err != nil {
		ui.PrintWarning(fmt.Sprintf("Could not detect session state: %v (proceeding anyway)", err))
	} else {
		switch currentState {
		case manifest.StateOffline:
			return fmt.Errorf("session '%s' is offline.\n\nSuggestions:\n  - Resume session: agm session resume %s", s.Name, s.Name)
		case manifest.StateUserPrompt:
			return fmt.Errorf("session '%s' is waiting for user input. Resolve the prompt before compacting.\n\nSuggestions:\n  - Attach to session: agm session resume %s", s.Name, s.Name)
		case manifest.StateWorking, manifest.StateCompacting:
			ui.PrintWarning(fmt.Sprintf("Session '%s' appears busy (state: %s). Compaction may not start until current work completes.", s.Name, currentState))
		case manifest.StateDone:
			// Ready to compact
		}
	}

	// Build and send /compact command
	command := buildCompactCommand(sessionCompactArgs)
	if err := tmux.SendSlashCommandSafe(tmuxName, command); err != nil {
		return fmt.Errorf("failed to send compact command: %w", err)
	}

	ui.PrintSuccess(fmt.Sprintf("Sent %s to session '%s'", command, s.Name))

	// Monitor for completion if requested
	if !sessionCompactMonitor {
		return nil
	}

	fmt.Println()
	monitorCompaction(tmuxName, s.Name, sessionCompactTimeout)
	return nil
}

// monitorCompaction polls session state until compaction completes or timeout.
func monitorCompaction(tmuxName, displayName string, timeout time.Duration) {
	const pollInterval = 2 * time.Second

	start := time.Now()
	deadline := start.Add(timeout)
	lastState := ""

	for time.Now().Before(deadline) {
		time.Sleep(pollInterval)

		state, err := session.DetectState(tmuxName)
		if err != nil {
			// Transient error, keep polling
			continue
		}

		elapsed := time.Since(start).Round(time.Second)

		if state != lastState {
			fmt.Printf("  [%s] State: %s\n", elapsed, state)
			lastState = state
		}

		if state == manifest.StateDone {
			ui.PrintSuccess(fmt.Sprintf("Compaction completed in %s", elapsed))
			return
		}
	}

	elapsed := time.Since(start).Round(time.Second)
	ui.PrintWarning(fmt.Sprintf("Monitoring timed out after %s. Compaction may still be running.", elapsed))
	fmt.Printf("\nCheck status:\n  agm session get %s\n  agm session context %s\n", displayName, displayName)
}
