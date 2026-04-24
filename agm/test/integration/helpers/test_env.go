//go:build integration

package helpers

import (
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// TestEnv provides test environment configuration and utilities
type TestEnv struct {
	SessionsDir        string          // Temporary sessions directory for tests
	TempDir            string          // Temporary directory for test files
	TmuxPrefix         string          // Prefix for test sessions (agm-test-)
	TmuxSocket         string          // Isolated tmux socket path for this test
	Claude             ClaudeInterface // Claude implementation (mock or real)
	CurrentSession     string          // Current test session name
	t                  interface{}     // Testing context (can be *testing.T or *testing.B)
	registeredSessions []string        // Additional session names registered for cleanup
}

// NewTestEnv creates a new test environment
func NewTestEnv(t interface{}) *TestEnv {
	tmpDir := filepath.Join(os.TempDir(), fmt.Sprintf("agm-test-%d", time.Now().UnixNano()))
	os.MkdirAll(tmpDir, 0700)

	sessionsDir := filepath.Join(tmpDir, "sessions")
	os.MkdirAll(sessionsDir, 0700)

	// For integration tests: Use main tmux socket (/tmp/agm.sock) but with unique session names
	// Tests will clean up their sessions in Cleanup()
	// We don't use isolated sockets because agm binary needs the real tmux server

	return &TestEnv{
		SessionsDir: sessionsDir,
		TempDir:     tmpDir,
		TmuxPrefix:  "agm-test-",
		TmuxSocket:  "/tmp/agm.sock", // Use main AGM socket for integration tests
		Claude:      NewClaudeForTest(),
		t:           t,
	}
}

// RegisterSession registers a session name for cleanup when the test finishes.
// Use this for sessions whose names don't match the TmuxPrefix (agm-test-).
func (e *TestEnv) RegisterSession(sessionName string) {
	e.registeredSessions = append(e.registeredSessions, sessionName)
}

// Cleanup removes all test sessions and manifest directories.
// Kills processes inside sessions before killing the sessions themselves
// to prevent orphaned Claude processes.
func (e *TestEnv) Cleanup(t interface{}) error {
	// Kill all explicitly registered sessions (with process cleanup)
	for _, session := range e.registeredSessions {
		KillSessionProcesses(session)
	}

	// Kill all agm-test-* tmux sessions on the main AGM socket
	sessions, _ := ListTmuxSessions(e.TmuxPrefix)
	for _, session := range sessions {
		KillSessionProcesses(session)
	}

	// Also kill any test-* sessions that tests may have created
	testSessions, _ := ListTmuxSessions("test-")
	for _, session := range testSessions {
		KillSessionProcesses(session)
	}

	// Remove temp directory
	if err := os.RemoveAll(e.TempDir); err != nil {
		return fmt.Errorf("failed to cleanup temp directory: %w", err)
	}

	return nil
}

// UniqueSessionName generates unique session name with prefix and suffix
func (e *TestEnv) UniqueSessionName(suffix string) string {
	// Use nanosecond timestamp for uniqueness
	timestamp := time.Now().UnixNano()
	return fmt.Sprintf("%s%s-%d", e.TmuxPrefix, suffix, timestamp)
}

// ManifestPath returns the manifest file path for a session
func (e *TestEnv) ManifestPath(sessionName string) string {
	return filepath.Join(e.SessionsDir, sessionName, "manifest.yaml")
}

// ManifestDir returns the manifest directory for a session
func (e *TestEnv) ManifestDir(sessionName string) string {
	return filepath.Join(e.SessionsDir, sessionName)
}
