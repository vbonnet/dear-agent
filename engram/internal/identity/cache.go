package identity

import (
	"sync"
	"time"
)

// Cache provides thread-safe in-memory caching of identity with TTL
type Cache struct {
	mu       sync.RWMutex
	identity *Identity
	ttl      time.Duration
}

// NewCache creates a cache with specified TTL
func NewCache(ttl time.Duration) *Cache {
	return &Cache{
		ttl: ttl,
	}
}

// Get retrieves cached identity if valid, nil if expired or not set
func (c *Cache) Get() *Identity {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if c.identity == nil {
		return nil
	}

	// Check TTL expiry
	if time.Since(c.identity.DetectedAt) > c.ttl {
		return nil // Expired
	}

	return c.identity
}

// Set stores identity in cache
func (c *Cache) Set(id *Identity) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.identity = id
}

// Clear removes cached identity, forcing re-detection
func (c *Cache) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.identity = nil
}
