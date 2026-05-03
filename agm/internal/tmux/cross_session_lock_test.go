package tmux

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestSendPromptLiteral_AcquiresLock verifies that SendPromptLiteral acquires
// the tmux server lock, preventing concurrent operations from interleaving.
//
// Bug fix (2026-04-02): Before this fix, SendPromptLiteral had no lock, allowing
// concurrent send-keys sequences to interleave at the tmux server level,
// causing cross-session byte leakage and spurious copy-mode activation.
func TestSendPromptLiteral_AcquiresLock(t *testing.T) {
	if _, err := exec.LookPath("tmux"); err != nil {
		t.Skip("tmux not available")
	}
	if os.Getenv("CI") != "" && os.Getenv("AGM_TEST_TMUX") == "" {
		t.Skip("Skipping tmux integration test in CI")
	}

	testSocket := fmt.Sprintf("/tmp/agm-spl-lock-%d.sock", os.Getpid())
	t.Setenv("AGM_TMUX_SOCKET", testSocket)
	t.Cleanup(func() {
		exec.Command("tmux", "-S", testSocket, "kill-server").Run()
		os.Remove(testSocket)
		os.Unsetenv("AGM_TMUX_SOCKET")
	})
	setupTestState(t)

	sessionName := "spl-lock-test"
	cmd := exec.Command("tmux", "-S", testSocket, "new-session", "-d", "-s", sessionName)
	require.NoError(t, cmd.Run())
	t.Cleanup(func() { exec.Command("tmux", "-S", testSocket, "kill-session", "-t", sessionName).Run() })
	time.Sleep(200 * time.Millisecond)

	// Pre-acquire the tmux lock to prove SendPromptLiteral tries to acquire it
	err := AcquireTmuxLock()
	require.NoError(t, err, "Should be able to acquire lock")
	defer ReleaseTmuxLock()

	// SendPromptLiteral should fail because the lock is already held
	err = SendPromptLiteral(sessionName, "test text", false)
	require.Error(t, err, "SendPromptLiteral should fail when lock is already held")
	assert.Contains(t, err.Error(), "tmux lock",
		"Error should mention tmux lock contention")
}

// TestSendCommandLiteral_AcquiresLock verifies that SendCommandLiteral acquires
// the tmux server lock.
func TestSendCommandLiteral_AcquiresLock(t *testing.T) {
	setupTestState(t)

	// Pre-acquire the tmux lock
	err := AcquireTmuxLock()
	require.NoError(t, err)
	defer ReleaseTmuxLock()

	// SendCommandLiteral should fail because the lock is already held
	err = SendCommandLiteral("any-session", "echo test")
	require.Error(t, err, "SendCommandLiteral should fail when lock is already held")
	assert.Contains(t, err.Error(), "tmux lock",
		"Error should mention tmux lock contention")
}

// TestSendKeys_AcquiresLock verifies that SendKeys acquires the tmux server lock.
func TestSendKeys_AcquiresLock(t *testing.T) {
	setupTestState(t)

	// Pre-acquire the tmux lock
	err := AcquireTmuxLock()
	require.NoError(t, err)
	defer ReleaseTmuxLock()

	// SendKeys should fail because the lock is already held
	err = SendKeys("any-session", "Space")
	require.Error(t, err, "SendKeys should fail when lock is already held")
	assert.Contains(t, err.Error(), "tmux lock",
		"Error should mention tmux lock contention")
}

// TestSendKeysToPane_AcquiresLock verifies that SendKeysToPane acquires the tmux server lock.
func TestSendKeysToPane_AcquiresLock(t *testing.T) {
	setupTestState(t)

	// Pre-acquire the tmux lock
	err := AcquireTmuxLock()
	require.NoError(t, err)
	defer ReleaseTmuxLock()

	// SendKeysToPane should fail because the lock is already held
	err = SendKeysToPane("any-session", "test")
	require.Error(t, err, "SendKeysToPane should fail when lock is already held")
	assert.Contains(t, err.Error(), "tmux lock",
		"Error should mention tmux lock contention")
}

// TestCrossSessionIsolation_Sequential verifies that sequential sends to
// different sessions do not contaminate each other's pane content.
func TestCrossSessionIsolation_Sequential(t *testing.T) {
	if _, err := exec.LookPath("tmux"); err != nil {
		t.Skip("tmux not available")
	}
	if os.Getenv("CI") != "" && os.Getenv("AGM_TEST_TMUX") == "" {
		t.Skip("Skipping tmux integration test in CI")
	}

	testSocket := fmt.Sprintf("/tmp/agm-isolation-%d.sock", os.Getpid())
	t.Setenv("AGM_TMUX_SOCKET", testSocket)
	t.Cleanup(func() {
		exec.Command("tmux", "-S", testSocket, "kill-server").Run()
		os.Remove(testSocket)
		os.Unsetenv("AGM_TMUX_SOCKET")
	})
	setupTestState(t)

	sessionA := "isolation-a"
	sessionB := "isolation-b"

	exec.Command("tmux", "-S", testSocket, "new-session", "-d", "-s", sessionA).Run()
	t.Cleanup(func() { exec.Command("tmux", "-S", testSocket, "kill-session", "-t", sessionA).Run() })
	exec.Command("tmux", "-S", testSocket, "new-session", "-d", "-s", sessionB).Run()
	t.Cleanup(func() { exec.Command("tmux", "-S", testSocket, "kill-session", "-t", sessionB).Run() })
	time.Sleep(200 * time.Millisecond)

	// Send distinct text to each session sequentially
	textA := "UNIQUE_ALPHA_TEXT"
	textB := "UNIQUE_BETA_TEXT"

	err := SendPromptLiteral(sessionA, textA, false)
	require.NoError(t, err, "Send to session A should succeed")

	err = SendPromptLiteral(sessionB, textB, false)
	require.NoError(t, err, "Send to session B should succeed")

	time.Sleep(300 * time.Millisecond)

	captureA := exec.Command("tmux", "-S", testSocket, "capture-pane", "-t", sessionA, "-p")
	outA, err := captureA.Output()
	require.NoError(t, err)

	captureB := exec.Command("tmux", "-S", testSocket, "capture-pane", "-t", sessionB, "-p")
	outB, err := captureB.Output()
	require.NoError(t, err)

	contentA := string(outA)
	contentB := string(outB)

	// Session A should have its text but NOT session B's text
	assert.Contains(t, contentA, textA, "Session A should contain its own text")
	assert.NotContains(t, contentA, textB, "Session A must NOT contain session B's text")

	// Session B should have its text but NOT session A's text
	assert.Contains(t, contentB, textB, "Session B should contain its own text")
	assert.NotContains(t, contentB, textA, "Session B must NOT contain session A's text")
}

// TestCrossSessionLocking_NoCopyMode verifies that rapid concurrent tmux
// operations on session A do not cause session B to enter copy-mode.
//
// This is the direct reproduction test for the reported bug: users get thrown
// into tmux copy-mode "(jump backward)" while typing in one session when AGM
// runs capture-pane or send-keys on a DIFFERENT session via the same socket.
func TestCrossSessionLocking_NoCopyMode(t *testing.T) {
	if _, err := exec.LookPath("tmux"); err != nil {
		t.Skip("tmux not available")
	}
	if os.Getenv("CI") != "" && os.Getenv("AGM_TEST_TMUX") == "" {
		t.Skip("Skipping tmux integration test in CI")
	}

	testSocket := fmt.Sprintf("/tmp/agm-copymode-test-%d.sock", os.Getpid())
	t.Setenv("AGM_TMUX_SOCKET", testSocket)
	t.Cleanup(func() {
		exec.Command("tmux", "-S", testSocket, "kill-server").Run()
		os.Remove(testSocket)
		os.Unsetenv("AGM_TMUX_SOCKET")
	})
	setupTestState(t)

	sessionA := "copymode-target"
	sessionB := "copymode-observer"

	cmdA := exec.Command("tmux", "-S", testSocket, "new-session", "-d", "-s", sessionA)
	require.NoError(t, cmdA.Run())
	t.Cleanup(func() { exec.Command("tmux", "-S", testSocket, "kill-session", "-t", sessionA).Run() })

	cmdB := exec.Command("tmux", "-S", testSocket, "new-session", "-d", "-s", sessionB)
	require.NoError(t, cmdB.Run())
	t.Cleanup(func() { exec.Command("tmux", "-S", testSocket, "kill-session", "-t", sessionB).Run() })

	time.Sleep(200 * time.Millisecond)

	// Hammer session A with rapid send operations while session B is idle.
	// With the lock, these serialize cleanly. Without it, interleaved bytes
	// could trigger copy-mode on session B.
	const iterations = 10
	for i := 0; i < iterations; i++ {
		text := fmt.Sprintf("echo iteration_%d", i)
		err := SendCommand(sessionA, text)
		// Ignore errors from lock contention — the key test is copy-mode on B
		_ = err
	}

	time.Sleep(500 * time.Millisecond)

	// Check if session B entered copy-mode (the bug symptom)
	checkCopyMode := exec.Command("tmux", "-S", testSocket, "display-message", "-t", sessionB, "-p", "#{pane_in_mode}")
	modeOut, err := checkCopyMode.Output()
	require.NoError(t, err)

	inMode := strings.TrimSpace(string(modeOut))
	assert.Equal(t, "0", inMode,
		"Session B should NOT be in copy-mode (pane_in_mode should be 0). "+
			"Got %q — indicates cross-session byte leakage triggered copy-mode", inMode)
}
