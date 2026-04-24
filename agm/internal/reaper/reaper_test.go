package reaper

import (
	"os"
	"path/filepath"
	"testing"
)

func TestNew(t *testing.T) {
	sessionName := "test-session"
	sessionsDir := "/test/sessions"
	r := New(sessionName, sessionsDir)

	if r.SessionName != sessionName {
		t.Errorf("New().SessionName = %q, expected %q", r.SessionName, sessionName)
	}

	if r.SessionsDir != sessionsDir {
		t.Errorf("New().SessionsDir = %q, expected %q", r.SessionsDir, sessionsDir)
	}

	homeDir, err := os.UserHomeDir()
	if err != nil {
		t.Fatalf("Failed to get home dir: %v", err)
	}
	expectedSocket := filepath.Join(homeDir, ".agm", "agm.sock")
	if r.SocketPath != expectedSocket {
		t.Errorf("New().SocketPath = %q, expected %q", r.SocketPath, expectedSocket)
	}
}

func TestGetSessionsDir(t *testing.T) {
	t.Run("with configured directory", func(t *testing.T) {
		customDir := "/custom/sessions"
		r := New("test-session", customDir)
		sessionsDir, err := r.getSessionsDir()

		if err != nil {
			t.Fatalf("getSessionsDir() returned error: %v", err)
		}

		if sessionsDir != customDir {
			t.Errorf("getSessionsDir() = %q, expected %q", sessionsDir, customDir)
		}
	})

	t.Run("with default directory", func(t *testing.T) {
		r := New("test-session", "")
		sessionsDir, err := r.getSessionsDir()

		if err != nil {
			t.Fatalf("getSessionsDir() returned error: %v", err)
		}

		homeDir, err := os.UserHomeDir()
		if err != nil {
			t.Fatalf("Failed to get home dir: %v", err)
		}

		expected := filepath.Join(homeDir, ".claude", "sessions")
		if sessionsDir != expected {
			t.Errorf("getSessionsDir() = %q, expected %q", sessionsDir, expected)
		}
	})
}

// Note: The Run() method and its sub-methods (waitForPrompt, sendExit,
// waitForPaneClose, archiveSession) require:
// 1. A running tmux session
// 2. A AGM session manifest
// 3. Claude Code running in the session
//
// These would be tested in integration tests rather than unit tests.
// Here we just verify the Reaper struct is properly constructed.

func TestReaperStructure(t *testing.T) {
	r := New("test-session", "/test/sessions")

	// Verify all fields are initialized
	if r.SessionName == "" {
		t.Error("Reaper.SessionName should not be empty")
	}

	if r.SessionsDir == "" {
		t.Error("Reaper.SessionsDir should not be empty")
	}

	if r.SocketPath == "" {
		t.Error("Reaper.SocketPath should not be empty")
	}

	// Verify Run method exists (compile-time check)
	var _ func() error = r.Run
}
