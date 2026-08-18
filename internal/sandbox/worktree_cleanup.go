package sandbox

import (
	"fmt"
	"sync"
)

// WorktreeCleanup records progress through provider cleanup so a retry does
// not repeat Git worktree removal after that irreversible phase succeeded.
// Run is serialized because callers may invoke a sandbox cleanup concurrently.
type WorktreeCleanup struct {
	mu              sync.Mutex
	worktreeRemoved bool
	complete        bool
}

// NewWorktreeCleanup creates retry state for a sandbox that either owns a Git
// worktree or uses only provider directories.
func NewWorktreeCleanup(hasWorktree bool) *WorktreeCleanup {
	return &WorktreeCleanup{worktreeRemoved: !hasWorktree}
}

// Run removes the Git worktree before provider directories. If directory
// cleanup fails after Git succeeds, a later call resumes at directory cleanup
// instead of treating the already-absent worktree as a new failure.
func (c *WorktreeCleanup) Run(removeWorktree, cleanupDirectories func() error) error {
	if c == nil {
		return fmt.Errorf("sandbox worktree cleanup state is required")
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.complete {
		return nil
	}
	if !c.worktreeRemoved {
		if removeWorktree == nil {
			return fmt.Errorf("sandbox worktree removal callback is required")
		}
		if err := removeWorktree(); err != nil {
			return err
		}
		c.worktreeRemoved = true
	}
	if cleanupDirectories == nil {
		return fmt.Errorf("sandbox directory cleanup callback is required")
	}
	if err := cleanupDirectories(); err != nil {
		return err
	}
	c.complete = true
	return nil
}
