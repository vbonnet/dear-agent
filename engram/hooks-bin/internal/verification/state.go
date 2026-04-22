// Package verification provides state tracking for post-action verification hooks.
// It enables escalation when verification findings are ignored by the session.
package verification

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

// PendingVerification represents an unaddressed verification finding.
type PendingVerification struct {
	ID            string    `json:"id"`
	Type          string    `json:"type"` // "bead_close" or "notification_send"
	CreatedAt     time.Time `json:"created_at"`
	Message       string    `json:"message"`
	ToolUsesSince int       `json:"tool_uses_since"` // incremented on each tool use
	SwarmLabel    string    `json:"swarm_label,omitempty"`
	BeadID        string    `json:"bead_id,omitempty"`
	Recipient     string    `json:"recipient,omitempty"`
}

// State holds all pending verifications for the current session.
type State struct {
	Pending []PendingVerification `json:"pending"`
}

// DefaultStatePath returns the default verification state file path.
func DefaultStatePath() string {
	home := os.Getenv("HOME")
	if home == "" {
		home = "/tmp"
	}
	return filepath.Join(home, ".claude", "verification-state.json")
}

// LoadState reads verification state from disk.
// Returns empty state if file doesn't exist or is unreadable.
func LoadState(path string) State {
	data, err := os.ReadFile(path)
	if err != nil {
		return State{}
	}
	var state State
	if err := json.Unmarshal(data, &state); err != nil {
		return State{}
	}
	return state
}

// SaveState writes verification state to disk.
// Creates parent directories if needed.
func SaveState(path string, state State) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0750); err != nil {
		return err
	}
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0600)
}

// AddPending adds a new pending verification to state.
func (s *State) AddPending(v PendingVerification) {
	v.CreatedAt = time.Now()
	v.ToolUsesSince = 0
	s.Pending = append(s.Pending, v)
}

// IncrementAll increments tool_uses_since for all pending verifications.
func (s *State) IncrementAll() {
	for i := range s.Pending {
		s.Pending[i].ToolUsesSince++
	}
}

// RemoveByType removes all pending verifications of a given type matching an ID.
func (s *State) RemoveByType(vType, id string) {
	filtered := s.Pending[:0]
	for _, v := range s.Pending {
		if v.Type != vType || v.ID != id {
			filtered = append(filtered, v)
		}
	}
	s.Pending = filtered
}

// RemoveBySwarm removes all bead_close verifications for a given swarm.
func (s *State) RemoveBySwarm(swarmLabel string) {
	filtered := s.Pending[:0]
	for _, v := range s.Pending {
		if v.Type != "bead_close" || v.SwarmLabel != swarmLabel {
			filtered = append(filtered, v)
		}
	}
	s.Pending = filtered
}

// PruneOld removes verifications older than maxAge.
func (s *State) PruneOld(maxAge time.Duration) {
	cutoff := time.Now().Add(-maxAge)
	filtered := s.Pending[:0]
	for _, v := range s.Pending {
		if v.CreatedAt.After(cutoff) {
			filtered = append(filtered, v)
		}
	}
	s.Pending = filtered
}
