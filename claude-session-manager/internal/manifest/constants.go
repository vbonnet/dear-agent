package manifest

import "time"

const (
	// Schema version for manifest v2
	SchemaVersion = "2.0"

	// Lifecycle states
	LifecycleArchived = "archived"

	// Validation limits
	MaxPurposeLen = 256
	MaxTagsCount  = 10
	MaxTagLen     = 32
	MaxNotesLen   = 1024

	// File locking
	LockTimeout = 60 * time.Second

	// Backup limits
	MaxBackupsPerSession = 10
)
