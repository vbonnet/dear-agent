//go:build darwin || linux

package messages

import (
	"crypto/sha256"
	"os"
	"path/filepath"
	"syscall"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type queueStorageDirectoryFingerprint struct {
	Mode   os.FileMode
	Inode  uint64
	SHA256 [sha256.Size]byte
}

func TestQueueStorageDirectoryChainCreatesPrivateChildren(t *testing.T) {
	homeDir := t.TempDir()
	storage, err := prepareMessageQueueStorage(homeDir)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, storage.Close()) })

	assert.Equal(t, os.FileMode(0o700), queueStorageDirectoryMode(t, filepath.Join(homeDir, ".config")).Perm())
	assert.Equal(t, os.FileMode(0o700), queueStorageDirectoryMode(t, filepath.Join(homeDir, ".config", "agm")).Perm())
}

func TestQueueStorageDirectoryChainRejectsSymlinksWithoutTouchingTargets(t *testing.T) {
	for _, boundary := range []string{"home", "config", "agm"} {
		t.Run(boundary, func(t *testing.T) {
			targetDir := t.TempDir()
			canaryPath := filepath.Join(targetDir, "target-canary")
			require.NoError(t, os.WriteFile(canaryPath, []byte("directory target must survive"), 0o640))
			before := queueStorageDirectoryFingerprintAt(t, canaryPath)

			var homeDir, linkPath string
			switch boundary {
			case "home":
				linkPath = filepath.Join(t.TempDir(), "home-link")
				require.NoError(t, os.Symlink(targetDir, linkPath))
				homeDir = linkPath
			case "config":
				homeDir = t.TempDir()
				linkPath = filepath.Join(homeDir, ".config")
				require.NoError(t, os.Symlink(targetDir, linkPath))
			case "agm":
				homeDir = t.TempDir()
				configDir := filepath.Join(homeDir, ".config")
				require.NoError(t, os.Mkdir(configDir, 0o700))
				linkPath = filepath.Join(configDir, "agm")
				require.NoError(t, os.Symlink(targetDir, linkPath))
			}

			storage, err := prepareMessageQueueStorage(homeDir)
			if storage != nil {
				require.NoError(t, storage.Close())
			}
			require.ErrorIs(t, err, ErrUnsafeQueueStorage)
			assert.NotContains(t, err.Error(), targetDir)
			assert.Equal(t, before, queueStorageDirectoryFingerprintAt(t, canaryPath))
			_, readlinkErr := os.Readlink(linkPath)
			require.NoError(t, readlinkErr)
		})
	}
}

func TestQueueStorageDirectoryChainRejectsBroadlyWritableComponents(t *testing.T) {
	t.Run("home", func(t *testing.T) {
		homeDir := t.TempDir()
		require.NoError(t, os.Chmod(homeDir, 0o777))
		t.Cleanup(func() { require.NoError(t, os.Chmod(homeDir, 0o700)) })
		before, err := os.Lstat(homeDir)
		require.NoError(t, err)

		storage, err := prepareMessageQueueStorage(homeDir)
		if storage != nil {
			require.NoError(t, storage.Close())
		}
		require.ErrorIs(t, err, ErrUnsafeQueueStorage)
		after, statErr := os.Lstat(homeDir)
		require.NoError(t, statErr)
		assert.Equal(t, os.FileMode(0o777), after.Mode().Perm())
		assert.Equal(t, queueStorageDirectoryInode(t, before), queueStorageDirectoryInode(t, after))
		_, statErr = os.Lstat(filepath.Join(homeDir, ".config"))
		require.ErrorIs(t, statErr, os.ErrNotExist)
	})

	t.Run("config", func(t *testing.T) {
		homeDir := t.TempDir()
		configDir := filepath.Join(homeDir, ".config")
		require.NoError(t, os.Mkdir(configDir, 0o700))
		require.NoError(t, os.Chmod(configDir, 0o777))
		before, err := os.Lstat(configDir)
		require.NoError(t, err)

		storage, err := prepareMessageQueueStorage(homeDir)
		if storage != nil {
			require.NoError(t, storage.Close())
		}
		require.ErrorIs(t, err, ErrUnsafeQueueStorage)
		after, statErr := os.Lstat(configDir)
		require.NoError(t, statErr)
		assert.Equal(t, os.FileMode(0o777), after.Mode().Perm())
		assert.Equal(t, queueStorageDirectoryInode(t, before), queueStorageDirectoryInode(t, after))
		_, statErr = os.Lstat(filepath.Join(configDir, "agm"))
		require.ErrorIs(t, statErr, os.ErrNotExist)
	})
}

func TestQueueStorageDirectoryChainRetainsSafeSharedConfigMode(t *testing.T) {
	homeDir := t.TempDir()
	configDir := filepath.Join(homeDir, ".config")
	require.NoError(t, os.Mkdir(configDir, 0o700))
	require.NoError(t, os.Chmod(configDir, 0o755))

	storage, err := prepareMessageQueueStorage(homeDir)
	require.NoError(t, err)
	defer func() { require.NoError(t, storage.Close()) }()

	assert.Equal(t, os.FileMode(0o755), queueStorageDirectoryMode(t, configDir).Perm())
	assert.Equal(t, os.FileMode(0o700), queueStorageDirectoryMode(t, filepath.Join(configDir, "agm")).Perm())
}

func TestQueueStorageDirectoryOwnerUsesExpectedUID(t *testing.T) {
	const expectedUID uint32 = 1000
	require.NoError(t, validateQueueStorageOwner("home directory", expectedUID, expectedUID))
	err := validateQueueStorageOwner("home directory", expectedUID+1, expectedUID)
	require.ErrorIs(t, err, ErrUnsafeQueueStorage)
}

func queueStorageDirectoryMode(t *testing.T, path string) os.FileMode {
	t.Helper()
	info, err := os.Lstat(path)
	require.NoError(t, err)
	return info.Mode()
}

func queueStorageDirectoryFingerprintAt(t *testing.T, path string) queueStorageDirectoryFingerprint {
	t.Helper()
	info, err := os.Lstat(path)
	require.NoError(t, err)
	contents, err := os.ReadFile(path)
	require.NoError(t, err)
	return queueStorageDirectoryFingerprint{
		Mode:   info.Mode(),
		Inode:  queueStorageDirectoryInode(t, info),
		SHA256: sha256.Sum256(contents),
	}
}

func queueStorageDirectoryInode(t *testing.T, info os.FileInfo) uint64 {
	t.Helper()
	stat, ok := info.Sys().(*syscall.Stat_t)
	require.True(t, ok)
	return stat.Ino
}
