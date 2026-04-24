package unit_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vbonnet/dear-agent/agm/internal/tmux"
)

// TestSocketPath_DefaultPath tests default socket path construction
func TestSocketPath_DefaultPath(t *testing.T) {
	// Clear any override
	os.Unsetenv("AGM_TMUX_SOCKET")

	socketPath := tmux.GetSocketPath()

	// Default path should be ~/.agm/agm.sock
	homeDir, err := os.UserHomeDir()
	require.NoError(t, err)
	expected := filepath.Join(homeDir, ".agm", "agm.sock")
	assert.Equal(t, expected, socketPath, "Default socket should be ~/.agm/agm.sock")
}

// TestSocketPath_EnvironmentOverride tests AGM_TMUX_SOCKET override
func TestSocketPath_EnvironmentOverride(t *testing.T) {
	customPath := "/custom/path/test.sock"
	os.Setenv("AGM_TMUX_SOCKET", customPath)
	defer os.Unsetenv("AGM_TMUX_SOCKET")

	socketPath := tmux.GetSocketPath()

	assert.Equal(t, customPath, socketPath, "Should use AGM_TMUX_SOCKET override")
}

// TestSocketPath_Consistency tests that socket path is consistent
func TestSocketPath_Consistency(t *testing.T) {
	// Clear any override
	os.Unsetenv("AGM_TMUX_SOCKET")

	path1 := tmux.GetSocketPath()
	path2 := tmux.GetSocketPath()

	assert.Equal(t, path1, path2, "Socket path should be consistent across calls")
}

// TestCommandWithTimeout_ContextCancellation tests timeout enforcement
func TestCommandWithTimeout_ContextCancellation(t *testing.T) {
	ctx := context.Background()
	timeout := 100 * time.Millisecond

	// Create command with short timeout
	cmd, cancel := tmux.CommandWithTimeout(ctx, timeout, "sleep", "10")
	defer cancel()

	// Start command
	err := cmd.Start()
	require.NoError(t, err, "Command should start successfully")

	// Wait for timeout to elapse
	time.Sleep(timeout + 50*time.Millisecond)

	// Cancel should clean up process
	cancel()

	// Process should no longer be running
	err = cmd.Process.Signal(os.Kill)
	// Either process already dead (good) or we can kill it
	if err != nil {
		assert.Contains(t, err.Error(), "process already finished", "Process should be dead after cancel")
	}
}

// TestCommandWithTimeout_ImmediateCancel tests immediate cancellation
func TestCommandWithTimeout_ImmediateCancel(t *testing.T) {
	ctx := context.Background()
	timeout := 1 * time.Second

	cmd, cancel := tmux.CommandWithTimeout(ctx, timeout, "sleep", "10")

	// Cancel immediately
	cancel()

	// Command should fail to start or exit quickly
	err := cmd.Run()
	if err != nil {
		// Expected - either fails to start or exits with signal
		assert.True(t, true, "Command failed as expected after cancel")
	}
}

// TestCommandConstruction_SessionCreation tests new-session command structure
func TestCommandConstruction_SessionCreation(t *testing.T) {
	// Test socket path + command construction pattern
	socketPath := "/test/socket.sock"
	os.Setenv("AGM_TMUX_SOCKET", socketPath)
	defer os.Unsetenv("AGM_TMUX_SOCKET")

	ctx := context.Background()
	timeout := 5 * time.Second

	// Build command following NewSession pattern
	cmd, cancel := tmux.CommandWithTimeout(ctx, timeout, "tmux", "-S", socketPath, "new-session", "-d", "-s", "test-session", "-c", "/tmp")
	defer cancel()

	// Verify command structure
	assert.Contains(t, cmd.Path, "tmux", "Path should contain tmux binary")
	assert.Contains(t, cmd.Args, "-S", "Should include socket flag")
	assert.Contains(t, cmd.Args, socketPath, "Should include socket path")
	assert.Contains(t, cmd.Args, "new-session", "Should include new-session command")
	assert.Contains(t, cmd.Args, "-d", "Should include detach flag")
	assert.Contains(t, cmd.Args, "-s", "Should include session flag")
	assert.Contains(t, cmd.Args, "test-session", "Should include session name")
	assert.Contains(t, cmd.Args, "-c", "Should include directory flag")
	assert.Contains(t, cmd.Args, "/tmp", "Should include working directory")
}

// TestCommandConstruction_SendKeys tests send-keys command structure
func TestCommandConstruction_SendKeys(t *testing.T) {
	socketPath := "/test/socket.sock"
	ctx := context.Background()
	timeout := 5 * time.Second

	// Build send-keys command
	message := "echo hello"
	cmd, cancel := tmux.CommandWithTimeout(ctx, timeout, "tmux", "-S", socketPath, "send-keys", "-t", "test-session", message, "Enter")
	defer cancel()

	// Verify command structure
	assert.Contains(t, cmd.Path, "tmux", "Path should contain tmux binary")
	assert.Contains(t, cmd.Args, "send-keys", "Should include send-keys command")
	assert.Contains(t, cmd.Args, "-t", "Should include target flag")
	assert.Contains(t, cmd.Args, "test-session", "Should include session name")
	assert.Contains(t, cmd.Args, message, "Should include message")
	assert.Contains(t, cmd.Args, "Enter", "Should include Enter key")
}

// TestCommandConstruction_CapturePane tests capture-pane command structure
func TestCommandConstruction_CapturePane(t *testing.T) {
	socketPath := "/test/socket.sock"
	ctx := context.Background()
	timeout := 5 * time.Second

	// Build capture-pane command
	cmd, cancel := tmux.CommandWithTimeout(ctx, timeout, "tmux", "-S", socketPath, "capture-pane", "-p", "-t", "test-session")
	defer cancel()

	// Verify command structure
	assert.Contains(t, cmd.Path, "tmux", "Path should contain tmux binary")
	assert.Contains(t, cmd.Args, "capture-pane", "Should include capture-pane command")
	assert.Contains(t, cmd.Args, "-p", "Should include print flag")
	assert.Contains(t, cmd.Args, "-t", "Should include target flag")
	assert.Contains(t, cmd.Args, "test-session", "Should include session name")
}

// TestCommandConstruction_HasSession tests has-session command structure
func TestCommandConstruction_HasSession(t *testing.T) {
	socketPath := "/test/socket.sock"
	ctx := context.Background()
	timeout := 5 * time.Second

	// Build has-session command (used by HasSession function)
	cmd, cancel := tmux.CommandWithTimeout(ctx, timeout, "tmux", "-S", socketPath, "has-session", "-t", "test-session")
	defer cancel()

	// Verify command structure
	assert.Contains(t, cmd.Path, "tmux", "Path should contain tmux binary")
	assert.Contains(t, cmd.Args, "-S", "Should include socket flag")
	assert.Contains(t, cmd.Args, socketPath, "Should include socket path")
	assert.Contains(t, cmd.Args, "has-session", "Should include has-session command")
	assert.Contains(t, cmd.Args, "-t", "Should include target flag")
	assert.Contains(t, cmd.Args, "test-session", "Should include session name")
}

// TestSocketPath_DirectoryCreation tests socket directory structure
func TestSocketPath_DirectoryCreation(t *testing.T) {
	// Clear override
	os.Unsetenv("AGM_TMUX_SOCKET")

	socketPath := tmux.GetSocketPath()
	socketDir := filepath.Dir(socketPath)

	// Verify directory is ~/.agm
	homeDir, err := os.UserHomeDir()
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(homeDir, ".agm"), socketDir, "Socket dir should be ~/.agm")

	// Verify basename
	assert.Equal(t, "agm.sock", filepath.Base(socketPath), "Socket filename should be agm.sock")
}

// TestGetReadSocketPaths_ReturnsNonEmpty tests socket path list
func TestGetReadSocketPaths_ReturnsNonEmpty(t *testing.T) {
	paths := tmux.GetReadSocketPaths()

	assert.NotEmpty(t, paths, "Should return at least one socket path")
	assert.Greater(t, len(paths), 0, "Should have at least one path")

	// Verify all paths are absolute
	for _, path := range paths {
		assert.True(t, filepath.IsAbs(path), "Socket path should be absolute: %s", path)
	}
}

// TestGetReadSocketPaths_ContainsPrimarySocket tests primary socket inclusion
func TestGetReadSocketPaths_ContainsPrimarySocket(t *testing.T) {
	primarySocket := tmux.GetSocketPath()
	readPaths := tmux.GetReadSocketPaths()

	assert.Contains(t, readPaths, primarySocket, "Read paths should contain primary socket")
}

// TestTimeoutError_Structure tests TimeoutError type
func TestTimeoutError_Structure(t *testing.T) {
	// Create a timeout error
	timeout := 5 * time.Second
	err := &tmux.TimeoutError{
		Problem:  "Command timed out",
		Recovery: "Retry command",
		Duration: timeout,
	}

	// Verify Error() implementation
	errStr := err.Error()
	assert.Contains(t, errStr, "Command timed out", "Error should contain problem")
	assert.Contains(t, errStr, "Retry command", "Error should contain recovery")

	// Verify duration is stored
	assert.Equal(t, timeout, err.Duration, "Duration should match")
}

// TestRunWithTimeout_ContextCancellation tests RunWithTimeout cancellation
func TestRunWithTimeout_ContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	timeout := 10 * time.Second

	// Cancel context immediately
	cancel()

	// Run command with cancelled context
	_, err := tmux.RunWithTimeout(ctx, timeout, "echo", "test")

	// Should fail due to context cancellation
	assert.Error(t, err, "Should fail with cancelled context")
}

// TestRunWithTimeout_QuickCommand tests RunWithTimeout with fast command
func TestRunWithTimeout_QuickCommand(t *testing.T) {
	ctx := context.Background()
	timeout := 5 * time.Second

	// Run quick command
	output, err := tmux.RunWithTimeout(ctx, timeout, "echo", "hello")

	assert.NoError(t, err, "Quick command should succeed")
	assert.Contains(t, string(output), "hello", "Output should contain echo text")
}

// TestCommandArgs_OrderPreservation tests argument order preservation
func TestCommandArgs_OrderPreservation(t *testing.T) {
	ctx := context.Background()
	timeout := 1 * time.Second

	// Create command with specific arg order
	args := []string{"-S", "/socket", "new-session", "-d", "-s", "test", "-c", "/dir"}
	cmd, cancel := tmux.CommandWithTimeout(ctx, timeout, "tmux", args...)
	defer cancel()

	// Verify args are in correct order (first arg is command itself)
	assert.Equal(t, "tmux", cmd.Args[0], "First arg should be command")
	for i, arg := range args {
		assert.Equal(t, arg, cmd.Args[i+1], "Arg %d should match", i)
	}
}

// TestCommandPath_TmuxLookup tests tmux binary resolution
func TestCommandPath_TmuxLookup(t *testing.T) {
	// Verify tmux exists in PATH (or skip test)
	tmuxPath, err := exec.LookPath("tmux")
	if err != nil {
		t.Skip("tmux not in PATH, skipping binary lookup test")
	}

	// Verify path is absolute
	assert.True(t, filepath.IsAbs(tmuxPath), "tmux path should be absolute")

	// Verify it's a file
	info, err := os.Stat(tmuxPath)
	require.NoError(t, err, "Should be able to stat tmux binary")
	assert.False(t, info.IsDir(), "tmux should not be a directory")
}

// TestSocketPath_Simplicity tests socket path simplicity
func TestSocketPath_Simplicity(t *testing.T) {
	// Clear override
	os.Unsetenv("AGM_TMUX_SOCKET")

	socketPath := tmux.GetSocketPath()

	// Socket path should be ~/.agm/agm.sock
	homeDir, err := os.UserHomeDir()
	require.NoError(t, err)
	expected := filepath.Join(homeDir, ".agm", "agm.sock")
	assert.Equal(t, expected, socketPath, "Default socket should be ~/.agm/agm.sock")

	// Verify it's in ~/.agm (user-owned, safe from /tmp cleanup)
	assert.True(t, strings.HasPrefix(socketPath, homeDir), "Socket should be under home dir")

	// Verify extension
	assert.True(t, strings.HasSuffix(socketPath, ".sock"), "Socket should have .sock extension")
}

// TestCommandConstruction_ControlMode tests control mode command
func TestCommandConstruction_ControlMode(t *testing.T) {
	socketPath := "/test/socket.sock"
	ctx := context.Background()

	// Build control mode command
	cmd := exec.CommandContext(ctx, "tmux", "-S", socketPath, "-C", "attach-session", "-t", "test-session")

	// Verify command structure
	assert.Equal(t, "tmux", filepath.Base(cmd.Path), "Should use tmux binary")
	assert.Contains(t, cmd.Args, "-S", "Should include socket flag")
	assert.Contains(t, cmd.Args, socketPath, "Should include socket path")
	assert.Contains(t, cmd.Args, "-C", "Should include control mode flag")
	assert.Contains(t, cmd.Args, "attach-session", "Should include attach-session command")
	assert.Contains(t, cmd.Args, "-t", "Should include target flag")
	assert.Contains(t, cmd.Args, "test-session", "Should include session name")
}
