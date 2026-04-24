package tmux

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIsServerDeadError_Nil(t *testing.T) {
	assert.False(t, IsServerDeadError(nil))
}

func TestIsServerDeadError_TypedError(t *testing.T) {
	err := &ServerDeadError{Reason: "test"}
	assert.True(t, IsServerDeadError(err))
}

func TestIsServerDeadError_StringPatterns(t *testing.T) {
	tests := []struct {
		name   string
		err    error
		expect bool
	}{
		{"no server running", errors.New("no server running on /tmp/agm.sock"), true},
		{"connection refused", errors.New("connection refused"), true},
		{"error connecting to", errors.New("error connecting to /tmp/agm.sock"), true},
		{"broken pipe", errors.New("broken pipe"), true},
		{"no such file", errors.New("no such file or directory"), true},
		{"normal error", errors.New("session not found"), false},
		{"timeout error", errors.New("context deadline exceeded"), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expect, IsServerDeadError(tt.err))
		})
	}
}

func TestServerDeadError_Format(t *testing.T) {
	err := &ServerDeadError{
		Reason:   "socket unreachable",
		Recovery: "  rm -f /tmp/agm.sock",
	}

	errStr := err.Error()
	assert.Contains(t, errStr, "dead")
	assert.Contains(t, errStr, "socket unreachable")
	assert.Contains(t, errStr, "Recovery")
}

func TestServerAlive_WithServer(t *testing.T) {
	if _, err := exec.LookPath("tmux"); err != nil {
		t.Skip("tmux not available")
	}

	testSocket := fmt.Sprintf("/tmp/agm-test-alive-%d.sock", os.Getpid())
	os.Setenv("AGM_TMUX_SOCKET", testSocket)
	t.Cleanup(func() {
		exec.Command("tmux", "-S", testSocket, "kill-server").Run()
		os.Remove(testSocket)
		os.Unsetenv("AGM_TMUX_SOCKET")
	})

	// Create a session so server is running
	sessionName := "test-server-alive"
	cmd := exec.Command("tmux", "-S", testSocket, "new-session", "-d", "-s", sessionName)
	require.NoError(t, cmd.Run())
	defer exec.Command("tmux", "-S", testSocket, "kill-session", "-t", sessionName).Run()

	time.Sleep(200 * time.Millisecond)

	// Server should be alive
	err := ServerAlive()
	assert.NoError(t, err, "Server should be alive after creating session")
}

func TestServerAlive_NoServer(t *testing.T) {
	// Point to non-existent socket
	testSocket := fmt.Sprintf("/tmp/agm-test-dead-%d.sock", os.Getpid())
	os.Setenv("AGM_TMUX_SOCKET", testSocket)
	defer os.Unsetenv("AGM_TMUX_SOCKET")

	err := ServerAlive()
	assert.Error(t, err, "Server should be dead with no socket")
	assert.True(t, IsServerDeadError(err))
}

func TestServerAliveOrRecover_DeadServer(t *testing.T) {
	// Point to non-existent socket
	testSocket := fmt.Sprintf("/tmp/agm-test-recover-%d.sock", os.Getpid())
	os.Setenv("AGM_TMUX_SOCKET", testSocket)
	defer os.Unsetenv("AGM_TMUX_SOCKET")

	// Should succeed (nothing to clean up = recovery successful)
	err := ServerAliveOrRecover()
	assert.NoError(t, err, "Should recover from dead server with no stale socket")
}

func TestGetAdaptiveTimeout(t *testing.T) {
	// Reset semaphore for clean state
	SetMaxConcurrentOps(maxConcurrentTmuxOps)

	// Save and restore global timeout
	origTimeout := globalTimeout
	globalTimeout = 10 * time.Second
	defer func() { globalTimeout = origTimeout }()

	// With 0 concurrent ops, should return base timeout
	timeout := getAdaptiveTimeout()
	assert.Equal(t, 10*time.Second, timeout, "Base timeout with no concurrent ops")

	// Simulate concurrent operations by filling semaphore
	ctx := context.Background()
	for i := 0; i < 5; i++ {
		acquireTmuxSemaphore(ctx)
	}
	timeout = getAdaptiveTimeout()
	assert.Equal(t, 15*time.Second, timeout, "1.5x timeout with 5 concurrent ops")

	for i := 0; i < 5; i++ {
		acquireTmuxSemaphore(ctx)
	}
	timeout = getAdaptiveTimeout()
	assert.Equal(t, 20*time.Second, timeout, "2.0x timeout with 10 concurrent ops")

	// Release all
	for i := 0; i < 10; i++ {
		releaseTmuxSemaphore()
	}
}
