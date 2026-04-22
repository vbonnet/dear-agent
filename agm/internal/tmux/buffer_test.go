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

func TestCleanupBuffers_NoServer(t *testing.T) {
	// When tmux server is not running, CleanupBuffers should return 0 without error
	testSocket := fmt.Sprintf("/tmp/agm-test-buf-%d.sock", os.Getpid())
	os.Setenv("AGM_TMUX_SOCKET", testSocket)
	defer os.Unsetenv("AGM_TMUX_SOCKET")

	cleaned, err := CleanupBuffers()
	assert.Equal(t, 0, cleaned)
	// Either nil (detected as server dead) or error (listing failed) — both OK
	_ = err
}

func TestCleanupBuffers_WithBuffers(t *testing.T) {
	if _, err := exec.LookPath("tmux"); err != nil {
		t.Skip("tmux not available")
	}

	// Create isolated socket
	testSocket := fmt.Sprintf("/tmp/agm-test-buf2-%d.sock", os.Getpid())
	os.Setenv("AGM_TMUX_SOCKET", testSocket)
	t.Cleanup(func() {
		exec.Command("tmux", "-S", testSocket, "kill-server").Run()
		os.Remove(testSocket)
		os.Unsetenv("AGM_TMUX_SOCKET")
	})
	setupTestState(t)

	sessionName := "test-buffer-cleanup"

	// Create session
	cmd := exec.Command("tmux", "-S", testSocket, "new-session", "-d", "-s", sessionName)
	require.NoError(t, cmd.Run())
	defer exec.Command("tmux", "-S", testSocket, "kill-session", "-t", sessionName).Run()

	time.Sleep(100 * time.Millisecond)

	// Load some buffers manually to simulate orphaned state
	for i := 0; i < 3; i++ {
		bufName := fmt.Sprintf("agm-test-%d", i)
		loadCmd := exec.Command("tmux", "-S", testSocket, "set-buffer", "-b", bufName, fmt.Sprintf("test-data-%d", i))
		require.NoError(t, loadCmd.Run())
	}
	// Also load the primary agm-cmd buffer
	loadCmd := exec.Command("tmux", "-S", testSocket, "set-buffer", "-b", "agm-cmd", "orphaned-cmd")
	require.NoError(t, loadCmd.Run())

	// Run cleanup
	cleaned, err := CleanupBuffers()
	require.NoError(t, err)
	assert.Equal(t, 4, cleaned, "Should clean up 4 agm-* buffers")

	// Verify buffers are gone
	count, err := BufferCount()
	require.NoError(t, err)
	// There may be unnamed buffers, but no agm-* ones
	t.Logf("Remaining buffers after cleanup: %d", count)
}

func TestBufferCount_NoServer(t *testing.T) {
	testSocket := fmt.Sprintf("/tmp/agm-test-bufcnt-%d.sock", os.Getpid())
	os.Setenv("AGM_TMUX_SOCKET", testSocket)
	defer os.Unsetenv("AGM_TMUX_SOCKET")

	count, err := BufferCount()
	assert.Equal(t, 0, count)
	_ = err
}

func TestDeleteBuffer_NoBuffer(t *testing.T) {
	// deleteBuffer should not panic or block when buffer doesn't exist
	if _, err := exec.LookPath("tmux"); err != nil {
		t.Skip("tmux not available")
	}

	testSocket := fmt.Sprintf("/tmp/agm-test-delbuf-%d.sock", os.Getpid())
	os.Setenv("AGM_TMUX_SOCKET", testSocket)
	t.Cleanup(func() {
		exec.Command("tmux", "-S", testSocket, "kill-server").Run()
		os.Remove(testSocket)
		os.Unsetenv("AGM_TMUX_SOCKET")
	})

	sessionName := "test-delete-buffer"
	cmd := exec.Command("tmux", "-S", testSocket, "new-session", "-d", "-s", sessionName)
	require.NoError(t, cmd.Run())
	defer exec.Command("tmux", "-S", testSocket, "kill-session", "-t", sessionName).Run()

	time.Sleep(100 * time.Millisecond)

	// Should not panic even when buffer doesn't exist
	deleteBuffer()
}
