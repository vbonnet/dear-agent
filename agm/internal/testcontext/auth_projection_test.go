package testcontext

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"syscall"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/sys/unix"
)

func TestForwardAuthProjectionExactAllowlist(t *testing.T) {
	tc := newAuthProjectionContext(t)
	hostHome := t.TempDir()

	credentials := []struct {
		relativePath string
		data         string
	}{
		{filepath.Join(".claude", ".credentials.json"), "claude-auth"},
		{filepath.Join(".codex", "auth.json"), "codex-auth"},
		{filepath.Join(".local", "share", "opencode", "auth.json"), "opencode-auth"},
		{filepath.Join(".config", "gcloud", "application_default_credentials.json"), "gcloud-auth"},
	}
	snapshots := []struct {
		relativePath string
		data         string
	}{
		{filepath.Join(".config", "gcloud", "configurations", "config_default"), "gcloud-config"},
		{filepath.Join(".config", "opencode", "opencode.json"), "opencode-json"},
		{filepath.Join(".config", "opencode", "opencode.jsonc"), "opencode-jsonc"},
		{filepath.Join(".config", "opencode", "tui.json"), "tui-json"},
		{filepath.Join(".config", "opencode", "tui.jsonc"), "tui-jsonc"},
	}
	for _, fixture := range credentials {
		writeAuthProjectionFixture(t, hostHome, fixture.relativePath, fixture.data, 0600)
	}
	for _, fixture := range snapshots {
		writeAuthProjectionFixture(t, hostHome, fixture.relativePath, fixture.data, 0644)
	}

	forbidden := []struct {
		relativePath string
		data         string
	}{
		{filepath.Join(".claude", "projects", "session.json"), "host-claude-session"},
		{filepath.Join(".codex", "config.toml"), "host-codex-trust"},
		{filepath.Join(".config", "gcloud", "active_config"), "host-active-profile"},
		{filepath.Join(".config", "gcloud", "configurations", "config_work"), "host-work-profile"},
		{filepath.Join(".local", "share", "opencode", "storage", "session.db"), "host-opencode-db"},
	}
	for _, fixture := range forbidden {
		writeAuthProjectionFixture(t, hostHome, fixture.relativePath, fixture.data, 0600)
	}

	require.NoError(t, tc.ForwardAuth(hostHome, AuthModeInherit))
	assert.Equal(t, hostHome, tc.HostHome)

	for _, fixture := range credentials {
		destination := filepath.Join(tc.HomeDir, fixture.relativePath)
		target, err := os.Readlink(destination)
		require.NoError(t, err, fixture.relativePath)
		assert.Equal(t, filepath.Join(hostHome, fixture.relativePath), target)
		assertAuthProjectionDirectory(t, filepath.Dir(destination))
	}
	for _, fixture := range snapshots {
		destination := filepath.Join(tc.HomeDir, fixture.relativePath)
		data, err := os.ReadFile(destination)
		require.NoError(t, err, fixture.relativePath)
		assert.Equal(t, fixture.data, string(data))
		info, err := os.Lstat(destination)
		require.NoError(t, err)
		assert.True(t, info.Mode().IsRegular())
		assert.Equal(t, os.FileMode(0600), info.Mode().Perm())
	}

	rotated := credentials[0]
	replacement := filepath.Join(filepath.Dir(filepath.Join(hostHome, rotated.relativePath)), "replacement-auth")
	require.NoError(t, os.WriteFile(replacement, []byte("refreshed-auth"), 0600))
	require.NoError(t, os.Rename(replacement, filepath.Join(hostHome, rotated.relativePath)))
	data, err := os.ReadFile(filepath.Join(tc.HomeDir, rotated.relativePath))
	require.NoError(t, err)
	assert.Equal(t, "refreshed-auth", string(data), "credential link should observe host replacement")

	detached := snapshots[0]
	require.NoError(t, os.WriteFile(filepath.Join(hostHome, detached.relativePath), []byte("changed-host-config"), 0644))
	data, err = os.ReadFile(filepath.Join(tc.HomeDir, detached.relativePath))
	require.NoError(t, err)
	assert.Equal(t, detached.data, string(data), "configuration snapshot should not follow host writes")

	for _, fixture := range forbidden {
		_, err := os.Lstat(filepath.Join(tc.HomeDir, fixture.relativePath))
		require.ErrorIs(t, err, os.ErrNotExist, fixture.relativePath)
		data, err := os.ReadFile(filepath.Join(hostHome, fixture.relativePath))
		require.NoError(t, err)
		assert.Equal(t, fixture.data, string(data), fixture.relativePath)
	}
}

func TestForwardAuthProjectionSkipsMissingSources(t *testing.T) {
	tc := newAuthProjectionContext(t)
	hostHome := t.TempDir()

	require.NoError(t, tc.ForwardAuth(hostHome, AuthModeInherit))
	entries, err := os.ReadDir(tc.HomeDir)
	require.NoError(t, err)
	assert.Empty(t, entries)
}

func TestForwardAuthProjectionRejectsUnsafeSourcesBeforeMutation(t *testing.T) {
	tests := []struct {
		name  string
		setup func(*testing.T, string)
	}{
		{
			name: "credential symlink",
			setup: func(t *testing.T, hostHome string) {
				target := filepath.Join(t.TempDir(), "credential")
				require.NoError(t, os.WriteFile(target, []byte("do-not-leak-secret"), 0600))
				destination := filepath.Join(hostHome, ".claude", ".credentials.json")
				require.NoError(t, os.MkdirAll(filepath.Dir(destination), 0700))
				require.NoError(t, os.Symlink(target, destination))
			},
		},
		{
			name: "dangling credential symlink",
			setup: func(t *testing.T, hostHome string) {
				destination := filepath.Join(hostHome, ".codex", "auth.json")
				require.NoError(t, os.MkdirAll(filepath.Dir(destination), 0700))
				require.NoError(t, os.Symlink(filepath.Join(hostHome, "missing"), destination))
			},
		},
		{
			name: "credential directory",
			setup: func(t *testing.T, hostHome string) {
				require.NoError(t, os.MkdirAll(filepath.Join(hostHome, ".codex", "auth.json"), 0700))
			},
		},
		{
			name: "credential fifo",
			setup: func(t *testing.T, hostHome string) {
				path := filepath.Join(hostHome, ".claude", ".credentials.json")
				require.NoError(t, os.MkdirAll(filepath.Dir(path), 0700))
				require.NoError(t, unix.Mkfifo(path, 0600))
			},
		},
		{
			name: "credential readable by group",
			setup: func(t *testing.T, hostHome string) {
				writeAuthProjectionFixture(t, hostHome, filepath.Join(".codex", "auth.json"), "do-not-leak-secret", 0640)
			},
		},
		{
			name: "configuration writable by group",
			setup: func(t *testing.T, hostHome string) {
				path := writeAuthProjectionFixture(
					t, hostHome, filepath.Join(".config", "opencode", "opencode.json"), "do-not-leak-secret", 0600,
				)
				require.NoError(t, os.Chmod(path, 0620))
			},
		},
		{
			name: "redirected provider directory",
			setup: func(t *testing.T, hostHome string) {
				target := t.TempDir()
				require.NoError(t, os.WriteFile(filepath.Join(target, ".credentials.json"), []byte("do-not-leak-secret"), 0600))
				require.NoError(t, os.Symlink(target, filepath.Join(hostHome, ".claude")))
			},
		},
		{
			name: "writable provider directory",
			setup: func(t *testing.T, hostHome string) {
				path := writeAuthProjectionFixture(
					t, hostHome, filepath.Join(".claude", ".credentials.json"), "do-not-leak-secret", 0600,
				)
				require.NoError(t, os.Chmod(filepath.Dir(path), 0770))
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			tc := newAuthProjectionContext(t)
			hostHome := t.TempDir()
			test.setup(t, hostHome)

			err := tc.ForwardAuth(hostHome, AuthModeInherit)
			require.Error(t, err)
			assert.NotContains(t, err.Error(), "do-not-leak-secret")
			entries, readErr := os.ReadDir(tc.HomeDir)
			require.NoError(t, readErr)
			assert.Empty(t, entries, "source failure mutated the selected home")
		})
	}
}

func TestForwardAuthProjectionEnforcesSnapshotBound(t *testing.T) {
	t.Run("one mebibyte accepted", func(t *testing.T) {
		tc := newAuthProjectionContext(t)
		hostHome := t.TempDir()
		data := bytes.Repeat([]byte{'x'}, int(maxAuthConfigBytes))
		relativePath := filepath.Join(".config", "opencode", "tui.json")
		writeAuthProjectionFixture(t, hostHome, relativePath, string(data), 0600)

		require.NoError(t, tc.ForwardAuth(hostHome, AuthModeInherit))
		info, err := os.Stat(filepath.Join(tc.HomeDir, relativePath))
		require.NoError(t, err)
		assert.Equal(t, maxAuthConfigBytes, info.Size())
	})

	t.Run("one byte over rejected", func(t *testing.T) {
		tc := newAuthProjectionContext(t)
		hostHome := t.TempDir()
		data := bytes.Repeat([]byte{'x'}, int(maxAuthConfigBytes+1))
		writeAuthProjectionFixture(
			t,
			hostHome,
			filepath.Join(".config", "opencode", "tui.json"),
			string(data),
			0600,
		)

		err := tc.ForwardAuth(hostHome, AuthModeInherit)
		require.ErrorContains(t, err, "exceeds")
		entries, readErr := os.ReadDir(tc.HomeDir)
		require.NoError(t, readErr)
		assert.Empty(t, entries)
	})
}

func TestForwardAuthProjectionRejectsChangedSnapshotSource(t *testing.T) {
	hostHome := t.TempDir()
	selectedHome := privateTempDir(t)
	relativePath := filepath.Join(".config", "opencode", "opencode.json")
	source := writeAuthProjectionFixture(t, hostHome, relativePath, "original", 0600)
	sentinel := filepath.Join(selectedHome, "sentinel")
	require.NoError(t, os.WriteFile(sentinel, []byte("selected"), 0600))

	err := projectInheritedAuthWithHooks(hostHome, selectedHome, nil, func() error {
		replacement := filepath.Join(filepath.Dir(source), "replacement")
		require.NoError(t, os.WriteFile(replacement, []byte("replacement"), 0600))
		require.NoError(t, os.Rename(replacement, source))
		return nil
	})
	require.ErrorContains(t, err, "changed while reading")
	entries, readErr := os.ReadDir(selectedHome)
	require.NoError(t, readErr)
	require.Len(t, entries, 1)
	assert.Equal(t, "sentinel", entries[0].Name())
}

func TestForwardAuthProjectionRejectsUnsafeDestinationsBeforeMutation(t *testing.T) {
	t.Run("legacy provider namespace link", func(t *testing.T) {
		tc := newAuthProjectionContext(t)
		hostHome := t.TempDir()
		target := t.TempDir()
		sentinel := filepath.Join(target, "preserve")
		require.NoError(t, os.WriteFile(sentinel, []byte("outside"), 0600))
		require.NoError(t, os.Symlink(target, filepath.Join(tc.HomeDir, ".codex")))

		err := tc.ForwardAuth(hostHome, AuthModeInherit)
		require.ErrorContains(t, err, "destination directory .codex")
		data, readErr := os.ReadFile(sentinel)
		require.NoError(t, readErr)
		assert.Equal(t, "outside", string(data))
		_, statErr := os.Lstat(filepath.Join(tc.HomeDir, ".claude"))
		require.ErrorIs(t, statErr, os.ErrNotExist)
	})

	t.Run("late manifest leaf conflict", func(t *testing.T) {
		tc := newAuthProjectionContext(t)
		hostHome := t.TempDir()
		writeAuthProjectionFixture(
			t, hostHome, filepath.Join(".claude", ".credentials.json"), "claude-auth", 0600,
		)
		conflict := filepath.Join(tc.HomeDir, ".config", "opencode", "tui.jsonc")
		require.NoError(t, os.MkdirAll(filepath.Dir(conflict), 0700))
		require.NoError(t, os.WriteFile(conflict, []byte("selected"), 0600))

		err := tc.ForwardAuth(hostHome, AuthModeInherit)
		require.ErrorContains(t, err, "already exists")
		_, statErr := os.Lstat(filepath.Join(tc.HomeDir, ".claude"))
		require.ErrorIs(t, statErr, os.ErrNotExist, "preflight created an earlier namespace")
		data, readErr := os.ReadFile(conflict)
		require.NoError(t, readErr)
		assert.Equal(t, "selected", string(data))
	})

	t.Run("insecure existing namespace", func(t *testing.T) {
		tc := newAuthProjectionContext(t)
		hostHome := t.TempDir()
		path := filepath.Join(tc.HomeDir, ".config")
		require.NoError(t, os.Mkdir(path, 0700))
		require.NoError(t, os.Chmod(path, 0755))

		err := tc.ForwardAuth(hostHome, AuthModeInherit)
		require.ErrorContains(t, err, "mode 0700")
		info, statErr := os.Lstat(path)
		require.NoError(t, statErr)
		assert.Equal(t, os.FileMode(0755), info.Mode().Perm())
	})
}

func TestForwardAuthProjectionRejectsUnsafeRoots(t *testing.T) {
	t.Run("relative host home", func(t *testing.T) {
		err := projectInheritedAuth("relative", t.TempDir())
		require.ErrorContains(t, err, "canonical absolute")
	})

	t.Run("unclean host home", func(t *testing.T) {
		hostHome := t.TempDir()
		err := projectInheritedAuth(hostHome+string(os.PathSeparator)+".", privateTempDir(t))
		require.ErrorContains(t, err, "canonical absolute")
	})

	t.Run("host home file", func(t *testing.T) {
		hostHome := filepath.Join(t.TempDir(), "home")
		require.NoError(t, os.WriteFile(hostHome, []byte("not-dir"), 0600))
		err := projectInheritedAuth(hostHome, t.TempDir())
		require.ErrorContains(t, err, "real directory")
	})

	t.Run("host home symlink", func(t *testing.T) {
		target := t.TempDir()
		hostHome := filepath.Join(t.TempDir(), "home")
		require.NoError(t, os.Symlink(target, hostHome))
		err := projectInheritedAuth(hostHome, t.TempDir())
		require.ErrorContains(t, err, "real directory")
	})

	t.Run("selected home file", func(t *testing.T) {
		selectedHome := filepath.Join(t.TempDir(), "home")
		require.NoError(t, os.WriteFile(selectedHome, []byte("not-dir"), 0600))
		err := projectInheritedAuth(t.TempDir(), selectedHome)
		require.ErrorContains(t, err, "real directory")
	})

	t.Run("selected home symlink", func(t *testing.T) {
		target := privateTempDir(t)
		selectedHome := filepath.Join(t.TempDir(), "home")
		require.NoError(t, os.Symlink(target, selectedHome))
		err := projectInheritedAuth(t.TempDir(), selectedHome)
		require.ErrorContains(t, err, "real directory")
	})

	t.Run("selected home not private", func(t *testing.T) {
		selectedHome := t.TempDir()
		require.NoError(t, os.Chmod(selectedHome, 0755))
		err := projectInheritedAuth(t.TempDir(), selectedHome)
		require.ErrorContains(t, err, "mode 0700")
	})

	t.Run("same root", func(t *testing.T) {
		home := privateTempDir(t)
		err := projectInheritedAuth(home, home)
		require.ErrorContains(t, err, "distinct")
	})

	t.Run("selected home inside host home", func(t *testing.T) {
		hostHome := t.TempDir()
		selectedHome := filepath.Join(hostHome, "selected")
		require.NoError(t, os.Mkdir(selectedHome, 0700))
		err := projectInheritedAuth(hostHome, selectedHome)
		require.ErrorContains(t, err, "must not contain")
	})

	t.Run("host home inside selected home", func(t *testing.T) {
		selectedHome := privateTempDir(t)
		hostHome := filepath.Join(selectedHome, "host")
		require.NoError(t, os.Mkdir(hostHome, 0700))
		err := projectInheritedAuth(hostHome, selectedHome)
		require.ErrorContains(t, err, "must not contain")
	})
}

func TestForwardAuthProjectionRejectsWrongOwnerMetadata(t *testing.T) {
	directoryInfo, err := os.Lstat(privateTempDir(t))
	require.NoError(t, err)
	wrongDirectory := authProjectionFileInfoWithUID{
		FileInfo: directoryInfo,
		uid:      uint32(os.Geteuid() + 1), // #nosec G115 -- test deliberately models a different Unix owner.
	}
	require.ErrorContains(
		t,
		validateOwnedDirectoryInfo(wrongDirectory, true, "selected home"),
		"owned by the effective user",
	)

	leaf := writeAuthProjectionFixture(
		t,
		t.TempDir(),
		filepath.Join(".codex", "auth.json"),
		"synthetic-auth",
		0600,
	)
	leafInfo, err := os.Lstat(leaf)
	require.NoError(t, err)
	wrongLeaf := authProjectionFileInfoWithUID{
		FileInfo: leafInfo,
		uid:      uint32(os.Geteuid() + 1), // #nosec G115 -- test deliberately models a different Unix owner.
	}
	require.ErrorContains(
		t,
		validateAuthLeafInfo(wrongLeaf, true, filepath.Join(".codex", "auth.json")),
		"owned by the effective user",
	)
}

func TestForwardAuthNoOpModesDoNotValidateRoots(t *testing.T) {
	for _, mode := range []AuthMode{AuthModeEnv, AuthModeNone} {
		t.Run(string(mode), func(t *testing.T) {
			tc := &TestContext{HomeDir: "also-relative"}
			require.NoError(t, tc.ForwardAuth("relative-host", mode))
			assert.Equal(t, "relative-host", tc.HostHome)
		})
	}
}

func TestForwardAuthProjectionRollback(t *testing.T) {
	hostHome := t.TempDir()
	selectedHome := privateTempDir(t)
	writeAuthProjectionFixture(
		t, hostHome, filepath.Join(".claude", ".credentials.json"), "claude-auth", 0600,
	)
	writeAuthProjectionFixture(t, hostHome, filepath.Join(".codex", "auth.json"), "codex-auth", 0600)
	sentinel := filepath.Join(selectedHome, "preserve")
	require.NoError(t, os.WriteFile(sentinel, []byte("selected"), 0600))

	err := projectInheritedAuthWithHook(hostHome, selectedHome, func(relativePath string) error {
		if relativePath == filepath.Join(".codex", "auth.json") {
			return errors.New("injected apply failure")
		}
		return nil
	})
	require.ErrorContains(t, err, "injected apply failure")
	for _, relativePath := range []string{".claude", ".codex"} {
		_, statErr := os.Lstat(filepath.Join(selectedHome, relativePath))
		require.ErrorIs(t, statErr, os.ErrNotExist, relativePath)
	}
	data, readErr := os.ReadFile(sentinel)
	require.NoError(t, readErr)
	assert.Equal(t, "selected", string(data))
}

func TestForwardAuthProjectionRollbackPreservesPreexistingDirectory(t *testing.T) {
	hostHome := t.TempDir()
	selectedHome := privateTempDir(t)
	writeAuthProjectionFixture(
		t, hostHome, filepath.Join(".claude", ".credentials.json"), "claude-auth", 0600,
	)
	writeAuthProjectionFixture(t, hostHome, filepath.Join(".codex", "auth.json"), "codex-auth", 0600)
	preexisting := filepath.Join(selectedHome, ".claude")
	require.NoError(t, os.Mkdir(preexisting, 0700))
	preexistingInfo, err := os.Lstat(preexisting)
	require.NoError(t, err)

	err = projectInheritedAuthWithHook(hostHome, selectedHome, func(relativePath string) error {
		if relativePath == filepath.Join(".codex", "auth.json") {
			return errors.New("injected apply failure")
		}
		return nil
	})
	require.ErrorContains(t, err, "injected apply failure")

	currentInfo, statErr := os.Lstat(preexisting)
	require.NoError(t, statErr)
	assert.True(t, os.SameFile(preexistingInfo, currentInfo))
	entries, readErr := os.ReadDir(preexisting)
	require.NoError(t, readErr)
	assert.Empty(t, entries)
	_, statErr = os.Lstat(filepath.Join(selectedHome, ".codex"))
	require.ErrorIs(t, statErr, os.ErrNotExist)
}

func TestForwardAuthProjectionRollbackPreservesChangedNodes(t *testing.T) {
	hostHome := t.TempDir()
	selectedHome := privateTempDir(t)
	firstRelative := filepath.Join(".claude", ".credentials.json")
	writeAuthProjectionFixture(t, hostHome, firstRelative, "claude-auth", 0600)
	writeAuthProjectionFixture(t, hostHome, filepath.Join(".codex", "auth.json"), "codex-auth", 0600)
	replacement := filepath.Join(selectedHome, firstRelative)

	err := projectInheritedAuthWithHook(hostHome, selectedHome, func(relativePath string) error {
		if relativePath != filepath.Join(".codex", "auth.json") {
			return nil
		}
		require.NoError(t, os.Remove(replacement))
		require.NoError(t, os.WriteFile(replacement, []byte("replacement"), 0600))
		return errors.New("injected apply failure")
	})
	require.ErrorContains(t, err, "injected apply failure")
	assert.ErrorContains(t, err, "identity changed")
	data, readErr := os.ReadFile(replacement)
	require.NoError(t, readErr)
	assert.Equal(t, "replacement", string(data))
}

func TestForwardAuthProjectionReauthenticatesCredentialBeforeLink(t *testing.T) {
	hostHome := t.TempDir()
	selectedHome := privateTempDir(t)
	relativePath := filepath.Join(".claude", ".credentials.json")
	source := writeAuthProjectionFixture(t, hostHome, relativePath, "original", 0600)

	err := projectInheritedAuthWithHook(hostHome, selectedHome, func(string) error {
		replacement := filepath.Join(filepath.Dir(source), "replacement")
		require.NoError(t, os.WriteFile(replacement, []byte("rotated"), 0600))
		require.NoError(t, os.Rename(replacement, source))
		return nil
	})
	require.ErrorContains(t, err, "changed after preparation")
	entries, readErr := os.ReadDir(selectedHome)
	require.NoError(t, readErr)
	assert.Empty(t, entries)
}

func TestForwardAuthProjectionKeepsWritesInsideOpenedSelectedHome(t *testing.T) {
	hostHome := t.TempDir()
	writeAuthProjectionFixture(
		t, hostHome, filepath.Join(".claude", ".credentials.json"), "claude-auth", 0600,
	)

	root := t.TempDir()
	selectedHome := filepath.Join(root, "selected")
	movedHome := filepath.Join(root, "moved-selected")
	replacementHome := filepath.Join(root, "replacement")
	require.NoError(t, os.Mkdir(selectedHome, 0700))
	require.NoError(t, os.Mkdir(replacementHome, 0700))
	sentinel := filepath.Join(replacementHome, "sentinel")
	require.NoError(t, os.WriteFile(sentinel, []byte("outside"), 0600))

	err := projectInheritedAuthWithHook(hostHome, selectedHome, func(string) error {
		require.NoError(t, os.Rename(selectedHome, movedHome))
		require.NoError(t, os.Symlink(replacementHome, selectedHome))
		return nil
	})
	require.ErrorContains(t, err, "selected home must be a real directory")

	data, readErr := os.ReadFile(sentinel)
	require.NoError(t, readErr)
	assert.Equal(t, "outside", string(data))
	entries, readErr := os.ReadDir(replacementHome)
	require.NoError(t, readErr)
	require.Len(t, entries, 1)
	assert.Equal(t, "sentinel", entries[0].Name())
	_, statErr := os.Lstat(filepath.Join(movedHome, ".claude"))
	require.ErrorIs(t, statErr, os.ErrNotExist, "rollback should use the retained selected-home root")
}

func TestForwardAuthProjectionSerializesSelectedHomeTransactions(t *testing.T) {
	selectedHome := privateTempDir(t)
	info, err := os.Lstat(selectedHome)
	require.NoError(t, err)

	first, err := newAuthProjectionTransaction(selectedHome, info)
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, first.close())
	})

	_, err = newAuthProjectionTransaction(selectedHome, info)
	require.ErrorContains(t, err, "active auth projection")
}

func TestForwardAuthProjectionCleansUntrackedStagedNodes(t *testing.T) {
	parentPath := privateTempDir(t)
	parent, err := os.Open(parentPath)
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, parent.Close())
	})

	tests := []struct {
		name      string
		directory bool
		create    func(string) error
	}{
		{
			name:      "directory",
			directory: true,
			create: func(name string) error {
				return unix.Mkdirat(int(parent.Fd()), name, 0700)
			},
		},
		{
			name: "credential link",
			create: func(name string) error {
				return unix.Symlinkat("synthetic-target", int(parent.Fd()), name)
			},
		},
		{
			name: "configuration snapshot",
			create: func(name string) error {
				fd, openErr := unix.Openat(
					int(parent.Fd()),
					name,
					unix.O_WRONLY|unix.O_CREAT|unix.O_EXCL|unix.O_CLOEXEC|unix.O_NOFOLLOW,
					0600,
				)
				if openErr != nil {
					return openErr
				}
				return unix.Close(fd)
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			name := ".injected-pre-ledger-failure"
			require.NoError(t, test.create(name))
			err := cleanupUntrackedAuthNode(
				parent,
				name,
				test.directory,
				errors.New("injected post-create failure"),
			)
			require.ErrorContains(t, err, "injected post-create failure")
			_, statErr := os.Lstat(filepath.Join(parentPath, name))
			require.ErrorIs(t, statErr, os.ErrNotExist)
		})
	}
}

func newAuthProjectionContext(t *testing.T) *TestContext {
	t.Helper()
	tc := New()
	require.NoError(t, tc.EnsureDirs())
	t.Cleanup(func() {
		require.NoError(t, tc.Cleanup())
	})
	return tc
}

func privateTempDir(t *testing.T) string {
	t.Helper()
	path := t.TempDir()
	require.NoError(t, os.Chmod(path, 0700))
	return path
}

func writeAuthProjectionFixture(
	t *testing.T,
	home, relativePath, data string,
	mode os.FileMode,
) string {
	t.Helper()
	path := filepath.Join(home, relativePath)
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0700))
	require.NoError(t, os.WriteFile(path, []byte(data), mode))
	require.NoError(t, os.Chmod(path, mode))
	return path
}

func assertAuthProjectionDirectory(t *testing.T, path string) {
	t.Helper()
	info, err := os.Lstat(path)
	require.NoError(t, err)
	assert.True(t, info.IsDir())
	assert.Equal(t, os.FileMode(0700), info.Mode().Perm())
}

type authProjectionFileInfoWithUID struct {
	os.FileInfo
	uid uint32
}

func (info authProjectionFileInfoWithUID) Sys() any {
	stat := *info.FileInfo.Sys().(*syscall.Stat_t)
	stat.Uid = info.uid
	return &stat
}
