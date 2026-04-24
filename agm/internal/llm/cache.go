package llm

import (
	"sync"
	"time"
)

// SearchResult represents a single search result with session ID and relevance
type SearchResult struct {
	SessionID string
	Relevance float64 // 0.0-1.0, higher is more relevant
	Reason    string  // Optional explanation from LLM
}

// CacheEntry holds search results with expiration
type CacheEntry struct {
	Results   []SearchResult
	ExpiresAt time.Time
}

// SearchCache provides in-memory caching for search results with TTL
type SearchCache struct {
	mu      sync.RWMutex
	entries map[string]*CacheEntry
	ttl     time.Duration
}

// NewSearchCache creates a new search cache with the given TTL
func NewSearchCache(ttl time.Duration) *SearchCache {
	return &SearchCache{
		entries: make(map[string]*CacheEntry),
		ttl:     ttl,
	}
}

// Get retrieves cached results for a query if they exist and are fresh
// Returns nil if query not found or results have expired
func (c *SearchCache) Get(query string) []SearchResult {
	c.mu.RLock()
	defer c.mu.RUnlock()

	entry, exists := c.entries[query]
	if !exists {
		return nil
	}

	// Check if expired
	if time.Now().After(entry.ExpiresAt) {
		return nil
	}

	return entry.Results
}

// Set stores search results for a query with TTL expiration
func (c *SearchCache) Set(query string, results []SearchResult) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.entries[query] = &CacheEntry{
		Results:   results,
		ExpiresAt: time.Now().Add(c.ttl),
	}
}

// Clear removes all entries from the cache
func (c *SearchCache) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.entries = make(map[string]*CacheEntry)
}

// CleanExpired removes all expired entries from the cache
// This can be called periodically to prevent unbounded growth
func (c *SearchCache) CleanExpired() int {
	c.mu.Lock()
	defer c.mu.Unlock()

	now := time.Now()
	removed := 0

	for query, entry := range c.entries {
		if now.After(entry.ExpiresAt) {
			delete(c.entries, query)
			removed++
		}
	}

	return removed
}

// Size returns the current number of cached queries
func (c *SearchCache) Size() int {
	c.mu.RLock()
	defer c.mu.RUnlock()

	return len(c.entries)
}
