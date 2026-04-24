package retrospective

import (
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/vbonnet/dear-agent/wayfinder/cmd/wayfinder-session/internal/history"
	"github.com/vbonnet/dear-agent/wayfinder/cmd/wayfinder-session/internal/status"
)

// LogRewindEvent is the main entry point for rewind retrospective logging
//
// Orchestrates:
// 1. Magnitude calculation (skip if magnitude 0)
// 2. User prompting (if needed)
// 3. Context capture (parallel)
// 4. Dual logging: WAYFINDER-HISTORY.md (JSON) + S11-retrospective.md (markdown)
//
// Errors are logged to stderr but don't block rewind operation (fail-gracefully).
func LogRewindEvent(projectDir string, fromPhase, toPhase string, flags RewindFlags) error {
	// Wrap in defer/recover to ensure rewind never fails due to logger crash
	defer func() {
		if r := recover(); r != nil {
			fmt.Fprintf(os.Stderr, "Error: retrospective logger panic: %v\n", r)
		}
	}()

	// Calculate magnitude (number of phases between from and to)
	magnitude, err := CalculateMagnitude(fromPhase, toPhase)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to calculate magnitude: %v\n", err)
		return nil // Non-blocking error
	}

	// Skip logging for magnitude 0 (no-op rewind)
	if magnitude == 0 {
		return nil
	}

	// Read status for context capture
	st, err := status.ReadFrom(projectDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to read status: %v\n", err)
		return nil // Non-blocking error
	}

	// Prompt user for context (if needed)
	userCtx, err := PromptUserForContext(magnitude, flags)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to prompt user: %v\n", err)
		userCtx = &UserProvidedContext{} // Continue with empty context
	}

	// Capture context snapshot (parallel git/deliverables/phase)
	snapshot := CaptureContext(projectDir, st)

	// Build rewind event data
	data := &RewindEventData{
		FromPhase: fromPhase,
		ToPhase:   toPhase,
		Magnitude: magnitude,
		Timestamp: time.Now(),
		Prompted:  (magnitude >= 1 && !flags.NoPrompt && flags.Reason == ""),
		Reason:    userCtx.Reason,
		Learnings: userCtx.Learnings,
		Context:   snapshot,
	}

	// Dual logging: History (JSON) + S11 (markdown)
	if err := LogToHistory(projectDir, data); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to log to history: %v\n", err)
	}

	if err := AppendToS11(projectDir, data); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to append to S11: %v\n", err)
	}

	return nil
}

// CalculateMagnitude calculates number of phases between from and to
//
// Returns |phaseIndex(from) - phaseIndex(to)|
// Handles edge cases: unknown phases, wrap-around (e.g., S11→W0)
func CalculateMagnitude(fromPhase, toPhase string) (int, error) {
	allPhases := status.AllPhases()

	// Find indices
	fromIdx := findPhaseIndex(allPhases, fromPhase)
	toIdx := findPhaseIndex(allPhases, toPhase)

	if fromIdx == -1 {
		return 0, fmt.Errorf("unknown from phase: %s", fromPhase)
	}
	if toIdx == -1 {
		return 0, fmt.Errorf("unknown to phase: %s", toPhase)
	}

	// Calculate magnitude (absolute difference)
	magnitude := fromIdx - toIdx
	if magnitude < 0 {
		magnitude = -magnitude
	}

	return magnitude, nil
}

// findPhaseIndex returns the index of phase in allPhases, or -1 if not found
func findPhaseIndex(allPhases []string, phase string) int {
	for i, p := range allPhases {
		if p == phase {
			return i
		}
	}
	return -1
}

// LogToHistory logs rewind event to WAYFINDER-HISTORY.md (JSON)
//
// Marshals RewindEventData to map[string]interface{} for history.Event.Data field.
// Reuses existing history.go infrastructure.
func LogToHistory(projectDir string, data *RewindEventData) error {
	// Initialize history logger
	hist := history.New(projectDir)

	// Marshal RewindEventData to map[string]interface{}
	jsonBytes, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("failed to marshal rewind data: %w", err)
	}

	var eventData map[string]interface{}
	if err := json.Unmarshal(jsonBytes, &eventData); err != nil {
		return fmt.Errorf("failed to unmarshal to map: %w", err)
	}

	// Add event type constant
	const EventTypeRewindLogged = "rewind.logged"

	// Log event using existing history infrastructure
	if err := hist.AppendEvent(EventTypeRewindLogged, data.ToPhase, eventData); err != nil {
		return fmt.Errorf("failed to append history event: %w", err)
	}

	return nil
}
