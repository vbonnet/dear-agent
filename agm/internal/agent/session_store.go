package agent

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// SessionStore manages the mapping between SessionIDs and tmux session names.
type SessionStore interface {
	// Get retrieves the session metadata for a given SessionID.
	Get(sessionID SessionID) (*SessionMetadata, error)

	// Set stores session metadata for a SessionID.
	Set(sessionID SessionID, metadata *SessionMetadata) error

	// Delete removes a session from the store.
	Delete(sessionID SessionID) error

	// List returns all stored sessions.
	List() (map[SessionID]*SessionMetadata, error)
}

// SessionMetadata contains information about a managed session.
type SessionMetadata struct {
	// TmuxName is the tmux session name.
	TmuxName string `json:"tmux_name"`

	// Title is the user-friendly session name (from /rename or /chat save).
	// If not set, display logic should fall back to TmuxName.
	Title string `json:"title,omitempty"`

	// CreatedAt is when the session was created.
	CreatedAt time.Time `json:"created_at"`

	// WorkingDir is the session's working directory.
	WorkingDir string `json:"working_dir"`

	// Project is the optional project identifier.
	Project string `json:"project,omitempty"`

	// UUID is the agent's native session identifier (Gemini UUID, Claude UUID, etc.).
	// For Gemini CLI, this is extracted from --list-sessions output.
	// Used for --resume flag to restore specific session state.
	UUID string `json:"uuid,omitempty"`

	// SystemPrompt is the optional system prompt/instruction for the session.
	// Updated via CommandSetSystemPrompt.
	SystemPrompt string `json:"system_prompt,omitempty"`
}

// JSONSessionStore implements SessionStore using a JSON file.
type JSONSessionStore struct {
	filePath string
	mu       sync.RWMutex
	sessions map[SessionID]*SessionMetadata
}

// NewJSONSessionStore creates a new JSON-backed session store.
//
// If filePath is empty, defaults to ~/.agm/sessions.json.
func NewJSONSessionStore(filePath string) (*JSONSessionStore, error) {
	if filePath == "" {
		homeDir, err := os.UserHomeDir()
		if err != nil {
			return nil, fmt.Errorf("failed to get home directory: %w", err)
		}
		filePath = filepath.Join(homeDir, ".agm", "sessions.json")
	}

	store := &JSONSessionStore{
		filePath: filePath,
		sessions: make(map[SessionID]*SessionMetadata),
	}

	// Create directory if it doesn't exist
	dir := filepath.Dir(filePath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create sessions directory: %w", err)
	}

	// Load existing sessions
	if err := store.load(); err != nil {
		return nil, err
	}

	return store, nil
}

// Get retrieves session metadata.
func (s *JSONSessionStore) Get(sessionID SessionID) (*SessionMetadata, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	metadata, exists := s.sessions[sessionID]
	if !exists {
		return nil, fmt.Errorf("session %s not found", sessionID)
	}

	return metadata, nil
}

// Set stores session metadata.
func (s *JSONSessionStore) Set(sessionID SessionID, metadata *SessionMetadata) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.sessions[sessionID] = metadata
	return s.save()
}

// Delete removes a session.
func (s *JSONSessionStore) Delete(sessionID SessionID) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	delete(s.sessions, sessionID)
	return s.save()
}

// List returns all sessions.
func (s *JSONSessionStore) List() (map[SessionID]*SessionMetadata, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	// Return a copy to prevent external modification
	result := make(map[SessionID]*SessionMetadata, len(s.sessions))
	for k, v := range s.sessions {
		result[k] = v
	}

	return result, nil
}

// load reads sessions from the JSON file.
func (s *JSONSessionStore) load() error {
	// If file doesn't exist, start with empty sessions
	if _, err := os.Stat(s.filePath); os.IsNotExist(err) {
		return nil
	}

	data, err := os.ReadFile(s.filePath)
	if err != nil {
		return fmt.Errorf("failed to read sessions file: %w", err)
	}

	// Handle empty file
	if len(data) == 0 {
		return nil
	}

	if err := json.Unmarshal(data, &s.sessions); err != nil {
		return fmt.Errorf("failed to parse sessions file: %w", err)
	}

	return nil
}

// save writes sessions to the JSON file atomically.
func (s *JSONSessionStore) save() error {
	data, err := json.MarshalIndent(s.sessions, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal sessions: %w", err)
	}

	// Write to temp file first for atomic replacement
	tempPath := s.filePath + ".tmp"
	if err := os.WriteFile(tempPath, data, 0644); err != nil {
		return fmt.Errorf("failed to write sessions file: %w", err)
	}

	// Atomic rename
	if err := os.Rename(tempPath, s.filePath); err != nil {
		return fmt.Errorf("failed to replace sessions file: %w", err)
	}

	return nil
}
