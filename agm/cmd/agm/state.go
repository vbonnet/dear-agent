package main

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/vbonnet/dear-agent/agm/internal/session"
)

var stateCmd = &cobra.Command{
	Use:   "state",
	Short: "Manage session state detection",
	Long: `Detect and manage session readiness states.

States:
  • READY            - Session at Claude prompt, ready for input
  • THINKING         - Session working/thinking
  • PERMISSION_PROMPT - Session waiting for user permission
  • COMPACTING       - Session compacting/summarizing context
  • OFFLINE          - Session doesn't exist in tmux

Examples:
  # Detect current state
  agm session state detect coordination-research

  # Detect with confidence scoring
  agm session state detect coordination-research --confidence`,
	Args: cobra.ArbitraryArgs,
	RunE: groupRunE,
}

var (
	stateDetectShowConfidence bool
	stateSetSource            string
)

var stateDetectCmd = &cobra.Command{
	Use:   "detect <session-name>",
	Short: "Detect current session state",
	Long: `Detect the current readiness state of a session by analyzing tmux pane content.

This command is used for validation and debugging of state detection logic.

States detected:
  • OFFLINE          - Session does not exist
  • COMPACTING       - Session is compacting context (Wrangling…)
  • PERMISSION_PROMPT - Waiting for bash/edit permission
  • READY            - At Claude prompt (❯)
  • THINKING         - Session exists but state unclear

Detection fragility:
  State detection uses tmux pane content parsing, which is inherently fragile.
  False positive rate should be measured on real sessions.

Examples:
  # Basic detection
  agm session state detect coordination-research

  # With confidence scoring
  agm session state detect coordination-research --confidence`,
	Args: cobra.ExactArgs(1),
	RunE: runStateDetect,
}

var stateSetCmd = &cobra.Command{
	Use:   "set <session-name> <state>",
	Short: "Set session state manually or via hook",
	Long: `Set the current state of a session. This is typically called by Claude Code hooks
to notify AGM of state transitions, but can also be used manually for debugging.

Valid states:
  • READY            - Session at Claude prompt, ready for input
  • THINKING         - Session working/thinking
  • PERMISSION_PROMPT - Session waiting for user permission
  • COMPACTING       - Session compacting/summarizing context
  • OFFLINE          - Session doesn't exist in tmux

Examples:
  # Set state from hook
  agm session state set coordination-research THINKING --source hook

  # Set state manually (for debugging)
  agm session state set coordination-research READY --source manual`,
	Args: cobra.ExactArgs(2),
	RunE: runStateSet,
}

func init() {
	stateDetectCmd.Flags().BoolVar(
		&stateDetectShowConfidence,
		"confidence",
		false,
		"Show confidence score and reasoning",
	)

	stateSetCmd.Flags().StringVar(
		&stateSetSource,
		"source",
		"manual",
		"Source of state update (hook|manual|tmux)",
	)

	stateCmd.AddCommand(stateDetectCmd)
	stateCmd.AddCommand(stateSetCmd)
	sessionCmd.AddCommand(stateCmd)
}

func runStateDetect(cmd *cobra.Command, args []string) error {
	sessionName := args[0]

	if stateDetectShowConfidence {
		// Detect with confidence scoring
		result, err := session.DetectStateWithConfidence(sessionName)
		if err != nil {
			return fmt.Errorf("failed to detect state: %w", err)
		}

		fmt.Printf("Session: %s\n", sessionName)
		fmt.Printf("State: %s\n", result.State)
		fmt.Printf("Confidence: %.1f%%\n", result.Confidence*100)
		fmt.Printf("Reason: %s\n", result.Reason)
	} else {
		// Basic detection
		state, err := session.DetectState(sessionName)
		if err != nil {
			return fmt.Errorf("failed to detect state: %w", err)
		}

		fmt.Printf("Session '%s' state: %s\n", sessionName, state)
	}

	return nil
}

func runStateSet(cmd *cobra.Command, args []string) error {
	sessionName := args[0]
	newState := args[1]

	// Validate state
	validStates := []string{
		"READY",
		"THINKING",
		"PERMISSION_PROMPT",
		"COMPACTING",
		"OFFLINE",
	}

	isValid := false
	for _, valid := range validStates {
		if newState == valid {
			isValid = true
			break
		}
	}

	if !isValid {
		return fmt.Errorf("invalid state '%s'. Valid states: %s", newState, validStates)
	}

	// Get Dolt adapter for session resolution
	adapter, _ := getStorage()
	if adapter != nil {
		defer adapter.Close()
	}

	// Resolve session
	m, manifestPath, err := session.ResolveIdentifier(sessionName, cfg.SessionsDir, adapter)
	if err != nil {
		return fmt.Errorf("failed to resolve session: %w", err)
	}

	// Update state
	err = session.UpdateSessionState(manifestPath, newState, stateSetSource, m.SessionID, adapter)
	if err != nil {
		return fmt.Errorf("failed to update state: %w", err)
	}

	fmt.Printf("Updated session '%s' state: %s (source: %s)\n", m.Name, newState, stateSetSource)

	return nil
}
