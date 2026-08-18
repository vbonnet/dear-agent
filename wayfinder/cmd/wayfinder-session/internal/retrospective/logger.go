package retrospective

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/vbonnet/dear-agent/wayfinder/cmd/wayfinder-session/internal/history"
	"github.com/vbonnet/dear-agent/wayfinder/cmd/wayfinder-session/internal/status"
)

// LogRewindEvent is the main entry point for rewind retrospective logging
//
// Orchestrates:
// 1. Magnitude calculation
// 2. User prompting (if needed)
// 3. Context capture (parallel)
// 4. Dual logging: WAYFINDER-HISTORY.jsonl (JSON Lines) + RETRO-retrospective.md (markdown)
//
// Required history and retrospective writes return an error to the rewind
// command so it cannot claim a trace-complete transition without its evidence.
func LogRewindEvent(projectDir string, fromPhase, toPhase string, flags RewindFlags) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("retrospective logger panic: %v", r)
		}
	}()
	return logRewindEvent(projectDir, fromPhase, toPhase, flags)
}

func logRewindEvent(projectDir string, fromPhase, toPhase string, flags RewindFlags) error {
	// Calculate magnitude (number of phases between from and to)
	magnitude, err := CalculateMagnitude(fromPhase, toPhase)
	if err != nil {
		return fmt.Errorf("calculate rewind magnitude: %w", err)
	}

	if err := history.New(projectDir).EnsureCurrentFile(); err != nil {
		return fmt.Errorf("prepare rewind history: %w", err)
	}

	// Read status for context capture
	st, err := status.ParseV2FromDir(projectDir)
	if err != nil {
		return fmt.Errorf("read rewind status: %w", err)
	}

	// Prompt user for context (if needed)
	userCtx, err := PromptUserForContext(magnitude, flags)
	if err != nil {
		return fmt.Errorf("collect rewind context: %w", err)
	}
	if userCtx == nil {
		userCtx = &UserProvidedContext{}
	}

	// Capture context snapshot (parallel git/deliverables/phase)
	snapshot := CaptureContextV2(projectDir, st)

	// Build rewind event data. Timestamp is stored UTC so the rendered
	// RFC3339 string carries a "Z" suffix and stays comparable across hosts.
	data := &RewindEventData{
		FromPhase: fromPhase,
		ToPhase:   toPhase,
		Magnitude: magnitude,
		Timestamp: time.Now().UTC(),
		Prompted:  (magnitude >= 1 && !flags.NoPrompt && flags.Reason == ""),
		Reason:    userCtx.Reason,
		Learnings: userCtx.Learnings,
		Context:   snapshot,
	}

	// Dual logging: History (JSON) + canonical RETRO deliverable (markdown)
	if err := LogToHistory(projectDir, data); err != nil {
		return fmt.Errorf("append rewind history: %w", err)
	}

	if err := AppendToRetro(projectDir, data); err != nil {
		return fmt.Errorf("append rewind retrospective: %w", err)
	}
	return nil
}

// CalculateMagnitude calculates number of phases between from and to
//
// Returns |phaseIndex(from) - phaseIndex(to)|
// Handles edge cases such as unknown canonical phase names.
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

// LogToHistory logs rewind event to WAYFINDER-HISTORY.jsonl (JSON Lines)
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
