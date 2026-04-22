package testcontext

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNew(t *testing.T) {
	tc := New()

	// RunID should be 8 chars (short UUID)
	assert.Len(t, tc.RunID, 8, "RunID should be 8 characters")

	// All paths should contain the RunID
	assert.Contains(t, tc.BaseDir, tc.RunID)
	assert.Contains(t, tc.SocketPath, tc.RunID)
	assert.Contains(t, tc.SessionsDir, tc.RunID)
	assert.Contains(t, tc.DBPath, tc.RunID)
	assert.Contains(t, tc.LockPath, tc.RunID)

	// Socket should be at /tmp/agm-test-{id}.sock (outside baseDir)
	assert.Equal(t, filepath.Join(os.TempDir(), "agm-test-"+tc.RunID+".sock"), tc.SocketPath)

	// SessionsDir should be under baseDir
	assert.Equal(t, filepath.Join(tc.BaseDir, "sessions"), tc.SessionsDir)

	// DB and lock should be under baseDir
	assert.Equal(t, filepath.Join(tc.BaseDir, "agm.db"), tc.DBPath)
	assert.Equal(t, filepath.Join(tc.BaseDir, "agm.lock"), tc.LockPath)
}

func TestNew_HasHomeDir(t *testing.T) {
	tc := New()

	assert.Equal(t, filepath.Join(tc.BaseDir, "home"), tc.HomeDir)
	assert.Contains(t, tc.HomeDir, tc.RunID)
}

func TestNew_UniqueIDs(t *testing.T) {
	tc1 := New()
	tc2 := New()
	assert.NotEqual(t, tc1.RunID, tc2.RunID, "each call should produce a unique RunID")
	assert.NotEqual(t, tc1.BaseDir, tc2.BaseDir, "each call should produce a unique BaseDir")
}

func TestNewNamed(t *testing.T) {
	tc := NewNamed("my-test")

	assert.Equal(t, "my-test", tc.RunID)
	assert.Contains(t, tc.BaseDir, "agm-test-my-test")
	assert.Equal(t, filepath.Join(tc.BaseDir, "home"), tc.HomeDir)
	assert.Equal(t, filepath.Join(tc.BaseDir, "sessions"), tc.SessionsDir)
}

func TestLoadNamed(t *testing.T) {
	tc := LoadNamed("existing-env")

	assert.Equal(t, "existing-env", tc.RunID)
	assert.Contains(t, tc.BaseDir, "agm-test-existing-env")
	assert.Equal(t, filepath.Join(tc.BaseDir, "home"), tc.HomeDir)
}

func TestSetEnvAndFromEnv(t *testing.T) {
	tc := New()

	// Save and restore env
	defer tc.UnsetEnv()

	err := tc.SetEnv()
	require.NoError(t, err)

	// Verify env vars are set
	assert.Equal(t, tc.RunID, os.Getenv(EnvTestRunID))
	assert.Equal(t, tc.RunID, os.Getenv(EnvTestEnv))
	assert.Equal(t, tc.SocketPath, os.Getenv(EnvTmuxSocket))
	assert.Equal(t, tc.SessionsDir, os.Getenv(EnvSessionsDir))
	assert.Equal(t, tc.DBPath, os.Getenv(EnvDBPath))
	assert.Equal(t, tc.LockPath, os.Getenv(EnvLockPath))
	assert.Equal(t, tc.HomeDir, os.Getenv("HOME"))

	// Reconstruct from env
	tc2, ok := FromEnv()
	require.True(t, ok, "FromEnv should succeed when env vars are set")
	assert.Equal(t, tc.RunID, tc2.RunID)
	assert.Equal(t, tc.SocketPath, tc2.SocketPath)
	assert.Equal(t, tc.SessionsDir, tc2.SessionsDir)
	assert.Equal(t, tc.DBPath, tc2.DBPath)
	assert.Equal(t, tc.LockPath, tc2.LockPath)
	assert.Equal(t, tc.BaseDir, tc2.BaseDir)
	assert.Equal(t, tc.HomeDir, tc2.HomeDir)
}

func TestSetEnv_IncludesHomeAndTestEnv(t *testing.T) {
	tc := New()
	defer tc.UnsetEnv()

	err := tc.SetEnv()
	require.NoError(t, err)

	assert.Equal(t, tc.HomeDir, os.Getenv("HOME"), "HOME should be set to HomeDir")
	assert.Equal(t, tc.RunID, os.Getenv(EnvTestEnv), "AGM_TEST_ENV should be set")
}

func TestFromEnv_NotSet(t *testing.T) {
	// Ensure env is clean
	os.Unsetenv(EnvTestRunID)
	os.Unsetenv(EnvTestEnv)

	tc, ok := FromEnv()
	assert.False(t, ok, "FromEnv should return false when env vars not set")
	assert.Nil(t, tc)
}

func TestFromEnv_ViaTestEnv(t *testing.T) {
	// Test that FromEnv works with AGM_TEST_ENV even without AGM_TEST_RUN_ID
	os.Unsetenv(EnvTestRunID)
	t.Setenv(EnvTestEnv, "from-env-test")
	t.Setenv(EnvSessionsDir, filepath.Join(os.TempDir(), "agm-test-from-env-test", "sessions"))

	tc, ok := FromEnv()
	require.True(t, ok, "FromEnv should succeed with AGM_TEST_ENV set")
	assert.Equal(t, "from-env-test", tc.RunID)
	assert.Equal(t, filepath.Join(tc.BaseDir, "home"), tc.HomeDir)
}

func TestUnsetEnv(t *testing.T) {
	tc := New()
	require.NoError(t, tc.SetEnv())

	tc.UnsetEnv()

	assert.Empty(t, os.Getenv(EnvTestRunID))
	assert.Empty(t, os.Getenv(EnvTestEnv))
	assert.Empty(t, os.Getenv(EnvTmuxSocket))
	assert.Empty(t, os.Getenv(EnvSessionsDir))
	assert.Empty(t, os.Getenv(EnvDBPath))
	assert.Empty(t, os.Getenv(EnvLockPath))
}

func TestEnviron(t *testing.T) {
	tc := New()
	env := tc.Environ()

	assert.Len(t, env, 7, "should return 7 environment variables (5 original + AGM_TEST_ENV + HOME)")

	// Check each var is present as KEY=VALUE
	found := map[string]bool{}
	for _, e := range env {
		parts := strings.SplitN(e, "=", 2)
		require.Len(t, parts, 2, "each entry should be KEY=VALUE")
		found[parts[0]] = true
	}
	assert.True(t, found[EnvTestRunID])
	assert.True(t, found[EnvTestEnv])
	assert.True(t, found[EnvTmuxSocket])
	assert.True(t, found[EnvSessionsDir])
	assert.True(t, found[EnvDBPath])
	assert.True(t, found[EnvLockPath])
	assert.True(t, found["HOME"])
}

func TestEnsureDirs(t *testing.T) {
	tc := New()
	defer tc.Cleanup()

	err := tc.EnsureDirs()
	require.NoError(t, err)

	// BaseDir should exist
	info, err := os.Stat(tc.BaseDir)
	require.NoError(t, err)
	assert.True(t, info.IsDir())

	// SessionsDir should exist
	info, err = os.Stat(tc.SessionsDir)
	require.NoError(t, err)
	assert.True(t, info.IsDir())

	// HomeDir should exist
	info, err = os.Stat(tc.HomeDir)
	require.NoError(t, err)
	assert.True(t, info.IsDir())

	// Permissions should be 0700
	assert.Equal(t, os.FileMode(0700), info.Mode().Perm())
}

func TestEnsureDirs_Idempotent(t *testing.T) {
	tc := New()
	defer tc.Cleanup()

	require.NoError(t, tc.EnsureDirs())
	require.NoError(t, tc.EnsureDirs(), "second call should also succeed")
}

func TestCleanup(t *testing.T) {
	tc := New()

	// Create dirs and a fake socket file
	require.NoError(t, tc.EnsureDirs())

	// Create a placeholder file at the socket path
	f, err := os.Create(tc.SocketPath)
	require.NoError(t, err)
	f.Close()

	// Create a file in sessions dir to verify recursive removal
	testFile := filepath.Join(tc.SessionsDir, "test-session.yaml")
	require.NoError(t, os.WriteFile(testFile, []byte("test"), 0600))

	// Cleanup
	err = tc.Cleanup()
	require.NoError(t, err)

	// BaseDir should be gone
	_, err = os.Stat(tc.BaseDir)
	assert.True(t, os.IsNotExist(err), "baseDir should be removed")

	// Socket should be gone
	_, err = os.Stat(tc.SocketPath)
	assert.True(t, os.IsNotExist(err), "socket should be removed")
}

func TestCleanup_NoFilesExist(t *testing.T) {
	tc := New()

	// Cleanup should not error even if nothing exists
	err := tc.Cleanup()
	assert.NoError(t, err, "cleanup should succeed even with no files")
}

// --- ForwardAuth tests ---

func TestForwardAuth_Inherit(t *testing.T) {
	tc := New()
	defer tc.Cleanup()
	require.NoError(t, tc.EnsureDirs())

	// Create a fake host home with credential dirs
	fakeHome := t.TempDir()
	claudeDir := filepath.Join(fakeHome, ".claude")
	require.NoError(t, os.MkdirAll(claudeDir, 0700))
	require.NoError(t, os.WriteFile(filepath.Join(claudeDir, "settings.json"), []byte("{}"), 0600))

	codexDir := filepath.Join(fakeHome, ".codex")
	require.NoError(t, os.MkdirAll(codexDir, 0700))

	err := tc.ForwardAuth(fakeHome, AuthModeInherit)
	require.NoError(t, err)

	// Verify symlinks created
	link, err := os.Readlink(filepath.Join(tc.HomeDir, ".claude"))
	require.NoError(t, err)
	assert.Equal(t, claudeDir, link)

	link, err = os.Readlink(filepath.Join(tc.HomeDir, ".codex"))
	require.NoError(t, err)
	assert.Equal(t, codexDir, link)

	// HostHome should be set
	assert.Equal(t, fakeHome, tc.HostHome)
}

func TestForwardAuth_Inherit_MissingSource(t *testing.T) {
	tc := New()
	defer tc.Cleanup()
	require.NoError(t, tc.EnsureDirs())

	// Create a fake host home with NO credential dirs
	fakeHome := t.TempDir()

	err := tc.ForwardAuth(fakeHome, AuthModeInherit)
	require.NoError(t, err, "should succeed even with no credential dirs")

	// No symlinks should be created
	_, err = os.Readlink(filepath.Join(tc.HomeDir, ".claude"))
	assert.True(t, os.IsNotExist(err), "no .claude symlink should exist")
}

func TestForwardAuth_Env(t *testing.T) {
	tc := New()
	defer tc.Cleanup()
	require.NoError(t, tc.EnsureDirs())

	fakeHome := t.TempDir()
	// Create a credential dir that should NOT be symlinked in env mode
	require.NoError(t, os.MkdirAll(filepath.Join(fakeHome, ".claude"), 0700))

	err := tc.ForwardAuth(fakeHome, AuthModeEnv)
	require.NoError(t, err)

	// No symlinks should be created
	_, err = os.Readlink(filepath.Join(tc.HomeDir, ".claude"))
	assert.True(t, os.IsNotExist(err), "env mode should not create symlinks")
}

func TestForwardAuth_None(t *testing.T) {
	tc := New()
	defer tc.Cleanup()
	require.NoError(t, tc.EnsureDirs())

	fakeHome := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(fakeHome, ".claude"), 0700))

	err := tc.ForwardAuth(fakeHome, AuthModeNone)
	require.NoError(t, err)

	// No symlinks should be created
	_, err = os.Readlink(filepath.Join(tc.HomeDir, ".claude"))
	assert.True(t, os.IsNotExist(err), "none mode should not create symlinks")
}

func TestForwardAuth_InvalidMode(t *testing.T) {
	tc := New()
	defer tc.Cleanup()
	require.NoError(t, tc.EnsureDirs())

	err := tc.ForwardAuth(t.TempDir(), "bogus")
	assert.Error(t, err, "unknown auth mode should return error")
	assert.Contains(t, err.Error(), "unknown auth mode")
}

func TestForwardAuth_Inherit_NestedPath(t *testing.T) {
	tc := New()
	defer tc.Cleanup()
	require.NoError(t, tc.EnsureDirs())

	// Create a fake host home with .config/gcloud/ (nested path)
	fakeHome := t.TempDir()
	gcloudDir := filepath.Join(fakeHome, ".config", "gcloud")
	require.NoError(t, os.MkdirAll(gcloudDir, 0700))

	err := tc.ForwardAuth(fakeHome, AuthModeInherit)
	require.NoError(t, err)

	// Verify symlink at nested path
	link, err := os.Readlink(filepath.Join(tc.HomeDir, ".config", "gcloud"))
	require.NoError(t, err)
	assert.Equal(t, gcloudDir, link)
}
