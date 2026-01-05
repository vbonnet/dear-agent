package session

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	testerrors "github.com/vbonnet/ai-tools/claude-session-manager/internal/testutil/errors"
	"github.com/vbonnet/ai-tools/claude-session-manager/internal/testutil/tmux"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestManager_Create_Success(t *testing.T) {
	mockClient := tmux.NewMock()
	mockClient.HasSessionFunc = func(string) bool { return false }
	mockClient.WaitForStartupFunc = func(string, time.Duration) error {
		return nil
	}

	mgr := New(mockClient)

	tmpDir := filepath.Join(os.TempDir(), fmt.Sprintf("csm-test-%d", time.Now().UnixNano()))
	defer os.RemoveAll(tmpDir)

	opts := CreateOptions{
		Name:           "test-session",
		WorkingDir:     os.TempDir(),
		SessionsDir:    tmpDir,
		StartupTimeout: 30 * time.Second,
	}

	session, err := mgr.Create(opts)
	require.NoError(t, err)
	assert.NotNil(t, session)
	assert.Equal(t, "test-session", session.Name)
	assert.Equal(t, "csm-test-test-session", session.TmuxSession)
	assert.Equal(t, tmpDir, session.SessionsDir)
	assert.GreaterOrEqual(t, session.StartupTimeMs, int64(0))  // Mock returns instantly, so can be 0

	// Verify calls
	assert.Len(t, mockClient.CreateSessionCalls, 1)
	assert.Equal(t, "csm-test-test-session", mockClient.CreateSessionCalls[0].Name)
	assert.Len(t, mockClient.SendKeysCalls, 1)
	assert.Equal(t, "claude", mockClient.SendKeysCalls[0].Keys)
}

func TestManager_Create_InvalidName(t *testing.T) {
	mockClient := tmux.NewMock()
	mgr := New(mockClient)

	opts := CreateOptions{
		Name:        "test session",  // Space is invalid
		WorkingDir:  os.TempDir(),
		SessionsDir: os.TempDir(),
	}

	session, err := mgr.Create(opts)
	assert.Nil(t, session)
	assert.Error(t, err)

	var userErr *testerrors.UserError
	assert.ErrorAs(t, err, &userErr)
	assert.Equal(t, "invalid session name", userErr.Title)
}

func TestManager_Create_SessionCollision(t *testing.T) {
	mockClient := tmux.NewMock()
	mockClient.HasSessionFunc = func(string) bool { return true }  // Simulate existing session

	mgr := New(mockClient)

	opts := CreateOptions{
		Name:        "existing",
		WorkingDir:  os.TempDir(),
		SessionsDir: os.TempDir(),
	}

	session, err := mgr.Create(opts)
	assert.Nil(t, session)
	assert.Error(t, err)

	var userErr *testerrors.UserError
	assert.ErrorAs(t, err, &userErr)
	assert.Equal(t, "session name collision", userErr.Title)
}

func TestManager_Create_TmuxFailure(t *testing.T) {
	mockClient := tmux.NewMock()
	mockClient.HasSessionFunc = func(string) bool { return false }
	mockClient.CreateSessionFunc = func(string, string) error {
		return fmt.Errorf("tmux not found")
	}

	mgr := New(mockClient)

	tmpDir := filepath.Join(os.TempDir(), fmt.Sprintf("csm-test-%d", time.Now().UnixNano()))
	defer os.RemoveAll(tmpDir)

	opts := CreateOptions{
		Name:        "test",
		WorkingDir:  os.TempDir(),
		SessionsDir: tmpDir,
	}

	session, err := mgr.Create(opts)
	assert.Nil(t, session)
	assert.Error(t, err)

	var sysErr *testerrors.SystemError
	assert.ErrorAs(t, err, &sysErr)
	assert.Equal(t, "failed to create tmux session", sysErr.Title)

	// Verify cleanup happened (directory should not exist)
	_, statErr := os.Stat(tmpDir)
	assert.True(t, os.IsNotExist(statErr), "cleanup should have removed directory")
}

func TestManager_Create_StartupTimeout(t *testing.T) {
	mockClient := tmux.NewMock()
	mockClient.HasSessionFunc = func(string) bool { return false }
	mockClient.WaitForStartupFunc = func(string, time.Duration) error {
		return fmt.Errorf("timeout after 30s")
	}

	mgr := New(mockClient)

	tmpDir := filepath.Join(os.TempDir(), fmt.Sprintf("csm-test-%d", time.Now().UnixNano()))
	defer os.RemoveAll(tmpDir)

	opts := CreateOptions{
		Name:           "test",
		WorkingDir:     os.TempDir(),
		SessionsDir:    tmpDir,
		StartupTimeout: 1 * time.Second,
	}

	session, err := mgr.Create(opts)
	assert.Nil(t, session)
	assert.Error(t, err)

	var timeoutErr *testerrors.TimeoutError
	assert.ErrorAs(t, err, &timeoutErr)
	assert.Equal(t, "Claude startup timeout", timeoutErr.Title)
}

func TestManager_Send_Success(t *testing.T) {
	mockClient := tmux.NewMock()
	mockClient.HasSessionFunc = func(string) bool { return true }

	mgr := New(mockClient)

	opts := SendOptions{
		Command:      "git status",
		SessionsDir:  os.TempDir(),
		Autocomplete: false,
	}

	err := mgr.Send("test-session", opts)
	require.NoError(t, err)

	assert.Len(t, mockClient.SendKeysCalls, 1)
	assert.Equal(t, "csm-test-test-session", mockClient.SendKeysCalls[0].SessionName)
	assert.Equal(t, "git status", mockClient.SendKeysCalls[0].Keys)
}

func TestManager_Send_Autocomplete(t *testing.T) {
	mockClient := tmux.NewMock()
	mockClient.HasSessionFunc = func(string) bool { return true }

	mgr := New(mockClient)

	opts := SendOptions{
		Command:      "/commit",
		SessionsDir:  os.TempDir(),
		Autocomplete: true,
		Delay:        10 * time.Millisecond,
	}

	err := mgr.Send("test-session", opts)
	require.NoError(t, err)

	// Should send command + autocomplete enter
	assert.Len(t, mockClient.SendKeysCalls, 2)
	assert.Equal(t, "/commit", mockClient.SendKeysCalls[0].Keys)
	assert.Equal(t, "", mockClient.SendKeysCalls[1].Keys)  // Empty = just Enter
}

func TestManager_Send_SessionNotFound(t *testing.T) {
	mockClient := tmux.NewMock()
	mockClient.HasSessionFunc = func(string) bool { return false }

	mgr := New(mockClient)

	opts := SendOptions{
		Command:     "git status",
		SessionsDir: os.TempDir(),
	}

	err := mgr.Send("nonexistent", opts)
	assert.Error(t, err)

	var userErr *testerrors.UserError
	assert.ErrorAs(t, err, &userErr)
	assert.Equal(t, "session not found", userErr.Title)
}

func TestManager_Capture_Success(t *testing.T) {
	mockClient := tmux.NewMock()
	mockClient.HasSessionFunc = func(string) bool { return true }
	mockClient.CapturePaneFunc = func(string, int) (string, error) {
		return "line1\nline2\nline3\n", nil
	}

	mgr := New(mockClient)

	opts := CaptureOptions{
		SessionsDir: os.TempDir(),
		Lines:       10,
	}

	result, err := mgr.Capture("test-session", opts)
	require.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, 3, result.Count)
	assert.Equal(t, []string{"line1", "line2", "line3"}, result.Lines)
}

func TestManager_Capture_SessionNotFound(t *testing.T) {
	mockClient := tmux.NewMock()
	mockClient.HasSessionFunc = func(string) bool { return false }

	mgr := New(mockClient)

	opts := CaptureOptions{
		SessionsDir: os.TempDir(),
		Lines:       10,
	}

	result, err := mgr.Capture("nonexistent", opts)
	assert.Nil(t, result)
	assert.Error(t, err)

	var userErr *testerrors.UserError
	assert.ErrorAs(t, err, &userErr)
}

func TestManager_Cleanup_Success(t *testing.T) {
	mockClient := tmux.NewMock()
	mockClient.HasSessionFunc = func(string) bool { return true }

	mgr := New(mockClient)

	// Create temp directory to clean up
	tmpDir := filepath.Join(os.TempDir(), fmt.Sprintf("csm-test-%d", time.Now().UnixNano()))
	err := os.MkdirAll(tmpDir, 0755)
	require.NoError(t, err)

	opts := CleanupOptions{
		SessionsDir: tmpDir,
	}

	status, err := mgr.Cleanup("test-session", opts)
	require.NoError(t, err)
	assert.NotNil(t, status)
	assert.True(t, status.TmuxKilled)
	assert.True(t, status.CSMArchived)
	assert.True(t, status.DirectoryClean)

	// Verify directory was removed
	_, statErr := os.Stat(tmpDir)
	assert.True(t, os.IsNotExist(statErr))

	// Verify tmux session was killed
	assert.Len(t, mockClient.KillSessionCalls, 1)
	assert.Equal(t, "csm-test-test-session", mockClient.KillSessionCalls[0])
}

func TestManager_Cleanup_AlreadyClean(t *testing.T) {
	mockClient := tmux.NewMock()
	mockClient.HasSessionFunc = func(string) bool { return false }  // No session exists

	mgr := New(mockClient)

	opts := CleanupOptions{
		SessionsDir: "/nonexistent",
	}

	status, err := mgr.Cleanup("test-session", opts)
	require.NoError(t, err)
	assert.True(t, status.TmuxKilled)  // Marked as killed because it doesn't exist
	assert.True(t, status.DirectoryClean)  // Removing nonexistent dir succeeds
}

func TestIsValidSessionName(t *testing.T) {
	tests := []struct {
		name  string
		input string
		valid bool
	}{
		{"alphanumeric", "test123", true},
		{"with hyphens", "test-session-1", true},
		{"with underscores", "test_session_1", true},
		{"mixed", "my-test_123", true},
		{"empty", "", false},
		{"with spaces", "test session", false},
		{"with dots", "test.session", false},
		{"with slashes", "test/session", false},
		{"with special chars", "test@session", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isValidSessionName(tt.input)
			assert.Equal(t, tt.valid, result, "input: %s", tt.input)
		})
	}
}
