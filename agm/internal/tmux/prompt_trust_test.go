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

// TestWaitForClaudePrompt_DoesNotAnswerShellTrustPrompt verifies that rendered
// trust chrome cannot authorize Enter unless a live Claude process owns the
// exact pane. The pure probe tests cover the corresponding live-Claude path.
func TestWaitForClaudePrompt_DoesNotAnswerShellTrustPrompt(t *testing.T) {
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
touch trust-answered
# After Enter, render the Claude prompt.
sleep 0.2
printf '\n❯ '
sleep 30
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

	err = WaitForClaudePrompt(sessionName, time.Second)
	require.Error(t, err, "shell-rendered trust chrome must fail closed")
	_, statErr := os.Stat(filepath.Join(tmpDir, "trust-answered"))
	require.ErrorIs(t, statErr, os.ErrNotExist, "shell-rendered trust chrome received Enter")
}

// TestWaitForClaudePrompt_DoesNotTrustShellComposer verifies that a bare prompt
// glyph rendered by a shell is not Claude readiness evidence.
func TestWaitForClaudePrompt_DoesNotTrustShellComposer(t *testing.T) {
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
sleep 30
`
	require.NoError(t, writeExecutableFile(script, scriptContent))

	err := NewSession(sessionName, tmpDir)
	require.NoError(t, err)

	socketPath := GetSocketPath()
	normalizedName := NormalizeTmuxSessionName(sessionName)
	cmd := exec.Command("tmux", "-S", socketPath, "send-keys", "-t", normalizedName, script, "C-m")
	require.NoError(t, cmd.Run())

	err = WaitForClaudePrompt(sessionName, time.Second)
	require.Error(t, err, "shell-rendered composer must fail closed")
}
