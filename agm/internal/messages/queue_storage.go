package messages

import (
	"errors"
	"fmt"
)

// ErrUnsafeQueueStorage identifies a queue filesystem boundary whose ownership,
// type, identity, or mode could not be trusted. Callers may use this identity to
// distinguish a security failure from an unavailable SQLite database.
var ErrUnsafeQueueStorage = errors.New("unsafe message queue storage")

// ErrQueueStorageUnavailable identifies an ordinary resource failure while
// admitting the queue boundary: descriptor exhaustion, no space, a read-only
// or failing filesystem. It is deliberately a separate identity from
// ErrUnsafeQueueStorage, which suppresses the caller's direct-delivery
// fallback. A transient outage must not silently drop a message, so only an
// established trust violation may claim that suppression.
var ErrQueueStorageUnavailable = errors.New("message queue storage unavailable")

const (
	queueDatabaseLeaf          = "message_queue.db"
	queueStorageAdmissionLimit = 8
	queueStorageInvalidFD      = -1
)

type queueStorageIdentity struct {
	device uint64
	inode  uint64
}

type queueStorageLeafSpec struct {
	name     string
	artifact string
	main     bool
}

var queueStorageLeafSpecs = [...]queueStorageLeafSpec{
	{name: queueDatabaseLeaf, artifact: "main database", main: true},
	{name: queueDatabaseLeaf + "-wal", artifact: "WAL sidecar"},
	{name: queueDatabaseLeaf + "-shm", artifact: "SHM sidecar"},
	{name: queueDatabaseLeaf + "-journal", artifact: "rollback journal"},
}

// messageQueueStorage is a retained, private capability for the admitted queue
// directory chain. SQLite receives only databasePath; the capability keeps the
// descriptors needed to verify that the visible boundary did not change.
type messageQueueStorage struct {
	homePath string
	rootPath string
	dbPath   string
	dbLeaf   string
	uid      uint32

	homeFD   int
	configFD int
	rootFD   int

	homeIdentity   queueStorageIdentity
	configIdentity queueStorageIdentity
	rootIdentity   queueStorageIdentity
	mainIdentity   queueStorageIdentity

	productionChain bool
	prepared        bool
	closed          bool
}

func (s *messageQueueStorage) databasePath() string {
	if s == nil {
		return ""
	}
	return s.dbPath
}

func unsafeQueueStorageError(artifact, invariant string) error {
	if artifact == "" {
		artifact = "queue artifact"
	}
	return fmt.Errorf("%w: %s %s", ErrUnsafeQueueStorage, artifact, invariant)
}

func validateQueueStorageOwner(artifact string, actualUID, expectedUID uint32) error {
	if actualUID != expectedUID {
		return unsafeQueueStorageError(artifact, "has an unexpected owner")
	}
	return nil
}
