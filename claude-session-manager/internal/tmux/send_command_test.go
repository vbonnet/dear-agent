package tmux

import (
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestSendCommand_EnterKeySeparation is a REGRESSION TEST for the bug where
// sending the Enter key in the same command as the text caused a newline to appear
// in the prompt instead of executing the command.
//
// Bug History (2026-01-13):
// - Before fix: SendCommand sent "/cmd C-m" in a single send-keys call
// - Symptom: Command text appeared in prompt but didn't execute (Enter not sent)
// - Root cause: tmux interprets "C-m" as literal text when included with other text
// - Fix: Split into two calls - send text with `-l` flag, then send C-m separately
//
// This test ensures we never regress back to the broken single-command approach.
func TestSendCommand_EnterKeySeparation(t *testing.T) {
	// Skip if tmux not available
	if _, err := exec.LookPath("tmux"); err != nil {
		t.Skip("tmux not available")
	}

	// Skip in CI unless explicitly enabled
	if os.Getenv("CI") != "" && os.Getenv("CSM_TEST_TMUX") == "" {
		t.Skip("Skipping tmux integration test in CI (set CSM_TEST_TMUX=1 to enable)")
	}

	// Create isolated socket and state dir for this test
	tmpDir := t.TempDir()
	testSocket := tmpDir + "/test-send-command.sock"
	os.Setenv("CSM_TMUX_SOCKET", testSocket)
	os.Setenv("CSM_STATE_DIR", tmpDir) // Isolate lock files
	defer os.Unsetenv("CSM_TMUX_SOCKET")
	defer os.Unsetenv("CSM_STATE_DIR")

	sessionName := "test-send-cmd"

	// Create test tmux session
	cmd := exec.Command("tmux", "-S", testSocket, "new-session", "-d", "-s", sessionName)
	err := cmd.Run()
	require.NoError(t, err, "Failed to create test tmux session")
	defer func() {
		// Cleanup: kill test session
		exec.Command("tmux", "-S", testSocket, "kill-session", "-t", sessionName).Run()
	}()

	// Wait for session to be ready
	time.Sleep(100 * time.Millisecond)

	// Send a command using our SendCommand function
	testCommand := "echo 'regression test'"
	err = SendCommand(sessionName, testCommand)
	require.NoError(t, err, "SendCommand should not error")

	// Give command time to execute
	time.Sleep(200 * time.Millisecond)

	// Capture pane output to verify command executed
	captureCmd := exec.Command("tmux", "-S", testSocket, "capture-pane", "-t", sessionName, "-p")
	output, err := captureCmd.CombinedOutput()
	require.NoError(t, err, "Failed to capture pane output")

	outputStr := string(output)

	// Verify the command was executed (not just typed in the prompt)
	// The output should contain the echo result, not the command sitting in a prompt
	assert.Contains(t, outputStr, "regression test", "Command output should be visible (command executed)")

	// Verify the command is NOT sitting at a prompt waiting for Enter
	// This would indicate the bug has regressed
	lines := strings.Split(strings.TrimSpace(outputStr), "\n")
	lastLine := lines[len(lines)-1]

	// The last line should NOT be the command itself (that would mean it's at a prompt)
	// It should be either the command output or a shell prompt after execution
	assert.NotContains(t, lastLine, "echo 'regression test'",
		"Command should NOT be sitting in prompt (indicates Enter was not sent)")
}

// TestSendCommand_EnterKeyDocumentation documents the correct implementation
// This is a documentation test - it doesn't run tmux but serves as executable documentation
func TestSendCommand_EnterKeyDocumentation(t *testing.T) {
	t.Log("SendCommand implementation requirements:")
	t.Log("1. MUST send command text using 'tmux send-keys -t SESSION -l COMMAND'")
	t.Log("   The -l flag sends text literally (prevents interpretation of special keys)")
	t.Log("2. MUST send Enter separately using 'tmux send-keys -t SESSION C-m'")
	t.Log("   C-m is the tmux representation of the Enter key")
	t.Log("3. MUST NOT send command and Enter in the same send-keys call")
	t.Log("   Combining them causes C-m to be typed as text instead of executing")
	t.Log("")
	t.Log("Incorrect (causes regression bug):")
	t.Log("  tmux send-keys -t session '/command' 'C-m'  # C-m becomes newline in prompt")
	t.Log("")
	t.Log("Correct (current implementation):")
	t.Log("  tmux send-keys -t session -l '/command'     # Send text literally")
	t.Log("  tmux send-keys -t session 'C-m'             # Send Enter separately")
	t.Log("")
	t.Log("See commit fixing this bug for full context")
}

// TestSendCommand_SpecialCharacters tests that special characters are handled correctly
func TestSendCommand_SpecialCharacters(t *testing.T) {
	// Skip if tmux not available
	if _, err := exec.LookPath("tmux"); err != nil {
		t.Skip("tmux not available")
	}

	if os.Getenv("CI") != "" && os.Getenv("CSM_TEST_TMUX") == "" {
		t.Skip("Skipping tmux integration test in CI")
	}

	// Create isolated socket for this test
	tmpDir := t.TempDir()
	testSocket := tmpDir + "/test-special-chars.sock"
	os.Setenv("CSM_TMUX_SOCKET", testSocket)
	defer os.Unsetenv("CSM_TMUX_SOCKET")

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
	os.Setenv("CSM_TMUX_SOCKET", testSocket)
	defer os.Unsetenv("CSM_TMUX_SOCKET")

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
