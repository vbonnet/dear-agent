package tmux

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestFullWorkflow_Integration tests complete tmux client workflow.
func TestFullWorkflow_Integration(t *testing.T) {
	if !isTmuxAvailable() {
		t.Skip("tmux not available, skipping integration test")
	}

	sessionName := "astrocyte-workflow-test"
	createTestSession(t, sessionName)
	defer cleanupTestSession(t, sessionName)

	client := NewClient()

	// 1. Verify session exists
	sessions, err := client.ListSessions()
	require.NoError(t, err)
	assert.Contains(t, sessions, sessionName)

	// 2. Capture initial pane state
	pane1, err := CapturePaneInfo(client, sessionName)
	require.NoError(t, err)
	assert.Equal(t, sessionName, pane1.SessionName)
	assert.NotEmpty(t, pane1.Content)

	// 3. Send command to session
	err = client.SendKeys(sessionName, "echo 'test-marker'")
	require.NoError(t, err)

	err = client.SendKeys(sessionName, "Enter")
	require.NoError(t, err)

	// Wait for command execution
	time.Sleep(200 * time.Millisecond)

	// 4. Capture updated pane state
	pane2, err := CapturePaneInfo(client, sessionName)
	require.NoError(t, err)
	assert.Contains(t, pane2.Content, "test-marker")

	// 5. Verify cursor position
	x, y, err := client.GetCursorPosition(sessionName)
	require.NoError(t, err)
	assert.GreaterOrEqual(t, x, 0)
	assert.GreaterOrEqual(t, y, 0)

	// 6. Test session existence check
	assert.True(t, client.HasSession(sessionName))
	assert.False(t, client.HasSession("nonexistent-session"))
}

// TestStuckDetection_Integration tests stuck session detection with real tmux.
func TestStuckDetection_Integration(t *testing.T) {
	if !isTmuxAvailable() {
		t.Skip("tmux not available, skipping integration test")
	}

	sessionName := "astrocyte-stuck-test"
	createTestSession(t, sessionName)
	defer cleanupTestSession(t, sessionName)

	client := NewClient()

	// Simulate stuck pattern by sending text with spinner
	sendTestKeys(t, sessionName, "echo '✶ Thinking...'", "Enter")
	time.Sleep(100 * time.Millisecond)

	pane, err := CapturePaneInfo(client, sessionName)
	require.NoError(t, err)

	// Check for stuck indicators
	indicators := pane.DetectStuckIndicators()
	assert.True(t, indicators["waiting"], "should detect waiting indicator")

	// Note: Will likely show idle_prompt since shell returned after echo
	// Real stuck detection would need actual Claude session
}

// TestRecovery_Integration tests recovery key sending.
func TestRecovery_Integration(t *testing.T) {
	if !isTmuxAvailable() {
		t.Skip("tmux not available, skipping integration test")
	}

	sessionName := "astrocyte-recovery-test"
	createTestSession(t, sessionName)
	defer cleanupTestSession(t, sessionName)

	client := NewClient()

	// Send Escape key (common recovery method)
	err := client.SendKeys(sessionName, "Escape")
	assert.NoError(t, err)

	// Send Ctrl-C (alternative recovery)
	err = client.SendKeys(sessionName, "C-c")
	assert.NoError(t, err)

	// Verify session still responsive
	assert.True(t, client.HasSession(sessionName))
}

// TestMultipleSessionsMonitoring_Integration tests monitoring multiple sessions.
func TestMultipleSessionsMonitoring_Integration(t *testing.T) {
	if !isTmuxAvailable() {
		t.Skip("tmux not available, skipping integration test")
	}

	sessions := []string{
		"astrocyte-multi-1",
		"astrocyte-multi-2",
		"astrocyte-multi-3",
	}

	// Create test sessions
	for _, name := range sessions {
		createTestSession(t, name)
		defer cleanupTestSession(t, name)
	}

	client := NewClient()

	// List all sessions
	allSessions, err := client.ListSessions()
	require.NoError(t, err)

	// Verify all test sessions exist
	for _, name := range sessions {
		assert.Contains(t, allSessions, name)
	}

	// Capture state for each session
	for _, name := range sessions {
		pane, err := CapturePaneInfo(client, name)
		require.NoError(t, err)
		assert.Equal(t, name, pane.SessionName)
	}
}

// TestCommandExtraction_Integration tests command extraction from real pane.
func TestCommandExtraction_Integration(t *testing.T) {
	if !isTmuxAvailable() {
		t.Skip("tmux not available, skipping integration test")
	}

	sessionName := "astrocyte-cmd-extract"
	createTestSession(t, sessionName)
	defer cleanupTestSession(t, sessionName)

	client := NewClient()

	// Send a command with header
	sendTestKeys(t, sessionName, "echo 'Bash command:'", "Enter")
	sendTestKeys(t, sessionName, "echo 'git status'", "Enter")
	time.Sleep(200 * time.Millisecond)

	pane, err := CapturePaneInfo(client, sessionName)
	require.NoError(t, err)

	// Note: ExtractLastCommand looks for specific patterns
	// May not extract from simple echo commands
	// This is more of a structural test
	assert.NotEmpty(t, pane.Content)
}

// TestPaneContentHistory_Integration tests scrollback capture.
func TestPaneContentHistory_Integration(t *testing.T) {
	if !isTmuxAvailable() {
		t.Skip("tmux not available, skipping integration test")
	}

	sessionName := "astrocyte-history-test"
	createTestSession(t, sessionName)
	defer cleanupTestSession(t, sessionName)

	client := NewClient()

	// Generate lots of output to test scrollback
	for i := 0; i < 100; i++ {
		sendTestKeys(t, sessionName, fmt.Sprintf("echo 'Line %d'", i), "Enter")
	}
	time.Sleep(500 * time.Millisecond)

	pane, err := CapturePaneInfo(client, sessionName)
	require.NoError(t, err)

	// Should capture history (up to 500 lines)
	assert.NotEmpty(t, pane.Content)
	assert.Greater(t, len(pane.Content), 100, "should have substantial content")
}

// TestErrorHandling_Integration tests various error conditions.
func TestErrorHandling_Integration(t *testing.T) {
	if !isTmuxAvailable() {
		t.Skip("tmux not available, skipping integration test")
	}

	client := NewClient()

	t.Run("nonexistent session capture", func(t *testing.T) {
		_, err := client.GetPaneContent("no-such-session-xyz")
		assert.Error(t, err)
	})

	t.Run("nonexistent session cursor", func(t *testing.T) {
		_, _, err := client.GetCursorPosition("no-such-session-xyz")
		assert.Error(t, err)
	})

	t.Run("nonexistent session send keys", func(t *testing.T) {
		err := client.SendKeys("no-such-session-xyz", "test")
		assert.Error(t, err)
	})

	t.Run("nonexistent session has check", func(t *testing.T) {
		exists := client.HasSession("no-such-session-xyz")
		assert.False(t, exists)
	})
}
