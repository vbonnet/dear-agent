package identity

import (
	"testing"
	"time"
)

// TestCache_GetSet tests basic cache get/set operations
func TestCache_GetSet(t *testing.T) {
	cache := NewCache(1 * time.Hour)

	// Initially empty
	if id := cache.Get(); id != nil {
		t.Error("New cache should be empty")
	}

	// Set identity
	expected := &Identity{
		Email:      "test@example.com",
		Domain:     "@example.com",
		Method:     "test",
		Verified:   true,
		DetectedAt: time.Now(),
	}
	cache.Set(expected)

	// Get should return same identity
	actual := cache.Get()
	if actual == nil {
		t.Fatal("Get() returned nil after Set()")
	}

	if actual.Email != expected.Email {
		t.Errorf("Email = %s, want %s", actual.Email, expected.Email)
	}
	if actual.Domain != expected.Domain {
		t.Errorf("Domain = %s, want %s", actual.Domain, expected.Domain)
	}
	if actual.Method != expected.Method {
		t.Errorf("Method = %s, want %s", actual.Method, expected.Method)
	}
}

// TestCache_Expiry tests TTL expiration
func TestCache_Expiry(t *testing.T) {
	cache := NewCache(100 * time.Millisecond)

	// Set identity
	id := &Identity{
		Email:      "test@example.com",
		Domain:     "@example.com",
		Method:     "test",
		DetectedAt: time.Now(),
	}
	cache.Set(id)

	// Should be valid immediately
	if got := cache.Get(); got == nil {
		t.Error("Cache should not be expired immediately after Set()")
	}

	// Wait for TTL to expire
	time.Sleep(150 * time.Millisecond)

	// Should be expired now
	if got := cache.Get(); got != nil {
		t.Error("Cache should be expired after TTL")
	}
}

// TestCache_Clear tests cache clearing
func TestCache_Clear(t *testing.T) {
	cache := NewCache(1 * time.Hour)

	// Set identity
	id := &Identity{
		Email:      "test@example.com",
		DetectedAt: time.Now(),
	}
	cache.Set(id)

	// Verify cached
	if got := cache.Get(); got == nil {
		t.Fatal("Identity not cached")
	}

	// Clear cache
	cache.Clear()

	// Should be empty after clear
	if got := cache.Get(); got != nil {
		t.Error("Cache should be empty after Clear()")
	}
}

// TestCache_Concurrent tests thread-safety
func TestCache_Concurrent(t *testing.T) {
	cache := NewCache(1 * time.Hour)

	done := make(chan bool)

	// Concurrent writes
	for i := 0; i < 10; i++ {
		go func(n int) {
			id := &Identity{
				Email:      "test@example.com",
				DetectedAt: time.Now(),
			}
			cache.Set(id)
			done <- true
		}(i)
	}

	// Concurrent reads
	for i := 0; i < 10; i++ {
		go func() {
			_ = cache.Get()
			done <- true
		}()
	}

	// Wait for all goroutines
	for i := 0; i < 20; i++ {
		<-done
	}

	// If we reach here without panic, thread-safety works
}

// TestCache_ZeroTTL tests cache with zero TTL (always expired)
func TestCache_ZeroTTL(t *testing.T) {
	cache := NewCache(0)

	id := &Identity{
		Email:      "test@example.com",
		DetectedAt: time.Now(),
	}
	cache.Set(id)

	// Should be expired immediately (TTL = 0)
	if got := cache.Get(); got != nil {
		t.Error("Cache with TTL=0 should always return nil")
	}
}
