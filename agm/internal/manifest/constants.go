package manifest

import "time"

const (
	// Schema version for manifest v2
	SchemaVersion = "2.0"

	// Lifecycle states
	LifecycleReaping  = "reaping"
	LifecycleArchived = "archived"

	// Workflow phase constants for multi-session coordination.
	// These track where a session is in its research/delegation lifecycle,
	// independent of the tmux-level State (DONE/WORKING/etc).
	WorkflowPhaseResearch = "research"
	WorkflowPhaseDelegate = "delegate"
	WorkflowPhaseWait     = "wait"
	WorkflowPhaseVerify   = "verify"
	WorkflowPhaseExit     = "exit"

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
