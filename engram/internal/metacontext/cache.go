package metacontext

import (
	"context"
	"fmt"
	"log"
	"sync"
	"sync/atomic"

	lru "github.com/hashicorp/golang-lru/v2"
)

// Cache interface defines cache operations for metacontext service.
// Three-tier cache: frontmatter (200), content (100), metacontext (50).
type Cache interface {
	// Frontmatter cache (200 entries)
	GetFrontmatter(ctx context.Context, path string) (*Frontmatter, bool)
	PutFrontmatter(ctx context.Context, path string, fm *Frontmatter) error

	// Content cache (100 entries)
	GetContent(ctx context.Context, path string) (string, bool)
	PutContent(ctx context.Context, path string, content string) error

	// Metacontext cache (50 entries)
	GetMetacontext(ctx context.Context, key string) (*Metacontext, bool)
	PutMetacontext(ctx context.Context, key string, mc *Metacontext) error

	// Global operations
	InvalidateAll(ctx context.Context) error
	InvalidateMetacontext(ctx context.Context) error
	GetCacheStats(ctx context.Context) (*CacheStats, error)
}

// Frontmatter represents parsed frontmatter from .ai.md files.
type Frontmatter struct {
	Tags     []string          `json:"tags"`
	Metadata map[string]string `json:"metadata"`
}

// CacheStats contains cache observability metrics.
type CacheStats struct {
	Hits        int64 `json:"hits"`
	Misses      int64 `json:"misses"`
	Evictions   int64 `json:"evictions"`
	Corruptions int64 `json:"corruptions"`
	Entries     int64 `json:"entries"`
}

// UnifiedCache implements three-tier LRU caching with panic recovery.
// Thread-safe with RWMutex per tier for read-heavy optimization.
type UnifiedCache struct {
	// Frontmatter cache (200 entries)
	frontmatter   *lru.Cache[string, *Frontmatter]
	frontmatterMu sync.RWMutex

	// Content cache (100 entries)
	content   *lru.Cache[string, string]
	contentMu sync.RWMutex

	// Metacontext cache (50 entries)
	metacontext   *lru.Cache[string, *Metacontext]
	metacontextMu sync.RWMutex

	// Metrics (atomic counters)
	hits        atomic.Int64
	misses      atomic.Int64
	evictions   atomic.Int64
	corruptions atomic.Int64
}

// NewUnifiedCache creates a new three-tier cache.
func NewUnifiedCache() (*UnifiedCache, error) {
	frontmatterCache, err := lru.New[string, *Frontmatter](200)
	if err != nil {
		return nil, fmt.Errorf("failed to create frontmatter cache: %w", err)
	}

	contentCache, err := lru.New[string, string](100)
	if err != nil {
		return nil, fmt.Errorf("failed to create content cache: %w", err)
	}

	metacontextCache, err := lru.New[string, *Metacontext](50)
	if err != nil {
		return nil, fmt.Errorf("failed to create metacontext cache: %w", err)
	}

	return &UnifiedCache{
		frontmatter: frontmatterCache,
		content:     contentCache,
		metacontext: metacontextCache,
	}, nil
}

// GetMetacontext retrieves cached metacontext with panic recovery and validation.
// Implements SRE CRITICAL #2: Cache corruption recovery.
func (c *UnifiedCache) GetMetacontext(ctx context.Context, key string) (*Metacontext, bool) {
	c.metacontextMu.RLock()
	defer c.metacontextMu.RUnlock()

	// Panic recovery
	defer func() {
		if r := recover(); r != nil {
			log.Printf("ERROR: Cache panic recovered: %v", r)
			// Emit telemetry (would be implemented with actual telemetry system)
			// telemetry.emit("cache.panic", ...)

			// Emergency purge (async to avoid blocking caller)
			go c.emergencyPurge()
		}
	}()

	mc, ok := c.metacontext.Get(key)
	if !ok {
		c.misses.Add(1)
		return nil, false
	}

	// Validate cached entry
	if err := validateMetacontext(mc); err != nil {
		c.corruptions.Add(1)
		c.metacontext.Remove(key)
		log.Printf("WARN: Cache corruption detected for key %s: %v", key, err)
		return nil, false
	}

	c.hits.Add(1)
	return mc, true
}

// PutMetacontext stores metacontext in cache.
func (c *UnifiedCache) PutMetacontext(ctx context.Context, key string, mc *Metacontext) error {
	c.metacontextMu.Lock()
	defer c.metacontextMu.Unlock()

	// Validate before caching
	if err := validateMetacontext(mc); err != nil {
		return fmt.Errorf("cannot cache invalid metacontext: %w", err)
	}

	evicted := c.metacontext.Add(key, mc)
	if evicted {
		c.evictions.Add(1)
	}

	return nil
}

// GetFrontmatter retrieves cached frontmatter.
func (c *UnifiedCache) GetFrontmatter(ctx context.Context, path string) (*Frontmatter, bool) {
	c.frontmatterMu.RLock()
	defer c.frontmatterMu.RUnlock()

	fm, ok := c.frontmatter.Get(path)
	if !ok {
		c.misses.Add(1)
		return nil, false
	}

	c.hits.Add(1)
	return fm, true
}

// PutFrontmatter stores frontmatter in cache.
func (c *UnifiedCache) PutFrontmatter(ctx context.Context, path string, fm *Frontmatter) error {
	c.frontmatterMu.Lock()
	defer c.frontmatterMu.Unlock()

	evicted := c.frontmatter.Add(path, fm)
	if evicted {
		c.evictions.Add(1)
	}

	return nil
}

// GetContent retrieves cached content.
func (c *UnifiedCache) GetContent(ctx context.Context, path string) (string, bool) {
	c.contentMu.RLock()
	defer c.contentMu.RUnlock()

	content, ok := c.content.Get(path)
	if !ok {
		c.misses.Add(1)
		return "", false
	}

	c.hits.Add(1)
	return content, true
}

// PutContent stores content in cache.
func (c *UnifiedCache) PutContent(ctx context.Context, path string, content string) error {
	c.contentMu.Lock()
	defer c.contentMu.Unlock()

	evicted := c.content.Add(path, content)
	if evicted {
		c.evictions.Add(1)
	}

	return nil
}

// InvalidateAll purges all cache tiers.
func (c *UnifiedCache) InvalidateAll(ctx context.Context) error {
	c.frontmatterMu.Lock()
	c.frontmatter.Purge()
	c.frontmatterMu.Unlock()

	c.contentMu.Lock()
	c.content.Purge()
	c.contentMu.Unlock()

	c.metacontextMu.Lock()
	c.metacontext.Purge()
	c.metacontextMu.Unlock()

	log.Printf("INFO: All cache tiers purged")
	return nil
}

// InvalidateMetacontext purges only metacontext cache.
func (c *UnifiedCache) InvalidateMetacontext(ctx context.Context) error {
	c.metacontextMu.Lock()
	defer c.metacontextMu.Unlock()

	c.metacontext.Purge()
	log.Printf("INFO: Metacontext cache purged")
	return nil
}

// GetCacheStats returns current cache statistics.
func (c *UnifiedCache) GetCacheStats(ctx context.Context) (*CacheStats, error) {
	c.frontmatterMu.RLock()
	frontmatterLen := c.frontmatter.Len()
	c.frontmatterMu.RUnlock()

	c.contentMu.RLock()
	contentLen := c.content.Len()
	c.contentMu.RUnlock()

	c.metacontextMu.RLock()
	metacontextLen := c.metacontext.Len()
	c.metacontextMu.RUnlock()

	return &CacheStats{
		Hits:        c.hits.Load(),
		Misses:      c.misses.Load(),
		Evictions:   c.evictions.Load(),
		Corruptions: c.corruptions.Load(),
		Entries:     int64(frontmatterLen + contentLen + metacontextLen),
	}, nil
}

// emergencyPurge purges all caches in response to panic.
func (c *UnifiedCache) emergencyPurge() {
	c.frontmatterMu.Lock()
	c.frontmatter.Purge()
	c.frontmatterMu.Unlock()

	c.contentMu.Lock()
	c.content.Purge()
	c.contentMu.Unlock()

	c.metacontextMu.Lock()
	c.metacontext.Purge()
	c.metacontextMu.Unlock()

	log.Printf("ERROR: Emergency cache purge completed after panic")
}
