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

// TestIsPasteStuck tests detection of unsubmitted paste-buffer content.
func TestIsPasteStuck(t *testing.T) {
	tests := []struct {
		name     string
		content  string
		expected bool
	}{
		{
			name:     "pasted text indicator",
			content:  "some output\n[Pasted text #1 +3 lines]\n❯",
			expected: true,
		},
		{
			name:     "pasted text partial",
			content:  "[Pasted text",
			expected: true,
		},
		{
			name:     "content on prompt input line",
			content:  "response\n❯ /rename my-session",
			expected: true,
		},
		{
			name:     "clean prompt — no stuck paste",
			content:  "response text\n❯",
			expected: false,
		},
		{
			name:     "clean prompt with trailing space",
			content:  "response text\n❯ ",
			expected: false,
		},
		{
			name:     "empty content",
			content:  "",
			expected: false,
		},
		{
			name:     "no prompt at all — processing",
			content:  "Thinking...\nRunning tests...",
			expected: false,
		},
		{
			name:     "bash prompt with content — not a harness prompt",
			content:  "$ echo hello",
			expected: false,
		},
		{
			name:     "pasted text with no prompt",
			content:  "[Pasted text #2 +1 lines]\nProcessing...",
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isPasteStuck(tt.content)
			assert.Equal(t, tt.expected, got, "isPasteStuck(%q)", tt.content)
		})
	}
}

// TestRetryEnterAfterPaste_NoRetryNeeded tests that no retry occurs when paste submits normally.
func TestRetryEnterAfterPaste_NoRetryNeeded(t *testing.T) {
	skipIfNoTmux(t)
	setupTestSocket(t)
	setupTestState(t)

	sessionName := "test-enter-retry-clean-" + time.Now().Format("150405")
	defer killTestSession(sessionName)

	// Create a test tmux session
	err := NewSession(sessionName, t.TempDir())
	require.NoError(t, err)

	time.Sleep(100 * time.Millisecond)

	// Send a command that will complete — pane should be clean afterward
	socketPath := GetSocketPath()
	normalizedName := NormalizeTmuxSessionName(sessionName)

	// Type a command and press Enter (simulating normal operation)
	cmdSend := exec.Command("tmux", "-S", socketPath, "send-keys", "-t", normalizedName, "echo done", "C-m")
	require.NoError(t, cmdSend.Run())

	time.Sleep(100 * time.Millisecond)

	// retryEnterAfterPaste should detect clean state and return immediately
	err = retryEnterAfterPaste(socketPath, normalizedName, 2)
	assert.NoError(t, err, "should succeed with no retry needed")
}

// TestRetryEnterAfterPaste_DetectsStuckPaste tests that retry fires when paste is stuck.
func TestRetryEnterAfterPaste_DetectsStuckPaste(t *testing.T) {
	skipIfNoTmux(t)
	setupTestSocket(t)
	setupTestState(t)

	sessionName := "test-enter-retry-stuck-" + time.Now().Format("150405")
	defer killTestSession(sessionName)

	// Create a test tmux session
	err := NewSession(sessionName, t.TempDir())
	require.NoError(t, err)

	time.Sleep(100 * time.Millisecond)

	socketPath := GetSocketPath()
	normalizedName := NormalizeTmuxSessionName(sessionName)

	// Simulate a stuck paste: load buffer and paste WITHOUT sending Enter
	cmdLoad := exec.Command("tmux", "-S", socketPath, "load-buffer", "-b", "agm-cmd", "-")
	stdin, err := cmdLoad.StdinPipe()
	require.NoError(t, err)
	require.NoError(t, cmdLoad.Start())
	_, err = stdin.Write([]byte("echo stuck-test"))
	require.NoError(t, err)
	stdin.Close()
	require.NoError(t, cmdLoad.Wait())

	cmdPaste := exec.Command("tmux", "-S", socketPath, "paste-buffer", "-b", "agm-cmd", "-t", normalizedName, "-d")
	require.NoError(t, cmdPaste.Run())

	time.Sleep(50 * time.Millisecond)

	// Verify text is on the input line (simulates stuck paste)
	cmdCapture := exec.Command("tmux", "-S", socketPath, "capture-pane", "-t", normalizedName, "-p", "-S", "-3")
	output, err := cmdCapture.Output()
	require.NoError(t, err)
	content := string(output)
	// In a bash session, the pasted text sits on the prompt line
	assert.Contains(t, content, "stuck-test", "pasted text should be visible on pane")

	// retryEnterAfterPaste should detect this and re-send Enter
	err = retryEnterAfterPaste(socketPath, normalizedName, 2)
	assert.NoError(t, err)

	// After retry, the command should have executed
	time.Sleep(200 * time.Millisecond)
	cmdCapture2 := exec.Command("tmux", "-S", socketPath, "capture-pane", "-t", normalizedName, "-p", "-S", "-5")
	output2, err := cmdCapture2.Output()
	require.NoError(t, err)
	// The echo command should have produced output (meaning Enter was delivered)
	assert.Contains(t, string(output2), "stuck-test", "command should have executed after retry")
}

// TestRetryEnterAfterPaste_MaxRetries tests that retries are bounded.
func TestRetryEnterAfterPaste_MaxRetries(t *testing.T) {
	skipIfNoTmux(t)
	setupTestSocket(t)
	setupTestState(t)

	sessionName := "test-enter-max-retry-" + time.Now().Format("150405")
	defer killTestSession(sessionName)

	err := NewSession(sessionName, t.TempDir())
	require.NoError(t, err)

	time.Sleep(100 * time.Millisecond)

	socketPath := GetSocketPath()
	normalizedName := NormalizeTmuxSessionName(sessionName)

	// With maxRetries=0, should return immediately without checking
	start := time.Now()
	err = retryEnterAfterPaste(socketPath, normalizedName, 0)
	elapsed := time.Since(start)
	assert.NoError(t, err)
	assert.Less(t, elapsed, 50*time.Millisecond, "zero retries should return immediately")
}

// TestRetryEnterAfterPaste_InvalidSession tests error handling for non-existent session.
func TestRetryEnterAfterPaste_InvalidSession(t *testing.T) {
	// When capture-pane fails (session doesn't exist), retryEnterAfterPaste
	// should return nil (best-effort, don't fail the send)
	err := retryEnterAfterPaste("/tmp/nonexistent.sock", "no-such-session", 2)
	assert.NoError(t, err, "should not error on capture-pane failure")
}

// TestSendCommand_EnterRetryIntegration tests that SendCommand includes
// the Enter retry step after paste-buffer.
func TestSendCommand_EnterRetryIntegration(t *testing.T) {
	skipIfNoTmux(t)
	setupTestSocket(t)
	setupTestState(t)

	sessionName := "test-sendcmd-retry-" + time.Now().Format("150405")
	defer killTestSession(sessionName)

	err := NewSession(sessionName, t.TempDir())
	require.NoError(t, err)

	time.Sleep(100 * time.Millisecond)

	// SendCommand should succeed and include the retry step
	err = SendCommand(sessionName, fmt.Sprintf("echo integration-test-%d", os.Getpid()))
	assert.NoError(t, err, "SendCommand should succeed with Enter retry")

	// Verify the command executed
	time.Sleep(300 * time.Millisecond)
	socketPath := GetSocketPath()
	cmdCapture := exec.Command("tmux", "-S", socketPath, "capture-pane", "-t", sessionName, "-p")
	output, err := cmdCapture.Output()
	require.NoError(t, err)
	assert.Contains(t, string(output), "integration-test-", "command should have executed")
}

// TestSendCommandLiteral_EnterRetryIntegration tests that SendCommandLiteral
// includes the Enter retry step.
func TestSendCommandLiteral_EnterRetryIntegration(t *testing.T) {
	skipIfNoTmux(t)
	setupTestSocket(t)
	setupTestState(t)

	sessionName := "test-cmdlit-retry-" + time.Now().Format("150405")
	defer killTestSession(sessionName)

	err := NewSession(sessionName, t.TempDir())
	require.NoError(t, err)

	time.Sleep(100 * time.Millisecond)

	// SendCommandLiteral should succeed and include the retry step
	err = SendCommandLiteral(sessionName, fmt.Sprintf("echo literal-test-%d", os.Getpid()))
	assert.NoError(t, err, "SendCommandLiteral should succeed with Enter retry")

	// Verify the command executed
	time.Sleep(300 * time.Millisecond)
	socketPath := GetSocketPath()
	cmdCapture := exec.Command("tmux", "-S", socketPath, "capture-pane", "-t", sessionName, "-p")
	output, err := cmdCapture.Output()
	require.NoError(t, err)
	assert.Contains(t, string(output), "literal-test-", "command should have executed")
}
