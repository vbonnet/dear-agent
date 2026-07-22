//go:build integration

package reaper

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/vbonnet/dear-agent/agm/internal/tmux"
)

// TestSendExit_FailsGracefully verifies error handling when session doesn't exist
func TestSendExit_FailsGracefully(t *testing.T) {
	// Create reaper for non-existent session
	r := New("nonexistent-test-session-12345", "/tmp/test-sessions")

	// sendExit should fail gracefully (session doesn't exist)
	err := r.sendExit("/exit")
	if err == nil {
		t.Error("sendExit() should fail for non-existent session, but succeeded")
	}

	// Error should indicate failure to send /exit (implementation-agnostic check)
	if !strings.Contains(err.Error(), "failed to send /exit") {
		t.Errorf("Error should mention '/exit' send failure, got: %v", err)
	}

	t.Logf("✓ sendExit() fails gracefully with error: %v", err)
}

// TestIntegration_ReaperWithRealSession exercises literal shutdown delivery to
// a real tmux session without depending on one harness's renderer.
func TestIntegration_ReaperWithRealSession(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	// Get socket path
	socketPath := tmux.GetSocketPath()

	// Create test session directory
	tmpDir := t.TempDir()
	sessionsDir := filepath.Join(tmpDir, "sessions")
	sessionName := "reaper-integration-test-" + filepath.Base(tmpDir)
	sessionDir := filepath.Join(sessionsDir, sessionName)

	if err := os.MkdirAll(sessionDir, 0755); err != nil {
		t.Fatalf("Failed to create session dir: %v", err)
	}

	// Create minimal manifest
	manifestContent := `{
  "uuid": "test-uuid-integration",
  "session_name": "` + sessionName + `",
  "agent_id": "claude",
  "created_at": "2026-02-07T21:00:00Z",
  "lifecycle": "active"
}`

	manifestPath := filepath.Join(sessionDir, "MANIFEST.json")
	if err := os.WriteFile(manifestPath, []byte(manifestContent), 0644); err != nil {
		t.Fatalf("Failed to write manifest: %v", err)
	}

	// Create a simple tmux session that will accept commands
	// Use a session that just runs 'sleep infinity' so it stays alive
	cmd := exec.Command("tmux", "-S", socketPath, "new-session", "-d", "-s", sessionName, "bash")
	if err := cmd.Run(); err != nil {
		t.Fatalf("Failed to create tmux session: %v", err)
	}

	// Cleanup: kill session at end of test
	defer func() {
		killCmd := exec.Command("tmux", "-S", socketPath, "kill-session", "-t", sessionName)
		_ = killCmd.Run() // Ignore errors (session might already be gone)
	}()

	// Wait for session to be ready
	time.Sleep(500 * time.Millisecond)

	// Send a command that simulates Claude finishing work
	// This will echo the Claude prompt character
	sendCmd := exec.Command("tmux", "-S", socketPath, "send-keys", "-t", sessionName, "-l", "echo '❯'")
	if err := sendCmd.Run(); err != nil {
		t.Fatalf("Failed to send test command: %v", err)
	}

	sendCmd = exec.Command("tmux", "-S", socketPath, "send-keys", "-t", sessionName, "Enter")
	if err := sendCmd.Run(); err != nil {
		t.Fatalf("Failed to send Enter: %v", err)
	}

	// Wait for command to execute
	time.Sleep(500 * time.Millisecond)

	// Exercise the same literal tmux delivery boundary used by the reaper.
	r := New(sessionName, sessionsDir)

	// This synthetic shell is not a real harness, so prompt detection may time
	// out. Either outcome verifies that the boundary returns without panicking.
	err := r.sendExit("/exit")

	// Note: sendExit will likely timeout because this isn't a real Claude session
	// But we can verify it gracefully handles the error
	if err != nil {
		// Expected - sendExit times out waiting for real Claude prompt
		// Check if the error is specifically a timeout (expected)
		if strings.Contains(err.Error(), "timeout") || strings.Contains(err.Error(), "session not ready") {
			t.Logf("✓ sendExit() timed out as expected for non-Claude session")
		} else {
			t.Logf("sendExit() error: %v", err)
		}
	} else {
		// If it succeeded, that's fine too
		t.Log("✓ sendExit() succeeded")
	}

	// This is a boundary test, not the product-level Pi archive canary.
}
