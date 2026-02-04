package helpers

import (
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// TestEnv provides test environment configuration and utilities
type TestEnv struct {
	SessionsDir    string          // Temporary sessions directory for tests
	TempDir        string          // Temporary directory for test files
	TmuxPrefix     string          // Prefix for test sessions (csm-test-)
	Claude         ClaudeInterface // Claude implementation (mock or real)
	CurrentSession string          // Current test session name
	t              interface{}     // Testing context (can be *testing.T or *testing.B)
}

// NewTestEnv creates a new test environment
func NewTestEnv(t interface{}) *TestEnv {
	tmpDir := filepath.Join(os.TempDir(), fmt.Sprintf("csm-test-%d", time.Now().UnixNano()))
	os.MkdirAll(tmpDir, 0700)

	sessionsDir := filepath.Join(tmpDir, "sessions")
	os.MkdirAll(sessionsDir, 0700)

	return &TestEnv{
		SessionsDir: sessionsDir,
		TempDir:     tmpDir,
		TmuxPrefix:  "csm-test-",
		Claude:      NewClaudeForTest(),
		t:           t,
	}
}

// Cleanup removes all test sessions and manifest directories
func (e *TestEnv) Cleanup(t interface{}) error {
	// Kill all csm-test-* tmux sessions
	sessions, _ := ListTmuxSessions(e.TmuxPrefix)
	for _, session := range sessions {
		KillTmuxSession(session)
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
