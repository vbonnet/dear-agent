package testcontext

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
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
	assert.Contains(t, tc.StateDir, tc.RunID)
	assert.Contains(t, tc.LockPath, tc.RunID)

	// Socket should be beneath the short per-user root (outside baseDir).
	assert.Equal(t, filepath.Join(canonicalEnvironmentRoot(), "agm-test-"+tc.RunID+".sock"), tc.SocketPath)

	// SessionsDir should be under baseDir
	assert.Equal(t, filepath.Join(tc.BaseDir, "sessions"), tc.SessionsDir)

	// DB and lock should be under baseDir
	assert.Equal(t, filepath.Join(tc.BaseDir, "agm.db"), tc.DBPath)
	assert.Equal(t, filepath.Join(tc.BaseDir, "state"), tc.StateDir)
	assert.Equal(t, filepath.Join(tc.BaseDir, "agm.lock"), tc.LockPath)
}

func TestCanonicalEnvironmentRootIsShortPrivateAndUserScoped(t *testing.T) {
	root := canonicalEnvironmentRoot()
	assert.Equal(t, filepath.Join("/tmp", "agm-u-"+strconv.Itoa(os.Geteuid())), root)
	assert.NotEqual(t, canonicalEnvironmentRootForUID(501), canonicalEnvironmentRootForUID(502))

	tc := New()
	require.NoError(t, tc.EnsureDirs())
	t.Cleanup(func() { require.NoError(t, tc.Cleanup()) })
	require.Less(t, len(tc.SocketPath), 100, "socket path must fit conservative Unix limits")

	info, err := os.Lstat(root)
	require.NoError(t, err)
	require.True(t, info.IsDir())
	assert.Equal(t, os.FileMode(0700), info.Mode().Perm())
	stat, ok := info.Sys().(*syscall.Stat_t)
	require.True(t, ok)
	assert.Equal(t, uint32(os.Geteuid()), stat.Uid)
}

func TestEnsureOwnedEnvironmentRootSecuresModeAndRejectsSymlink(t *testing.T) {
	root := filepath.Join(t.TempDir(), "owned-root")
	require.NoError(t, os.Mkdir(root, 0755))
	require.NoError(t, ensureOwnedEnvironmentRoot(root))
	info, err := os.Lstat(root)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0700), info.Mode().Perm())

	target := t.TempDir()
	symlink := filepath.Join(t.TempDir(), "root-link")
	require.NoError(t, os.Symlink(target, symlink))
	require.ErrorContains(t, ensureOwnedEnvironmentRoot(symlink), "not a directory")
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
	tc, err := NewNamed("my-test")
	require.NoError(t, err)

	assert.Equal(t, "my-test", tc.RunID)
	assert.Contains(t, tc.BaseDir, "agm-test-my-test")
	assert.Equal(t, filepath.Join(tc.BaseDir, "home"), tc.HomeDir)
	assert.Equal(t, filepath.Join(tc.BaseDir, "sessions"), tc.SessionsDir)
}

func TestLoadNamed(t *testing.T) {
	tc, err := LoadNamed("existing-env")
	require.NoError(t, err)

	assert.Equal(t, "existing-env", tc.RunID)
	assert.Contains(t, tc.BaseDir, "agm-test-existing-env")
	assert.Equal(t, filepath.Join(tc.BaseDir, "home"), tc.HomeDir)
}

func TestNamedEnvironmentRejectsUnownedPaths(t *testing.T) {
	t.Parallel()

	for _, name := range []string{
		"",
		"../escape",
		`..\escape`,
		"/absolute",
		"line\nbreak",
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if _, err := NewNamed(name); err == nil {
				t.Fatalf("NewNamed(%q) accepted an unsafe name", name)
			}
			if _, err := LoadNamed(name); err == nil {
				t.Fatalf("LoadNamed(%q) accepted an unsafe name", name)
			}
		})
	}
}

func TestNewNamedRejectsOverlongButLoadNamedRetainsCleanupAccess(t *testing.T) {
	t.Parallel()
	name := strings.Repeat("x", maxNewEnvironmentName+1)
	if _, err := NewNamed(name); err == nil {
		t.Fatalf("NewNamed accepted %d-byte name", len(name))
	}
	if _, err := LoadNamed(name); err != nil {
		t.Fatalf("LoadNamed rejected path-safe legacy name: %v", err)
	}
}

func TestListNamedSharesLifecycleRoot(t *testing.T) {
	name := "list-" + New().RunID
	tc, err := NewNamed(name)
	require.NoError(t, err)
	require.NoError(t, tc.EnsureDirs())
	t.Cleanup(func() { require.NoError(t, tc.Cleanup()) })

	reconstructed, err := LoadNamed(name)
	require.NoError(t, err)
	assert.Equal(t, tc.BaseDir, reconstructed.BaseDir)
	assert.Equal(t, tc.SocketPath, reconstructed.SocketPath)

	contexts, err := ListNamed()
	require.NoError(t, err)
	found := false
	for _, candidate := range contexts {
		if candidate.RunID == name {
			assert.Equal(t, tc.BaseDir, candidate.BaseDir)
			found = true
			break
		}
	}
	assert.True(t, found, "ListNamed did not return the created environment")

	require.NoError(t, tc.Cleanup())
	contexts, err = ListNamed()
	require.NoError(t, err)
	for _, candidate := range contexts {
		assert.NotEqual(t, name, candidate.RunID, "cleaned environment remained discoverable")
	}
}

func TestRetiredNamedEnvironmentIsDiscoveredAndCleanedExactly(t *testing.T) {
	t.Setenv("TMPDIR", t.TempDir())
	roots := namedEnvironmentRoots()
	require.Len(t, roots, 3)
	retiredRoot := roots[2]
	name := New().RunID + strings.Repeat("l", maxNewEnvironmentName)
	retired := newWithRoot(name, retiredRoot)
	require.NoError(t, retired.EnsureDirs())
	t.Cleanup(func() { require.NoError(t, retired.Cleanup()) })

	credentialTarget := t.TempDir()
	require.NoError(t, os.Symlink(credentialTarget, filepath.Join(retired.HomeDir, ".codex")))
	require.NoError(t, os.WriteFile(retired.SocketPath, []byte("retired socket"), 0600))

	sibling := newWithRoot("sibling-"+New().RunID, retiredRoot)
	require.NoError(t, sibling.EnsureDirs())
	require.NoError(t, os.WriteFile(filepath.Join(sibling.HomeDir, "preserve"), []byte("unrelated"), 0600))
	t.Cleanup(func() { require.NoError(t, sibling.Cleanup()) })

	contexts, err := ListNamed()
	require.NoError(t, err)
	found := false
	for _, candidate := range contexts {
		if candidate.RunID == name {
			assert.Equal(t, retired.BaseDir, candidate.BaseDir)
			found = true
			break
		}
	}
	require.True(t, found, "retired environment was not discoverable for cleanup")

	loaded, err := LoadNamed(name)
	require.NoError(t, err)
	_, err = NewNamed(name)
	require.ErrorContains(t, err, "must not exceed")
	assert.Equal(t, retired.BaseDir, loaded.BaseDir)
	assert.Equal(t, retired.SocketPath, loaded.SocketPath)
	assert.Equal(t, retired.SessionsDir, loaded.SessionsDir)
	require.NoError(t, loaded.Cleanup())

	for _, removed := range []string{retired.BaseDir, retired.SocketPath} {
		_, err := os.Lstat(removed)
		require.ErrorIs(t, err, os.ErrNotExist, "retired resource survived exact cleanup: %s", removed)
	}
	_, err = os.Stat(filepath.Join(sibling.HomeDir, "preserve"))
	require.NoError(t, err, "retired compatibility cleanup removed an unrelated sibling")
}

func TestLoadNamedResolvesGlobalShortRootBeforeCanonicalFallback(t *testing.T) {
	name := "global-" + New().RunID
	retired := newWithRoot(name, shortTestEnvironmentRoot)
	require.NoError(t, retired.EnsureDirs())
	t.Cleanup(func() { require.NoError(t, retired.Cleanup()) })

	loaded, err := LoadNamed(name)
	require.NoError(t, err)
	assert.Equal(t, retired.BaseDir, loaded.BaseDir)
	assert.Equal(t, retired.SocketPath, loaded.SocketPath)
	assert.NotEqual(t, filepath.Join(canonicalEnvironmentRoot(), testEnvironmentPrefix+name), loaded.BaseDir)
}

func TestListNamedPrefersCanonicalPerUserRootForDuplicate(t *testing.T) {
	name := "duplicate-" + New().RunID
	canonical, err := NewNamed(name)
	require.NoError(t, err)
	require.NoError(t, canonical.EnsureDirs())
	retired := newWithRoot(name, shortTestEnvironmentRoot)
	require.NoError(t, retired.EnsureDirs())
	t.Cleanup(func() {
		require.NoError(t, canonical.Cleanup())
		require.NoError(t, retired.Cleanup())
	})

	contexts, err := ListNamed()
	require.NoError(t, err)
	for _, candidate := range contexts {
		if candidate.RunID == name {
			assert.Equal(t, canonical.BaseDir, candidate.BaseDir)
			return
		}
	}
	t.Fatalf("duplicate named environment %q was not listed", name)
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
	assert.Equal(t, tc.StateDir, os.Getenv(EnvStateDir))
	assert.Equal(t, tc.LockPath, os.Getenv(EnvLockPath))
	assert.Equal(t, tc.HomeDir, os.Getenv("HOME"))

	// Reconstruct from env
	tc2, ok := FromEnv()
	require.True(t, ok, "FromEnv should succeed when env vars are set")
	assert.Equal(t, tc.RunID, tc2.RunID)
	assert.Equal(t, tc.SocketPath, tc2.SocketPath)
	assert.Equal(t, tc.SessionsDir, tc2.SessionsDir)
	assert.Equal(t, tc.DBPath, tc2.DBPath)
	assert.Equal(t, tc.StateDir, tc2.StateDir)
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

func TestFromEnvRejectsUnownedRunID(t *testing.T) {
	t.Setenv(EnvTestRunID, "../outside")
	t.Setenv(EnvTestEnv, "")
	t.Setenv(EnvSessionsDir, "")

	tc, ok := FromEnv()
	assert.False(t, ok)
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
	assert.Empty(t, os.Getenv(EnvStateDir))
	assert.Empty(t, os.Getenv(EnvLockPath))
}

func TestEnviron(t *testing.T) {
	tc := New()
	env := tc.Environ()

	assert.Len(t, env, 8, "should return all isolated paths, run markers, and HOME")

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
	assert.True(t, found[EnvStateDir])
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

	info, err = os.Stat(tc.StateDir)
	require.NoError(t, err)
	assert.True(t, info.IsDir())
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
