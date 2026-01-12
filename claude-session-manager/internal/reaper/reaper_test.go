package reaper

import (
	"os"
	"path/filepath"
	"testing"
)

func TestNew(t *testing.T) {
	sessionName := "test-session"
	r := New(sessionName)

	if r.SessionName != sessionName {
		t.Errorf("New().SessionName = %q, expected %q", r.SessionName, sessionName)
	}

	expectedSocket := "/tmp/csm.sock"
	if r.SocketPath != expectedSocket {
		t.Errorf("New().SocketPath = %q, expected %q", r.SocketPath, expectedSocket)
	}
}

func TestGetSessionsDir(t *testing.T) {
	r := New("test-session")
	sessionsDir, err := r.getSessionsDir()

	if err != nil {
		t.Fatalf("getSessionsDir() returned error: %v", err)
	}

	homeDir, err := os.UserHomeDir()
	if err != nil {
		t.Fatalf("Failed to get home dir: %v", err)
	}

	expected := filepath.Join(homeDir, "sessions")
	if sessionsDir != expected {
		t.Errorf("getSessionsDir() = %q, expected %q", sessionsDir, expected)
	}
}

// Note: The Run() method and its sub-methods (waitForPrompt, sendExit,
// waitForPaneClose, archiveSession) require:
// 1. A running tmux session
// 2. A CSM session manifest
// 3. Claude Code running in the session
//
// These would be tested in integration tests rather than unit tests.
// Here we just verify the Reaper struct is properly constructed.

func TestReaperStructure(t *testing.T) {
	r := New("test-session")

	// Verify all fields are initialized
	if r.SessionName == "" {
		t.Error("Reaper.SessionName should not be empty")
	}

	if r.SocketPath == "" {
		t.Error("Reaper.SocketPath should not be empty")
	}

	// Verify Run method exists (compile-time check)
	var _ func() error = r.Run
}
