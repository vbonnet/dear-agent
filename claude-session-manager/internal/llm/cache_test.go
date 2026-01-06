package llm

import (
	"testing"
	"time"
)

func TestNewSearchCache(t *testing.T) {
	cache := NewSearchCache(5 * time.Minute)
	if cache == nil {
		t.Fatal("NewSearchCache returned nil")
	}
	if cache.ttl != 5*time.Minute {
		t.Errorf("expected TTL 5m, got %v", cache.ttl)
	}
	if cache.Size() != 0 {
		t.Errorf("expected empty cache, got size %d", cache.Size())
	}
}

func TestSearchCache_SetAndGet(t *testing.T) {
	cache := NewSearchCache(5 * time.Minute)
	query := "test query"
	results := []SearchResult{
		{SessionID: "session-1", Relevance: 0.9, Reason: "match"},
		{SessionID: "session-2", Relevance: 0.7, Reason: "partial"},
	}

	// Set results
	cache.Set(query, results)

	// Get results
	cached := cache.Get(query)
	if cached == nil {
		t.Fatal("Get returned nil for cached query")
	}
	if len(cached) != 2 {
		t.Fatalf("expected 2 results, got %d", len(cached))
	}
	if cached[0].SessionID != "session-1" {
		t.Errorf("expected session-1, got %s", cached[0].SessionID)
	}
	if cached[1].Relevance != 0.7 {
		t.Errorf("expected relevance 0.7, got %f", cached[1].Relevance)
	}
}

func TestSearchCache_GetNonExistent(t *testing.T) {
	cache := NewSearchCache(5 * time.Minute)

	result := cache.Get("nonexistent")
	if result != nil {
		t.Errorf("expected nil for nonexistent query, got %v", result)
	}
}

func TestSearchCache_Expiration(t *testing.T) {
	cache := NewSearchCache(100 * time.Millisecond)
	query := "expiring query"
	results := []SearchResult{{SessionID: "session-1"}}

	cache.Set(query, results)

	// Should be available immediately
	if cache.Get(query) == nil {
		t.Error("expected cached results immediately after set")
	}

	// Wait for expiration
	time.Sleep(150 * time.Millisecond)

	// Should be expired now
	if cache.Get(query) != nil {
		t.Error("expected nil after expiration, cache should have expired")
	}
}

func TestSearchCache_Clear(t *testing.T) {
	cache := NewSearchCache(5 * time.Minute)

	cache.Set("query1", []SearchResult{{SessionID: "s1"}})
	cache.Set("query2", []SearchResult{{SessionID: "s2"}})
	cache.Set("query3", []SearchResult{{SessionID: "s3"}})

	if cache.Size() != 3 {
		t.Errorf("expected size 3, got %d", cache.Size())
	}

	cache.Clear()

	if cache.Size() != 0 {
		t.Errorf("expected size 0 after clear, got %d", cache.Size())
	}
	if cache.Get("query1") != nil {
		t.Error("expected nil after clear")
	}
}

func TestSearchCache_CleanExpired(t *testing.T) {
	cache := NewSearchCache(100 * time.Millisecond)

	// Add some entries
	cache.Set("fresh1", []SearchResult{{SessionID: "s1"}})
	cache.Set("old1", []SearchResult{{SessionID: "s2"}})
	cache.Set("old2", []SearchResult{{SessionID: "s3"}})

	// Wait for some to expire
	time.Sleep(50 * time.Millisecond)

	// Add fresh entry
	cache.Set("fresh2", []SearchResult{{SessionID: "s4"}})

	// Wait for old ones to fully expire
	time.Sleep(60 * time.Millisecond)

	// Clean expired
	removed := cache.CleanExpired()

	// Should have removed 3 old entries (fresh1, old1, old2)
	// fresh2 should still be valid
	if removed != 3 {
		t.Errorf("expected to remove 3 entries, removed %d", removed)
	}
	if cache.Size() != 1 {
		t.Errorf("expected 1 entry remaining, got %d", cache.Size())
	}
	if cache.Get("fresh2") == nil {
		t.Error("expected fresh2 to still be cached")
	}
}

func TestSearchCache_Size(t *testing.T) {
	cache := NewSearchCache(5 * time.Minute)

	if cache.Size() != 0 {
		t.Errorf("expected size 0, got %d", cache.Size())
	}

	cache.Set("q1", []SearchResult{{SessionID: "s1"}})
	if cache.Size() != 1 {
		t.Errorf("expected size 1, got %d", cache.Size())
	}

	cache.Set("q2", []SearchResult{{SessionID: "s2"}})
	cache.Set("q3", []SearchResult{{SessionID: "s3"}})
	if cache.Size() != 3 {
		t.Errorf("expected size 3, got %d", cache.Size())
	}
}

func TestSearchCache_OverwriteExisting(t *testing.T) {
	cache := NewSearchCache(5 * time.Minute)
	query := "test"

	// Set initial value
	cache.Set(query, []SearchResult{{SessionID: "s1"}})
	result1 := cache.Get(query)
	if len(result1) != 1 || result1[0].SessionID != "s1" {
		t.Error("initial set failed")
	}

	// Overwrite
	cache.Set(query, []SearchResult{{SessionID: "s2"}, {SessionID: "s3"}})
	result2 := cache.Get(query)
	if len(result2) != 2 {
		t.Errorf("expected 2 results after overwrite, got %d", len(result2))
	}
	if result2[0].SessionID != "s2" {
		t.Errorf("expected s2, got %s", result2[0].SessionID)
	}
}

func TestSearchCache_EmptyResults(t *testing.T) {
	cache := NewSearchCache(5 * time.Minute)
	query := "no results"

	// Set empty results
	cache.Set(query, []SearchResult{})

	// Should still be cached
	result := cache.Get(query)
	if result == nil {
		t.Error("expected empty slice, got nil")
	}
	if len(result) != 0 {
		t.Errorf("expected empty slice, got %d results", len(result))
	}
}
