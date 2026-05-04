package manifest

import "time"

const (
	// SchemaVersion is the current manifest schema version (v2).
	SchemaVersion = "2.0"

	// LifecycleReaping marks a session whose resources are being torn down.
	LifecycleReaping = "reaping"
	// LifecycleArchived marks a session that has been archived.
	LifecycleArchived = "archived"

	// WorkflowPhaseResearch indicates the session is in the research phase.
	// Workflow phase constants track where a session is in its research/delegation
	// lifecycle, independent of the tmux-level State (DONE/WORKING/etc).
	WorkflowPhaseResearch = "research"
	// WorkflowPhaseDelegate indicates the session is delegating subtasks.
	WorkflowPhaseDelegate = "delegate"
	// WorkflowPhaseWait indicates the session is waiting on delegated work.
	WorkflowPhaseWait = "wait"
	// WorkflowPhaseVerify indicates the session is verifying delegated results.
	WorkflowPhaseVerify = "verify"
	// WorkflowPhaseExit indicates the session is exiting.
	WorkflowPhaseExit = "exit"

	// MaxPurposeLen is the maximum allowed length for a session purpose string.
	MaxPurposeLen = 256
	// MaxTagsCount is the maximum number of tags allowed on a session.
	MaxTagsCount = 10
	// MaxTagLen is the maximum allowed length for a single tag.
	MaxTagLen = 32
	// MaxNotesLen is the maximum allowed length of session notes.
	MaxNotesLen = 1024

	// LockTimeout is the maximum time to wait acquiring a manifest file lock.
	LockTimeout = 60 * time.Second

	// MaxBackupsPerSession is the maximum number of backup files retained per session.
	MaxBackupsPerSession = 10
)
