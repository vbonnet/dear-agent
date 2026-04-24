package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/vbonnet/dear-agent/agm/internal/manifest"
	"github.com/vbonnet/dear-agent/agm/internal/session"
	"github.com/vbonnet/dear-agent/agm/internal/tmux"
	"github.com/vbonnet/dear-agent/agm/internal/ui"
)

// Default wake prompt sent to resume the monitoring loop.
const defaultWakePrompt = `/loop 5m /orchestrate`

var sendWakeLoopCmd = &cobra.Command{
	Use:   "wake-loop <session>",
	Short: "Wake a session's monitoring loop via direct tmux input",
	Long: `Send a wake command to restart a session's monitoring loop.

This is used by the daemon when it detects a stale loop heartbeat.
The command checks the session state and only sends the wake if the
session is at a ready prompt (DONE state).

State behavior:
  DONE      → send wake prompt directly via tmux
  WORKING   → skip (session is active, loop may resume on its own)
  OFFLINE   → skip (session is down, cannot wake)
  COMPACTING → skip (session is busy, will retry later)
  Other     → skip with warning

Examples:
  agm send wake-loop orchestrator-v2
  agm send wake-loop meta-orchestrator --prompt "/loop 10m /orchestrate"`,
	Args: cobra.ExactArgs(1),
	RunE: runSendWakeLoop,
}

var wakeLoopPrompt string

func init() {
	sendGroupCmd.AddCommand(sendWakeLoopCmd)
	sendWakeLoopCmd.Flags().StringVar(&wakeLoopPrompt, "prompt", defaultWakePrompt, "Wake prompt to send")
}

func runSendWakeLoop(_ *cobra.Command, args []string) error {
	targetSession := args[0]

	// Check tmux session exists
	exists, err := tmux.HasSession(targetSession)
	if err != nil || !exists {
		fmt.Fprintf(os.Stderr, "Session '%s' not found in tmux (offline or not running)\n", targetSession)
		return nil // Not an error — session is just not available
	}

	// Get storage adapter (best-effort — wake can work without Dolt)
	adapter, err := getStorage()
	var currentState string
	if err == nil {
		defer adapter.Close()
		// Resolve session state
		m, manifestPath, resolveErr := session.ResolveIdentifier(targetSession, cfg.SessionsDir, adapter)
		if resolveErr == nil && m != nil {
			tmuxName := m.Tmux.SessionName
			if tmuxName == "" {
				tmuxName = targetSession
			}
			currentState = session.ResolveSessionState(tmuxName, m.State, m.Claude.UUID, m.StateUpdatedAt)
			if currentState != m.State {
				if err := session.UpdateSessionState(manifestPath, currentState, "hybrid", m.SessionID, adapter); err != nil {
					fmt.Fprintf(os.Stderr, "Warning: failed to persist session state: %v\n", err)
				}
			}
		}
	}

	// If we couldn't determine state, default to DONE (attempt wake)
	if currentState == "" {
		currentState = manifest.StateDone
	}

	switch currentState {
	case manifest.StateDone:
		// Session is at prompt — send wake
		if err := tmux.SendMultiLinePromptSafe(targetSession, wakeLoopPrompt, false); err != nil {
			return fmt.Errorf("failed to send wake prompt: %w", err)
		}
		ui.PrintSuccess(fmt.Sprintf("Sent wake-loop to '%s': %s", targetSession, wakeLoopPrompt))
		return nil

	case manifest.StateWorking, manifest.StateWaitingAgent, manifest.StateLooping:
		fmt.Fprintf(os.Stderr, "Session '%s' is %s — skipping wake (not idle)\n", targetSession, currentState)
		return nil

	case manifest.StateOffline:
		fmt.Fprintf(os.Stderr, "Session '%s' is OFFLINE — cannot wake\n", targetSession)
		return nil

	case manifest.StateCompacting:
		fmt.Fprintf(os.Stderr, "Session '%s' is COMPACTING — will retry later\n", targetSession)
		return nil

	default:
		fmt.Fprintf(os.Stderr, "Session '%s' has unknown state '%s' — skipping wake\n", targetSession, currentState)
		return nil
	}
}
