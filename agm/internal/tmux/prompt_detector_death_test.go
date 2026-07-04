package tmux

import (
	"os/exec"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// These tests cover the ce-5zbg fast-fail paths in WaitForClaudePrompt: when
// the harness process dies at startup (or the whole session disappears), the
// wait must fail within seconds with the pane's actual output, not burn the
// full 90s timeout while a dispatcher's shorter subprocess timeout SIGKILLs
// the spawn and makes it look like a hang.

// withFastFailThresholds tightens the package-level detection thresholds for
// the duration of a test so the tests don't take tens of seconds.
func withFastFailThresholds(t *testing.T, exited, neverStarted, gone int) {
	t.Helper()
	prevExited, prevNever, prevGone := harnessExitedChecks, harnessNeverStartedChecks, sessionGoneChecks
	harnessExitedChecks, harnessNeverStartedChecks, sessionGoneChecks = exited, neverStarted, gone
	t.Cleanup(func() {
		harnessExitedChecks, harnessNeverStartedChecks, sessionGoneChecks = prevExited, prevNever, prevGone
	})
}

func TestWaitForClaudePrompt_FailsFastWhenHarnessNeverStarts(t *testing.T) {
	skipIfNoTmux(t)
	socketPath, cleanup := setupTestSocket(t)
	defer cleanup()
	setupTestState(t)
	withFastFailThresholds(t, 3, 4, 6)

	sessionName := "test-harness-never-starts"
	require.NoError(t, exec.Command("tmux", "-S", socketPath,
		"new-session", "-d", "-s", sessionName, "-c", t.TempDir()).Run())
	defer killSession(sessionName)
	time.Sleep(500 * time.Millisecond)

	// Nothing is launched in the pane: the foreground stays a plain shell,
	// exactly like a harness command that dies instantly (Bun ENOENT).
	start := time.Now()
	err := WaitForClaudePrompt(sessionName, 30*time.Second)
	elapsed := time.Since(start)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "never started")
	assert.Less(t, elapsed, 15*time.Second,
		"fast-fail should trigger well before the 30s timeout (took %v)", elapsed)
}

func TestWaitForClaudePrompt_FailsFastWhenHarnessExits(t *testing.T) {
	skipIfNoTmux(t)
	socketPath, cleanup := setupTestSocket(t)
	defer cleanup()
	setupTestState(t)
	withFastFailThresholds(t, 2, 30, 6)

	sessionName := "test-harness-exits"
	require.NoError(t, exec.Command("tmux", "-S", socketPath,
		"new-session", "-d", "-s", sessionName, "-c", t.TempDir()).Run())
	defer killSession(sessionName)
	time.Sleep(500 * time.Millisecond)

	// Run a short-lived non-shell foreground process, mimicking a harness
	// that starts and then dies before ever rendering its prompt.
	require.NoError(t, exec.Command("tmux", "-S", socketPath,
		"send-keys", "-t", sessionName, "sleep 3", "Enter").Run())

	start := time.Now()
	err := WaitForClaudePrompt(sessionName, 30*time.Second)
	elapsed := time.Since(start)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "exited before becoming ready")
	assert.Less(t, elapsed, 15*time.Second,
		"fast-fail should trigger well before the 30s timeout (took %v)", elapsed)
}

func TestWaitForClaudePrompt_FailsFastWhenSessionDisappears(t *testing.T) {
	skipIfNoTmux(t)
	socketPath, cleanup := setupTestSocket(t)
	defer cleanup()
	setupTestState(t)
	withFastFailThresholds(t, 3, 30, 2)

	sessionName := "test-session-disappears"
	require.NoError(t, exec.Command("tmux", "-S", socketPath,
		"new-session", "-d", "-s", sessionName, "-c", t.TempDir()).Run())
	time.Sleep(500 * time.Millisecond)

	go func() {
		time.Sleep(1 * time.Second)
		_ = exec.Command("tmux", "-S", socketPath, "kill-session", "-t", sessionName).Run()
	}()

	start := time.Now()
	err := WaitForClaudePrompt(sessionName, 30*time.Second)
	elapsed := time.Since(start)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "no longer exists")
	assert.Less(t, elapsed, 15*time.Second,
		"fast-fail should trigger well before the 30s timeout (took %v)", elapsed)
}
