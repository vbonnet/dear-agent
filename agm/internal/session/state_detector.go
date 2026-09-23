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
//   - READY:    Harness prompt/composer visible, harness process still running — idle, waiting for input
//   - DONE:     Prompt visible with no known running harness process, or unknown terminal state
//   - WORKING:  Spinner visible or stuck (actively processing)
//   - USER_PROMPT: Blocked on auth (y/N) or input (numbered options)
func DetectState(sessionName string) (string, error) {
	result, err := detectStateWithProbes(sessionName, productionStateDetectionProbes())
	if result == nil {
		return manifest.StateOffline, err
	}
	return result.State, err
}

// ObservationEvidence records why a compatibility display state was produced.
// Strict lifecycle consumers must require EvidenceLive instead of treating the
// display projection alone as authority.
type ObservationEvidence string

const (
	// EvidenceLive means an identity-aware liveness probe confirmed the harness.
	EvidenceLive ObservationEvidence = "live"
	// EvidenceTerminal means terminal parsing found a prompt without a live harness.
	EvidenceTerminal ObservationEvidence = "terminal"
	// EvidenceUnknown means the available probes could not establish provenance.
	EvidenceUnknown ObservationEvidence = "unknown"
	// EvidenceUnreadable means the tmux pane existed but could not be captured.
	EvidenceUnreadable ObservationEvidence = "unreadable"
	// EvidenceAbsent means the tmux session did not exist.
	EvidenceAbsent ObservationEvidence = "absent"
)

// DetectionResult contains detection results with confidence scoring and typed
// observation provenance.
type DetectionResult struct {
	State      string
	Confidence float64 // 0.0-1.0, where 1.0 = very confident
	Reason     string  // Human-readable explanation
	Evidence   ObservationEvidence
}

// DetectStateWithConfidence performs state detection with confidence scoring
// by parsing terminal content and mapping to manifest states. Because this
// compatibility API has no expected-harness or exact-pane identity, it never
// mints positive live evidence; mutation gates must use an identity-bound
// readiness capability.
func DetectStateWithConfidence(sessionName string) (*DetectionResult, error) {
	result, err := detectStateWithProbes(sessionName, productionStateDetectionProbes())
	if err != nil {
		return nil, err
	}
	return result, nil
}

type stateDetectionProbes struct {
	hasSession               func(string) (bool, error)
	capturePane              func(string, int) (string, error)
	detectTerminal           func(string, time.Time) state.DetectionResult
	isInteractiveHarnessIdle func(string) (bool, error)
	isHarnessAlive           func(string) (bool, error)
	now                      func() time.Time
}

func productionStateDetectionProbes() stateDetectionProbes {
	detector := state.NewDetector()
	return stateDetectionProbes{
		hasSession:  tmux.HasSession,
		capturePane: tmux.CapturePaneLogicalANSIOutput,
		detectTerminal: func(output string, observedAt time.Time) state.DetectionResult {
			return detector.DetectState(output, observedAt)
		},
		isInteractiveHarnessIdle: isInteractiveHarnessIdle,
		now:                      time.Now,
	}
}

func detectStateWithProbes(sessionName string, probes stateDetectionProbes) (*DetectionResult, error) {
	exists, err := probes.hasSession(sessionName)
	if err != nil {
		return &DetectionResult{State: manifest.StateOffline, Evidence: EvidenceUnknown}, fmt.Errorf("failed to check session: %w", err)
	}
	if !exists {
		return &DetectionResult{
			State:      manifest.StateOffline,
			Confidence: 1.0,
			Reason:     "Session does not exist in tmux",
			Evidence:   EvidenceAbsent,
		}, nil
	}

	paneContent, err := probes.capturePane(sessionName, 30)
	if err != nil {
		return &DetectionResult{ //nolint:nilerr // intentional: caller signals via separate bool/optional
			State:      manifest.StateDone,
			Confidence: 0.5,
			Reason:     "Cannot read terminal content, defaulting to DONE",
			Evidence:   EvidenceUnreadable,
		}, nil
	}

	result := probes.detectTerminal(paneContent, probes.now())
	mappedState, idle, err := refineDetectedState(sessionName, result.State, probes)
	if err != nil {
		return &DetectionResult{State: manifest.StateDone, Evidence: EvidenceUnknown}, err
	}

	harnessAlive, livenessKnown, err := detectHarnessLiveness(sessionName, probes)
	if err != nil {
		return &DetectionResult{State: mappedState, Evidence: EvidenceUnknown}, err
	}

	return &DetectionResult{
		State:      mappedState,
		Confidence: confidenceScore(result.Confidence),
		Reason:     fmt.Sprintf("Terminal parsing: %s (%s)", result.State, result.Evidence),
		Evidence:   observationEvidence(result.State, idle, livenessKnown, harnessAlive),
	}, nil
}

func refineDetectedState(sessionName string, detected state.State, probes stateDetectionProbes) (string, bool, error) {
	mappedState := mapTerminalStateToManifest(detected)
	if mappedState != manifest.StateDone {
		return mappedState, false, nil
	}

	// If terminal shows a prompt/composer, distinguish READY from DONE by
	// checking whether a known interactive harness is still running. Unknown
	// parsing retains unknown evidence even when its compatibility projection is
	// refined to READY.
	idle, err := probes.isInteractiveHarnessIdle(sessionName)
	if err != nil {
		return mappedState, false, fmt.Errorf("failed to refine ready state: %w", err)
	}
	if idle {
		mappedState = manifest.StateReady
	}
	return mappedState, idle, nil
}

func detectHarnessLiveness(sessionName string, probes stateDetectionProbes) (alive, known bool, err error) {
	if probes.isHarnessAlive == nil {
		return false, false, nil
	}
	alive, err = probes.isHarnessAlive(sessionName)
	if err != nil {
		return false, false, fmt.Errorf("failed to establish harness liveness: %w", err)
	}
	return alive, true, nil
}

func observationEvidence(detected state.State, idle, livenessKnown, harnessAlive bool) ObservationEvidence {
	switch detected {
	case state.StateReady:
		if idle && livenessKnown && harnessAlive {
			return EvidenceLive
		}
		if !idle && livenessKnown && !harnessAlive {
			return EvidenceTerminal
		}
		return EvidenceUnknown
	case state.StateUnknown:
		return EvidenceUnknown
	case state.StateThinking,
		state.StateBlockedAuth,
		state.StateBlockedInput,
		state.StateBlockedPermission,
		state.StateStuck,
		state.StateWaitingAgent,
		state.StateLooping,
		state.StateBackgroundTasksView:
		if livenessKnown && harnessAlive {
			return EvidenceLive
		}
		return EvidenceUnknown
	}
	return EvidenceUnknown
}

func confidenceScore(confidence string) float64 {
	switch confidence {
	case "high":
		return 0.95
	case "medium":
		return 0.7
	default:
		return 0.4
	}
}

func isInteractiveHarnessIdle(sessionName string) (bool, error) {
	claudeRunning, err := tmux.IsClaudeRunning(sessionName)
	if err != nil {
		return false, fmt.Errorf("failed during Claude process check: %w", err)
	}
	if claudeRunning {
		return true, nil
	}

	codexIdle, err := tmux.IsCodexIdle(sessionName)
	if err != nil {
		return false, fmt.Errorf("failed during Codex composer check: %w", err)
	}
	if codexIdle {
		return true, nil
	}

	agyIdle, err := tmux.IsAgyIdle(sessionName)
	if err != nil {
		return false, fmt.Errorf("failed during AGY prompt check: %w", err)
	}
	if agyIdle {
		return true, nil
	}

	piIdle, err := tmux.IsPiIdle(sessionName)
	if err != nil {
		return false, fmt.Errorf("failed during Pi managed-state check: %w", err)
	}
	if piIdle {
		return true, nil
	}
	return false, nil
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
	paneContent, err := tmux.CapturePaneLogicalANSIOutput(tmuxName, 30)
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
		return fmt.Errorf("dolt adapter and sessionID required")
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
