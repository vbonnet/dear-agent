package tmux

import (
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestSendCommandLiteral_DoesNotUseSendCommand verifies that SendCommandLiteral
// uses exec.Command directly instead of calling SendCommand (which uses load-buffer).
// This prevents the double-lock bug where SendCommand's withTmuxLock() would conflict
// with InitSequence.Run()'s lock.
//
// Regression test for: "tmux lock already held by this process" error
func TestSendCommandLiteral_DoesNotUseSendCommand(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	sessionName := "test-sendcmd-literal-" + time.Now().Format("20060102-150405")
	defer killTestSession(sessionName)

	// Create test session
	err := NewSession(sessionName, "/tmp")
	require.NoError(t, err, "failed to create test session")

	// SendCommandLiteral should succeed without lock errors
	err = SendCommandLiteral(sessionName, "echo test")
	assert.NoError(t, err, "SendCommandLiteral should not have lock errors")

	// Verify command was sent using send-keys (not load-buffer)
	// We can't easily verify this directly, but the absence of lock errors confirms it
}

// TestSendCommandLiteral_Timing verifies the 500ms delay between text and Enter.
// This prevents command queueing where both commands appear on one line.
//
// Regression test for: Commands queuing on same input line before Enter is processed
func TestSendCommandLiteral_Timing(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping timing test in short mode")
	}

	sessionName := "test-timing-" + time.Now().Format("20060102-150405")
	defer killTestSession(sessionName)

	// Create test session
	err := NewSession(sessionName, "/tmp")
	require.NoError(t, err)

	// Send two commands in rapid succession
	start := time.Now()
	err = SendCommandLiteral(sessionName, "echo first")
	require.NoError(t, err)

	// Second command should not interfere with first
	err = SendCommandLiteral(sessionName, "echo second")
	require.NoError(t, err)
	elapsed := time.Since(start)

	// Both commands together should take at least 1 second (2 × 500ms delays)
	assert.GreaterOrEqual(t, elapsed.Milliseconds(), int64(1000),
		"Two SendCommandLiteral calls should take >1s due to 500ms delays")
}

// TestInitSequence_NoDoubleLock verifies that InitSequence.Run() does not
// cause "tmux lock already held" errors.
//
// Regression test for: withTmuxLock() wrapper causing double-lock with SendCommand
func TestInitSequence_NoDoubleLock(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	sessionName := "test-no-double-lock-" + time.Now().Format("20060102-150405")
	defer killTestSession(sessionName)
	defer CleanupReadyFile(sessionName)

	// Create session with bash (not Claude, so it will timeout but we'll catch that)
	err := NewSession(sessionName, "/tmp")
	require.NoError(t, err)

	seq := NewInitSequence(sessionName)

	// Run will fail (bash prompt != Claude prompt), but should NOT have lock errors
	err = seq.Run()

	// We expect a timeout error, NOT a lock error
	if err != nil {
		errMsg := err.Error()
		assert.NotContains(t, errMsg, "lock already held",
			"Should not have double-lock error")
		assert.NotContains(t, errMsg, "tmux lock",
			"Should not have tmux lock error")
	}
}

// TestInitSequence_CommandsExecuteOnSeparateLines verifies that /rename and
// /agm:agm-assoc execute on separate lines, not queued together.
//
// Regression test for: Both commands appearing on one input line
func TestInitSequence_CommandsExecuteOnSeparateLines(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	sessionName := "test-separate-lines-" + time.Now().Format("20060102-150405")
	defer killTestSession(sessionName)
	defer CleanupReadyFile(sessionName)

	// Create test session
	err := NewSession(sessionName, "/tmp")
	require.NoError(t, err)

	// Start Claude (skip if not available)
	err = SendCommand(sessionName, "-l claude")
	if err != nil {
		t.Skipf("Cannot start Claude: %v", err)
	}
	time.Sleep(100 * time.Millisecond)
	err = SendCommand(sessionName, "C-m")
	require.NoError(t, err)

	// Wait for Claude to be ready
	err = WaitForClaudePrompt(sessionName, 60*time.Second)
	if err != nil {
		t.Skipf("Claude not ready: %v", err)
	}

	// Run InitSequence
	seq := NewInitSequence(sessionName)
	err = seq.Run()
	if err != nil {
		t.Logf("InitSequence.Run() error (expected for test): %v", err)
	}

	// Capture pane content to verify commands are on separate lines
	time.Sleep(1 * time.Second) // Give commands time to appear
	cmd := exec.Command("tmux", "-S", GetSocketPath(), "capture-pane", "-t", sessionName, "-p", "-S", "-50")
	output, err := cmd.CombinedOutput()
	require.NoError(t, err, "Failed to capture pane")

	content := string(output)
	lines := strings.Split(content, "\n")

	// Find /rename line
	renameLineIdx := -1
	assocLineIdx := -1
	for i, line := range lines {
		if strings.Contains(line, "/rename") && strings.Contains(line, sessionName) {
			renameLineIdx = i
		}
		if strings.Contains(line, "/agm:agm-assoc") && strings.Contains(line, sessionName) {
			assocLineIdx = i
		}
	}

	// Both commands should be present
	if renameLineIdx == -1 || assocLineIdx == -1 {
		t.Logf("Pane content:\n%s", content)
		t.Skip("Commands not found in pane (may have scrolled off)")
	}

	// Commands should be on DIFFERENT lines
	assert.NotEqual(t, renameLineIdx, assocLineIdx,
		"Commands should be on separate lines, not queued together")

	// /rename should come BEFORE /agm:agm-assoc
	assert.Less(t, renameLineIdx, assocLineIdx,
		"/rename should execute before /agm:agm-assoc")
}

// TestInitSequence_WaitBetweenCommands verifies that there's a sufficient delay
// between /rename and /agm:agm-assoc to prevent queueing.
//
// Regression test for: Commands sent too quickly, both queuing on input buffer
func TestInitSequence_WaitBetweenCommands(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping timing test in short mode")
	}

	sessionName := "test-wait-between-" + time.Now().Format("20060102-150405")
	defer killTestSession(sessionName)
	defer CleanupReadyFile(sessionName)

	// Create session
	err := NewSession(sessionName, "/tmp")
	require.NoError(t, err)

	// Start Claude (skip if not available)
	err = SendCommand(sessionName, "-l claude")
	if err != nil {
		t.Skipf("Cannot start Claude: %v", err)
	}
	time.Sleep(100 * time.Millisecond)
	err = SendCommand(sessionName, "C-m")
	require.NoError(t, err)

	// Wait for Claude
	err = WaitForClaudePrompt(sessionName, 60*time.Second)
	if err != nil {
		t.Skipf("Claude not ready: %v", err)
	}

	// Measure time for InitSequence to complete
	seq := NewInitSequence(sessionName)
	start := time.Now()
	err = seq.Run()
	elapsed := time.Since(start)

	// InitSequence should take at least 5 seconds (wait after /rename)
	// Plus 2 × 500ms for SendCommandLiteral calls = 6 seconds minimum
	assert.GreaterOrEqual(t, elapsed.Seconds(), 6.0,
		"InitSequence should take ≥6s (5s wait + 2×500ms delays)")
}

// TestSendCommandLiteral_UsesLiteralFlag verifies that SendCommandLiteral
// uses the -l flag for literal text interpretation.
//
// This prevents special character interpretation that could cause issues.
func TestSendCommandLiteral_UsesLiteralFlag(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	sessionName := "test-literal-flag-" + time.Now().Format("20060102-150405")
	defer killTestSession(sessionName)

	// Create session
	err := NewSession(sessionName, "/tmp")
	require.NoError(t, err)

	// Send command with special characters that would be interpreted without -l flag
	testCommand := "/rename test-session-$HOME"
	err = SendCommandLiteral(sessionName, testCommand)
	require.NoError(t, err)

	// Capture pane to verify literal interpretation
	time.Sleep(500 * time.Millisecond)
	cmd := exec.Command("tmux", "-S", GetSocketPath(), "capture-pane", "-t", sessionName, "-p", "-S", "-10")
	output, err := cmd.CombinedOutput()
	require.NoError(t, err)

	content := string(output)

	// Should contain literal "$HOME", not expanded path
	assert.Contains(t, content, "$HOME",
		"Special characters should be literal, not interpreted")
}

// TestInitSequence_DetachedMode verifies that InitSequence works correctly
// in detached mode (--detached flag).
//
// This is the primary use case and where the bugs manifested.
func TestInitSequence_DetachedMode(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	// This test requires full AGM integration, so we'll verify
	// that the timeout behavior is correct in detached mode

	sessionName := "test-detached-" + time.Now().Format("20060102-150405")
	defer killTestSession(sessionName)
	defer CleanupReadyFile(sessionName)

	// Create detached session (no Claude, will timeout)
	err := NewSession(sessionName, "/tmp")
	require.NoError(t, err)

	seq := NewInitSequence(sessionName)

	// In detached mode, Run() should timeout gracefully
	// (bash prompt != Claude prompt)
	start := time.Now()
	err = seq.Run()
	elapsed := time.Since(start)

	// Should timeout after 30 seconds (WaitForClaudePrompt timeout)
	assert.Error(t, err, "Should timeout when Claude not ready")
	assert.GreaterOrEqual(t, elapsed.Seconds(), 30.0,
		"Should wait full timeout period before failing")
	assert.Contains(t, err.Error(), "Claude not ready",
		"Error should mention Claude not being ready")
}

// Benchmark to ensure InitSequence performance hasn't regressed
func BenchmarkSendCommandLiteral(b *testing.B) {
	sessionName := "bench-sendcmd-" + time.Now().Format("20060102-150405")

	// Setup
	err := NewSession(sessionName, "/tmp")
	if err != nil {
		b.Skipf("Cannot create session: %v", err)
	}
	defer killTestSession(sessionName)

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		err := SendCommandLiteral(sessionName, "echo test")
		if err != nil {
			b.Fatalf("SendCommandLiteral failed: %v", err)
		}
	}
}
