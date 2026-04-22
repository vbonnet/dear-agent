package session

import (
	"fmt"
	"time"

	"github.com/vbonnet/dear-agent/agm/internal/dolt"
	"github.com/vbonnet/dear-agent/agm/internal/manifest"
	"github.com/vbonnet/dear-agent/agm/internal/state"
	"github.com/vbonnet/dear-agent/agm/internal/tmux"
)

// DetectState determines the current state of a session by parsing terminal content.
//
// States:
//   - OFFLINE:  Session doesn't exist in tmux
//   - READY:    Claude prompt (❯) visible, Claude process still running — idle, waiting for input
//   - DONE:     Claude prompt (❯) visible, Claude process exited — session finished
//   - WORKING:  Spinner visible or stuck (actively processing)
//   - USER_PROMPT: Blocked on auth (y/N) or input (numbered options)
func DetectState(sessionName string) (string, error) {
	exists, err := tmux.HasSession(sessionName)
	if err != nil {
		return manifest.StateOffline, fmt.Errorf("failed to check session: %w", err)
	}
	if !exists {
		return manifest.StateOffline, nil
	}

	// Parse terminal content for accurate state detection
	paneContent, err := tmux.CapturePaneOutput(sessionName, 30)
	if err != nil {
		// Can't read pane — fall back to DONE (safe default)
		return manifest.StateDone, nil
	}

	detector := state.NewDetector()
	result := detector.DetectState(paneContent, time.Now())

	mappedState := mapTerminalStateToManifest(result.State)

	// If terminal shows a prompt, distinguish READY (Claude still running)
	// from DONE (Claude process has exited). A visible prompt with Claude
	// still running means it's idle and waiting for input, not finished.
	if mappedState == manifest.StateDone {
		claudeRunning, err := tmux.IsClaudeRunning(sessionName)
		if err == nil && claudeRunning {
			return manifest.StateReady, nil
		}
	}

	return mappedState, nil
}

// DetectionResult contains detection results with confidence scoring
type DetectionResult struct {
	State      string
	Confidence float64 // 0.0-1.0, where 1.0 = very confident
	Reason     string  // Human-readable explanation
}

// DetectStateWithConfidence performs state detection with confidence scoring
// by parsing terminal content and mapping to manifest states.
func DetectStateWithConfidence(sessionName string) (*DetectionResult, error) {
	exists, err := tmux.HasSession(sessionName)
	if err != nil {
		return nil, fmt.Errorf("failed to check session: %w", err)
	}
	if !exists {
		return &DetectionResult{
			State:      manifest.StateOffline,
			Confidence: 1.0,
			Reason:     "Session does not exist in tmux",
		}, nil
	}

	paneContent, err := tmux.CapturePaneOutput(sessionName, 30)
	if err != nil {
		return &DetectionResult{
			State:      manifest.StateDone,
			Confidence: 0.5,
			Reason:     "Cannot read terminal content, defaulting to DONE",
		}, nil
	}

	detector := state.NewDetector()
	result := detector.DetectState(paneContent, time.Now())
	mappedState := mapTerminalStateToManifest(result.State)

	// If terminal shows a prompt, distinguish READY from DONE by checking
	// whether Claude's process is still running.
	if mappedState == manifest.StateDone {
		claudeRunning, err := tmux.IsClaudeRunning(sessionName)
		if err == nil && claudeRunning {
			mappedState = manifest.StateReady
		}
	}

	var confidence float64
	switch result.Confidence {
	case "high":
		confidence = 0.95
	case "medium":
		confidence = 0.7
	default:
		confidence = 0.4
	}

	return &DetectionResult{
		State:      mappedState,
		Confidence: confidence,
		Reason:     fmt.Sprintf("Terminal parsing: %s (%s)", result.State, result.Evidence),
	}, nil
}

// mapTerminalStateToManifest converts terminal-parsed state.State values to
// manifest state constants used by the rest of AGM.
func mapTerminalStateToManifest(s state.State) string {
	switch s {
	case state.StateReady:
		return manifest.StateDone
	case state.StateThinking:
		return manifest.StateWorking
	case state.StateBlockedAuth, state.StateBlockedInput:
		return manifest.StateUserPrompt
	case state.StateBlockedPermission:
		return manifest.StatePermissionPrompt
	case state.StateStuck:
		return manifest.StateWorking
	case state.StateWaitingAgent:
		return manifest.StateWaitingAgent
	case state.StateLooping:
		return manifest.StateLooping
	case state.StateBackgroundTasksView:
		return manifest.StateBackgroundTasks
	case state.StateUnknown:
		return manifest.StateDone
	default:
		return manifest.StateDone
	}
}

// CheckSessionDelivery determines if a session can receive input by checking
// tmux session existence and reading pane content. This is the sole authority
// for delivery decisions — display state is irrelevant.
//
// Returns:
//   - CanReceiveNotFound: tmux session does not exist
//   - CanReceiveYes:      prompt (❯) visible, can deliver
//   - CanReceiveNo:       permission dialog active, needs human
//   - CanReceiveQueue:    busy (no prompt), queue for later
func CheckSessionDelivery(tmuxName string) state.CanReceive {
	// Axis 1: Does the tmux session exist?
	exists, err := tmux.HasSession(tmuxName)
	if err != nil || !exists {
		return state.CanReceiveNotFound
	}

	// Axis 2: Can we type into it right now?
	paneContent, err := tmux.CapturePaneOutput(tmuxName, 30)
	if err != nil {
		// Session exists but can't read pane — assume busy, queue
		return state.CanReceiveQueue
	}
	detector := state.NewDetector()
	return detector.CheckCanReceive(paneContent)
}

// UpdateSessionState updates the state field in manifest with timestamp and source
func UpdateSessionState(manifestPath string, state string, source string, sessionID string, adapter *dolt.Adapter) error {
	// Read from Dolt
	if adapter == nil || sessionID == "" {
		return fmt.Errorf("Dolt adapter and sessionID required")
	}

	m, err := adapter.GetSession(sessionID)
	if err != nil {
		return fmt.Errorf("failed to read session from Dolt: %w", err)
	}

	m.State = state
	m.StateUpdatedAt = time.Now()
	m.StateSource = source
	m.UpdatedAt = time.Now()

	// Write to Dolt
	if err := adapter.UpdateSession(m); err != nil {
		return fmt.Errorf("failed to update session in Dolt: %w", err)
	}

	return nil
}
