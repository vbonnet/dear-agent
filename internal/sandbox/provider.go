package sandbox

import "context"

// Provider defines the interface for all sandbox implementations.
// Implementations must be safe for concurrent use.
type Provider interface {
	// Create provisions a new isolated sandbox environment.
	// Returns Sandbox metadata including the merged path where agents operate.
	//
	// The sandbox MUST guarantee:
	//   - Read-only access to all repos in LowerDirs
	//   - Copy-on-write for modifications (saved in UpperDir)
	//   - Complete isolation (rm -rf in sandbox doesn't affect host)
	//
	// Context cancellation should abort creation and clean up partial state.
	Create(ctx context.Context, req SandboxRequest) (*Sandbox, error)

	// Destroy tears down a sandbox and cleans up all associated resources.
	// Must unmount filesystems, delete temporary directories, and release locks.
	//
	// Idempotent: calling Destroy on a non-existent sandbox returns nil.
	// Returns error only for cleanup failures that leave corrupted state.
	Destroy(ctx context.Context, id string) error

	// Validate checks if a sandbox exists and is healthy.
	// Used for debugging and health checks.
	//
	// Returns:
	//   - nil if sandbox exists and is functional
	//   - ErrSandboxNotFound if sandbox doesn't exist
	//   - ErrMountStale if filesystem mount is corrupted
	Validate(ctx context.Context, id string) error

	// Name returns a human-readable name for this provider.
	// Examples: "overlayfs-native", "apfs-reflink", "fuse-overlayfs"
	Name() string
}
