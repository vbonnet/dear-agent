package agent

import (
	"testing"
)

// TestAgyAdapterImplementsAgentInterface verifies AgyAdapter implements Agent interface.
func TestAgyAdapterImplementsAgentInterface(t *testing.T) {
	// Create adapter with mock store
	mockStore := &MockSessionStore{
		sessions: make(map[SessionID]*SessionMetadata),
	}

	adapter, err := NewAgyAdapter(mockStore)
	if err != nil {
		t.Fatalf("NewAgyAdapter failed: %v", err)
	}

	// Verify adapter implements Agent interface (type already Agent from NewAgyAdapter)
	_ = adapter
}

// TestAgyAdapterName tests Name() method.
func TestAgyAdapterName(t *testing.T) {
	mockStore := &MockSessionStore{
		sessions: make(map[SessionID]*SessionMetadata),
	}

	adapter, err := NewAgyAdapter(mockStore)
	if err != nil {
		t.Fatalf("NewAgyAdapter failed: %v", err)
	}

	if got := adapter.Name(); got != "agy" {
		t.Errorf("Name() = %q, want %q", got, "agy")
	}
}

// TestAgyAdapterVersion tests Version() method.
func TestAgyAdapterVersion(t *testing.T) {
	mockStore := &MockSessionStore{
		sessions: make(map[SessionID]*SessionMetadata),
	}

	adapter, err := NewAgyAdapter(mockStore)
	if err != nil {
		t.Fatalf("NewAgyAdapter failed: %v", err)
	}

	version := adapter.Version()
	if version == "" {
		t.Errorf("Version() returned empty string")
	}
}

// TestAgyAdapterCapabilities tests Capabilities() method.
func TestAgyAdapterCapabilities(t *testing.T) {
	mockStore := &MockSessionStore{
		sessions: make(map[SessionID]*SessionMetadata),
	}

	adapter, err := NewAgyAdapter(mockStore)
	if err != nil {
		t.Fatalf("NewAgyAdapter failed: %v", err)
	}

	caps := adapter.Capabilities()

	// Verify expected capabilities
	if !caps.SupportsSlashCommands {
		t.Error("SupportsSlashCommands should be true for Agy")
	}

	if !caps.SupportsTools {
		t.Error("SupportsTools should be true for Agy")
	}

	if caps.MaxContextWindow != 200000 {
		t.Errorf("MaxContextWindow = %d, want 200000", caps.MaxContextWindow)
	}

	if caps.ModelName != "antigravity-1.0" {
		t.Errorf("ModelName = %q, want %q", caps.ModelName, "antigravity-1.0")
	}
}
