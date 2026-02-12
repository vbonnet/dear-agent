//go:build integration

package helpers

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// SetupTestTmuxSession creates detached tmux session for testing
func SetupTestTmuxSession(t *testing.T, sessionName string) {
	// Create detached session
	cmd := exec.Command("tmux", "new-session", "-d", "-s", sessionName)
	err := cmd.Run()
	require.NoError(t, err, "Failed to create test tmux session")

	// Wait for session to be ready
	WaitForTmuxSession(t, sessionName, 5*time.Second)
}

// CleanupTestTmuxSession removes test tmux session
func CleanupTestTmuxSession(t *testing.T, sessionName string) {
	// Kill session (ignore error if already gone)
	exec.Command("tmux", "kill-session", "-t", sessionName).Run()

	// Remove ready-files
	readyGlob := filepath.Join(os.Getenv("HOME"), ".agm", fmt.Sprintf("claude-ready-%s*", sessionName))
	matches, _ := filepath.Glob(readyGlob)
	for _, file := range matches {
		os.Remove(file)
	}

	// Remove pending-files
	pendingGlob := filepath.Join(os.Getenv("HOME"), ".agm", fmt.Sprintf("pending-%s*", sessionName))
	matches, _ = filepath.Glob(pendingGlob)
	for _, file := range matches {
		os.Remove(file)
	}
}

// WaitForTmuxSession polls until session exists or timeout
func WaitForTmuxSession(t *testing.T, sessionName string, timeout time.Duration) {
	deadline := time.Now().Add(timeout)

	for time.Now().Before(deadline) {
		cmd := exec.Command("tmux", "has-session", "-t", sessionName)
		if cmd.Run() == nil {
			return // Session exists
		}
		time.Sleep(100 * time.Millisecond)
	}

	require.Fail(t, "Timeout waiting for tmux session", "Session: %s", sessionName)
}

// SetupManifestWithUUID creates test manifest with UUID
func SetupManifestWithUUID(sessionName string, uuid string) string {
	manifestDir := filepath.Join(os.Getenv("HOME"), ".agm", "sessions", sessionName)
	os.MkdirAll(manifestDir, 0755)

	manifestPath := filepath.Join(manifestDir, "manifest.yaml")
	content := fmt.Sprintf(`version: "2"
session_id: %s
agent_type: claude
created_at: "2026-02-08T00:00:00Z"
lifecycle: active
tmux:
  name: %s
worktree:
  path: /tmp/test-project
`, uuid, sessionName)

	os.WriteFile(manifestPath, []byte(content), 0644)
	return manifestPath
}

// EnsureNoTmuxSession verifies tmux session does NOT exist
func EnsureNoTmuxSession(sessionName string) error {
	cmd := exec.Command("tmux", "has-session", "-t", sessionName)
	if cmd.Run() == nil {
		// Session exists, kill it
		return exec.Command("tmux", "kill-session", "-t", sessionName).Run()
	}
	return nil
}
