package worktree

import "sync"

// ProvisionCache provides in-memory caching for worktree provisioning results.
// It stores the mapping of session IDs to worktree paths to avoid repeated
// filesystem checks and git operations within a single hook execution context.
//
// Thread-safety: This cache is safe for concurrent access using a read-write mutex.
// However, since each hook invocation runs as a separate process, the cache
// does not persist across hook calls.
type ProvisionCache struct {
	mu    sync.RWMutex
	cache map[string]string // sessionID -> worktreePath
}

// NewProvisionCache creates a new empty provision cache.
//
// Example:
//
//	cache := NewProvisionCache()
//	cache.Set("abc123", "/tmp/test/worktrees/session-abc123")
//	path := cache.Get("abc123") // Returns "/tmp/test/worktrees/session-abc123"
func NewProvisionCache() *ProvisionCache {
	return &ProvisionCache{
		cache: make(map[string]string),
	}
}

// Get retrieves the worktree path for a given session ID.
// Returns an empty string if the session ID is not in the cache.
//
// This method is thread-safe and uses a read lock to allow
// concurrent reads without blocking.
//
// Parameters:
//   - sessionID: The session UUID or fallback ID
//
// Returns:
//   - string: Absolute path to the worktree, or empty string if not cached
//
// Performance: O(1) map lookup, <1μs typical
func (c *ProvisionCache) Get(sessionID string) string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.cache[sessionID]
}

// Set stores the worktree path for a given session ID in the cache.
// If an entry already exists for this session ID, it is overwritten.
//
// This method is thread-safe and uses a write lock to ensure
// exclusive access during updates.
//
// Parameters:
//   - sessionID: The session UUID or fallback ID
//   - path: Absolute path to the worktree directory
//
// Performance: O(1) map insertion, <1μs typical
func (c *ProvisionCache) Set(sessionID, path string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.cache[sessionID] = path
}

// Clear removes all entries from the cache.
// This is primarily useful for testing or cleanup scenarios.
func (c *ProvisionCache) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.cache = make(map[string]string)
}

// Size returns the number of entries currently in the cache.
// This is primarily useful for testing and debugging.
func (c *ProvisionCache) Size() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c.cache)
}
