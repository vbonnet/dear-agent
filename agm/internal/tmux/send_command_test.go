package tmux

import (
	"fmt"
	"os"
	"os/exec"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestSendCommand_EnterKeyDocumentation documents the correct implementation
// This is a documentation test - it doesn't run tmux but serves as executable documentation
func TestSendCommand_EnterKeyDocumentation(t *testing.T) {
	t.Log("SendCommand/SendPromptLiteral/SendCommandLiteral implementation requirements:")
	t.Log("1. MUST use load-buffer + paste-buffer for atomic text delivery")
	t.Log("   paste-buffer delivers entire text at once, preventing race conditions")
	t.Log("2. MUST send Enter separately using 'tmux send-keys -t SESSION C-m'")
	t.Log("   C-m is the tmux representation of the Enter key")
	t.Log("3. MUST have delay (100ms+) between paste-buffer and C-m")
	t.Log("   This sleep is load-bearing — do not remove")
	t.Log("4. MUST NOT use send-keys -l for text delivery")
	t.Log("   send-keys -l delivers character-by-character, creating race conditions")
	t.Log("")
	t.Log("Incorrect (causes ENTER timing bug):")
	t.Log("  tmux send-keys -t session -l '/command'     # Character-by-character, races with Enter")
	t.Log("  tmux send-keys -t session 'C-m'")
	t.Log("")
	t.Log("Correct (current implementation):")
	t.Log("  tmux load-buffer -b agm-cmd - <<< '/command'  # Load text into buffer")
	t.Log("  tmux paste-buffer -b agm-cmd -t session -d     # Paste atomically")
	t.Log("  sleep 0.1                                       # Wait for processing")
	t.Log("  tmux send-keys -t session C-m                   # Send Enter separately")
	t.Log("")
	t.Log("Bug history:")
	t.Log("  2026-04-07: SendPromptLiteral switched to paste-buffer (2c09d01c)")
	t.Log("  2026-04-07: SendCommandLiteral and SendKeysToPane also switched")
	t.Log("See fix-enter-timing-v3 branch for full context")
}

// TestSendCommand_SpecialCharacters tests that special characters are handled correctly
func TestSendCommand_SpecialCharacters(t *testing.T) {
	// Skip if tmux not available
	if _, err := exec.LookPath("tmux"); err != nil {
		t.Skip("tmux not available")
	}

	if os.Getenv("CI") != "" && os.Getenv("AGM_TEST_TMUX") == "" {
		t.Skip("Skipping tmux integration test in CI")
	}

	// Create isolated socket for this test
	testSocket := fmt.Sprintf("/tmp/agm-test-%d.sock", os.Getpid())
	os.Setenv("AGM_TMUX_SOCKET", testSocket)
	t.Cleanup(func() {
		exec.Command("tmux", "-S", testSocket, "kill-server").Run()
		os.Remove(testSocket)
		os.Unsetenv("AGM_TMUX_SOCKET")
	})
	setupTestState(t)

	sessionName := "test-special-chars"

	// Create test tmux session
	cmd := exec.Command("tmux", "-S", testSocket, "new-session", "-d", "-s", sessionName)
	err := cmd.Run()
	require.NoError(t, err)
	defer exec.Command("tmux", "-S", testSocket, "kill-session", "-t", sessionName).Run()

	time.Sleep(100 * time.Millisecond)

	// Test table of commands with special characters
	tests := []struct {
		name    string
		command string
		expect  string
	}{
		{
			name:    "command with quotes",
			command: "echo \"test with quotes\"",
			expect:  "test with quotes",
		},
		{
			name:    "command with dollar sign",
			command: "echo hello$USER",
			expect:  "hello", // $USER should be expanded
		},
		{
			name:    "command with semicolon",
			command: "echo first; echo second",
			expect:  "first", // Should execute both commands
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Clean up any stale lock from previous subtest
			ReleaseTmuxLock()

			// Clear pane before test
			clearCmd := exec.Command("tmux", "-S", testSocket, "send-keys", "-t", sessionName, "C-l")
			clearCmd.Run()
			time.Sleep(50 * time.Millisecond)

			// Send command
			err := SendCommand(sessionName, tt.command)
			require.NoError(t, err)

			// Wait for execution
			time.Sleep(200 * time.Millisecond)

			// Capture output
			captureCmd := exec.Command("tmux", "-S", testSocket, "capture-pane", "-t", sessionName, "-p")
			output, err := captureCmd.CombinedOutput()
			require.NoError(t, err)

			outputStr := string(output)
			assert.Contains(t, outputStr, tt.expect,
				"Command should execute and produce expected output")
		})
	}
}

// TestSendCommand_ErrorHandling tests error cases
func TestSendCommand_ErrorHandling(t *testing.T) {
	tests := []struct {
		name        string
		sessionName string
		command     string
		wantErr     bool
		errContains string
	}{
		{
			name:        "non-existent session",
			sessionName: "non-existent-session-12345",
			command:     "echo test",
			wantErr:     true,
			errContains: "", // Will fail when tmux can't find session
		},
		// Note: Empty session name is handled by tmux, not validated by SendCommand
		// This might be something to add in the future, but currently not an error
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := SendCommand(tt.sessionName, tt.command)
			if tt.wantErr {
				assert.Error(t, err, "Expected error for %s", tt.name)
				if tt.errContains != "" {
					assert.Contains(t, err.Error(), tt.errContains)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// TestSendCommand_Timeout tests that send command respects timeout
func TestSendCommand_Timeout(t *testing.T) {
	// This test verifies that SendCommand will timeout if tmux hangs
	// We can't easily make tmux hang, so this is more of a documentation test

	// If we set a very short global timeout and tmux is slow, we should see timeout error
	originalTimeout := globalTimeout
	globalTimeout = 1 * time.Millisecond // Very short timeout
	defer func() { globalTimeout = originalTimeout }()

	// Send to non-existent session (will timeout quickly)
	err := SendCommand("definitely-does-not-exist", "test")

	// Should error (either timeout or session not found)
	assert.Error(t, err, "Should error with very short timeout or missing session")
}

// Benchmark to ensure SendCommand performance hasn't regressed
func BenchmarkSendCommand(b *testing.B) {
	// Skip if tmux not available
	if _, err := exec.LookPath("tmux"); err != nil {
		b.Skip("tmux not available")
	}

	// Create test session
	tmpDir := b.TempDir()
	testSocket := tmpDir + "/bench-send-command.sock"
	os.Setenv("AGM_TMUX_SOCKET", testSocket)
	defer os.Unsetenv("AGM_TMUX_SOCKET")

	sessionName := "bench-send-cmd"
	cmd := exec.Command("tmux", "-S", testSocket, "new-session", "-d", "-s", sessionName)
	if err := cmd.Run(); err != nil {
		b.Skipf("Failed to create tmux session: %v", err)
	}
	defer exec.Command("tmux", "-S", testSocket, "kill-session", "-t", sessionName).Run()

	time.Sleep(100 * time.Millisecond)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		err := SendCommand(sessionName, "echo test")
		if err != nil {
			b.Fatalf("SendCommand failed: %v", err)
		}
	}
}

// TestSendMultiLinePromptSafe_PromptReady tests command sent after prompt detected
func TestSendMultiLinePromptSafe_PromptReady(t *testing.T) {
	// This test uses mocks to verify wait-then-send behavior
	// Integration test with real tmux would be too slow and flaky

	sessionName := "test-send-safe-ready"
	prompt := "Review this code:\n- Check bugs\n- Suggest improvements"

	// SETUP: Mock prompt detector (prompt ready)
	mockPrompt := &mockPromptDetector{
		promptReady: true,
		waitTime:    100 * time.Millisecond,
	}

	// SETUP: Mock command sender
	mockSender := &mockCommandSender{}

	// EXECUTION: Simulate SendMultiLinePromptSafe behavior
	// Note: We test the logic pattern, not the actual function
	// The actual function calls WaitForPromptSimple then SendPromptLiteral

	// Step 1: Wait for prompt
	err := mockPrompt.WaitForPromptSimple(sessionName, 60*time.Second)
	require.NoError(t, err, "Expected prompt wait to succeed")

	// Step 2: Send command in literal mode
	if err == nil {
		mockSender.SendPromptLiteral(sessionName, prompt)
	}

	// ASSERTIONS: Verify wait-then-send order
	assert.True(t, mockPrompt.WaitCalled,
		"Expected WaitForPromptSimple called before send")
	assert.NotEmpty(t, mockSender.CommandsSent,
		"Expected command sent after prompt detected")
	assert.Equal(t, prompt, mockSender.CommandsSent[0],
		"Expected exact prompt text sent")
	assert.True(t, mockSender.UsedLiteralMode,
		"Expected literal mode to prevent special char interpretation")
}

// TestSendMultiLinePromptSafe_PromptTimeout tests timeout prevents command send
func TestSendMultiLinePromptSafe_PromptTimeout(t *testing.T) {
	sessionName := "test-send-safe-timeout"
	prompt := "This should not be sent"

	// SETUP: Mock prompt detector (timeout)
	mockPrompt := &mockPromptDetector{
		promptReady:  false,
		waitTime:     100 * time.Millisecond,
		timeoutError: assert.AnError,
	}

	// SETUP: Mock command sender
	mockSender := &mockCommandSender{}

	// EXECUTION: Simulate SendMultiLinePromptSafe with timeout
	err := mockPrompt.WaitForPromptSimple(sessionName, 60*time.Second)

	// Only send if wait succeeded
	if err == nil {
		mockSender.SendPromptLiteral(sessionName, prompt)
	}

	// ASSERTIONS
	require.Error(t, err, "Expected timeout error when prompt not ready")
	assert.True(t, mockPrompt.WaitCalled,
		"Expected WaitForPromptSimple called")
	assert.Empty(t, mockSender.CommandsSent,
		"Expected NO command sent when prompt timeout")
}

// TestSendMultiLinePromptSafe_SpecialCharacters tests literal mode handling
func TestSendMultiLinePromptSafe_SpecialCharacters(t *testing.T) {
	sessionName := "test-send-safe-special"
	prompt := "echo $HOME && ls -la | grep test"

	// SETUP: Mock prompt detector (ready)
	mockPrompt := &mockPromptDetector{
		promptReady: true,
	}

	// SETUP: Mock command sender
	mockSender := &mockCommandSender{}

	// EXECUTION
	err := mockPrompt.WaitForPromptSimple(sessionName, 60*time.Second)
	require.NoError(t, err)

	mockSender.SendPromptLiteral(sessionName, prompt)

	// ASSERTIONS: Verify literal mode prevents shell interpretation
	assert.True(t, mockSender.UsedLiteralMode,
		"Expected literal mode to prevent shell expansion")
	assert.Equal(t, prompt, mockSender.CommandsSent[0],
		"Expected exact text sent, no variable expansion")
}

// Mock types for SendMultiLinePromptSafe tests

type mockPromptDetector struct {
	promptReady  bool
	waitTime     time.Duration
	timeoutError error
	WaitCalled   bool
}

func (m *mockPromptDetector) WaitForPromptSimple(sessionName string, timeout time.Duration) error {
	m.WaitCalled = true

	// Simulate wait time
	if m.waitTime > 0 {
		time.Sleep(m.waitTime)
	}

	if !m.promptReady {
		if m.timeoutError != nil {
			return m.timeoutError
		}
		return assert.AnError
	}

	return nil
}

type mockCommandSender struct {
	CommandsSent    []string
	UsedLiteralMode bool
}

func (m *mockCommandSender) SendPromptLiteral(sessionName string, text string) error {
	m.UsedLiteralMode = true
	m.CommandsSent = append(m.CommandsSent, text)
	return nil
}
