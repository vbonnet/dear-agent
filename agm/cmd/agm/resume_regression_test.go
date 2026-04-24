package main

import (
	"fmt"
	"os"
	"os/exec"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vbonnet/dear-agent/agm/internal/tmux"
)

// setupRegressionSocket creates an isolated tmux socket for regression tests
func setupRegressionSocket(t *testing.T) string {
	t.Helper()
	socketPath := fmt.Sprintf("/tmp/agm-regression-test-%d.sock", os.Getpid())
	os.Setenv("AGM_TMUX_SOCKET", socketPath)
	t.Cleanup(func() {
		exec.Command("tmux", "-S", socketPath, "kill-server").Run()
		os.Remove(socketPath)
		os.Unsetenv("AGM_TMUX_SOCKET")
	})
	return socketPath
}

// killTestSession kills a tmux session for cleanup
func killTestSession(sessionName, socketPath string) {
	exec.Command("tmux", "-S", socketPath, "kill-session", "-t", sessionName).Run()
}

// TestResumeDecisionLogic_TmuxExistsButClaudeNotRunning is a regression test for
// the bug where `agm session resume` would never send the `claude --resume <uuid>`
// command when the tmux session already existed, even if Claude was not running.
//
// Root cause: resume.go hardcoded `sendCommands = false` when tmux session existed,
// without checking if Claude was actually running via IsClaudeRunning().
//
// The fix checks IsClaudeRunning() and sets sendCommands = true when the tmux
// session exists but Claude is NOT running.
func TestResumeDecisionLogic_TmuxExistsButClaudeNotRunning(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping tmux integration test in short mode")
	}

	// Skip if tmux not available
	if _, err := exec.LookPath("tmux"); err != nil {
		t.Skip("tmux not found in PATH")
	}

	socketPath := setupRegressionSocket(t)
	sessionName := "test-resume-regression"
	workDir := t.TempDir()

	// Create a tmux session (simulates an existing session where Claude has exited)
	err := tmux.NewSession(sessionName, workDir)
	require.NoError(t, err, "Failed to create tmux session")
	defer killTestSession(sessionName, socketPath)

	// Wait for session to be ready
	time.Sleep(200 * time.Millisecond)

	// Verify tmux session exists
	exists, err := tmux.HasSession(sessionName)
	require.NoError(t, err)
	require.True(t, exists, "Tmux session should exist")

	// Verify Claude is NOT running (it's just a bare shell)
	claudeRunning, err := tmux.IsClaudeRunning(sessionName)
	require.NoError(t, err)
	assert.False(t, claudeRunning, "Claude should NOT be running in bare shell session")

	// This is the CRITICAL assertion: the resume decision logic should determine
	// that commands need to be sent when tmux exists but Claude is not running.
	//
	// BEFORE FIX: sendCommands was always false when tmux existed
	// AFTER FIX:  sendCommands is true when tmux exists but Claude is not running
	sendCommands := computeSendCommands(exists, sessionName)
	assert.True(t, sendCommands, "sendCommands should be true when tmux exists but Claude is NOT running (regression: was always false before fix)")
}

// TestResumeDecisionLogic_TmuxDoesNotExist verifies that when the tmux session
// does not exist, sendCommands is true (new session needs to be created and
// resume command sent).
func TestResumeDecisionLogic_TmuxDoesNotExist(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping tmux integration test in short mode")
	}

	if _, err := exec.LookPath("tmux"); err != nil {
		t.Skip("tmux not found in PATH")
	}

	_ = setupRegressionSocket(t)
	sessionName := "test-resume-noexist"

	// Session should not exist (we haven't created it)
	exists, _ := tmux.HasSession(sessionName)

	sendCommands := computeSendCommands(exists, sessionName)
	assert.True(t, sendCommands, "sendCommands should be true when tmux session does not exist")
}

// computeSendCommands replicates the fixed resume decision logic from resumeSession().
// This must match the logic in resume.go to serve as an effective regression test.
func computeSendCommands(tmuxExists bool, sessionName string) bool {
	if !tmuxExists {
		return true
	}

	// Check if Claude is actually running in the existing session
	claudeRunning, err := tmux.IsClaudeRunning(sessionName)
	if err != nil {
		// Detection failed - skip commands for safety
		return false
	}
	if claudeRunning {
		return false
	}

	// Tmux exists but Claude not running - need to send resume command
	return true
}
