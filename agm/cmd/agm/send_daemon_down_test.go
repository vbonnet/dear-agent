package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vbonnet/dear-agent/agm/internal/daemon"
)

// TestQueueMessage_DaemonDown verifies that daemon.IsRunning returns false
// when the PID file points to a non-existent process.
// The queueMessage function now falls back to direct tmux delivery instead
// of returning an error when daemon is not running.
//
// Bug History:
//   - 2026-03-31: queueMessage silently queued messages when daemon was down.
//     Fixed to return error instead.
//   - 2026-04-10: queueMessage error on daemon-down was too strict — messages
//     that could be sent directly via tmux were being dropped entirely.
//     Fixed to fall back to direct tmux delivery via sendDirectly.
func TestQueueMessage_DaemonDown(t *testing.T) {
	// Create a temp directory with a PID file pointing to a non-existent process
	tmpDir := t.TempDir()
	pidFile := filepath.Join(tmpDir, "daemon.pid")

	// Write a PID that definitely doesn't exist (PID -1)
	err := os.WriteFile(pidFile, []byte("-1"), 0644)
	require.NoError(t, err)

	// Verify daemon.IsRunning returns false for this PID file
	running := daemon.IsRunning(pidFile)
	assert.False(t, running, "daemon.IsRunning should return false for PID -1")
}

// TestQueueMessage_DaemonDown_NoPidFile verifies that a missing PID file
// is treated as daemon not running.
func TestQueueMessage_DaemonDown_NoPidFile(t *testing.T) {
	tmpDir := t.TempDir()
	pidFile := filepath.Join(tmpDir, "nonexistent.pid")

	running := daemon.IsRunning(pidFile)
	assert.False(t, running, "daemon.IsRunning should return false when PID file doesn't exist")
}
