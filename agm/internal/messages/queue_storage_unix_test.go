//go:build darwin || linux

package messages

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/sys/unix"
)

const (
	queueStorageUmaskHelperEnv = "AGM_TEST_QUEUE_STORAGE_UMASK_HELPER"
	queueStorageModeReportEnv  = "AGM_TEST_QUEUE_STORAGE_MODE_REPORT"
	queueStorageCrashHelperEnv = "AGM_TEST_QUEUE_STORAGE_CRASH_HELPER"
)

type queueStorageModeReport struct {
	Config os.FileMode `json:"config"`
	Root   os.FileMode `json:"root"`
	DB     os.FileMode `json:"db"`
	WAL    os.FileMode `json:"wal"`
	SHM    os.FileMode `json:"shm"`
}

type queueStorageFileFingerprint struct {
	Mode   os.FileMode
	Size   int64
	Inode  uint64
	SHA256 [sha256.Size]byte
}

func TestMessageQueueFreshStorageIsPrivateUnderUmask022(t *testing.T) {
	if os.Getenv(queueStorageUmaskHelperEnv) == "1" {
		runQueueStorageUmaskHelper(t)
		return
	}

	homeDir := t.TempDir()
	reportPath := filepath.Join(t.TempDir(), "queue-storage-modes.json")
	command := exec.Command(
		os.Args[0],
		"-test.run=^TestMessageQueueFreshStorageIsPrivateUnderUmask022$",
	)
	command.Env = queueStorageTestEnvironment(map[string]string{
		"HOME":                     homeDir,
		queueStorageUmaskHelperEnv: "1",
		queueStorageModeReportEnv:  reportPath,
	})
	output, err := command.CombinedOutput()
	require.NoError(t, err, "subprocess failed:\n%s", output)

	reportJSON, err := os.ReadFile(reportPath)
	require.NoError(t, err)
	var report queueStorageModeReport
	require.NoError(t, json.Unmarshal(reportJSON, &report))
	assert.Equal(t, os.FileMode(0o700), report.Config.Perm(), ".config must be private when created")
	assert.Equal(t, os.FileMode(0o700), report.Root.Perm(), "AGM root must be private")
	assert.Equal(t, os.FileMode(0o600), report.DB.Perm(), "main DB must be private")
	assert.Equal(t, os.FileMode(0o600), report.WAL.Perm(), "live WAL must be private")
	assert.Equal(t, os.FileMode(0o600), report.SHM.Perm(), "live SHM must be private")
}

func TestMessageQueueRepairsExistingDatabasePermissionsWithoutReplacement(t *testing.T) {
	for _, initialMode := range []os.FileMode{0o400, 0o644, 0o666} {
		t.Run(initialMode.String(), func(t *testing.T) {
			homeDir := t.TempDir()
			t.Setenv("HOME", homeDir)
			rootDir, dbPath := seedPrivateQueueStorage(t)

			canaryPath := filepath.Join(rootDir, "unrelated-config-canary")
			require.NoError(t, os.WriteFile(canaryPath, []byte("unrelated AGM config must survive"), 0o640))
			canaryBefore := queueStorageFingerprint(t, canaryPath)
			dbBefore := queueStorageFingerprint(t, dbPath)

			require.NoError(t, os.Chmod(rootDir, 0o755))
			require.NoError(t, os.Chmod(dbPath, initialMode))

			queue, err := NewMessageQueue()
			require.NoError(t, err)
			t.Cleanup(func() { require.NoError(t, queue.Close()) })

			var body string
			require.NoError(t, queue.db.QueryRowContext(
				context.Background(),
				`SELECT message FROM message_queue WHERE message_id = 'permission-repair-canary'`,
			).Scan(&body))
			assert.Equal(t, "private queue body", body)

			dbAfter := queueStorageFingerprint(t, dbPath)
			assert.Equal(t, dbBefore.Inode, dbAfter.Inode, "permission repair must preserve DB identity")
			assert.Equal(t, dbBefore.Size, dbAfter.Size, "permission repair must preserve DB size")
			assert.Equal(t, dbBefore.SHA256, dbAfter.SHA256, "permission repair must preserve DB bytes")
			assert.Equal(t, os.FileMode(0o600), dbAfter.Mode.Perm())
			assert.Equal(t, os.FileMode(0o700), queueStorageMode(t, rootDir).Perm())
			assert.Equal(t, canaryBefore, queueStorageFingerprint(t, canaryPath),
				"permission repair must not mutate unrelated AGM configuration")

			var tableSQL string
			require.NoError(t, queue.db.QueryRowContext(context.Background(), `
				SELECT sql FROM sqlite_schema WHERE type = 'table' AND name = 'message_queue'
			`).Scan(&tableSQL))
			assert.Equal(t, currentQueueTableSQL(queueTableName), tableSQL)
			var sequence int
			require.NoError(t, queue.db.QueryRowContext(context.Background(), `
				SELECT seq FROM sqlite_sequence WHERE name = 'message_queue'
			`).Scan(&sequence))
			assert.Equal(t, 1, sequence)
			var indexCount int
			require.NoError(t, queue.db.QueryRowContext(context.Background(), `
				SELECT COUNT(*) FROM sqlite_schema
				WHERE type = 'index' AND name LIKE 'idx_%'
			`).Scan(&indexCount))
			assert.Equal(t, len(queueIndexDefinitions), indexCount)
		})
	}
}

func TestMessageQueueRepairsCrashLeftSidecarsWithoutReplacement(t *testing.T) {
	if os.Getenv(queueStorageCrashHelperEnv) == "1" {
		runQueueStorageCrashHelper(t)
		return
	}

	homeDir := t.TempDir()
	command := exec.Command(
		os.Args[0],
		"-test.run=^TestMessageQueueRepairsCrashLeftSidecarsWithoutReplacement$",
	)
	command.Env = queueStorageTestEnvironment(map[string]string{
		"HOME":                     homeDir,
		queueStorageCrashHelperEnv: "1",
	})
	output, err := command.CombinedOutput()
	require.NoError(t, err, "crash helper failed:\n%s", output)

	rootDir := filepath.Join(homeDir, ".config", "agm")
	dbPath := filepath.Join(rootDir, queueDatabaseLeaf)
	before := make(map[string]queueStorageFileFingerprint, 2)
	for _, suffix := range []string{"-wal", "-shm"} {
		path := dbPath + suffix
		fingerprint := queueStorageFingerprint(t, path)
		require.Positive(t, fingerprint.Size, "%s must be a non-empty crash artifact", suffix)
		assert.Equal(t, os.FileMode(0o644), fingerprint.Mode.Perm())
		before[suffix] = fingerprint
	}

	storage, err := prepareMessageQueueStorage(homeDir)
	require.NoError(t, err)
	require.NoError(t, storage.prepareForSQLite())
	require.NoError(t, storage.Close())

	assert.Equal(t, os.FileMode(0o700), queueStorageMode(t, rootDir).Perm())
	for _, suffix := range []string{"-wal", "-shm"} {
		after := queueStorageFingerprint(t, dbPath+suffix)
		assert.Equal(t, before[suffix].Inode, after.Inode, "%s identity must survive admission", suffix)
		assert.Equal(t, before[suffix].Size, after.Size, "%s size must survive admission", suffix)
		assert.Equal(t, before[suffix].SHA256, after.SHA256, "%s bytes must survive admission", suffix)
		assert.Equal(t, os.FileMode(0o600), after.Mode.Perm())
	}

	t.Setenv("HOME", homeDir)
	queue, err := NewMessageQueue()
	require.NoError(t, err)
	defer func() { require.NoError(t, queue.Close()) }()
	var body string
	require.NoError(t, queue.db.QueryRowContext(
		context.Background(),
		`SELECT message FROM message_queue WHERE message_id = 'crash-left-canary'`,
	).Scan(&body))
	assert.Equal(t, "crash queue body", body)
}

func TestMessageQueueRejectsUnsafeStorageWithoutTouchingTargets(t *testing.T) {
	t.Run("main symlink", func(t *testing.T) {
		homeDir, rootDir, dbPath := newQueueStorageDirectories(t, 0o700)
		t.Setenv("HOME", homeDir)
		targetPath := filepath.Join(t.TempDir(), "private-symlink-target")
		require.NoError(t, os.WriteFile(targetPath, []byte("symlink target canary"), 0o664))
		targetBefore := queueStorageFingerprint(t, targetPath)
		require.NoError(t, os.Symlink(targetPath, dbPath))

		queue, err := NewMessageQueue()
		if queue != nil {
			require.NoError(t, queue.Close())
		}
		require.ErrorIs(t, err, ErrUnsafeQueueStorage)
		assert.Equal(t, targetBefore, queueStorageFingerprint(t, targetPath))
		linkTarget, readlinkErr := os.Readlink(dbPath)
		require.NoError(t, readlinkErr)
		assert.Equal(t, targetPath, linkTarget)
		assert.Equal(t, os.FileMode(0o700), queueStorageMode(t, rootDir).Perm())
	})

	t.Run("non-regular main", func(t *testing.T) {
		homeDir, _, dbPath := newQueueStorageDirectories(t, 0o700)
		t.Setenv("HOME", homeDir)
		require.NoError(t, os.Mkdir(dbPath, 0o755))
		before, err := os.Lstat(dbPath)
		require.NoError(t, err)

		queue, err := NewMessageQueue()
		if queue != nil {
			require.NoError(t, queue.Close())
		}
		require.ErrorIs(t, err, ErrUnsafeQueueStorage)
		after, statErr := os.Lstat(dbPath)
		require.NoError(t, statErr)
		assert.True(t, after.IsDir())
		assert.Equal(t, before.Mode(), after.Mode())
		assert.Equal(t, queueStorageInode(t, before), queueStorageInode(t, after))
	})

	t.Run("hard-linked main", func(t *testing.T) {
		fixtureHome := t.TempDir()
		t.Setenv("HOME", fixtureHome)
		_, fixtureDB := seedPrivateQueueStorage(t)
		targetBefore := queueStorageFingerprint(t, fixtureDB)

		unsafeHome, _, unsafeDB := newQueueStorageDirectories(t, 0o700)
		require.NoError(t, os.Link(fixtureDB, unsafeDB))
		linkedBefore := queueStorageFingerprint(t, fixtureDB)
		require.Equal(t, targetBefore.Inode, linkedBefore.Inode)
		t.Setenv("HOME", unsafeHome)

		queue, err := NewMessageQueue()
		if queue != nil {
			require.NoError(t, queue.Close())
		}
		require.ErrorIs(t, err, ErrUnsafeQueueStorage)
		assert.Equal(t, linkedBefore, queueStorageFingerprint(t, fixtureDB))
		assert.Equal(t, linkedBefore, queueStorageFingerprint(t, unsafeDB))
	})

	t.Run("broadly writable AGM root", func(t *testing.T) {
		homeDir, rootDir, dbPath := newQueueStorageDirectories(t, 0o777)
		t.Setenv("HOME", homeDir)
		before, err := os.Lstat(rootDir)
		require.NoError(t, err)

		queue, err := NewMessageQueue()
		if queue != nil {
			require.NoError(t, queue.Close())
		}
		require.ErrorIs(t, err, ErrUnsafeQueueStorage)
		after, statErr := os.Lstat(rootDir)
		require.NoError(t, statErr)
		assert.Equal(t, os.FileMode(0o777), after.Mode().Perm())
		assert.Equal(t, queueStorageInode(t, before), queueStorageInode(t, after))
		_, statErr = os.Lstat(dbPath)
		require.ErrorIs(t, statErr, os.ErrNotExist)
	})

	t.Run("sidecar without main", func(t *testing.T) {
		homeDir, rootDir, dbPath := newQueueStorageDirectories(t, 0o755)
		t.Setenv("HOME", homeDir)
		sidecarPath := dbPath + "-wal"
		require.NoError(t, os.WriteFile(sidecarPath, []byte("orphan WAL canary"), 0o644))
		before := queueStorageFingerprint(t, sidecarPath)

		queue, err := NewMessageQueue()
		if queue != nil {
			require.NoError(t, queue.Close())
		}
		require.ErrorIs(t, err, ErrUnsafeQueueStorage)
		assert.Equal(t, before, queueStorageFingerprint(t, sidecarPath))
		assert.Equal(t, os.FileMode(0o755), queueStorageMode(t, rootDir).Perm(),
			"rejection must precede directory repair")
		_, statErr := os.Lstat(dbPath)
		require.ErrorIs(t, statErr, os.ErrNotExist)
	})
}

func TestMessageQueueRejectsSymlinkedStorageDirectoriesWithoutTouchingTargets(t *testing.T) {
	for _, boundary := range []string{"home", "config", "agm"} {
		t.Run(boundary, func(t *testing.T) {
			targetDir := t.TempDir()
			canaryPath := filepath.Join(targetDir, "target-canary")
			require.NoError(t, os.WriteFile(canaryPath, []byte("directory target must survive"), 0o640))
			before := queueStorageFingerprint(t, canaryPath)

			var linkPath string
			switch boundary {
			case "home":
				linkPath = filepath.Join(t.TempDir(), "home-link")
				require.NoError(t, os.Symlink(targetDir, linkPath))
				t.Setenv("HOME", linkPath)
			case "config":
				homeDir := t.TempDir()
				linkPath = filepath.Join(homeDir, ".config")
				require.NoError(t, os.Symlink(targetDir, linkPath))
				t.Setenv("HOME", homeDir)
			case "agm":
				homeDir := t.TempDir()
				configDir := filepath.Join(homeDir, ".config")
				require.NoError(t, os.Mkdir(configDir, 0o700))
				linkPath = filepath.Join(configDir, "agm")
				require.NoError(t, os.Symlink(targetDir, linkPath))
				t.Setenv("HOME", homeDir)
			}

			queue, err := NewMessageQueue()
			if queue != nil {
				require.NoError(t, queue.Close())
			}
			require.ErrorIs(t, err, ErrUnsafeQueueStorage)
			assert.NotContains(t, err.Error(), targetDir, "bounded errors must not disclose symlink targets")
			assert.Equal(t, before, queueStorageFingerprint(t, canaryPath))
			_, readlinkErr := os.Readlink(linkPath)
			require.NoError(t, readlinkErr)
		})
	}
}

func TestMessageQueueRejectsBroadlyWritableConfigDirectoryWithoutMutation(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)
	configDir := filepath.Join(homeDir, ".config")
	require.NoError(t, os.Mkdir(configDir, 0o700))
	require.NoError(t, os.Chmod(configDir, 0o777))
	before, err := os.Lstat(configDir)
	require.NoError(t, err)

	queue, err := NewMessageQueue()
	if queue != nil {
		require.NoError(t, queue.Close())
	}
	require.ErrorIs(t, err, ErrUnsafeQueueStorage)
	after, statErr := os.Lstat(configDir)
	require.NoError(t, statErr)
	assert.Equal(t, os.FileMode(0o777), after.Mode().Perm())
	assert.Equal(t, queueStorageInode(t, before), queueStorageInode(t, after))
	_, statErr = os.Lstat(filepath.Join(configDir, "agm"))
	require.ErrorIs(t, statErr, os.ErrNotExist)
}

func TestMessageQueueRejectsBroadlyWritableHomeWithoutMutation(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)
	require.NoError(t, os.Chmod(homeDir, 0o777))
	t.Cleanup(func() { require.NoError(t, os.Chmod(homeDir, 0o700)) })
	before, err := os.Lstat(homeDir)
	require.NoError(t, err)

	queue, err := NewMessageQueue()
	if queue != nil {
		require.NoError(t, queue.Close())
	}
	require.ErrorIs(t, err, ErrUnsafeQueueStorage)
	after, statErr := os.Lstat(homeDir)
	require.NoError(t, statErr)
	assert.Equal(t, os.FileMode(0o777), after.Mode().Perm())
	assert.Equal(t, queueStorageInode(t, before), queueStorageInode(t, after))
	_, statErr = os.Lstat(filepath.Join(homeDir, ".config"))
	require.ErrorIs(t, statErr, os.ErrNotExist)
}

func TestMessageQueueRetainsSafeSharedConfigMode(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)
	configDir := filepath.Join(homeDir, ".config")
	require.NoError(t, os.Mkdir(configDir, 0o700))
	require.NoError(t, os.Chmod(configDir, 0o755))

	queue, err := NewMessageQueue()
	require.NoError(t, err)
	defer func() { require.NoError(t, queue.Close()) }()
	assert.Equal(t, os.FileMode(0o755), queueStorageMode(t, configDir).Perm())
	assert.Equal(t, os.FileMode(0o700), queueStorageMode(t, filepath.Join(configDir, "agm")).Perm())
}

func TestMessageQueueRejectsUnsafeSidecarsBeforeRepair(t *testing.T) {
	for _, suffix := range []string{"-wal", "-shm", "-journal"} {
		t.Run("symlink"+suffix, func(t *testing.T) {
			homeDir := t.TempDir()
			t.Setenv("HOME", homeDir)
			rootDir, dbPath := seedPrivateQueueStorage(t)
			require.NoError(t, os.Chmod(rootDir, 0o755))
			require.NoError(t, os.Chmod(dbPath, 0o644))
			dbBefore := queueStorageFingerprint(t, dbPath)

			targetPath := filepath.Join(t.TempDir(), "sidecar-target")
			require.NoError(t, os.WriteFile(targetPath, []byte("sidecar target canary"), 0o664))
			before := queueStorageFingerprint(t, targetPath)
			require.NoError(t, os.Symlink(targetPath, dbPath+suffix))

			queue, err := NewMessageQueue()
			if queue != nil {
				require.NoError(t, queue.Close())
			}
			require.ErrorIs(t, err, ErrUnsafeQueueStorage)
			assert.NotContains(t, err.Error(), targetPath)
			assert.Equal(t, before, queueStorageFingerprint(t, targetPath))
			assert.Equal(t, dbBefore, queueStorageFingerprint(t, dbPath),
				"a later unsafe leaf must prevent earlier main-file repair")
			assert.Equal(t, os.FileMode(0o755), queueStorageMode(t, rootDir).Perm(),
				"all leaf classification must precede AGM-directory repair")
		})
	}

	for _, kind := range []string{"directory", "fifo"} {
		t.Run(kind, func(t *testing.T) {
			homeDir := t.TempDir()
			t.Setenv("HOME", homeDir)
			rootDir, dbPath := seedPrivateQueueStorage(t)
			require.NoError(t, os.Chmod(rootDir, 0o755))
			require.NoError(t, os.Chmod(dbPath, 0o644))
			dbBefore := queueStorageFingerprint(t, dbPath)
			unsafePath := dbPath + "-wal"
			if kind == "directory" {
				require.NoError(t, os.Mkdir(unsafePath, 0o755))
			} else {
				require.NoError(t, unix.Mkfifo(unsafePath, 0o644))
			}
			before, err := os.Lstat(unsafePath)
			require.NoError(t, err)

			queue, err := NewMessageQueue()
			if queue != nil {
				require.NoError(t, queue.Close())
			}
			require.ErrorIs(t, err, ErrUnsafeQueueStorage)
			after, statErr := os.Lstat(unsafePath)
			require.NoError(t, statErr)
			assert.Equal(t, before.Mode(), after.Mode())
			assert.Equal(t, queueStorageInode(t, before), queueStorageInode(t, after))
			assert.Equal(t, dbBefore, queueStorageFingerprint(t, dbPath))
			assert.Equal(t, os.FileMode(0o755), queueStorageMode(t, rootDir).Perm())
		})
	}

	t.Run("hard-linked sidecar", func(t *testing.T) {
		homeDir := t.TempDir()
		t.Setenv("HOME", homeDir)
		rootDir, dbPath := seedPrivateQueueStorage(t)
		require.NoError(t, os.Chmod(rootDir, 0o755))
		require.NoError(t, os.Chmod(dbPath, 0o644))
		dbBefore := queueStorageFingerprint(t, dbPath)

		targetPath := filepath.Join(t.TempDir(), "hard-link-target")
		require.NoError(t, os.WriteFile(targetPath, []byte("hard link target canary"), 0o664))
		require.NoError(t, os.Link(targetPath, dbPath+"-shm"))
		targetBefore := queueStorageFingerprint(t, targetPath)

		queue, err := NewMessageQueue()
		if queue != nil {
			require.NoError(t, queue.Close())
		}
		require.ErrorIs(t, err, ErrUnsafeQueueStorage)
		assert.Equal(t, targetBefore, queueStorageFingerprint(t, targetPath))
		assert.Equal(t, targetBefore, queueStorageFingerprint(t, dbPath+"-shm"))
		assert.Equal(t, dbBefore, queueStorageFingerprint(t, dbPath))
		assert.Equal(t, os.FileMode(0o755), queueStorageMode(t, rootDir).Perm())
	})
}

func TestMessageQueueRejectsUnreadableMainWithoutPathChmodFallback(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root can descriptor-open a mode-000 fixture")
	}
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)
	_, dbPath := seedPrivateQueueStorage(t)
	require.NoError(t, os.Chmod(dbPath, 0o000))
	before, err := os.Lstat(dbPath)
	require.NoError(t, err)

	queue, err := NewMessageQueue()
	if queue != nil {
		require.NoError(t, queue.Close())
	}
	require.ErrorIs(t, err, ErrUnsafeQueueStorage)
	after, statErr := os.Lstat(dbPath)
	require.NoError(t, statErr)
	assert.Equal(t, os.FileMode(0), after.Mode().Perm())
	assert.Equal(t, queueStorageInode(t, before), queueStorageInode(t, after))
}

func TestQueueStoragePostcheckVerifiesWithoutRepair(t *testing.T) {
	homeDir := t.TempDir()
	storage, err := prepareMessageQueueStorage(homeDir)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, storage.Close()) })
	require.NoError(t, storage.prepareForSQLite())

	sidecarPath := storage.databasePath() + "-wal"
	require.NoError(t, os.WriteFile(sidecarPath, []byte("new permissive sidecar"), 0o644))
	require.NoError(t, os.Chmod(sidecarPath, 0o644))
	before := queueStorageFingerprint(t, sidecarPath)

	err = storage.verifyAfterSQLite()
	require.ErrorIs(t, err, ErrUnsafeQueueStorage)
	assert.Equal(t, before, queueStorageFingerprint(t, sidecarPath),
		"post-initialization attestation must not silently repair a new sidecar")
}

func TestQueueStoragePostcheckRejectsReboundMainIdentity(t *testing.T) {
	homeDir := t.TempDir()
	storage, err := prepareMessageQueueStorage(homeDir)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, storage.Close()) })
	require.NoError(t, storage.prepareForSQLite())

	dbPath := storage.databasePath()
	admittedPath := dbPath + ".admitted"
	require.NoError(t, os.Rename(dbPath, admittedPath))
	require.NoError(t, os.WriteFile(dbPath, []byte("same-user replacement"), 0o600))
	replacementBefore := queueStorageFingerprint(t, dbPath)

	err = storage.verifyAfterSQLite()
	require.ErrorIs(t, err, ErrUnsafeQueueStorage)
	assert.Equal(t, replacementBefore, queueStorageFingerprint(t, dbPath))
	_, statErr := os.Stat(admittedPath)
	require.NoError(t, statErr, "postcheck must not remove the formerly admitted inode")
}

func TestQueueStorageRetriesWholeSnapshotWhenSidecarDisappears(t *testing.T) {
	homeDir := t.TempDir()
	storage, err := prepareMessageQueueStorage(homeDir)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, storage.Close()) })

	dbPath := storage.databasePath()
	require.NoError(t, os.WriteFile(dbPath, []byte("main"), 0o600))
	require.NoError(t, os.WriteFile(dbPath+"-wal", []byte("transient WAL"), 0o600))
	leaves, mainPresent, retry, err := storage.classifyQueueStorageLeaves()
	require.NoError(t, err)
	require.True(t, mainPresent)
	require.False(t, retry)
	require.Len(t, leaves, 2)
	require.NoError(t, os.Remove(dbPath+"-wal"))

	retry, err = storage.admitAndRepairQueueStorageLeaves(leaves)
	require.NoError(t, err)
	assert.True(t, retry, "a disappearing auxiliary must discard and retry the whole snapshot")
	assert.Equal(t, os.FileMode(0o600), queueStorageMode(t, dbPath).Perm())
}

func TestQueueStorageRevalidatesEveryLeafAfterSealingRootBeforeRepair(t *testing.T) {
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

	for i := range leaves {
		leaves[i].fd, err = unix.Openat(
			storage.rootFD,
			leaves[i].spec.name,
			unix.O_RDONLY|unix.O_NONBLOCK|unix.O_NOFOLLOW|unix.O_CLOEXEC,
			0,
		)
		require.NoError(t, err)
	}
	defer closeQueueStorageLeafDescriptors(leaves)

	linkedPath := filepath.Join(t.TempDir(), "raced-hard-link")
	require.NoError(t, os.Link(dbPath, linkedPath))
	before := queueStorageFingerprint(t, linkedPath)
	require.NoError(t, chmodAndVerifyQueueStorageDirectory(
		storage.rootFD,
		"AGM directory",
		storage.uid,
		storage.rootIdentity,
	))

	retry, err = storage.revalidateSealedQueueStorageLeaves(leaves)
	require.ErrorIs(t, err, ErrUnsafeQueueStorage)
	assert.False(t, retry)
	assert.Equal(t, before, queueStorageFingerprint(t, dbPath),
		"post-seal rejection must precede the first leaf chmod")
	assert.Equal(t, before, queueStorageFingerprint(t, linkedPath),
		"the raced hard-link target must remain byte- and mode-identical")
	assert.Equal(t, os.FileMode(0o700), queueStorageMode(t, storage.rootPath).Perm())
}

func TestValidateQueueStorageOwnerUsesExpectedUID(t *testing.T) {
	const expectedUID uint32 = 1000

	require.NoError(t, validateQueueStorageOwner("main database", expectedUID, expectedUID))
	err := validateQueueStorageOwner("main database", expectedUID+1, expectedUID)
	require.ErrorIs(t, err, ErrUnsafeQueueStorage)
	assert.ErrorContains(t, err, "main database")
}

func TestMessageQueueConcurrentFreshConstructorsRemainPrivate(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)

	const constructorCount = 16
	start := make(chan struct{})
	results := make(chan error, constructorCount)
	var constructors sync.WaitGroup
	constructors.Add(constructorCount)
	for range constructorCount {
		go func() {
			defer constructors.Done()
			<-start
			queue, err := NewMessageQueue()
			if err == nil {
				err = queue.Close()
			}
			results <- err
		}()
	}
	close(start)

	done := make(chan struct{})
	go func() {
		constructors.Wait()
		close(results)
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(30 * time.Second):
		t.Fatal("concurrent queue constructors did not finish within 30 seconds")
	}
	for err := range results {
		require.NoError(t, err)
	}

	rootDir := filepath.Join(homeDir, ".config", "agm")
	dbPath := filepath.Join(rootDir, "message_queue.db")
	assert.Equal(t, os.FileMode(0o700), queueStorageMode(t, rootDir).Perm())
	assert.Equal(t, os.FileMode(0o600), queueStorageMode(t, dbPath).Perm())

	queue, err := NewMessageQueue()
	require.NoError(t, err)
	defer func() { require.NoError(t, queue.Close()) }()
	var tableCount int
	require.NoError(t, queue.db.QueryRowContext(context.Background(), `
		SELECT COUNT(*) FROM sqlite_schema WHERE type = 'table' AND name = 'message_queue'
	`).Scan(&tableCount))
	assert.Equal(t, 1, tableCount)
}

func runQueueStorageUmaskHelper(t *testing.T) {
	reportPath := os.Getenv(queueStorageModeReportEnv)
	require.NotEmpty(t, reportPath)

	oldUmask := syscall.Umask(0o022)
	defer syscall.Umask(oldUmask)

	queue, err := NewMessageQueue()
	require.NoError(t, err)
	defer func() { require.NoError(t, queue.Close()) }()

	_, err = queue.db.ExecContext(context.Background(), `
		INSERT INTO message_queue
			(message_id, from_session, to_session, message, priority, queued_at, status)
		VALUES
			('live-mode-canary', 'source', 'target', 'private body', 'MEDIUM', CURRENT_TIMESTAMP, 'queued')
	`)
	require.NoError(t, err)

	homeDir, err := os.UserHomeDir()
	require.NoError(t, err)
	rootDir := filepath.Join(homeDir, ".config", "agm")
	dbPath := filepath.Join(rootDir, "message_queue.db")
	report := queueStorageModeReport{
		Config: queueStorageMode(t, filepath.Join(homeDir, ".config")),
		Root:   queueStorageMode(t, rootDir),
		DB:     queueStorageMode(t, dbPath),
		WAL:    queueStorageMode(t, dbPath+"-wal"),
		SHM:    queueStorageMode(t, dbPath+"-shm"),
	}
	reportJSON, err := json.Marshal(report)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(reportPath, reportJSON, 0o600))
}

func runQueueStorageCrashHelper(t *testing.T) {
	queue, err := NewMessageQueue()
	require.NoError(t, err)
	_, err = queue.db.ExecContext(context.Background(), `
		PRAGMA wal_autocheckpoint = 0;
		INSERT INTO message_queue
			(message_id, from_session, to_session, message, priority, queued_at, status)
		VALUES
			('crash-left-canary', 'source', 'target', 'crash queue body',
			 'HIGH', CURRENT_TIMESTAMP, 'queued');
	`)
	require.NoError(t, err)

	homeDir, err := os.UserHomeDir()
	require.NoError(t, err)
	rootDir := filepath.Join(homeDir, ".config", "agm")
	dbPath := filepath.Join(rootDir, queueDatabaseLeaf)
	for _, path := range []string{rootDir, dbPath, dbPath + "-wal", dbPath + "-shm"} {
		_, statErr := os.Stat(path)
		require.NoError(t, statErr, "expected live crash fixture %s", filepath.Base(path))
	}
	require.NoError(t, os.Chmod(rootDir, 0o755))
	for _, path := range []string{dbPath, dbPath + "-wal", dbPath + "-shm"} {
		require.NoError(t, os.Chmod(path, 0o644))
	}

	// Deliberately bypass database Close and test cleanup so SQLite cannot
	// unlink its live WAL/SHM. The parent process validates and recovers them.
	os.Exit(0)
}

func seedPrivateQueueStorage(t *testing.T) (rootDir, dbPath string) {
	t.Helper()
	homeDir, err := os.UserHomeDir()
	require.NoError(t, err)
	queue, err := NewMessageQueue()
	require.NoError(t, err)
	_, err = queue.db.ExecContext(context.Background(), `
		INSERT INTO message_queue
			(message_id, from_session, to_session, message, priority, queued_at, status)
		VALUES
			('permission-repair-canary', 'source', 'target', 'private queue body',
			 'HIGH', CURRENT_TIMESTAMP, 'queued')
	`)
	require.NoError(t, err)
	require.NoError(t, queue.Close())
	rootDir = filepath.Join(homeDir, ".config", "agm")
	dbPath = filepath.Join(rootDir, "message_queue.db")
	return rootDir, dbPath
}

func newQueueStorageDirectories(t *testing.T, rootMode os.FileMode) (homeDir, rootDir, dbPath string) {
	t.Helper()
	homeDir = t.TempDir()
	configDir := filepath.Join(homeDir, ".config")
	rootDir = filepath.Join(configDir, "agm")
	require.NoError(t, os.Mkdir(configDir, 0o700))
	require.NoError(t, os.Mkdir(rootDir, 0o700))
	require.NoError(t, os.Chmod(rootDir, rootMode))
	return homeDir, rootDir, filepath.Join(rootDir, "message_queue.db")
}

func queueStorageMode(t *testing.T, path string) os.FileMode {
	t.Helper()
	info, err := os.Lstat(path)
	require.NoError(t, err, "lstat %s", filepath.Base(path))
	return info.Mode()
}

func queueStorageFingerprint(t *testing.T, path string) queueStorageFileFingerprint {
	t.Helper()
	info, err := os.Lstat(path)
	require.NoError(t, err, "lstat %s", filepath.Base(path))
	contents, err := os.ReadFile(path)
	require.NoError(t, err, "read %s", filepath.Base(path))
	return queueStorageFileFingerprint{
		Mode:   info.Mode(),
		Size:   info.Size(),
		Inode:  queueStorageInode(t, info),
		SHA256: sha256.Sum256(contents),
	}
}

func queueStorageInode(t *testing.T, info os.FileInfo) uint64 {
	t.Helper()
	stat, ok := info.Sys().(*syscall.Stat_t)
	require.True(t, ok, "unexpected stat payload %T", info.Sys())
	return stat.Ino
}

func queueStorageTestEnvironment(overrides map[string]string) []string {
	environment := make([]string, 0, len(os.Environ())+len(overrides))
	for _, entry := range os.Environ() {
		key, _, _ := strings.Cut(entry, "=")
		if _, overridden := overrides[key]; !overridden {
			environment = append(environment, entry)
		}
	}
	for key, value := range overrides {
		environment = append(environment, fmt.Sprintf("%s=%s", key, value))
	}
	return environment
}
