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
	"golang.org/x/sys/unix"
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

func TestQueueStorageExistingLeafSetRepairsOnlyAfterCompleteAdmission(t *testing.T) {
	homeDir := t.TempDir()
	storage, err := prepareMessageQueueStorage(homeDir)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, storage.Close()) })

	dbPath := storage.databasePath()
	walPath := dbPath + "-wal"
	require.NoError(t, os.WriteFile(dbPath, []byte("main database bytes"), 0o644))
	require.NoError(t, os.WriteFile(walPath, []byte("WAL bytes"), 0o644))
	require.NoError(t, os.Chmod(storage.rootPath, 0o755))
	require.NoError(t, os.Chmod(dbPath, 0o644))
	require.NoError(t, os.Chmod(walPath, 0o644))
	dbBefore := queueStorageDirectoryFingerprintAt(t, dbPath)
	walBefore := queueStorageDirectoryFingerprintAt(t, walPath)

	leaves, mainPresent, retry, err := storage.classifyQueueStorageLeaves()
	require.NoError(t, err)
	require.True(t, mainPresent)
	require.False(t, retry)
	retry, err = storage.admitAndRepairQueueStorageLeaves(leaves)
	require.NoError(t, err)
	require.False(t, retry)

	dbAfter := queueStorageDirectoryFingerprintAt(t, dbPath)
	walAfter := queueStorageDirectoryFingerprintAt(t, walPath)
	assert.Equal(t, dbBefore.Inode, dbAfter.Inode)
	assert.Equal(t, dbBefore.SHA256, dbAfter.SHA256)
	assert.Equal(t, walBefore.Inode, walAfter.Inode)
	assert.Equal(t, walBefore.SHA256, walAfter.SHA256)
	assert.Equal(t, os.FileMode(0o600), dbAfter.Mode.Perm())
	assert.Equal(t, os.FileMode(0o600), walAfter.Mode.Perm())
	assert.Equal(t, os.FileMode(0o700), queueStorageDirectoryMode(t, storage.rootPath).Perm())
}

func TestQueueStorageExistingLeafSetRejectsLateUnsafeSidecarBeforeRepair(t *testing.T) {
	homeDir := t.TempDir()
	storage, err := prepareMessageQueueStorage(homeDir)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, storage.Close()) })

	dbPath := storage.databasePath()
	require.NoError(t, os.WriteFile(dbPath, []byte("main database bytes"), 0o644))
	require.NoError(t, os.Chmod(dbPath, 0o644))
	require.NoError(t, os.Chmod(storage.rootPath, 0o755))
	dbBefore := queueStorageDirectoryFingerprintAt(t, dbPath)

	targetPath := filepath.Join(t.TempDir(), "sidecar-target")
	require.NoError(t, os.WriteFile(targetPath, []byte("target bytes"), 0o664))
	targetBefore := queueStorageDirectoryFingerprintAt(t, targetPath)
	require.NoError(t, os.Symlink(targetPath, dbPath+"-shm"))

	_, _, _, err = storage.classifyQueueStorageLeaves()
	require.ErrorIs(t, err, ErrUnsafeQueueStorage)
	assert.Equal(t, dbBefore, queueStorageDirectoryFingerprintAt(t, dbPath))
	assert.Equal(t, targetBefore, queueStorageDirectoryFingerprintAt(t, targetPath))
	assert.Equal(t, os.FileMode(0o755), queueStorageDirectoryMode(t, storage.rootPath).Perm())
}

func TestQueueStorageExistingLeafSetRejectsHardLinkedMain(t *testing.T) {
	homeDir := t.TempDir()
	storage, err := prepareMessageQueueStorage(homeDir)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, storage.Close()) })

	targetPath := filepath.Join(t.TempDir(), "hard-link-target")
	require.NoError(t, os.WriteFile(targetPath, []byte("linked bytes"), 0o664))
	require.NoError(t, os.Link(targetPath, storage.databasePath()))
	before := queueStorageDirectoryFingerprintAt(t, targetPath)

	_, _, _, err = storage.classifyQueueStorageLeaves()
	require.ErrorIs(t, err, ErrUnsafeQueueStorage)
	assert.Equal(t, before, queueStorageDirectoryFingerprintAt(t, targetPath))
	assert.Equal(t, before, queueStorageDirectoryFingerprintAt(t, storage.databasePath()))
}

func TestQueueStorageExistingLeafSetRetriesWholeDisappearingSnapshot(t *testing.T) {
	homeDir := t.TempDir()
	storage, err := prepareMessageQueueStorage(homeDir)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, storage.Close()) })

	dbPath := storage.databasePath()
	require.NoError(t, os.WriteFile(dbPath, []byte("main"), 0o644))
	require.NoError(t, os.WriteFile(dbPath+"-wal", []byte("transient WAL"), 0o644))
	require.NoError(t, os.Chmod(dbPath, 0o644))
	before := queueStorageDirectoryFingerprintAt(t, dbPath)

	leaves, mainPresent, retry, err := storage.classifyQueueStorageLeaves()
	require.NoError(t, err)
	require.True(t, mainPresent)
	require.False(t, retry)
	require.NoError(t, os.Remove(dbPath+"-wal"))

	retry, err = storage.admitAndRepairQueueStorageLeaves(leaves)
	require.NoError(t, err)
	require.True(t, retry)
	assert.Equal(t, before, queueStorageDirectoryFingerprintAt(t, dbPath))
}

func TestQueueStorageExistingLeafSetRevalidatesAfterRootSeal(t *testing.T) {
	homeDir := t.TempDir()
	storage, err := prepareMessageQueueStorage(homeDir)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, storage.Close()) })
	require.NoError(t, os.Chmod(storage.rootPath, 0o755))

	dbPath := storage.databasePath()
	require.NoError(t, os.WriteFile(dbPath, []byte("pre-seal database bytes"), 0o666))
	require.NoError(t, os.Chmod(dbPath, 0o666))
	leaves, mainPresent, retry, err := storage.classifyQueueStorageLeaves()
	require.NoError(t, err)
	require.True(t, mainPresent)
	require.False(t, retry)
	require.Len(t, leaves, 1)

	leaves[0].fd, err = unix.Openat(
		storage.rootFD,
		leaves[0].spec.name,
		unix.O_RDONLY|unix.O_NONBLOCK|unix.O_NOFOLLOW|unix.O_CLOEXEC,
		0,
	)
	require.NoError(t, err)
	defer closeQueueStorageLeafDescriptors(leaves)

	linkedPath := filepath.Join(t.TempDir(), "raced-hard-link")
	require.NoError(t, os.Link(dbPath, linkedPath))
	before := queueStorageDirectoryFingerprintAt(t, linkedPath)
	require.NoError(t, chmodAndVerifyQueueStorageDirectory(
		storage.rootFD,
		"AGM directory",
		storage.uid,
		storage.rootIdentity,
	))

	retry, err = storage.revalidateSealedQueueStorageLeaves(leaves)
	require.ErrorIs(t, err, ErrUnsafeQueueStorage)
	require.False(t, retry)
	assert.Equal(t, before, queueStorageDirectoryFingerprintAt(t, dbPath))
	assert.Equal(t, before, queueStorageDirectoryFingerprintAt(t, linkedPath))
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
