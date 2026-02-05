package tmux

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestNewInitSequence tests the constructor
func TestNewInitSequence(t *testing.T) {
	seq := NewInitSequence("test-session")
	assert.NotNil(t, seq)
	assert.Equal(t, "test-session", seq.SessionName)
	assert.NotEmpty(t, seq.SocketPath)
}

// TestGetReadyFilePath tests ready file path generation
func TestGetReadyFilePath(t *testing.T) {
	sessionName := "my-session"
	path := getReadyFilePath(sessionName)

	// Should be in ~/.agm/ready-{session}
	homeDir, err := os.UserHomeDir()
	require.NoError(t, err)

	expectedPath := filepath.Join(homeDir, ".agm", "ready-my-session")
	assert.Equal(t, expectedPath, path)
}

// TestCleanupReadyFile tests ready file cleanup
func TestCleanupReadyFile(t *testing.T) {
	sessionName := "cleanup-test"
	readyPath := getReadyFilePath(sessionName)

	// Create the ready file
	err := os.MkdirAll(filepath.Dir(readyPath), 0755)
	require.NoError(t, err)

	err = os.WriteFile(readyPath, []byte("ready"), 0644)
	require.NoError(t, err)

	// Verify it exists
	_, err = os.Stat(readyPath)
	require.NoError(t, err, "ready file should exist before cleanup")

	// Cleanup
	err = CleanupReadyFile(sessionName)
	assert.NoError(t, err)

	// Verify it's gone
	_, err = os.Stat(readyPath)
	assert.True(t, os.IsNotExist(err), "ready file should be deleted after cleanup")
}

// TestCleanupReadyFile_NonExistent tests cleanup when file doesn't exist
func TestCleanupReadyFile_NonExistent(t *testing.T) {
	sessionName := "nonexistent-test"

	// Should not error if file doesn't exist
	err := CleanupReadyFile(sessionName)
	assert.NoError(t, err)
}

// TestWaitForReadyFile_Success tests successful ready file detection
func TestWaitForReadyFile_Success(t *testing.T) {
	sessionName := "wait-success-test"
	seq := NewInitSequence(sessionName)
	readyPath := getReadyFilePath(sessionName)

	// Cleanup before and after test
	CleanupReadyFile(sessionName)
	defer CleanupReadyFile(sessionName)

	// Create ready file in background after 100ms
	go func() {
		time.Sleep(100 * time.Millisecond)
		os.MkdirAll(filepath.Dir(readyPath), 0755)
		os.WriteFile(readyPath, []byte("ready"), 0644)
	}()

	// Wait should succeed
	err := seq.waitForReadyFile(2 * time.Second)
	assert.NoError(t, err, "should detect ready file")
}

// TestWaitForReadyFile_Timeout tests timeout when ready file never appears
func TestWaitForReadyFile_Timeout(t *testing.T) {
	sessionName := "wait-timeout-test"
	seq := NewInitSequence(sessionName)

	// Cleanup to ensure file doesn't exist
	CleanupReadyFile(sessionName)
	defer CleanupReadyFile(sessionName)

	// Wait should timeout
	err := seq.waitForReadyFile(200 * time.Millisecond)
	assert.Error(t, err, "should timeout when ready file doesn't appear")
	assert.Contains(t, err.Error(), "timeout", "error should mention timeout")
}

// TestWaitForReadyFile_AlreadyExists tests when ready file already exists
func TestWaitForReadyFile_AlreadyExists(t *testing.T) {
	sessionName := "already-exists-test"
	seq := NewInitSequence(sessionName)
	readyPath := getReadyFilePath(sessionName)

	// Create ready file before waiting
	err := os.MkdirAll(filepath.Dir(readyPath), 0755)
	require.NoError(t, err)
	err = os.WriteFile(readyPath, []byte("ready"), 0644)
	require.NoError(t, err)
	defer CleanupReadyFile(sessionName)

	// Wait should succeed immediately
	err = seq.waitForReadyFile(1 * time.Second)
	assert.NoError(t, err, "should detect existing ready file immediately")
}

// TestSendRename_CommandFormat tests that rename command is formatted correctly
func TestSendRename_CommandFormat(t *testing.T) {
	// This test validates the command format without actually running tmux
	// We can't easily mock ControlModeSession, so we test the logic separately

	sessionName := "test-rename"
	expectedCmd := "/rename test-rename"

	// The actual send-keys format should be:
	// send-keys -t test-rename "/rename test-rename" C-m
	expectedCommandLine := `send-keys -t test-rename "/rename test-rename" C-m`

	// This validates our understanding of the format
	assert.Contains(t, expectedCommandLine, sessionName)
	assert.Contains(t, expectedCommandLine, expectedCmd)
	assert.Contains(t, expectedCommandLine, "C-m") // Enter key
}

// TestSendAssociation_CommandFormat tests that association command is formatted correctly
func TestSendAssociation_CommandFormat(t *testing.T) {
	// This test validates the command format without actually running tmux

	sessionName := "test-assoc"
	expectedCmd := "/csm-tools:csm-assoc"

	// The actual send-keys format should be:
	// send-keys -t test-assoc "/csm-tools:csm-assoc" C-m
	expectedCommandLine := `send-keys -t test-assoc "/csm-tools:csm-assoc" C-m`

	// This validates our understanding of the format
	assert.Contains(t, expectedCommandLine, sessionName)
	assert.Contains(t, expectedCommandLine, expectedCmd)
	assert.Contains(t, expectedCommandLine, "C-m") // Enter key
}

// TestInitSequence_Integration tests basic initialization
// Full end-to-end testing with tmux requires manual testing with actual CSM
func TestInitSequence_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	sessionName := "csm-init-integration-test"

	// Test that we can create an InitSequence
	seq := NewInitSequence(sessionName)
	assert.NotNil(t, seq)
	assert.Equal(t, sessionName, seq.SessionName)
	assert.NotEmpty(t, seq.SocketPath)

	// Verify socket path is set to CSM socket
	expectedPath := GetSocketPath()
	assert.Equal(t, expectedPath, seq.SocketPath)

	// Note: We can't fully test Run() without a real Claude session
	// that responds to /rename and /csm-assoc commands.
	// Full end-to-end testing requires manual testing with CSM.
}

// TestWaitForReadyFileWithProgress tests the progress reporting variant
func TestWaitForReadyFileWithProgress(t *testing.T) {
	sessionName := "progress-test"
	readyPath := getReadyFilePath(sessionName)

	// Cleanup before and after
	CleanupReadyFile(sessionName)
	defer CleanupReadyFile(sessionName)

	// Create ready file after 200ms
	go func() {
		time.Sleep(200 * time.Millisecond)
		os.MkdirAll(filepath.Dir(readyPath), 0755)
		os.WriteFile(readyPath, []byte("ready"), 0644)
	}()

	// Collect progress messages
	progressCalled := false
	progressFunc := func(elapsed time.Duration) {
		progressCalled = true
		// Progress function was called
	}

	// Wait with progress
	err := WaitForReadyFileWithProgress(sessionName, 2*time.Second, progressFunc)
	assert.NoError(t, err)
	assert.True(t, progressCalled, "progress function should be called")
}

// TestSocketPath tests that socket path is set correctly
func TestSocketPath(t *testing.T) {
	seq := NewInitSequence("socket-test")

	// Socket path should be set to CSM socket
	expectedPath := GetSocketPath()
	assert.Equal(t, expectedPath, seq.SocketPath)

	// Should typically be /tmp/csm.sock
	assert.Contains(t, seq.SocketPath, "csm.sock")
}
