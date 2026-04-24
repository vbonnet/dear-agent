package hippocampus

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// SessionLineage tracks parent-child relationships across session transitions
// (e.g., /clear, plan->implementation transitions, cross-machine teleportation).
type SessionLineage struct {
	SessionID       string    `json:"session_id"`
	ParentSessionID string    `json:"parent_session_id,omitempty"`
	CreatedAt       time.Time `json:"created_at"`
	TransitionType  string    `json:"transition_type,omitempty"` // "clear", "plan_to_impl", "teleport"
	Machine         string    `json:"machine,omitempty"`         // hostname for teleportation tracking
	Teleported      bool      `json:"teleported,omitempty"`      // true if session was teleported
}

// LineageStore persists session lineage to a JSON state file.
type LineageStore struct {
	stateFile string
}

// LineageState is the on-disk format for lineage data.
type LineageState struct {
	Sessions []SessionLineage `json:"sessions"`
}

// NewLineageStore creates a store backed by the given state file path.
func NewLineageStore(stateFile string) *LineageStore {
	return &LineageStore{stateFile: stateFile}
}

// DefaultLineageStore returns a store using the default path ~/.engram/lineage/state.json.
func DefaultLineageStore() *LineageStore {
	home, _ := os.UserHomeDir()
	return NewLineageStore(filepath.Join(home, ".engram", "lineage", "state.json"))
}

// TrackParent records a new session as a child of the given parent session.
func (ls *LineageStore) TrackParent(sessionID, parentSessionID, transitionType string) error {
	state, err := ls.load()
	if err != nil {
		state = &LineageState{}
	}

	hostname, _ := os.Hostname()

	entry := SessionLineage{
		SessionID:       sessionID,
		ParentSessionID: parentSessionID,
		CreatedAt:       time.Now(),
		TransitionType:  transitionType,
		Machine:         hostname,
	}

	state.Sessions = append(state.Sessions, entry)
	return ls.save(state)
}

// RegenerateSession creates a new session ID and records the current session as its parent.
// Returns the new session ID.
func (ls *LineageStore) RegenerateSession(currentSessionID, transitionType string) (string, error) {
	newID, err := generateSessionID()
	if err != nil {
		return "", fmt.Errorf("generate session ID: %w", err)
	}

	if err := ls.TrackParent(newID, currentSessionID, transitionType); err != nil {
		return "", err
	}

	return newID, nil
}

// MarkTeleported marks a session as having been teleported to the current machine.
func (ls *LineageStore) MarkTeleported(sessionID string) error {
	state, err := ls.load()
	if err != nil {
		return err
	}

	hostname, _ := os.Hostname()

	for i := range state.Sessions {
		if state.Sessions[i].SessionID == sessionID {
			state.Sessions[i].Teleported = true
			state.Sessions[i].Machine = hostname
			return ls.save(state)
		}
	}

	// Session not found — add a teleportation record
	state.Sessions = append(state.Sessions, SessionLineage{
		SessionID:  sessionID,
		CreatedAt:  time.Now(),
		Machine:    hostname,
		Teleported: true,
	})
	return ls.save(state)
}

// GetLineageChain returns the full parent chain for a session, ordered from
// the given session back to the root (oldest ancestor).
func (ls *LineageStore) GetLineageChain(sessionID string) ([]SessionLineage, error) {
	state, err := ls.load()
	if err != nil {
		return nil, err
	}

	// Build lookup map
	byID := make(map[string]SessionLineage, len(state.Sessions))
	for _, s := range state.Sessions {
		byID[s.SessionID] = s
	}

	var chain []SessionLineage
	visited := make(map[string]bool) // cycle protection
	current := sessionID

	for current != "" && !visited[current] {
		visited[current] = true
		entry, ok := byID[current]
		if !ok {
			break
		}
		chain = append(chain, entry)
		current = entry.ParentSessionID
	}

	return chain, nil
}

// load reads the lineage state from disk.
func (ls *LineageStore) load() (*LineageState, error) {
	data, err := os.ReadFile(ls.stateFile)
	if err != nil {
		if os.IsNotExist(err) {
			return &LineageState{}, nil
		}
		return nil, fmt.Errorf("read lineage state: %w", err)
	}

	var state LineageState
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, fmt.Errorf("parse lineage state: %w", err)
	}
	return &state, nil
}

// save writes the lineage state to disk atomically.
func (ls *LineageStore) save(state *LineageState) error {
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal lineage state: %w", err)
	}
	return atomicWriteFile(ls.stateFile, data)
}

// generateSessionID creates a random 8-byte hex session ID.
func generateSessionID() (string, error) {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
