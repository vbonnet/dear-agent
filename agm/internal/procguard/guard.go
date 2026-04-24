package procguard

import (
	"fmt"
	"sync/atomic"
	"syscall"
)

const (
	// MaxSessionDepth is the maximum allowed nesting depth for session hierarchies.
	// A root session has depth 0, its children depth 1, etc.
	MaxSessionDepth = 5

	// MaxChildrenPerSession is the maximum number of direct child sessions
	// a single parent session can spawn.
	MaxChildrenPerSession = 10

	// MaxTotalActiveSessions is the maximum number of concurrently active sessions
	// across the entire system.
	MaxTotalActiveSessions = 50

	// DefaultNprocLimit is the default RLIMIT_NPROC value applied to spawned
	// agent processes to prevent fork bombs at the OS level.
	DefaultNprocLimit = 256
)

// activeCount tracks the number of currently active spawned processes system-wide.
var activeCount int64

// ActiveCount returns the current number of active spawned processes.
func ActiveCount() int64 {
	return atomic.LoadInt64(&activeCount)
}

// IncrementActive increments the active process counter. Call when a process is spawned.
func IncrementActive() {
	atomic.AddInt64(&activeCount, 1)
}

// DecrementActive decrements the active process counter. Call when a process exits.
func DecrementActive() {
	atomic.AddInt64(&activeCount, -1)
}

// ResetActiveCount resets the counter to zero (for testing only).
func ResetActiveCount() {
	atomic.StoreInt64(&activeCount, 0)
}

// SpawnLimits holds the configurable limits for process spawning.
type SpawnLimits struct {
	MaxDepth       int
	MaxChildren    int
	MaxTotalActive int
	NprocLimit     uint64
}

// DefaultLimits returns SpawnLimits with the default constants.
func DefaultLimits() SpawnLimits {
	return SpawnLimits{
		MaxDepth:       MaxSessionDepth,
		MaxChildren:    MaxChildrenPerSession,
		MaxTotalActive: MaxTotalActiveSessions,
		NprocLimit:     DefaultNprocLimit,
	}
}

// ValidateSpawn checks whether spawning a new child session is allowed given
// the current session depth, sibling count, and total active sessions.
// Returns nil if spawning is allowed, or an error describing the violation.
func ValidateSpawn(limits SpawnLimits, currentDepth int, currentChildren int, totalActive int) error {
	if currentDepth >= limits.MaxDepth {
		return fmt.Errorf("fork bomb prevention: session depth %d exceeds maximum allowed depth %d", currentDepth, limits.MaxDepth)
	}

	if currentChildren >= limits.MaxChildren {
		return fmt.Errorf("fork bomb prevention: session already has %d children, maximum is %d", currentChildren, limits.MaxChildren)
	}

	if totalActive >= limits.MaxTotalActive {
		return fmt.Errorf("fork bomb prevention: %d active sessions exceeds maximum of %d", totalActive, limits.MaxTotalActive)
	}

	return nil
}

// ProcessGroupAttr returns a SysProcAttr that places the child process in its
// own process group. This enables clean group-kill of all descendants.
func ProcessGroupAttr() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{
		Setpgid: true,
	}
}
