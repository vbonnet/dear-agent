package tmux

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// writeExecutableFile writes content to a path with 0755 permissions.
func writeExecutableFile(path, content string) error {
	return os.WriteFile(path, []byte(content), 0o755)
}

// TestWaitForClaudePrompt_AutoAnswersTrustPrompt verifies that when a Claude
// trust prompt ("Do you trust the files in this folder?") appears in the tmux
// pane, WaitForClaudePrompt auto-answers it by sending Enter and continues
// waiting for the Claude prompt (❯).
//
// This is a regression test for the VROOM startup blocker where Claude opening
// in a sandbox path would show a trust prompt that blocked the ❯ from rendering,
// causing WaitForClaudePrompt to time out.
func TestWaitForClaudePrompt_AutoAnswersTrustPrompt(t *testing.T) {
	skipIfNoTmux(t)
	setupTestSocket(t)
	setupTestState(t)

	sessionName := "test-trust-prompt-" + time.Now().Format("150405")
	defer killTestSession(sessionName)

	// Build a shell script that:
	//   1. Prints the trust prompt UI (including "Yes, proceed" so the
	//      detector matches the answerable state).
	//   2. Reads a single line from stdin (blocks until Enter is sent).
	//   3. Prints the Claude ❯ prompt to signal readiness.
	tmpDir := t.TempDir()
	script := filepath.Join(tmpDir, "fake-claude.sh")
	scriptContent := `#!/bin/bash
echo "Welcome to Claude Code"
echo "Do you trust the files in this folder?"
echo "  ❯ 1. Yes, proceed"
echo "    2. No, exit"
read -r answer
# After Enter, render the Claude prompt.
sleep 0.2
printf '\n❯ '
sleep 5
`
	require.NoError(t, writeExecutableFile(script, scriptContent))

	// Start a tmux session running our fake Claude.
	err := NewSession(sessionName, tmpDir)
	require.NoError(t, err)

	socketPath := GetSocketPath()
	normalizedName := NormalizeTmuxSessionName(sessionName)
	cmd := exec.Command("tmux", "-S", socketPath, "send-keys", "-t", normalizedName, script, "C-m")
	require.NoError(t, cmd.Run(), "should send fake-claude command to tmux")

	// Give the script a moment to print the trust prompt before we start polling.
	time.Sleep(300 * time.Millisecond)

	// WaitForClaudePrompt must:
	//  1. See the trust prompt
	//  2. Send Enter to answer it
	//  3. See the ❯ prompt that the script prints after Enter
	// All within 10s — far below the 90s production timeout.
	start := time.Now()
	err = WaitForClaudePrompt(sessionName, 10*time.Second)
	elapsed := time.Since(start)
	require.NoError(t, err, "should detect ❯ after auto-answering trust prompt (waited %v)", elapsed)
	require.Less(t, elapsed, 10*time.Second, "should complete well under timeout")
}

// TestWaitForClaudePrompt_NoTrustPrompt verifies the existing behavior is
// preserved: when ❯ appears directly with no trust prompt, the function returns
// promptly and does NOT send any keys.
func TestWaitForClaudePrompt_NoTrustPrompt(t *testing.T) {
	skipIfNoTmux(t)
	setupTestSocket(t)
	setupTestState(t)

	sessionName := "test-no-trust-" + time.Now().Format("150405")
	defer killTestSession(sessionName)

	tmpDir := t.TempDir()
	script := filepath.Join(tmpDir, "fake-claude.sh")
	scriptContent := `#!/bin/bash
echo "Welcome to Claude Code"
sleep 0.2
printf '\n❯ '
sleep 5
`
	require.NoError(t, writeExecutableFile(script, scriptContent))

	err := NewSession(sessionName, tmpDir)
	require.NoError(t, err)

	socketPath := GetSocketPath()
	normalizedName := NormalizeTmuxSessionName(sessionName)
	cmd := exec.Command("tmux", "-S", socketPath, "send-keys", "-t", normalizedName, script, "C-m")
	require.NoError(t, cmd.Run())

	start := time.Now()
	err = WaitForClaudePrompt(sessionName, 5*time.Second)
	elapsed := time.Since(start)
	require.NoError(t, err, "should detect ❯ without trust prompt (waited %v)", elapsed)
}
