package trigger

import (
	"encoding/json"
	"os"
	"time"
)

// TriggerState tracks cooldown and injection history.
type TriggerState struct {
	LastInjected map[string]time.Time `json:"last_injected"` // engramPath -> timestamp
}

// NewTriggerState creates a new empty TriggerState.
func NewTriggerState() *TriggerState {
	return &TriggerState{
		LastInjected: make(map[string]time.Time),
	}
}

// ShouldInject checks if the engram's cooldown has elapsed.
// cooldown is a duration string like "1h", "30m". Empty means always inject.
func (s *TriggerState) ShouldInject(engramPath string, cooldown string) bool {
	if cooldown == "" {
		return true
	}

	dur, err := time.ParseDuration(cooldown)
	if err != nil {
		// If cooldown can't be parsed, allow injection.
		return true
	}

	lastTime, ok := s.LastInjected[engramPath]
	if !ok {
		// Never injected before.
		return true
	}

	return time.Since(lastTime) >= dur
}

// RecordInjection marks an engram as injected now.
func (s *TriggerState) RecordInjection(engramPath string) {
	s.LastInjected[engramPath] = time.Now()
}

// LoadTriggerState reads state from JSON file. Returns empty state if file doesn't exist.
func LoadTriggerState(path string) (*TriggerState, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return NewTriggerState(), nil
		}
		return nil, err
	}

	state := NewTriggerState()
	if err := json.Unmarshal(data, state); err != nil {
		return nil, err
	}

	// Ensure map is initialized even if JSON had null.
	if state.LastInjected == nil {
		state.LastInjected = make(map[string]time.Time)
	}

	return state, nil
}

// Save writes state to JSON file.
func (s *TriggerState) Save(path string) error {
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(path, data, 0644)
}
