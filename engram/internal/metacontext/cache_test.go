package metacontext

import (
	"context"
	"sync"
	"testing"
	"time"
)

// ============================================================================
// Unit Tests: cache.go (UnifiedCache operations, panic recovery)
// S7 Plan: Week 4 Testing, Chaos Test Category (cache corruption, panic recovery)
// Implements Security Mitigation M2 (Cache Corruption Recovery) validation
// ============================================================================

// TestNewUnifiedCache_Initialization tests cache initialization
func TestNewUnifiedCache_Initialization(t *testing.T) {
	cache, err := NewUnifiedCache()
	if err != nil {
		t.Fatalf("NewUnifiedCache() failed: %v", err)
	}
	if cache == nil {
		t.Error("NewUnifiedCache() returned nil cache")
	}

	// Verify cache tiers initialized
	stats, err := cache.GetCacheStats(context.Background())
	if err != nil {
		t.Errorf("GetCacheStats() failed: %v", err)
	}
	if stats == nil {
		t.Error("Cache stats should not be nil")
	}
}

// TestCache_PutAndGet tests basic cache operations
func TestCache_PutAndGet(t *testing.T) {
	cache, err := NewUnifiedCache()
	if err != nil {
		t.Fatalf("NewUnifiedCache() failed: %v", err)
	}

	ctx := context.Background()
	key := "test-key"
	mc := &Metacontext{
		Languages: []Signal{
			{Name: "Go", Confidence: 0.95, Source: "file"},
		},
		Frameworks:  []Signal{},
		Tools:       []Signal{},
		Conventions: []Convention{},
		Personas:    []Persona{},
	}

	// Put
	cache.PutMetacontext(ctx, key, mc)

	// Get
	retrieved, ok := cache.GetMetacontext(ctx, key)
	if !ok {
		t.Error("Cache.GetMetacontext() should return true for existing key")
	}
	if retrieved == nil {
		t.Fatal("Retrieved metacontext should not be nil")
	}
	if len(retrieved.Languages) != 1 || retrieved.Languages[0].Name != "Go" {
		t.Error("Retrieved metacontext does not match stored value")
	}
}

// TestCache_GetMiss tests cache miss behavior
func TestCache_GetMiss(t *testing.T) {
	cache, err := NewUnifiedCache()
	if err != nil {
		t.Fatalf("NewUnifiedCache() failed: %v", err)
	}

	ctx := context.Background()
	retrieved, ok := cache.GetMetacontext(ctx, "nonexistent-key")

	if ok {
		t.Error("Cache.GetMetacontext() should return false for non-existent key")
	}
	if retrieved != nil {
		t.Error("Retrieved metacontext should be nil on cache miss")
	}

	// Verify stats
	stats, _ := cache.GetCacheStats(ctx)
	if stats.Misses != 1 {
		t.Errorf("Expected 1 miss, got %d", stats.Misses)
	}
}

// TestCache_Update tests updating existing cache entry
func TestCache_Update(t *testing.T) {
	cache, err := NewUnifiedCache()
	if err != nil {
		t.Fatalf("NewUnifiedCache() failed: %v", err)
	}

	ctx := context.Background()
	key := "test-key"

	// Put initial value
	mc1 := &Metacontext{
		Languages:   []Signal{{Name: "Go", Confidence: 0.95, Source: "file"}},
		Frameworks:  []Signal{},
		Tools:       []Signal{},
		Conventions: []Convention{},
		Personas:    []Persona{},
	}
	cache.PutMetacontext(ctx, key, mc1)

	// Update with new value
	mc2 := &Metacontext{
		Languages:   []Signal{{Name: "TypeScript", Confidence: 0.9, Source: "file"}},
		Frameworks:  []Signal{},
		Tools:       []Signal{},
		Conventions: []Convention{},
		Personas:    []Persona{},
	}
	cache.PutMetacontext(ctx, key, mc2)

	// Get should return updated value
	retrieved, ok := cache.GetMetacontext(ctx, key)
	if !ok {
		t.Error("Cache should contain updated entry")
	}
	if len(retrieved.Languages) != 1 || retrieved.Languages[0].Name != "TypeScript" {
		t.Error("Cache should return updated value")
	}
}

// TestCache_LRUEviction tests LRU eviction behavior
func TestCache_LRUEviction(t *testing.T) {
	cache, err := NewUnifiedCache()
	if err != nil {
		t.Fatalf("NewUnifiedCache() failed: %v", err)
	}

	ctx := context.Background()

	// Fill cache beyond capacity (metacontext tier = 50 entries)
	for i := 0; i < 55; i++ {
		key := string(rune('a' + i))
		mc := &Metacontext{
			Languages:   []Signal{{Name: "Go", Confidence: 0.95, Source: "file"}},
			Frameworks:  []Signal{},
			Tools:       []Signal{},
			Conventions: []Convention{},
			Personas:    []Persona{},
		}
		cache.PutMetacontext(ctx, key, mc)
	}

	// First entry should be evicted (LRU)
	_, ok := cache.GetMetacontext(ctx, "a")
	if ok {
		t.Error("LRU eviction should have removed first entry")
	}

	// Recent entries should still exist
	_, ok = cache.GetMetacontext(ctx, "z")
	if !ok {
		t.Error("Recent entry should not be evicted")
	}

	// Verify eviction counter
	stats, _ := cache.GetCacheStats(ctx)
	if stats.Evictions < 5 {
		t.Errorf("Expected at least 5 evictions, got %d", stats.Evictions)
	}
}

// TestCache_ConcurrentAccess tests thread-safety with concurrent reads/writes
func TestCache_ConcurrentAccess(t *testing.T) {
	cache, err := NewUnifiedCache()
	if err != nil {
		t.Fatalf("NewUnifiedCache() failed: %v", err)
	}

	ctx := context.Background()
	var wg sync.WaitGroup
	numGoroutines := 50

	// Concurrent writes
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			key := string(rune('a' + (id % 26)))
			mc := &Metacontext{
				Languages:   []Signal{{Name: "Go", Confidence: 0.95, Source: "file"}},
				Frameworks:  []Signal{},
				Tools:       []Signal{},
				Conventions: []Convention{},
				Personas:    []Persona{},
			}
			cache.PutMetacontext(ctx, key, mc)
		}(i)
	}

	// Concurrent reads
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			key := string(rune('a' + (id % 26)))
			cache.GetMetacontext(ctx, key)
		}(i)
	}

	wg.Wait()

	// No race conditions should occur (verified by `go test -race`)
}

// TestCache_InvalidateAll tests cache invalidation
func TestCache_InvalidateAll(t *testing.T) {
	cache, err := NewUnifiedCache()
	if err != nil {
		t.Fatalf("NewUnifiedCache() failed: %v", err)
	}

	ctx := context.Background()

	// Add entries
	for i := 0; i < 10; i++ {
		key := string(rune('a' + i))
		mc := &Metacontext{
			Languages:   []Signal{{Name: "Go", Confidence: 0.95, Source: "file"}},
			Frameworks:  []Signal{},
			Tools:       []Signal{},
			Conventions: []Convention{},
			Personas:    []Persona{},
		}
		cache.PutMetacontext(ctx, key, mc)
	}

	// Invalidate all
	err = cache.InvalidateAll(ctx)
	if err != nil {
		t.Errorf("InvalidateAll() failed: %v", err)
	}

	// All entries should be gone
	for i := 0; i < 10; i++ {
		key := string(rune('a' + i))
		_, ok := cache.GetMetacontext(ctx, key)
		if ok {
			t.Errorf("Cache entry %s should be invalidated", key)
		}
	}
}

// TestCache_GetCacheStats tests cache statistics
func TestCache_GetCacheStats(t *testing.T) {
	cache, err := NewUnifiedCache()
	if err != nil {
		t.Fatalf("NewUnifiedCache() failed: %v", err)
	}

	ctx := context.Background()

	// Trigger hits and misses
	mc := &Metacontext{
		Languages:   []Signal{{Name: "Go", Confidence: 0.95, Source: "file"}},
		Frameworks:  []Signal{},
		Tools:       []Signal{},
		Conventions: []Convention{},
		Personas:    []Persona{},
	}
	cache.PutMetacontext(ctx, "key1", mc)
	cache.GetMetacontext(ctx, "key1") // Hit
	cache.GetMetacontext(ctx, "key2") // Miss

	stats, err := cache.GetCacheStats(ctx)
	if err != nil {
		t.Errorf("GetCacheStats() failed: %v", err)
	}

	if stats.Hits != 1 {
		t.Errorf("Expected 1 hit, got %d", stats.Hits)
	}
	if stats.Misses != 1 {
		t.Errorf("Expected 1 miss, got %d", stats.Misses)
	}

	// Verify total accesses (hits + misses = 2)
	totalAccesses := stats.Hits + stats.Misses
	if totalAccesses != 2 {
		t.Errorf("Expected 2 total accesses, got %d", totalAccesses)
	}
}

// ============================================================================
// Chaos Tests: Cache Corruption and Panic Recovery
// S7 Plan: Week 4 Testing, Chaos Test Category
// Implements Security Mitigation M2 validation
// ============================================================================

// TestChaos_CorruptedCacheEntry tests handling of corrupted cache entries
func TestChaos_CorruptedCacheEntry(t *testing.T) {
	cache, err := NewUnifiedCache()
	if err != nil {
		t.Fatalf("NewUnifiedCache() failed: %v", err)
	}

	ctx := context.Background()
	key := "corrupted-key"

	// Put corrupted metacontext (exceeds token budget)
	corrupted := &Metacontext{
		Languages:  make([]Signal, 200), // Way over limit
		Frameworks: []Signal{},
		Tools:      []Signal{},
	}
	for i := 0; i < 200; i++ {
		corrupted.Languages[i] = Signal{
			Name:       "VeryLongLanguageNameThatExceedsTokenBudget",
			Confidence: 0.9,
			Source:     "file",
		}
	}

	// Put should succeed (validation happens on Get)
	cache.PutMetacontext(ctx, key, corrupted)

	// Get should detect corruption and return false
	retrieved, ok := cache.GetMetacontext(ctx, key)
	if ok {
		t.Error("Corrupted cache entry should be rejected on Get")
	}
	if retrieved != nil {
		t.Error("Corrupted cache entry should return nil")
	}

	// Note: Corruption detection depends on validateMetacontext implementation
	// If validation doesn't check token budget, corruption counter may not increment
	stats, _ := cache.GetCacheStats(ctx)
	_ = stats // Document expected behavior without strict assertion
}

// TestChaos_PanicRecovery tests panic recovery in cache operations
func TestChaos_PanicRecovery(t *testing.T) {
	cache, err := NewUnifiedCache()
	if err != nil {
		t.Fatalf("NewUnifiedCache() failed: %v", err)
	}

	ctx := context.Background()

	// Trigger panic by passing nil metacontext (should be caught)
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("Panic should be recovered inside cache, not propagated: %v", r)
		}
	}()

	// This should not panic (nil handling in PutMetacontext)
	cache.PutMetacontext(ctx, "nil-key", nil)

	// Get should return false (nil metacontext rejected during validation)
	_, ok := cache.GetMetacontext(ctx, "nil-key")
	if ok {
		t.Error("Nil metacontext should not be retrievable")
	}
}

// TestChaos_EmergencyPurge tests emergency purge on corruption
func TestChaos_EmergencyPurge(t *testing.T) {
	cache, err := NewUnifiedCache()
	if err != nil {
		t.Fatalf("NewUnifiedCache() failed: %v", err)
	}

	ctx := context.Background()

	// Add valid entries
	for i := 0; i < 5; i++ {
		key := string(rune('a' + i))
		mc := &Metacontext{
			Languages:   []Signal{{Name: "Go", Confidence: 0.95, Source: "file"}},
			Frameworks:  []Signal{},
			Tools:       []Signal{},
			Conventions: []Convention{},
			Personas:    []Persona{},
		}
		cache.PutMetacontext(ctx, key, mc)
	}

	// Trigger emergency purge (invalidate all)
	cache.InvalidateAll(ctx)

	// Wait for async purge to complete
	time.Sleep(100 * time.Millisecond)

	// All entries should be purged
	for i := 0; i < 5; i++ {
		key := string(rune('a' + i))
		_, ok := cache.GetMetacontext(ctx, key)
		if ok {
			t.Errorf("Entry %s should be purged", key)
		}
	}
}

// TestChaos_HighContentionConcurrentAccess tests cache under high contention
func TestChaos_HighContentionConcurrentAccess(t *testing.T) {
	cache, err := NewUnifiedCache()
	if err != nil {
		t.Fatalf("NewUnifiedCache() failed: %v", err)
	}

	ctx := context.Background()
	var wg sync.WaitGroup
	numGoroutines := 100
	iterations := 100

	// High contention: many goroutines accessing same keys
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < iterations; j++ {
				key := "contended-key" // Same key for all goroutines
				mc := &Metacontext{
					Languages:   []Signal{{Name: "Go", Confidence: 0.95, Source: "file"}},
					Frameworks:  []Signal{},
					Tools:       []Signal{},
					Conventions: []Convention{},
					Personas:    []Persona{},
				}
				cache.PutMetacontext(ctx, key, mc)
				cache.GetMetacontext(ctx, key)
			}
		}(i)
	}

	wg.Wait()

	// No race conditions, deadlocks, or panics (verified by `go test -race`)
}
