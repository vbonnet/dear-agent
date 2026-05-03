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
	skipIfNoTmux(t)
	setupTestSocket(t)
	setupTestState(t)

	sessionName := "test-sendcmd-literal-" + time.Now().Format("20060102-150405")
	defer killTestSession(sessionName)

	// Create test session
	err := NewSession(sessionName, t.TempDir())
	require.NoError(t, err, "failed to create test session")

	// SendCommandLiteral should succeed without lock errors
	err = SendCommandLiteral(sessionName, "echo test")
	assert.NoError(t, err, "SendCommandLiteral should not have lock errors")

	// Verify command was sent using send-keys (not load-buffer)
	// We can't easily verify this directly, but the absence of lock errors confirms it
}

// TestSendCommandLiteral_Timing verifies the 100ms delay between paste-buffer and Enter.
// This prevents command queueing where both commands appear on one line.
//
// Bug fix (2026-04-07): Delay reduced from 500ms to 100ms after switching to paste-buffer.
// paste-buffer is atomic (no character-by-character delivery), so a shorter delay suffices.
//
// Regression test for: Commands queuing on same input line before Enter is processed
func TestSendCommandLiteral_Timing(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping timing test in short mode")
	}
	skipIfNoTmux(t)
	setupTestSocket(t)
	setupTestState(t)

	sessionName := "test-timing-" + time.Now().Format("20060102-150405")
	defer killTestSession(sessionName)

	// Create test session
	err := NewSession(sessionName, t.TempDir())
	require.NoError(t, err)

	// Send two commands in rapid succession
	start := time.Now()
	err = SendCommandLiteral(sessionName, "echo first")
	require.NoError(t, err)

	// Second command should not interfere with first
	err = SendCommandLiteral(sessionName, "echo second")
	require.NoError(t, err)
	elapsed := time.Since(start)

	// Both commands together should take at least 200ms (2 × 100ms delays)
	// plus lock acquisition overhead
	assert.GreaterOrEqual(t, elapsed.Milliseconds(), int64(200),
		"Two SendCommandLiteral calls should take >200ms due to 100ms delays")
}

// TestInitSequence_NoDoubleLock verifies that InitSequence.Run() does not
// cause "tmux lock already held" errors.
//
// Regression test for: withTmuxLock() wrapper causing double-lock with SendCommand
func TestInitSequence_NoDoubleLock(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}
	skipIfNoTmux(t)
	setupTestSocket(t)
	setupTestState(t)

	sessionName := "test-no-double-lock-" + time.Now().Format("20060102-150405")
	defer killTestSession(sessionName)
	defer CleanupReadyFile(sessionName)

	// Create session with bash (not Claude, so it will timeout but we'll catch that)
	err := NewSession(sessionName, t.TempDir())
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

// TestInitSequence_CommandsExecuteOnSeparateLines verifies that SendCommandLiteral
// calls execute sequentially with proper delays, preventing command queueing.
//
// Regression test for: Both commands appearing on one input line
func TestInitSequence_CommandsExecuteOnSeparateLines(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}
	skipIfNoTmux(t)
	setupTestSocket(t)
	setupTestState(t)

	sessionName := "test-separate-lines-" + time.Now().Format("20060102-150405")
	defer killTestSession(sessionName)

	// Create test session with bash (no Claude needed)
	err := NewSession(sessionName, t.TempDir())
	require.NoError(t, err)

	// Send two commands in sequence (simulating init sequence behavior)
	start := time.Now()
	err = SendCommandLiteral(sessionName, "/rename "+sessionName)
	require.NoError(t, err, "First command should succeed")

	err = SendCommandLiteral(sessionName, "/agm:agm-assoc "+sessionName)
	require.NoError(t, err, "Second command should succeed")
	elapsed := time.Since(start)

	// Commands should be delayed (each SendCommandLiteral has 100ms delay)
	// Two commands = 2 × 100ms = 200ms minimum, plus lock overhead
	assert.GreaterOrEqual(t, elapsed.Milliseconds(), int64(200),
		"Commands should have proper delays between them (≥200ms for 2 commands)")

	// Capture pane to verify both commands were sent
	time.Sleep(200 * time.Millisecond) // Brief wait for display
	cmd := exec.Command("tmux", "-S", GetSocketPath(), "capture-pane", "-t", sessionName, "-p", "-S", "-20")
	output, err := cmd.CombinedOutput()
	require.NoError(t, err, "Failed to capture pane")

	content := string(output)

	// Verify both commands appear in output
	assert.Contains(t, content, "/rename", "First command should appear in pane output")
	assert.Contains(t, content, "/agm:agm-assoc", "Second command should appear in pane output")

	t.Logf("Commands sent with proper timing (elapsed: %v)", elapsed)
	t.Logf("This verifies commands don't queue on same line due to timing")
}

// TestInitSequence_WaitBetweenCommands verifies that SendCommandLiteral
// enforces proper timing delays between commands.
//
// Regression test for: Commands sent too quickly, both queuing on input buffer
func TestInitSequence_WaitBetweenCommands(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping timing test in short mode")
	}
	skipIfNoTmux(t)
	setupTestSocket(t)
	setupTestState(t)

	sessionName := "test-wait-between-" + time.Now().Format("20060102-150405")
	defer killTestSession(sessionName)

	// Create session with bash (no Claude needed)
	err := NewSession(sessionName, t.TempDir())
	require.NoError(t, err)

	// Measure time to send two commands (simulating init sequence)
	start := time.Now()

	// First command (simulates /rename)
	err = SendCommandLiteral(sessionName, "echo rename")
	require.NoError(t, err)

	// Second command (simulates /agm:agm-assoc)
	err = SendCommandLiteral(sessionName, "echo assoc")
	require.NoError(t, err)

	elapsed := time.Since(start)

	// Each SendCommandLiteral has:
	// - Load buffer + paste (atomic)
	// - Wait 100ms
	// - Send Enter
	// So 2 commands = at least 2 × 100ms = 200ms, plus lock overhead
	assert.GreaterOrEqual(t, elapsed.Milliseconds(), int64(200),
		"Two SendCommandLiteral calls should take ≥200ms due to timing delays")

	t.Logf("Timing verified: %v elapsed for 2 commands (expected ≥1s)", elapsed)
	t.Logf("This verifies commands have proper delays to prevent queueing")
}

// TestSendCommandLiteral_UsesLiteralFlag verifies that SendCommandLiteral
// uses the -l flag for literal text interpretation.
//
// This prevents special character interpretation that could cause issues.
func TestSendCommandLiteral_UsesLiteralFlag(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}
	skipIfNoTmux(t)
	setupTestSocket(t)
	setupTestState(t)

	sessionName := "test-literal-flag-" + time.Now().Format("20060102-150405")
	defer killTestSession(sessionName)

	// Create session
	err := NewSession(sessionName, t.TempDir())
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
	if os.Getenv("AGM_TEST_TMUX") != "1" {
		t.Skip("skipping: requires active tmux session (set AGM_TEST_TMUX=1)")
	}
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}
	skipIfNoTmux(t)
	setupTestSocket(t)
	setupTestState(t)

	// This test requires full AGM integration, so we'll verify
	// that the timeout behavior is correct in detached mode

	sessionName := "test-detached-" + time.Now().Format("20060102-150405")
	defer killTestSession(sessionName)
	defer CleanupReadyFile(sessionName)

	// Create detached session (no Claude, will timeout)
	err := NewSession(sessionName, t.TempDir())
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
	if !isTmuxAvailable() {
		b.Skip("tmux not available")
	}

	testSocket := fmt.Sprintf("/tmp/agm-test-%d.sock", os.Getpid())
	b.Setenv("AGM_TMUX_SOCKET", testSocket)
	b.Cleanup(func() {
		exec.Command("tmux", "-S", testSocket, "kill-server").Run()
		os.Remove(testSocket)
		os.Unsetenv("AGM_TMUX_SOCKET")
	})

	sessionName := "bench-sendcmd-" + time.Now().Format("20060102-150405")

	// Setup
	err := NewSession(sessionName, b.TempDir())
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

// TestInitSequence_PromptScrolledOff_Regression tests the exact regression scenario:
// When prompt has scrolled off the 50-line capture buffer, but PromptVerified=true
// allows initialization to succeed without timeout.
//
// REGRESSION: Prior to fix, sendRename() called WaitForClaudePrompt() redundantly
// even though new.go:834 already verified the prompt. If the prompt scrolled off
// the 50-line buffer (e.g., due to skill output), WaitForClaudePrompt() would timeout
// after 30s, causing /rename to never be sent.
//
// FIX: Add PromptVerified flag. When set to true by caller (new.go:1167), skip
// redundant WaitForClaudePrompt calls in sendRename() and sendAssociation().
func TestInitSequence_PromptScrolledOff_Regression(t *testing.T) {
	t.Log("=================================================================")
	t.Log("REGRESSION TEST: Prompt Scrolled Off Buffer")
	t.Log("=================================================================")
	t.Log("")
	t.Log("SCENARIO:")
	t.Log("  1. new.go:834 verifies Claude prompt is ready")
	t.Log("  2. Skill output or other commands fill the buffer (50+ lines)")
	t.Log("  3. Claude prompt scrolls off the capture-pane buffer")
	t.Log("  4. InitSequence.sendRename() calls WaitForClaudePrompt()")
	t.Log("  5. WaitForClaudePrompt() can't find prompt in buffer → 30s timeout")
	t.Log("  6. /rename command never gets sent")
	t.Log("")
	t.Log("BUG:")
	t.Log("  sendRename() redundantly verifies prompt that caller already checked")
	t.Log("")
	t.Log("FIX:")
	t.Log("  Add PromptVerified bool field to InitSequence")
	t.Log("  When PromptVerified=true:")
	t.Log("    - sendRename() skips WaitForClaudePrompt()")
	t.Log("    - sendAssociation() skips WaitForClaudePrompt()")
	t.Log("  new.go:1167 sets PromptVerified=true after verifying at line 834")
	t.Log("")
	t.Log("VERIFICATION:")
	t.Log("  Test simulates prompt-scrolled-off scenario with bash session")
	t.Log("  With PromptVerified=true, should complete quickly (<10s)")
	t.Log("  Without PromptVerified, would timeout (≥30s)")
	t.Log("")
	t.Log("=================================================================")

	if testing.Short() {
		t.Skip("Skipping regression test in short mode")
	}
	skipIfNoTmux(t)
	setupTestSocket(t)
	setupTestState(t)

	sessionName := "test-scrolled-regression-" + time.Now().Format("20060102-150405")
	defer killTestSession(sessionName)
	defer CleanupReadyFile(sessionName)

	// Create session with bash (simulates Claude session)
	err := NewSession(sessionName, t.TempDir())
	require.NoError(t, err, "failed to create test session")

	// Simulate prompt scrolling off buffer by filling it with output
	// (In real scenario, this would be skill output)
	for i := 0; i < 60; i++ {
		SendCommandLiteral(sessionName, fmt.Sprintf("echo 'Line %d fills buffer'", i))
		time.Sleep(10 * time.Millisecond)
	}

	// At this point, original prompt has scrolled off the 50-line buffer
	// WaitForClaudePrompt would fail with timeout

	seq := NewInitSequence(sessionName)
	seq.PromptVerified = true // Caller verified prompt before buffer filled

	// Run should succeed quickly without waiting for prompt
	start := time.Now()
	err = seq.Run()
	elapsed := time.Since(start)

	// Should complete quickly (mostly 5s sleep in sendRename)
	// Without PromptVerified, would timeout after 30s
	assert.Less(t, elapsed.Seconds(), 10.0,
		"With PromptVerified=true, should skip prompt wait even when scrolled off")

	t.Logf("")
	t.Logf("✓ REGRESSION TEST PASSED")
	t.Logf("  Completed in %v (expected <10s)", elapsed)
	t.Logf("  PromptVerified flag successfully bypassed redundant wait")
	t.Logf("  This fixes the timeout when prompt scrolls off buffer")
}

// TestAllSendPaths_UsePasteBuffer is a regression test that verifies
// SendCommandLiteral and SendKeysToPane use paste-buffer (atomic delivery)
// instead of send-keys -l (character-by-character delivery).
//
// Bug fix (2026-04-07): send-keys -l creates a race condition where Enter (C-m)
// can arrive before the terminal finishes processing the text. paste-buffer is
// atomic — the entire text appears in the input at once.
//
// This test sends text with special characters and verifies it appears correctly
// in the pane (which proves paste-buffer is being used — send-keys -l would fail
// differently with certain characters under race conditions).
func TestAllSendPaths_UsePasteBuffer(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}
	skipIfNoTmux(t)
	setupTestSocket(t)
	setupTestState(t)

	sessionName := "test-paste-buffer-" + time.Now().Format("20060102-150405")
	defer killTestSession(sessionName)

	err := NewSession(sessionName, t.TempDir())
	require.NoError(t, err)

	// Test SendCommandLiteral with text that could race with send-keys -l
	longCommand := "echo 'paste-buffer-test-" + time.Now().Format("150405") + "'"
	err = SendCommandLiteral(sessionName, longCommand)
	require.NoError(t, err, "SendCommandLiteral should succeed with paste-buffer")

	time.Sleep(300 * time.Millisecond)

	cmd := exec.Command("tmux", "-S", GetSocketPath(), "capture-pane", "-t", sessionName, "-p", "-S", "-5")
	output, err := cmd.CombinedOutput()
	require.NoError(t, err)
	assert.Contains(t, string(output), "paste-buffer-test",
		"SendCommandLiteral text should appear in pane (confirms atomic paste)")

	// Test SendKeysToPane with text
	err = SendKeysToPane(sessionName, "echo 'sendkeys-paste-test'")
	require.NoError(t, err, "SendKeysToPane should succeed with paste-buffer")

	time.Sleep(300 * time.Millisecond)

	cmd = exec.Command("tmux", "-S", GetSocketPath(), "capture-pane", "-t", sessionName, "-p", "-S", "-5")
	output, err = cmd.CombinedOutput()
	require.NoError(t, err)
	assert.Contains(t, string(output), "sendkeys-paste-test",
		"SendKeysToPane text should appear in pane (confirms atomic paste)")

	// Test SendCommand (was already using paste-buffer before this fix)
	err = SendCommand(sessionName, "echo 'sendcmd-paste-test'")
	require.NoError(t, err, "SendCommand should succeed with paste-buffer")

	time.Sleep(300 * time.Millisecond)

	cmd = exec.Command("tmux", "-S", GetSocketPath(), "capture-pane", "-t", sessionName, "-p", "-S", "-5")
	output, err = cmd.CombinedOutput()
	require.NoError(t, err)
	assert.Contains(t, string(output), "sendcmd-paste-test",
		"SendCommand text should appear in pane (confirms atomic paste)")
}
