package tmux

import (
	"context"
	"fmt"
	"time"
)

// maxConcurrentTmuxOps limits the number of concurrent tmux command invocations.
// Each tmux command spawns a process and opens pipes/fds. Under high load with
// 15+ sessions all receiving messages simultaneously, unbounded concurrency can
// exhaust file descriptors and cause the tmux server to crash.
//
// Bug fix (2026-04-02): Previously there was no limit on concurrent tmux operations.
// With many sessions, SendCommand, SendPromptLiteral, and retry loops could spawn
// dozens of concurrent tmux processes, causing fd exhaustion and server crash.
const maxConcurrentTmuxOps = 20

// semaphoreAcquireTimeout is how long to wait for a semaphore slot before giving up.
const semaphoreAcquireTimeout = 30 * time.Second

// tmuxSemaphore limits concurrent tmux operations to prevent resource exhaustion.
// Buffered channel acts as a counting semaphore.
var tmuxSemaphore = make(chan struct{}, maxConcurrentTmuxOps)

// acquireTmuxSemaphore acquires a slot in the tmux concurrency semaphore.
// Returns error if the semaphore cannot be acquired within the timeout,
// which indicates the system is overloaded.
func acquireTmuxSemaphore(ctx context.Context) error {
	// Create a timeout context if the parent doesn't have one
	timeoutCtx, cancel := context.WithTimeout(ctx, semaphoreAcquireTimeout)
	defer cancel()

	select {
	case tmuxSemaphore <- struct{}{}:
		return nil
	case <-timeoutCtx.Done():
		return fmt.Errorf("tmux operations overloaded: %d concurrent operations in progress (max %d), waited %v",
			len(tmuxSemaphore), maxConcurrentTmuxOps, semaphoreAcquireTimeout)
	}
}

// releaseTmuxSemaphore releases a slot in the tmux concurrency semaphore.
// Safe to call even if the semaphore is empty (will not block).
func releaseTmuxSemaphore() {
	select {
	case <-tmuxSemaphore:
	default:
		// Semaphore already empty — no-op to prevent blocking on mismatched release
	}
}

// TmuxConcurrentOps returns the current number of in-flight tmux operations.
// Useful for monitoring and diagnostics.
func TmuxConcurrentOps() int {
	return len(tmuxSemaphore)
}

// SetMaxConcurrentOps replaces the semaphore with a new capacity.
// Only safe to call during init or tests when no operations are in flight.
func SetMaxConcurrentOps(capacity int) {
	tmuxSemaphore = make(chan struct{}, capacity)
}
