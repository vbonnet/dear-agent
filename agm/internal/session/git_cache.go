package session

import (
	"sync"
	"time"
)

// gitCacheEntry holds cached git operation results with TTL
type gitCacheEntry struct {
	value     interface{}
	expiresAt time.Time
}

// GitCache provides thread-safe caching for git operations with 5-second TTL
type GitCache struct {
	mu      sync.RWMutex
	entries map[string]*gitCacheEntry
	ttl     time.Duration
}

// NewGitCache creates a new git cache with 5-second TTL
func NewGitCache() *GitCache {
	return &GitCache{
		entries: make(map[string]*gitCacheEntry),
		ttl:     5 * time.Second,
	}
}

// Get retrieves a cached value if it exists and is not expired
func (c *GitCache) Get(key string) (interface{}, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	entry, exists := c.entries[key]
	if !exists {
		return nil, false
	}

	// Check if expired
	if time.Now().After(entry.expiresAt) {
		return nil, false
	}

	return entry.value, true
}

// Set stores a value in the cache with TTL
func (c *GitCache) Set(key string, value interface{}) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.entries[key] = &gitCacheEntry{
		value:     value,
		expiresAt: time.Now().Add(c.ttl),
	}
}

// Clear removes all entries from the cache
func (c *GitCache) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.entries = make(map[string]*gitCacheEntry)
}

// CleanExpired removes expired entries from the cache
func (c *GitCache) CleanExpired() {
	c.mu.Lock()
	defer c.mu.Unlock()

	now := time.Now()
	for key, entry := range c.entries {
		if now.After(entry.expiresAt) {
			delete(c.entries, key)
		}
	}
}

// Global cache instance for git operations
var globalGitCache = NewGitCache()

// GetCurrentBranchCached returns the current git branch with caching
func GetCurrentBranchCached(dir string) (string, error) {
	cacheKey := "branch:" + dir

	// Try cache first
	if cached, ok := globalGitCache.Get(cacheKey); ok {
		if branch, ok := cached.(string); ok {
			return branch, nil
		}
	}

	// Cache miss - call original function
	branch, err := getCurrentBranch(dir)
	if err != nil {
		return "", err
	}

	// Store in cache
	globalGitCache.Set(cacheKey, branch)

	return branch, nil
}

// GetUncommittedCountCached returns the uncommitted count with caching
func GetUncommittedCountCached(dir string) (int, error) {
	cacheKey := "uncommitted:" + dir

	// Try cache first
	if cached, ok := globalGitCache.Get(cacheKey); ok {
		if count, ok := cached.(int); ok {
			return count, nil
		}
	}

	// Cache miss - call original function
	count, err := getUncommittedCount(dir)
	if err != nil {
		return 0, err
	}

	// Store in cache
	globalGitCache.Set(cacheKey, count)

	return count, nil
}

// ClearGitCache clears all cached git operation results
func ClearGitCache() {
	globalGitCache.Clear()
}
