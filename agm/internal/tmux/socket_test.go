package tmux

import (
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetSocketPath(t *testing.T) {
	// Test default socket path
	os.Unsetenv("AGM_TMUX_SOCKET")
	path := GetSocketPath()
	assert.Equal(t, DefaultSocketPath(), path, "should return default socket path")

	// Test environment variable override
	customPath := "/tmp/test-agm.sock"
	t.Setenv("AGM_TMUX_SOCKET", customPath)
	defer os.Unsetenv("AGM_TMUX_SOCKET")

	path = GetSocketPath()
	assert.Equal(t, customPath, path, "should return custom socket path from env var")
}

func TestCleanStaleSocket_NoSocket(t *testing.T) {
	// Use a socket path that doesn't exist
	t.Setenv("AGM_TMUX_SOCKET", "/tmp/test-nonexistent.sock")
	defer os.Unsetenv("AGM_TMUX_SOCKET")

	err := CleanStaleSocket()
	assert.NoError(t, err, "should not error when socket doesn't exist")
}

func TestCleanStaleSocket_StaleSocket(t *testing.T) {
	// Create a temporary directory for test
	tmpDir := t.TempDir()
	socketPath := filepath.Join(tmpDir, "stale.sock")

	t.Setenv("AGM_TMUX_SOCKET", socketPath)
	defer os.Unsetenv("AGM_TMUX_SOCKET")

	// Create a stale socket file (not connected to any server)
	// We can't easily create a Unix socket without a server, so create a regular file
	// and let CleanStaleSocket try to connect and fail
	file, err := os.Create(socketPath)
	require.NoError(t, err)
	file.Close()

	// Note: Since the file is not actually a socket, CleanStaleSocket will fail
	// This test verifies error handling
	err = CleanStaleSocket()
	assert.Error(t, err, "should error when path is not a socket")
	assert.Contains(t, err.Error(), "not a socket", "error should mention it's not a socket")
}

func TestCleanStaleSocket_LiveSocket(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	// This test requires actually running a tmux server, which is complex in unit tests
	// We'll skip this test in CI environments
	if os.Getenv("CI") != "" {
		t.Skip("Skipping live socket test in CI")
	}

	// Create a temporary socket for testing
	tmpDir := t.TempDir()
	socketPath := filepath.Join(tmpDir, "live.sock")

	// Start a simple Unix socket server
	listener, err := net.Listen("unix", socketPath)
	if err != nil {
		t.Skipf("Cannot create Unix socket: %v", err)
	}
	defer listener.Close()

	// Accept connections in background
	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			conn.Close()
		}
	}()

	t.Setenv("AGM_TMUX_SOCKET", socketPath)
	defer os.Unsetenv("AGM_TMUX_SOCKET")

	// Clean should NOT remove live socket
	err = CleanStaleSocket()
	assert.NoError(t, err, "should not error for live socket")

	// Verify socket still exists
	_, err = os.Stat(socketPath)
	assert.NoError(t, err, "socket should still exist")
}

func TestEnsureSocketDir(t *testing.T) {
	tmpDir := t.TempDir()
	socketPath := filepath.Join(tmpDir, "subdir", "test.sock")

	t.Setenv("AGM_TMUX_SOCKET", socketPath)
	defer os.Unsetenv("AGM_TMUX_SOCKET")

	err := EnsureSocketDir()
	assert.NoError(t, err, "should create socket directory")

	// Verify directory was created
	dir := filepath.Dir(socketPath)
	info, err := os.Stat(dir)
	assert.NoError(t, err, "directory should exist")
	assert.True(t, info.IsDir(), "should be a directory")
}

func TestCheckSocketPermissions_NoSocket(t *testing.T) {
	t.Setenv("AGM_TMUX_SOCKET", "/tmp/test-nonexistent-perms.sock")
	defer os.Unsetenv("AGM_TMUX_SOCKET")

	err := CheckSocketPermissions()
	assert.NoError(t, err, "should not error when socket doesn't exist")
}

func TestCheckSocketPermissions_InsecureSocket(t *testing.T) {
	tmpDir := t.TempDir()
	socketPath := filepath.Join(tmpDir, "insecure.sock")

	// Create a socket-like file with world-writable permissions
	file, err := os.OpenFile(socketPath, os.O_CREATE, 0666)
	require.NoError(t, err)
	file.Close()

	t.Setenv("AGM_TMUX_SOCKET", socketPath)
	defer os.Unsetenv("AGM_TMUX_SOCKET")

	err = CheckSocketPermissions()
	assert.Error(t, err, "should error for insecure permissions")
	assert.Contains(t, err.Error(), "insecure permissions", "error should mention insecure permissions")
}

func TestGetSocketInfo_NonExistent(t *testing.T) {
	t.Setenv("AGM_TMUX_SOCKET", "/tmp/test-nonexistent-info.sock")
	defer os.Unsetenv("AGM_TMUX_SOCKET")

	info, err := GetSocketInfo()
	assert.NoError(t, err, "should not error for non-existent socket")
	assert.NotNil(t, info, "should return info struct")
	assert.False(t, info.Exists, "exists should be false")
	assert.False(t, info.IsSocket, "is_socket should be false")
}

func TestGetSocketInfo_ExistingFile(t *testing.T) {
	tmpDir := t.TempDir()
	socketPath := filepath.Join(tmpDir, "test.sock")

	// Create a regular file (not a socket)
	file, err := os.Create(socketPath)
	require.NoError(t, err)
	file.Close()

	t.Setenv("AGM_TMUX_SOCKET", socketPath)
	defer os.Unsetenv("AGM_TMUX_SOCKET")

	info, err := GetSocketInfo()
	assert.NoError(t, err, "should not error")
	assert.True(t, info.Exists, "exists should be true")
	assert.False(t, info.IsSocket, "is_socket should be false for regular file")
	assert.True(t, info.IsStale, "is_stale should be true (can't connect)")
	assert.False(t, info.Accessible, "accessible should be false")
}

func TestRemoveSocket(t *testing.T) {
	tmpDir := t.TempDir()
	socketPath := filepath.Join(tmpDir, "remove.sock")

	// Create a file to remove
	file, err := os.Create(socketPath)
	require.NoError(t, err)
	file.Close()

	t.Setenv("AGM_TMUX_SOCKET", socketPath)
	defer os.Unsetenv("AGM_TMUX_SOCKET")

	err = RemoveSocket()
	assert.NoError(t, err, "should not error when removing socket")

	// Verify socket was removed
	_, err = os.Stat(socketPath)
	assert.True(t, os.IsNotExist(err), "socket should be removed")
}

func TestRemoveSocket_NonExistent(t *testing.T) {
	t.Setenv("AGM_TMUX_SOCKET", "/tmp/test-nonexistent-remove.sock")
	defer os.Unsetenv("AGM_TMUX_SOCKET")

	err := RemoveSocket()
	assert.NoError(t, err, "should not error when socket doesn't exist")
}

func TestLockSocket(t *testing.T) {
	tmpDir := t.TempDir()
	socketPath := filepath.Join(tmpDir, "lock.sock")

	t.Setenv("AGM_TMUX_SOCKET", socketPath)
	defer os.Unsetenv("AGM_TMUX_SOCKET")

	// Acquire lock
	unlock, err := LockSocket()
	assert.NoError(t, err, "should acquire lock")
	assert.NotNil(t, unlock, "should return unlock function")

	// Try to acquire lock again (should fail)
	_, err = LockSocket()
	assert.Error(t, err, "should not acquire lock twice")
	assert.Contains(t, err.Error(), "held by another process", "error should mention lock is held")

	// Release lock
	err = unlock()
	assert.NoError(t, err, "should release lock")

	// Should be able to acquire lock again
	unlock2, err := LockSocket()
	assert.NoError(t, err, "should acquire lock after release")
	assert.NotNil(t, unlock2, "should return unlock function")
	unlock2()
}

func TestLockSocket_StaleLock(t *testing.T) {
	tmpDir := t.TempDir()
	socketPath := filepath.Join(tmpDir, "stale-lock.sock")
	lockPath := socketPath + ".lock"

	t.Setenv("AGM_TMUX_SOCKET", socketPath)
	defer os.Unsetenv("AGM_TMUX_SOCKET")

	// Create a stale lock file (old timestamp)
	file, err := os.Create(lockPath)
	require.NoError(t, err)
	file.Close()

	// Set old modification time
	oldTime := time.Now().Add(-10 * time.Second)
	err = os.Chtimes(lockPath, oldTime, oldTime)
	require.NoError(t, err)

	// Should be able to acquire lock (stale lock is removed)
	unlock, err := LockSocket()
	assert.NoError(t, err, "should acquire lock by removing stale lock")
	assert.NotNil(t, unlock, "should return unlock function")
	unlock()
}

func TestIsSocketInUse_NonExistent(t *testing.T) {
	t.Setenv("AGM_TMUX_SOCKET", "/tmp/test-nonexistent-inuse.sock")
	defer os.Unsetenv("AGM_TMUX_SOCKET")

	inUse, err := IsSocketInUse()
	assert.NoError(t, err, "should not error")
	assert.False(t, inUse, "should not be in use")
}

func TestWaitForSocket_Timeout(t *testing.T) {
	t.Setenv("AGM_TMUX_SOCKET", "/tmp/test-nonexistent-wait.sock")
	defer os.Unsetenv("AGM_TMUX_SOCKET")

	err := WaitForSocket(100 * time.Millisecond)
	assert.Error(t, err, "should timeout")
	assert.Contains(t, err.Error(), "timeout", "error should mention timeout")
}

func TestWaitForSocket_Success(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	if os.Getenv("CI") != "" {
		t.Skip("Skipping live socket test in CI")
	}

	tmpDir := t.TempDir()
	socketPath := filepath.Join(tmpDir, "wait.sock")

	t.Setenv("AGM_TMUX_SOCKET", socketPath)
	defer os.Unsetenv("AGM_TMUX_SOCKET")

	// Start socket server in background
	go func() {
		time.Sleep(50 * time.Millisecond)
		listener, err := net.Listen("unix", socketPath)
		if err != nil {
			return
		}
		defer listener.Close()
		time.Sleep(200 * time.Millisecond)
	}()

	err := WaitForSocket(500 * time.Millisecond)
	assert.NoError(t, err, "should not timeout")
}

func TestGetSocketOwner(t *testing.T) {
	tmpDir := t.TempDir()
	socketPath := filepath.Join(tmpDir, "owner.sock")

	// Create a file
	file, err := os.Create(socketPath)
	require.NoError(t, err)
	file.Close()

	t.Setenv("AGM_TMUX_SOCKET", socketPath)
	defer os.Unsetenv("AGM_TMUX_SOCKET")

	uid, err := GetSocketOwner()
	assert.NoError(t, err, "should not error")
	assert.Greater(t, uid, -1, "should return valid UID")
	assert.Equal(t, os.Getuid(), uid, "should match current user UID")
}

func TestWarnLegacySocket_NoLegacy(t *testing.T) {
	// Point to a custom socket so legacy path differs
	tmpDir := t.TempDir()
	t.Setenv("AGM_TMUX_SOCKET", filepath.Join(tmpDir, "agm.sock"))
	defer os.Unsetenv("AGM_TMUX_SOCKET")

	// Should not panic when legacy socket doesn't exist
	WarnLegacySocket()
}

func TestDefaultSocketPath_InHomeDir(t *testing.T) {
	home, err := os.UserHomeDir()
	require.NoError(t, err)

	path := DefaultSocketPath()
	assert.Equal(t, filepath.Join(home, ".agm", "agm.sock"), path)
	assert.True(t, filepath.IsAbs(path), "should be absolute")
}

func TestLegacySocketPath_Value(t *testing.T) {
	assert.Equal(t, "/tmp/agm.sock", LegacySocketPath)
}
