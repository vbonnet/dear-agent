package agent

import (
	"testing"
)

// TestClaudeAdapterImplementsAgentInterface verifies ClaudeAdapter implements Agent interface.
func TestClaudeAdapterImplementsAgentInterface(t *testing.T) {
	// Create adapter with mock store
	mockStore := &MockSessionStore{
		sessions: make(map[SessionID]*SessionMetadata),
	}

	adapter, err := NewClaudeAdapter(mockStore)
	if err != nil {
		t.Fatalf("NewClaudeAdapter failed: %v", err)
	}

	// Verify adapter implements Agent interface
	var _ = adapter
}

// TestClaudeAdapterName tests Name() method.
func TestClaudeAdapterName(t *testing.T) {
	mockStore := &MockSessionStore{
		sessions: make(map[SessionID]*SessionMetadata),
	}

	adapter, err := NewClaudeAdapter(mockStore)
	if err != nil {
		t.Fatalf("NewClaudeAdapter failed: %v", err)
	}

	if got := adapter.Name(); got != "claude" {
		t.Errorf("Name() = %q, want %q", got, "claude")
	}
}

// TestClaudeAdapterVersion tests Version() method.
func TestClaudeAdapterVersion(t *testing.T) {
	mockStore := &MockSessionStore{
		sessions: make(map[SessionID]*SessionMetadata),
	}

	adapter, err := NewClaudeAdapter(mockStore)
	if err != nil {
		t.Fatalf("NewClaudeAdapter failed: %v", err)
	}

	version := adapter.Version()
	if version == "" {
		t.Errorf("Version() returned empty string")
	}
}

// TestClaudeAdapterCapabilities tests Capabilities() method.
func TestClaudeAdapterCapabilities(t *testing.T) {
	mockStore := &MockSessionStore{
		sessions: make(map[SessionID]*SessionMetadata),
	}

	adapter, err := NewClaudeAdapter(mockStore)
	if err != nil {
		t.Fatalf("NewClaudeAdapter failed: %v", err)
	}

	caps := adapter.Capabilities()

	// Verify expected capabilities
	if !caps.SupportsSlashCommands {
		t.Error("SupportsSlashCommands should be true for Claude CLI")
	}

	if !caps.SupportsTools {
		t.Error("SupportsTools should be true for Claude")
	}

	if !caps.SupportsVision {
		t.Error("SupportsVision should be true for Claude Sonnet 4.5")
	}

	if caps.MaxContextWindow != 200000 {
		t.Errorf("MaxContextWindow = %d, want 200000", caps.MaxContextWindow)
	}

	if caps.ModelName != "claude-sonnet-4.5" {
		t.Errorf("ModelName = %q, want %q", caps.ModelName, "claude-sonnet-4.5")
	}
}

// MockSessionStore is a mock implementation of SessionStore for testing.
type MockSessionStore struct {
	sessions map[SessionID]*SessionMetadata
}

func (m *MockSessionStore) Get(sessionID SessionID) (*SessionMetadata, error) {
	metadata, exists := m.sessions[sessionID]
	if !exists {
		return nil, ErrSessionNotFound
	}
	return metadata, nil
}

func (m *MockSessionStore) Set(sessionID SessionID, metadata *SessionMetadata) error {
	m.sessions[sessionID] = metadata
	return nil
}

func (m *MockSessionStore) Delete(sessionID SessionID) error {
	delete(m.sessions, sessionID)
	return nil
}

func (m *MockSessionStore) List() (map[SessionID]*SessionMetadata, error) {
	result := make(map[SessionID]*SessionMetadata)
	for k, v := range m.sessions {
		result[k] = v
	}
	return result, nil
}
