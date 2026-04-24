package sandbox

import "time"

// SandboxRequest contains all parameters needed to create a sandbox.
//
//nolint:revive // SandboxRequest is intentionally verbose for clarity in API
type SandboxRequest struct {
	// SessionID uniquely identifies the AGM session.
	// Used for tracking and cleanup.
	SessionID string

	// LowerDirs are read-only host paths to include in the sandbox.
	// Mounted as the lower layer in OverlayFS (or cloned for APFS).
	//
	// Example: ["~/src/ai-tools", "~/src/engram"]
	LowerDirs []string

	// WorkspaceDir is where the sandbox creates its scratch space.
	// Provider creates: upper/, work/, merged/ inside this directory.
	//
	// Example: "~/.agm/sandboxes/abc-123"
	WorkspaceDir string

	// Secrets are injected into the sandbox environment.
	// Provider writes these to upperdir/.env or similar.
	//
	// Example: {"ANTHROPIC_API_KEY": "sk-ant-..."}
	Secrets map[string]string

	// Timeout for sandbox creation (optional, 0 = no timeout)
	Timeout time.Duration

	// ShareNetwork allows the sandbox to share the host network namespace.
	// Default (false) restricts network access for security.
	ShareNetwork bool

	// TargetRepo is the preferred repository to use for worktree creation.
	// When set, the sandbox provider uses this repo directly instead of
	// scanning LowerDirs alphabetically (which can pick the wrong repo).
	// Optional: if empty, the provider falls back to scanning LowerDirs.
	TargetRepo string
}

// Sandbox represents a provisioned sandbox environment.
type Sandbox struct {
	// ID is the unique identifier for this sandbox.
	// Usually matches SessionID from the request.
	ID string

	// MergedPath is where agents operate.
	// This directory contains the unified view of all LowerDirs.
	//
	// Example: "~/.agm/sandboxes/abc-123/merged"
	MergedPath string

	// UpperPath is where agent modifications are stored.
	// Hidden from agents, used by provider for copy-up operations.
	//
	// Example: "~/.agm/sandboxes/abc-123/upper"
	UpperPath string

	// WorkPath is the overlay working directory.
	// Hidden from agents, used by overlay filesystem for atomic operations.
	//
	// Example: "~/.agm/sandboxes/abc-123/work"
	WorkPath string

	// Type identifies which provider created this sandbox.
	// Values: "overlayfs", "apfs-reflink", "macfuse", "fallback"
	Type string

	// CreatedAt timestamp
	CreatedAt time.Time

	// CleanupFunc is called by Destroy() to tear down the sandbox.
	// Provider-specific cleanup logic (unmount, delete dirs, etc.)
	CleanupFunc func() error
}

// SandboxStats provides runtime statistics about a sandbox.
//
//nolint:revive // SandboxStats is intentionally verbose for clarity in API
type SandboxStats struct {
	ID           string
	SizeOnDisk   int64 // Bytes used by upperdir
	FileCount    int   // Files in upperdir (modified/created)
	MountHealthy bool  // Is filesystem still mounted?
	LastAccessed time.Time
}
