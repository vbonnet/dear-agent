package taskqueue

import (
	"fmt"
	"os"
	"sync"
	"time"

	"gopkg.in/yaml.v3"
)

// Coordinator manages thread-safe access to the task queue
type Coordinator struct {
	filePath string
	queue    *TaskQueue
	mu       sync.RWMutex
}

// NewCoordinator creates a new Coordinator for the given file path
func NewCoordinator(filePath string) *Coordinator {
	return &Coordinator{
		filePath: filePath,
	}
}

// Load reads the task queue from disk with read lock
func (c *Coordinator) Load() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	tq, err := ParseTaskQueue(c.filePath)
	if err != nil {
		return fmt.Errorf("failed to load task queue: %w", err)
	}

	c.queue = tq
	return nil
}

// Save writes the task queue to disk with atomic write pattern
func (c *Coordinator) Save() error {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if c.queue == nil {
		return fmt.Errorf("queue not loaded, call Load() first")
	}

	// Update timestamp
	c.queue.LastUpdated = time.Now()

	// Marshal to YAML
	data, err := yaml.Marshal(c.queue)
	if err != nil {
		return fmt.Errorf("failed to marshal task queue: %w", err)
	}

	// Atomic write: temp file + rename
	tmpPath := c.filePath + ".tmp"
	if err := os.WriteFile(tmpPath, data, 0644); err != nil {
		return fmt.Errorf("failed to write temp file: %w", err)
	}

	if err := os.Rename(tmpPath, c.filePath); err != nil {
		_ = os.Remove(tmpPath) // Cleanup on failure
		return fmt.Errorf("failed to rename temp file: %w", err)
	}

	return nil
}

// QueryNext finds the first ready bead (O(n) scan)
func (c *Coordinator) QueryNext() (*Bead, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if c.queue == nil {
		return nil, fmt.Errorf("queue not loaded, call Load() first")
	}

	if len(c.queue.Ready) == 0 {
		return nil, nil // No ready beads
	}

	// Return first ready bead (O(n) scan acceptable for V1)
	bead := c.queue.Ready[0]
	return &bead, nil
}

// Claim moves a bead from ready to in_progress
func (c *Coordinator) Claim(beadID string, sessionName string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.queue == nil {
		return fmt.Errorf("queue not loaded, call Load() first")
	}

	// Find bead in ready section
	index := -1
	for i, bead := range c.queue.Ready {
		if bead.ID == beadID {
			index = i
			break
		}
	}

	if index == -1 {
		return fmt.Errorf("bead %s not found in ready section", beadID)
	}

	// Get bead for dependency validation
	bead := c.queue.Ready[index]

	// Validate all dependencies are completed before claiming.
	// This prevents launching beads before their prerequisites are met,
	// which would cause agent failures and wasted resources.
	if len(bead.DependsOn) > 0 {
		// Build map of completed bead IDs for O(1) lookup
		completed := make(map[string]bool)
		for _, b := range c.queue.Completed {
			completed[b.ID] = true
		}

		// Check each dependency
		var missingDeps []string
		for _, depID := range bead.DependsOn {
			if !completed[depID] {
				missingDeps = append(missingDeps, depID)
			}
		}

		if len(missingDeps) > 0 {
			return fmt.Errorf("cannot claim bead %s: missing dependencies %v", beadID, missingDeps)
		}
	}

	// Update bead metadata
	bead.Metadata.SessionName = sessionName
	bead.Metadata.Iterations++
	bead.Metadata.LastAttempt = time.Now()

	// Move to in_progress
	c.queue.InProgress = append(c.queue.InProgress, bead)
	c.queue.Ready = append(c.queue.Ready[:index], c.queue.Ready[index+1:]...)

	return nil
}

// Complete moves a bead from in_progress to completed
func (c *Coordinator) Complete(beadID string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.queue == nil {
		return fmt.Errorf("queue not loaded, call Load() first")
	}

	// Find bead in in_progress section
	index := -1
	for i, bead := range c.queue.InProgress {
		if bead.ID == beadID {
			index = i
			break
		}
	}

	if index == -1 {
		return fmt.Errorf("bead %s not found in in_progress section", beadID)
	}

	// Move to completed
	bead := c.queue.InProgress[index]
	c.queue.Completed = append(c.queue.Completed, bead)
	c.queue.InProgress = append(c.queue.InProgress[:index], c.queue.InProgress[index+1:]...)

	// Check if any blocked beads can be unblocked
	c.unblockDependents(beadID)

	return nil
}

// Unblock checks dependencies and moves blocked beads to ready
func (c *Coordinator) Unblock() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.queue == nil {
		return fmt.Errorf("queue not loaded, call Load() first")
	}

	// Build set of completed bead IDs
	completed := make(map[string]bool)
	for _, bead := range c.queue.Completed {
		completed[bead.ID] = true
	}

	// Check each blocked bead
	var stillBlocked []Bead
	for _, bead := range c.queue.Blocked {
		allDepsCompleted := true
		for _, depID := range bead.DependsOn {
			if !completed[depID] {
				allDepsCompleted = false
				break
			}
		}

		if allDepsCompleted {
			// Move to ready
			c.queue.Ready = append(c.queue.Ready, bead)
		} else {
			// Still blocked
			stillBlocked = append(stillBlocked, bead)
		}
	}

	c.queue.Blocked = stillBlocked
	return nil
}

// unblockDependents checks if completing a bead unblocks any blocked beads
// Must be called with write lock held
func (c *Coordinator) unblockDependents(completedBeadID string) {
	// Build set of completed bead IDs
	completed := make(map[string]bool)
	for _, bead := range c.queue.Completed {
		completed[bead.ID] = true
	}

	// Check each blocked bead
	var stillBlocked []Bead
	for _, bead := range c.queue.Blocked {
		allDepsCompleted := true
		for _, depID := range bead.DependsOn {
			if !completed[depID] {
				allDepsCompleted = false
				break
			}
		}

		if allDepsCompleted {
			// Move to ready
			c.queue.Ready = append(c.queue.Ready, bead)
		} else {
			// Still blocked
			stillBlocked = append(stillBlocked, bead)
		}
	}

	c.queue.Blocked = stillBlocked
}

// GetQueue returns a copy of the current queue state (for testing/debugging)
func (c *Coordinator) GetQueue() *TaskQueue {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if c.queue == nil {
		return nil
	}

	// Return shallow copy (sufficient for read-only access)
	queueCopy := *c.queue
	return &queueCopy
}
