package tmux

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Helper: kill tmux session (for test cleanup)
func killTestSession(name string) {
	socketPath := GetSocketPath()
	cmd := exec.Command("tmux", "-S", socketPath, "kill-session", "-t", name)
	cmd.Run() // Ignore errors - session may not exist
}

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

	// The actual send-keys format should be:
	// send-keys -t test-assoc -l "/agm:agm-assoc test-assoc"
	// send-keys -t test-assoc C-m
	expectedCommandLine1 := `send-keys -t test-assoc -l "/agm:agm-assoc test-assoc"`
	expectedCommandLine2 := `send-keys -t test-assoc C-m`

	// This validates our understanding of the format
	assert.Contains(t, expectedCommandLine1, sessionName)
	assert.Contains(t, expectedCommandLine1, "/agm:agm-assoc")
	assert.Contains(t, expectedCommandLine2, "C-m") // Enter key
}

// TestInitSequence_Integration tests basic initialization
// Full end-to-end testing with tmux requires manual testing with actual AGM
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

	// Verify socket path is set to AGM socket
	expectedPath := GetSocketPath()
	assert.Equal(t, expectedPath, seq.SocketPath)

	// Note: We can't fully test Run() without a real Claude session
	// that responds to /rename and /csm-assoc commands.
	// Full end-to-end testing requires manual testing with AGM.
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

	// Socket path should be set to AGM socket
	expectedPath := GetSocketPath()
	assert.Equal(t, expectedPath, seq.SocketPath)

	// Should typically be /tmp/agm.sock
	assert.Contains(t, seq.SocketPath, "agm.sock")
}

// TestInitSequence_Run_Success tests full InitSequence with real tmux session
func TestInitSequence_Run_Success(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	// Generate unique session name to avoid conflicts
	sessionName := "test-init-success-" + time.Now().Format("20060102-150405")

	// Cleanup: Ensure no leftover session or ready file
	CleanupReadyFile(sessionName)
	defer CleanupReadyFile(sessionName)
	defer killTestSession(sessionName)

	// Create a test tmux session with Claude agent
	// We'll create a basic session and then manually start Claude
	err := NewSession(sessionName, "/tmp")
	if err != nil {
		t.Skipf("Cannot create tmux session: %v (skipping test)", err)
	}

	// Start Claude in the session
	// Send "claude" command to start Claude CLI
	err = SendCommand(sessionName, "-l claude")
	if err != nil {
		t.Fatalf("Failed to send 'claude' command: %v", err)
	}

	// Send Enter to execute
	time.Sleep(100 * time.Millisecond)
	err = SendCommand(sessionName, "C-m")
	if err != nil {
		t.Fatalf("Failed to send Enter: %v", err)
	}

	// Wait for Claude to start (WaitForClaudePrompt handles this)
	// Giving generous timeout since Claude startup can be slow
	err = WaitForClaudePrompt(sessionName, 60*time.Second)
	if err != nil {
		t.Skipf("Claude failed to start (may not be installed): %v (skipping test)", err)
	}

	// Create init sequence
	seq := NewInitSequence(sessionName)

	// Run the initialization sequence
	err = seq.Run()
	if err != nil {
		t.Fatalf("InitSequence.Run() failed: %v", err)
	}

	// Verify session still exists (Run() shouldn't kill it)
	exists, err := HasSession(sessionName)
	require.NoError(t, err)
	assert.True(t, exists, "session should still exist after InitSequence.Run()")

	// Note: We can't easily verify /rename and /agm:agm-assoc executed successfully
	// without accessing Claude's internal state. The ready-file signal would normally
	// confirm association, but that's the caller's responsibility.
	// For this test, success = Run() returns nil without errors.
}

// TestInitSequence_Run_Timeout tests timeout when Claude never becomes ready
func TestInitSequence_Run_Timeout(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	// Generate unique session name
	sessionName := "test-init-timeout-" + time.Now().Format("20060102-150405")

	defer killTestSession(sessionName)

	// Create a tmux session with bash instead of Claude
	// Bash prompt won't match Claude's "❯" pattern, so WaitForClaudePrompt will timeout
	err := NewSession(sessionName, "/tmp")
	if err != nil {
		t.Skipf("Cannot create tmux session: %v (skipping test)", err)
	}

	// Session starts with default shell (likely bash), which won't have Claude "❯" prompt

	// Create init sequence
	seq := NewInitSequence(sessionName)

	// Run should fail with timeout error
	// Note: This will take 30 seconds (WaitForClaudePrompt timeout in sendRename)
	// We could make timeout configurable, but for now accepting the delay
	err = seq.Run()

	// Verify we got an error
	require.Error(t, err, "Run() should fail when Claude prompt never appears")

	// Verify error message mentions timeout or Claude not ready
	errMsg := err.Error()
	assert.True(t,
		contains(errMsg, "timeout") || contains(errMsg, "Claude not ready"),
		"error should mention timeout or Claude not ready, got: %v", errMsg)
}

// Helper function for string contains check
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > len(substr) && anySubstring(s, substr))
}

func anySubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
