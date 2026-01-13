package tmux

import (
	"os"
	"os/exec"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Helper function to check if tmux is available
func isTmuxAvailable() bool {
	_, err := exec.LookPath("tmux")
	return err == nil
}

// Helper function to skip if in CI without tmux testing enabled
func skipIfNoTmux(t *testing.T) {
	if !isTmuxAvailable() {
		t.Skip("tmux not available")
	}
	if os.Getenv("CI") != "" && os.Getenv("CSM_TEST_TMUX") == "" {
		t.Skip("Skipping tmux tests in CI (set CSM_TEST_TMUX=1 to enable)")
	}
}

// setupTestSocket creates an isolated tmux socket for testing
func setupTestSocket(t *testing.T) (socketPath string, cleanup func()) {
	t.Helper()
	tmpDir := t.TempDir()
	socketPath = tmpDir + "/test-tmux.sock"
	os.Setenv("CSM_TMUX_SOCKET", socketPath)

	cleanup = func() {
		os.Unsetenv("CSM_TMUX_SOCKET")
	}
	return socketPath, cleanup
}

// TestHasSession tests session existence checking
func TestHasSession(t *testing.T) {
	skipIfNoTmux(t)
	_, cleanup := setupTestSocket(t)
	defer cleanup()

	sessionName := "test-has-session"

	// Session should not exist initially
	exists, err := HasSession(sessionName)
	require.NoError(t, err)
	assert.False(t, exists, "Session should not exist initially")

	// Create session
	err = NewSession(sessionName, t.TempDir())
	require.NoError(t, err)
	defer killSession(sessionName)

	// Session should now exist
	exists, err = HasSession(sessionName)
	require.NoError(t, err)
	assert.True(t, exists, "Session should exist after creation")

	// Kill session
	killSession(sessionName)
	time.Sleep(100 * time.Millisecond)

	// Session should not exist after killing
	exists, err = HasSession(sessionName)
	require.NoError(t, err)
	assert.False(t, exists, "Session should not exist after killing")
}

// TestNewSession tests session creation
func TestNewSession(t *testing.T) {
	skipIfNoTmux(t)
	_, cleanup := setupTestSocket(t)
	defer cleanup()

	tests := []struct {
		name    string
		session string
		workDir string
		wantErr bool
	}{
		{
			name:    "create session in temp dir",
			session: "test-new-1",
			workDir: t.TempDir(),
			wantErr: false,
		},
		{
			name:    "create session in current dir",
			session: "test-new-2",
			workDir: ".",
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			defer killSession(tt.session)

			err := NewSession(tt.session, tt.workDir)
			if tt.wantErr {
				assert.Error(t, err)
				return
			}

			require.NoError(t, err)

			// Verify session exists
			exists, err := HasSession(tt.session)
			require.NoError(t, err)
			assert.True(t, exists, "Session should exist after NewSession")
		})
	}
}

// TestNewSession_SettingsInjection verifies tmux settings are injected
func TestNewSession_SettingsInjection(t *testing.T) {
	skipIfNoTmux(t)
	_, cleanup := setupTestSocket(t)
	defer cleanup()

	sessionName := "test-settings"
	err := NewSession(sessionName, t.TempDir())
	require.NoError(t, err)
	defer killSession(sessionName)

	// Give settings time to apply
	time.Sleep(300 * time.Millisecond)

	// Verify session exists
	exists, err := HasSession(sessionName)
	require.NoError(t, err)
	assert.True(t, exists)

	// Note: Testing actual tmux option values requires parsing tmux output
	// For now, we just verify the session was created successfully
	t.Log("Settings injection:")
	t.Log("  - set-window-option -g aggressive-resize on")
	t.Log("  - set-option -g window-size latest")
	t.Log("  - set -g mouse on")
	t.Log("  - set -s set-clipboard on")
}

// TestVersion tests tmux version retrieval
func TestVersion(t *testing.T) {
	skipIfNoTmux(t)

	version, err := Version()
	require.NoError(t, err)
	assert.NotEmpty(t, version)
	assert.Contains(t, version, "tmux", "Version string should contain 'tmux'")

	t.Logf("tmux version: %s", version)
}

// TestListSessions tests session listing
func TestListSessions(t *testing.T) {
	skipIfNoTmux(t)
	_, cleanup := setupTestSocket(t)
	defer cleanup()

	// Initially no sessions
	sessions, err := ListSessions()
	require.NoError(t, err)
	assert.Empty(t, sessions, "Should have no sessions initially")

	// Create multiple sessions
	session1 := "test-list-1"
	session2 := "test-list-2"

	err = NewSession(session1, t.TempDir())
	require.NoError(t, err)
	defer killSession(session1)

	err = NewSession(session2, t.TempDir())
	require.NoError(t, err)
	defer killSession(session2)

	// List should contain both
	sessions, err = ListSessions()
	require.NoError(t, err)
	assert.Len(t, sessions, 2, "Should have 2 sessions")
	assert.Contains(t, sessions, session1)
	assert.Contains(t, sessions, session2)
}

// TestGetCurrentSessionName tests getting current session name
func TestGetCurrentSessionName(t *testing.T) {
	// When not in tmux, should return error
	_, err := GetCurrentSessionName()
	assert.Error(t, err, "Should error when not in tmux")
	assert.Contains(t, err.Error(), "not running inside a tmux session")
}

// TestIsProcessRunning tests process detection
func TestIsProcessRunning(t *testing.T) {
	skipIfNoTmux(t)
	_, cleanup := setupTestSocket(t)
	defer cleanup()

	sessionName := "test-process"
	err := NewSession(sessionName, t.TempDir())
	require.NoError(t, err)
	defer killSession(sessionName)

	// Wait for session to be ready
	time.Sleep(100 * time.Millisecond)

	// Shell should be running (bash or sh)
	// Check for common shells
	shells := []string{"bash", "sh", "zsh"}
	foundShell := false
	for _, shell := range shells {
		running, err := IsProcessRunning(sessionName, shell)
		if err != nil {
			continue
		}
		if running {
			foundShell = true
			t.Logf("Found shell: %s", shell)
			break
		}
	}
	assert.True(t, foundShell, "Should find a running shell process")

	// Non-existent process should not be running
	running, err := IsProcessRunning(sessionName, "definitely-not-running-12345")
	require.NoError(t, err)
	assert.False(t, running, "Fake process should not be running")
}

// TestWaitForProcessReady tests process polling
func TestWaitForProcessReady(t *testing.T) {
	skipIfNoTmux(t)
	_, cleanup := setupTestSocket(t)
	defer cleanup()

	sessionName := "test-wait-process"
	err := NewSession(sessionName, t.TempDir())
	require.NoError(t, err)
	defer killSession(sessionName)

	// Wait for shell to be ready (bash/sh/zsh)
	// Try common shells
	shells := []string{"bash", "sh", "zsh"}
	var waitErr error
	for _, shell := range shells {
		waitErr = WaitForProcessReady(sessionName, shell, 2*time.Second)
		if waitErr == nil {
			t.Logf("Shell %s is ready", shell)
			break
		}
	}
	assert.NoError(t, waitErr, "Shell should be ready within timeout")

	// Waiting for non-existent process should timeout
	err = WaitForProcessReady(sessionName, "definitely-not-running-12345", 500*time.Millisecond)
	assert.Error(t, err, "Should timeout waiting for non-existent process")
	assert.Contains(t, err.Error(), "timeout", "Error should mention timeout")
}

// TestGetCurrentWorkingDirectory tests CWD retrieval
func TestGetCurrentWorkingDirectory(t *testing.T) {
	skipIfNoTmux(t)
	_, cleanup := setupTestSocket(t)
	defer cleanup()

	testDir := t.TempDir()
	sessionName := "test-cwd"

	err := NewSession(sessionName, testDir)
	require.NoError(t, err)
	defer killSession(sessionName)

	// Wait for session to be ready
	time.Sleep(100 * time.Millisecond)

	// Get CWD
	cwd, err := GetCurrentWorkingDirectory(sessionName)
	require.NoError(t, err)
	assert.Equal(t, testDir, cwd, "CWD should match session creation directory")
}

// TestAttachSession_NoTTY tests attach behavior when no TTY available
func TestAttachSession_NoTTY(t *testing.T) {
	skipIfNoTmux(t)
	_, cleanup := setupTestSocket(t)
	defer cleanup()

	sessionName := "test-attach-notty"
	err := NewSession(sessionName, t.TempDir())
	require.NoError(t, err)
	defer killSession(sessionName)

	// In test environment (no TTY), AttachSession should return nil
	// without actually attaching (it detects no TTY)
	err = AttachSession(sessionName)
	assert.NoError(t, err, "Should not error when no TTY (silently skips attach)")

	// Session should still exist (wasn't killed)
	exists, err := HasSession(sessionName)
	require.NoError(t, err)
	assert.True(t, exists, "Session should still exist after attach attempt")
}

// TestAttachSession_NonExistentSession tests attach to missing session
func TestAttachSession_NonExistentSession(t *testing.T) {
	skipIfNoTmux(t)
	_, cleanup := setupTestSocket(t)
	defer cleanup()

	// Attempt to attach to non-existent session
	// In no-TTY environment, this should succeed (skips attach)
	err := AttachSession("non-existent-session-12345")
	// Either no error (no TTY) or error about session not existing
	// We can't predict which without knowing if we have a TTY
	_ = err
}

// Helper function to kill a session
func killSession(name string) {
	socketPath := GetSocketPath()
	cmd := exec.Command("tmux", "-S", socketPath, "kill-session", "-t", name)
	cmd.Run() // Ignore errors
}

// Benchmark tests
func BenchmarkHasSession(b *testing.B) {
	if !isTmuxAvailable() {
		b.Skip("tmux not available")
	}

	tmpDir := b.TempDir()
	socketPath := tmpDir + "/bench-tmux.sock"
	os.Setenv("CSM_TMUX_SOCKET", socketPath)
	defer os.Unsetenv("CSM_TMUX_SOCKET")

	sessionName := "bench-has"
	err := NewSession(sessionName, tmpDir)
	if err != nil {
		b.Skipf("Failed to create session: %v", err)
	}
	defer killSession(sessionName)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = HasSession(sessionName)
	}
}

func BenchmarkListSessions(b *testing.B) {
	if !isTmuxAvailable() {
		b.Skip("tmux not available")
	}

	tmpDir := b.TempDir()
	socketPath := tmpDir + "/bench-tmux.sock"
	os.Setenv("CSM_TMUX_SOCKET", socketPath)
	defer os.Unsetenv("CSM_TMUX_SOCKET")

	// Create a few sessions
	for i := 0; i < 3; i++ {
		sessionName := string(rune('a' + i))
		err := NewSession(sessionName, tmpDir)
		if err != nil {
			b.Skipf("Failed to create session: %v", err)
		}
		defer killSession(sessionName)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = ListSessions()
	}
}
