package hippocampus

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"time"
)

// TriggerState tracks consolidation trigger conditions.
type TriggerState struct {
	LastConsolidation time.Time `json:"last_consolidation"`
	SessionCount      int       `json:"session_count"`
	LastSessionID     string    `json:"last_session_id"`
}

// ShouldConsolidate checks if conditions are met for consolidation.
// Returns true if minGap has passed since last consolidation AND
// session count >= minSessions. A zero-value LastConsolidation (first run)
// satisfies the time condition.
func ShouldConsolidate(state TriggerState, minGap time.Duration, minSessions int) bool {
	timeMet := state.LastConsolidation.IsZero() || time.Since(state.LastConsolidation) >= minGap
	sessionsMet := state.SessionCount >= minSessions
	return timeMet && sessionsMet
}

// LoadTriggerState reads trigger state from a JSON file.
// Returns zero-value TriggerState if the file does not exist (not an error).
func LoadTriggerState(path string) (TriggerState, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return TriggerState{}, nil
		}
		return TriggerState{}, err
	}

	var state TriggerState
	if err := json.Unmarshal(data, &state); err != nil {
		return TriggerState{}, err
	}
	return state, nil
}

// SaveTriggerState writes trigger state to a JSON file.
// Creates parent directories if needed.
func SaveTriggerState(path string, state TriggerState) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}

	data, err := json.Marshal(state)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

// IncrementSession increments the session counter and updates the last session ID.
func (s *TriggerState) IncrementSession(sessionID string) {
	s.SessionCount++
	s.LastSessionID = sessionID
}

// OnSessionEnd is the session lifecycle hook that increments the session counter.
// Call this when a session ends to track consolidation trigger conditions.
// stateFile is the path to the trigger state JSON file.
func OnSessionEnd(stateFile, sessionID string) error {
	state, err := LoadTriggerState(stateFile)
	if err != nil {
		return fmt.Errorf("load trigger state: %w", err)
	}

	state.IncrementSession(sessionID)

	if err := SaveTriggerState(stateFile, state); err != nil {
		return fmt.Errorf("save trigger state: %w", err)
	}

	return nil
}

// ResetAfterConsolidation resets the state after successful consolidation.
func (s *TriggerState) ResetAfterConsolidation() {
	s.SessionCount = 0
	s.LastConsolidation = time.Now()
}
