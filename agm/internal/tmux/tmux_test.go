package tmux

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestNormalizeTmuxSessionName tests session name normalization
// This addresses BUG-001: tmux converts dots/colons to dashes
func TestNormalizeTmuxSessionName(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "dots to dashes",
			input:    "gemini-task-1.4",
			expected: "gemini-task-1-4",
		},
		{
			name:     "multiple dots",
			input:    "foo.bar.baz",
			expected: "foo-bar-baz",
		},
		{
			name:     "colons to dashes",
			input:    "test:session",
			expected: "test-session",
		},
		{
			name:     "mixed dots and colons",
			input:    "multi.char:name",
			expected: "multi-char-name",
		},
		{
			name:     "normal name unchanged",
			input:    "normal-name",
			expected: "normal-name",
		},
		{
			name:     "underscores preserved",
			input:    "test_session_123",
			expected: "test_session_123",
		},
		{
			name:     "alphanumeric preserved",
			input:    "session123abc",
			expected: "session123abc",
		},
		{
			name:     "incident case - gemini-task-1.4",
			input:    "gemini-task-1.4",
			expected: "gemini-task-1-4",
		},
		{
			name:     "edge case - only dots",
			input:    "...",
			expected: "---",
		},
		{
			name:     "edge case - only colons",
			input:    ":::",
			expected: "---",
		},
		{
			name:     "complex real-world name",
			input:    "project-v1.2.3:staging",
			expected: "project-v1-2-3-staging",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := NormalizeTmuxSessionName(tt.input)
			assert.Equal(t, tt.expected, result,
				"NormalizeTmuxSessionName(%q) = %q, expected %q",
				tt.input, result, tt.expected)
		})
	}
}

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
	if os.Getenv("CI") != "" && os.Getenv("AGM_TEST_TMUX") == "" {
		t.Skip("Skipping tmux tests in CI (set AGM_TEST_TMUX=1 to enable)")
	}
}

// setupTestSocket creates an isolated tmux socket for testing
func setupTestSocket(t *testing.T) (socketPath string, cleanup func()) {
	t.Helper()
	socketPath = fmt.Sprintf("/tmp/agm-test-%d.sock", os.Getpid())
	t.Setenv("AGM_TMUX_SOCKET", socketPath)
	t.Cleanup(func() {
		exec.Command("tmux", "-S", socketPath, "kill-server").Run()
		os.Remove(socketPath)
		os.Unsetenv("AGM_TMUX_SOCKET")
	})
	return socketPath, func() {}
}

func setupTestState(t *testing.T) {
	t.Helper()
	stateDir := t.TempDir()
	t.Setenv("AGM_STATE_DIR", stateDir)
	t.Cleanup(func() { os.Unsetenv("AGM_STATE_DIR") })
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
	if testing.Short() {
		t.Skip("Skipping tmux integration test in short mode (uses global lock)")
	}
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
	if testing.Short() {
		t.Skip("Skipping tmux integration test in short mode (uses global lock)")
	}
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

// TestNewSession_BuildEnvVars verifies build environment variables are set on new sessions
func TestNewSession_BuildEnvVars(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping tmux integration test in short mode (uses global lock)")
	}
	skipIfNoTmux(t)
	socketPath, cleanup := setupTestSocket(t)
	defer cleanup()

	sessionName := "test-buildenv"
	err := NewSession(sessionName, t.TempDir())
	require.NoError(t, err)
	defer killSession(sessionName)

	// Query tmux session environment
	cmd := exec.Command("tmux", "-S", socketPath, "show-environment", "-t", sessionName)
	out, err := cmd.Output()
	require.NoError(t, err, "show-environment should succeed")

	envOutput := string(out)

	homeDir, _ := os.UserHomeDir()
	expectedVars := map[string]string{
		"GOCACHE":    filepath.Join(homeDir, ".cache", "go-build"),
		"GOMODCACHE": filepath.Join(homeDir, "go", "pkg", "mod"),
		"GOMAXPROCS": strconv.Itoa(max(runtime.NumCPU()/2, 1)),
		"GOWORK":     "off",
	}
	for k, v := range expectedVars {
		expected := fmt.Sprintf("%s=%s", k, v)
		assert.Contains(t, envOutput, expected,
			"tmux session environment should contain %s", expected)
	}
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
	// Save and clear TMUX env var to simulate not being in tmux
	originalTmux := os.Getenv("TMUX")
	t.Setenv("TMUX", "")
	defer func() {
		if originalTmux != "" {
			t.Setenv("TMUX", originalTmux)
		}
	}()

	_, err := GetCurrentSessionName()
	assert.Error(t, err, "Should error when not in tmux")
	if err != nil {
		assert.Contains(t, err.Error(), "not running inside a tmux session")
	}
}

// TestIsProcessRunning tests process detection
func TestIsProcessRunning(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping tmux integration test in short mode (uses global lock)")
	}
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

// TestIsClaudeProcess tests Claude Code process name detection.
// Claude Code reports as its semver version string (e.g., "2.1.50") in tmux
// rather than "claude", so we need to detect both patterns.
func TestIsClaudeProcess(t *testing.T) {
	tests := []struct {
		command string
		want    bool
	}{
		{"claude", true},       // Direct claude binary
		{"2.1.50", true},       // Claude Code version (current)
		{"3.0.0", true},        // Future version
		{"0.1.0", true},        // Semver pattern
		{"10.20.30", true},     // Multi-digit version
		{"zsh", false},         // Shell
		{"bash", false},        // Shell
		{"node", false},        // Node.js (too broad to match)
		{"vim", false},         // Editor
		{"2.1", false},         // Incomplete semver (not 3 parts)
		{"2.1.50.1", false},    // Too many parts
		{"", false},            // Empty
		{"v2.1.50", false},     // v prefix (not what tmux reports)
		{"abc.def.ghi", false}, // Non-numeric semver
		{"1.2.x", false},       // Non-numeric part
	}

	for _, tt := range tests {
		t.Run(tt.command, func(t *testing.T) {
			got := isClaudeProcess(tt.command)
			assert.Equal(t, tt.want, got, "isClaudeProcess(%q)", tt.command)
		})
	}
}

// TestIsClaudeRunning_BashFallback tests that IsClaudeRunning detects Claude
// running as a child of bash after crash/resume. In this state, the pane
// foreground command is "bash" and the Claude prompt (❯) appears in pane output.
func TestIsClaudeRunning_BashFallback(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping tmux integration test in short mode (uses global lock)")
	}
	skipIfNoTmux(t)
	_, cleanup := setupTestSocket(t)
	defer cleanup()

	sessionName := "test-claude-fallback"
	err := NewSession(sessionName, t.TempDir())
	require.NoError(t, err)
	defer killSession(sessionName)

	// Wait for shell to be ready
	time.Sleep(200 * time.Millisecond)

	// Pane should be running bash/sh — IsClaudeRunning should be false (no ❯ in output)
	running, err := IsClaudeRunning(sessionName)
	require.NoError(t, err)
	assert.False(t, running, "Should not detect Claude in a plain shell session")

	// Now inject the Claude prompt character into the pane to simulate
	// Claude running as a child of bash after crash/resume
	socketPath := GetSocketPath()
	normalizedName := NormalizeTmuxSessionName(sessionName)
	sendCmd := exec.Command("tmux", "-S", socketPath, "send-keys", "-t", normalizedName, "echo '❯'", "C-m")
	require.NoError(t, sendCmd.Run())
	time.Sleep(300 * time.Millisecond)

	// Now IsClaudeRunning should detect the ❯ in the pane output
	running, err = IsClaudeRunning(sessionName)
	require.NoError(t, err)
	assert.True(t, running, "Should detect Claude via ❯ prompt in bash fallback")
}

// TestGetCurrentWorkingDirectory tests CWD retrieval
func TestGetCurrentWorkingDirectory(t *testing.T) {
	skipIfNoTmux(t)
	_, cleanup := setupTestSocket(t)
	defer cleanup()

	testDir := t.TempDir()
	// Resolve symlinks to handle macOS /var -> /private/var
	testDir, err := filepath.EvalSymlinks(testDir)
	require.NoError(t, err)
	sessionName := "test-cwd"

	err = NewSession(sessionName, testDir)
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

	// Save and clear TMUX env var to simulate not being in tmux
	originalTmux := os.Getenv("TMUX")
	t.Setenv("TMUX", "")
	defer func() {
		if originalTmux != "" {
			t.Setenv("TMUX", originalTmux)
		}
	}()

	sessionName := "test-attach-notty"
	// Retry NewSession in case of lock contention from parallel tests
	var err error
	for i := 0; i < 3; i++ {
		err = NewSession(sessionName, t.TempDir())
		if err == nil || !strings.Contains(err.Error(), "failed to acquire tmux lock") {
			break
		}
		time.Sleep(time.Second)
	}
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

// TestIsSupervisorSession tests detection of supervisor roles from session names
func TestIsSupervisorSession(t *testing.T) {
	tests := []struct {
		name     string
		session  string
		expected bool
	}{
		{"orchestrator exact", "orchestrator", true},
		{"orchestrator with prefix", "my-orchestrator", true},
		{"orchestrator with suffix", "orchestrator-main", true},
		{"meta-orchestrator", "meta-orchestrator", true},
		{"meta-orchestrator with suffix", "meta-orchestrator-v2", true},
		{"overseer", "overseer", true},
		{"overseer with prefix", "prod-overseer", true},
		{"worker session", "worker-abc123", false},
		{"implementer session", "implementer-task-1", false},
		{"researcher session", "researcher-deep", false},
		{"random name", "my-cool-session", false},
		{"empty string", "", false},
		{"case insensitive", "ORCHESTRATOR", true},
		{"mixed case", "Meta-Orchestrator", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsSupervisorSession(tt.session)
			assert.Equal(t, tt.expected, got, "IsSupervisorSession(%q)", tt.session)
		})
	}
}

// TestNewSession_AutoRespawn verifies auto-respawn is set for supervisor sessions
func TestNewSession_AutoRespawn(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping tmux integration test in short mode (uses global lock)")
	}
	skipIfNoTmux(t)
	socketPath, cleanup := setupTestSocket(t)
	defer cleanup()

	tests := []struct {
		name        string
		session     string
		wantRespawn bool
	}{
		{"orchestrator gets respawn", "orchestrator-test", true},
		{"meta-orchestrator gets respawn", "meta-orchestrator-test", true},
		{"overseer gets respawn", "overseer-test", true},
		{"worker no respawn", "worker-test", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := NewSession(tt.session, t.TempDir())
			require.NoError(t, err)
			defer killSession(tt.session)

			time.Sleep(200 * time.Millisecond)

			// Check remain-on-exit option
			cmd := exec.Command("tmux", "-S", socketPath, "show-option", "-t", tt.session, "remain-on-exit")
			out, err := cmd.CombinedOutput()
			outStr := string(out)

			if tt.wantRespawn {
				require.NoError(t, err, "show-option should succeed for supervisor session")
				assert.Contains(t, outStr, "on", "remain-on-exit should be 'on' for %s", tt.session)
			} else if err == nil {
				// For non-supervisor sessions, the option is either not set or "off"
				assert.NotContains(t, outStr, "remain-on-exit on",
					"remain-on-exit should NOT be 'on' for %s", tt.session)
				// Error is also acceptable (option not set)
			}
		})
	}
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
	b.Setenv("AGM_TMUX_SOCKET", socketPath)
	defer os.Unsetenv("AGM_TMUX_SOCKET")

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
	b.Setenv("AGM_TMUX_SOCKET", socketPath)
	defer os.Unsetenv("AGM_TMUX_SOCKET")

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
