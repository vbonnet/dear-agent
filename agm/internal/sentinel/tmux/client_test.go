package tmux

import (
	"os"
	"os/exec"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	agmtmux "github.com/vbonnet/dear-agent/agm/internal/tmux"
)

// TestNewClient verifies client initialization.
func TestNewClient(t *testing.T) {
	client := NewClient()
	assert.NotNil(t, client)
	assert.NotNil(t, client.socketPaths)
	assert.Greater(t, len(client.socketPaths), 0, "should have at least one socket path")
}

// TestGetReadSocketPaths verifies socket path detection.
func TestGetReadSocketPaths(t *testing.T) {
	paths := getReadSocketPaths()
	assert.NotNil(t, paths)
	assert.Greater(t, len(paths), 0, "should return at least one socket path")
}

// TestListSessions_NoTmux tests session listing when tmux is not available.
func TestListSessions_NoTmux(t *testing.T) {
	// This test may fail if tmux is actually running
	// Skip if tmux has active sessions
	if isTmuxAvailable() {
		t.Skip("tmux is running, skipping NoTmux test")
	}

	client := NewClient()
	sessions, err := client.ListSessions()

	// Should not error even if no sessions
	assert.NoError(t, err)
	assert.NotNil(t, sessions)
}

// TestListSessions_Integration is an integration test requiring tmux.
func TestListSessions_Integration(t *testing.T) {
	if !isTmuxAvailable() {
		t.Skip("tmux not available, skipping integration test")
	}

	// Create a test session
	sessionName := "astrocyte-test-session"
	createTestSession(t, sessionName)
	defer cleanupTestSession(t, sessionName)

	client := NewClient()
	sessions, err := client.ListSessions()

	require.NoError(t, err)
	assert.Contains(t, sessions, sessionName, "should find test session")
}

// TestGetPaneContent_Integration tests pane content capture.
func TestGetPaneContent_Integration(t *testing.T) {
	if !isTmuxAvailable() {
		t.Skip("tmux not available, skipping integration test")
	}

	sessionName := "astrocyte-test-pane"
	createTestSession(t, sessionName)
	defer cleanupTestSession(t, sessionName)

	// Send some text to the pane
	sendTestKeys(t, sessionName, "echo 'Hello Astrocyte'", "Enter")

	client := NewClient()
	content, err := client.GetPaneContent(sessionName)

	require.NoError(t, err)
	assert.NotEmpty(t, content, "pane content should not be empty")
}

// TestGetPaneContent_NonExistentSession tests error handling for missing session.
func TestGetPaneContent_NonExistentSession(t *testing.T) {
	if !isTmuxAvailable() {
		t.Skip("tmux not available, skipping integration test")
	}

	client := NewClient()
	_, err := client.GetPaneContent("nonexistent-session-12345")

	assert.Error(t, err, "should error for nonexistent session")
	assert.Contains(t, err.Error(), "not found", "error should mention session not found")
}

// TestGetCursorPosition_Integration tests cursor position retrieval.
func TestGetCursorPosition_Integration(t *testing.T) {
	if !isTmuxAvailable() {
		t.Skip("tmux not available, skipping integration test")
	}

	sessionName := "astrocyte-test-cursor"
	createTestSession(t, sessionName)
	defer cleanupTestSession(t, sessionName)

	client := NewClient()
	x, y, err := client.GetCursorPosition(sessionName)

	require.NoError(t, err)
	assert.GreaterOrEqual(t, x, 0, "cursor x should be >= 0")
	assert.GreaterOrEqual(t, y, 0, "cursor y should be >= 0")
}

// TestSendKeys_Integration tests sending keys to a pane.
func TestSendKeys_Integration(t *testing.T) {
	if !isTmuxAvailable() {
		t.Skip("tmux not available, skipping integration test")
	}

	sessionName := "astrocyte-test-sendkeys"
	createTestSession(t, sessionName)
	defer cleanupTestSession(t, sessionName)

	client := NewClient()

	// Send Escape key
	err := client.SendKeys(sessionName, "Escape")
	assert.NoError(t, err, "should send Escape key without error")

	// Send Ctrl-C
	err = client.SendKeys(sessionName, "C-c")
	assert.NoError(t, err, "should send Ctrl-C without error")
}

// TestHasSession_Integration tests session existence check.
func TestHasSession_Integration(t *testing.T) {
	if !isTmuxAvailable() {
		t.Skip("tmux not available, skipping integration test")
	}

	sessionName := "astrocyte-test-hassession"
	createTestSession(t, sessionName)
	defer cleanupTestSession(t, sessionName)

	client := NewClient()

	assert.True(t, client.HasSession(sessionName), "should find existing session")
	assert.False(t, client.HasSession("nonexistent-xyz"), "should not find nonexistent session")
}

// Helper functions

// isTmuxAvailable checks if tmux is installed and accessible.
func isTmuxAvailable() bool {
	cmd := exec.Command("tmux", "-V")
	return cmd.Run() == nil
}

// createTestSession creates a detached tmux session for testing.
func createTestSession(t *testing.T, sessionName string) {
	t.Helper()

	// Kill existing test session if it exists
	exec.Command("tmux", "kill-session", "-t", sessionName).Run()

	cmd := exec.Command("tmux", "new-session", "-d", "-s", sessionName)
	err := cmd.Run()
	require.NoError(t, err, "failed to create test session")

	// Give tmux time to initialize
	// time.Sleep(100 * time.Millisecond)
}

// cleanupTestSession removes a test session.
func cleanupTestSession(t *testing.T, sessionName string) {
	t.Helper()
	exec.Command("tmux", "kill-session", "-t", sessionName).Run()
}

// sendTestKeys sends keys to a test session.
func sendTestKeys(t *testing.T, sessionName string, keys ...string) {
	t.Helper()
	for _, key := range keys {
		cmd := exec.Command("tmux", "send-keys", "-t", sessionName, key)
		err := cmd.Run()
		require.NoError(t, err, "failed to send keys")
	}
}

// TestFindSessionSocket_Integration tests socket detection for a session.
func TestFindSessionSocket_Integration(t *testing.T) {
	if !isTmuxAvailable() {
		t.Skip("tmux not available, skipping integration test")
	}

	sessionName := "astrocyte-test-socket"
	createTestSession(t, sessionName)
	defer cleanupTestSession(t, sessionName)

	client := NewClient()
	socket, err := client.findSessionSocket(sessionName)

	require.NoError(t, err)
	// Socket can be empty string (default) or a path
	if socket != "" {
		assert.True(t, strings.HasPrefix(socket, "/") || socket == "",
			"socket should be absolute path or empty")
	}
}

// TestClient_MultipleSocketPaths tests client behavior with multiple sockets.
func TestClient_MultipleSocketPaths(t *testing.T) {
	client := &Client{
		socketPaths: []string{"/tmp/test-socket-1", "/tmp/test-socket-2", ""},
	}

	assert.Equal(t, 3, len(client.socketPaths))
}

// Benchmark tests

func BenchmarkListSessions(b *testing.B) {
	if !isTmuxAvailable() {
		b.Skip("tmux not available")
	}

	client := NewClient()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = client.ListSessions()
	}
}

func BenchmarkGetPaneContent(b *testing.B) {
	if !isTmuxAvailable() {
		b.Skip("tmux not available")
	}

	sessionName := "astrocyte-bench-pane"
	createTestSession(&testing.T{}, sessionName)
	defer cleanupTestSession(&testing.T{}, sessionName)

	client := NewClient()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = client.GetPaneContent(sessionName)
	}
}

// TestClient_EmptySocketPath tests behavior when socket path is empty.
func TestClient_EmptySocketPath(t *testing.T) {
	client := &Client{
		socketPaths: []string{""},
	}

	// Should not panic with empty socket path
	sessions, err := client.ListSessions()
	assert.NoError(t, err)
	assert.NotNil(t, sessions)
}

// TestGetReadSocketPaths_NoAGMSocket tests socket detection without AGM.
func TestGetReadSocketPaths_NoAGMSocket(t *testing.T) {
	// Temporarily rename AGM socket if it exists
	agmSocket := agmtmux.DefaultSocketPath()
	agmSocketBackup := agmSocket + ".backup-test"

	if _, err := os.Stat(agmSocket); err == nil {
		os.Rename(agmSocket, agmSocketBackup)
		defer os.Rename(agmSocketBackup, agmSocket)
	}

	// Also handle legacy socket
	legacySocket := agmtmux.LegacySocketPath
	legacyBackup := legacySocket + ".backup-test"
	if _, err := os.Stat(legacySocket); err == nil {
		os.Rename(legacySocket, legacyBackup)
		defer os.Rename(legacyBackup, legacySocket)
	}

	paths := getReadSocketPaths()
	assert.NotNil(t, paths)
	assert.Greater(t, len(paths), 0, "should have at least default socket")

	// Should not contain AGM socket
	for _, path := range paths {
		assert.NotEqual(t, agmSocket, path, "should not contain AGM socket")
		assert.NotEqual(t, legacySocket, path, "should not contain legacy socket")
	}
}
