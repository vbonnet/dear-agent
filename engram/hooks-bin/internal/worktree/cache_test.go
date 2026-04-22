package worktree

import (
	"sync"
	"testing"
)

func TestProvisionCache_GetSet(t *testing.T) {
	cache := NewProvisionCache()

	// Initially empty
	path := cache.Get("session-1")
	if path != "" {
		t.Errorf("Expected empty string for missing key, got %q", path)
	}

	// Set a value
	cache.Set("session-1", "/path/to/worktree-1")

	// Get should return the value
	path = cache.Get("session-1")
	if path != "/path/to/worktree-1" {
		t.Errorf("Get() = %q, want %q", path, "/path/to/worktree-1")
	}
}

func TestProvisionCache_Overwrite(t *testing.T) {
	cache := NewProvisionCache()

	// Set initial value
	cache.Set("session-1", "/path/old")

	// Overwrite with new value
	cache.Set("session-1", "/path/new")

	// Should return new value
	path := cache.Get("session-1")
	if path != "/path/new" {
		t.Errorf("Expected overwritten value %q, got %q", "/path/new", path)
	}
}

func TestProvisionCache_MultipleEntries(t *testing.T) {
	cache := NewProvisionCache()

	// Set multiple entries
	cache.Set("session-1", "/worktree-1")
	cache.Set("session-2", "/worktree-2")
	cache.Set("session-3", "/worktree-3")

	// Verify all entries
	tests := []struct {
		sessionID    string
		expectedPath string
	}{
		{"session-1", "/worktree-1"},
		{"session-2", "/worktree-2"},
		{"session-3", "/worktree-3"},
	}

	for _, tt := range tests {
		path := cache.Get(tt.sessionID)
		if path != tt.expectedPath {
			t.Errorf("Get(%q) = %q, want %q", tt.sessionID, path, tt.expectedPath)
		}
	}
}

func TestProvisionCache_Size(t *testing.T) {
	cache := NewProvisionCache()

	// Initially empty
	if cache.Size() != 0 {
		t.Errorf("Expected size 0, got %d", cache.Size())
	}

	// Add entries
	cache.Set("session-1", "/path-1")
	if cache.Size() != 1 {
		t.Errorf("Expected size 1, got %d", cache.Size())
	}

	cache.Set("session-2", "/path-2")
	if cache.Size() != 2 {
		t.Errorf("Expected size 2, got %d", cache.Size())
	}

	// Overwriting doesn't increase size
	cache.Set("session-1", "/path-1-new")
	if cache.Size() != 2 {
		t.Errorf("Expected size 2 after overwrite, got %d", cache.Size())
	}
}

func TestProvisionCache_Clear(t *testing.T) {
	cache := NewProvisionCache()

	// Add entries
	cache.Set("session-1", "/path-1")
	cache.Set("session-2", "/path-2")

	if cache.Size() != 2 {
		t.Errorf("Expected size 2 before clear, got %d", cache.Size())
	}

	// Clear
	cache.Clear()

	// Should be empty
	if cache.Size() != 0 {
		t.Errorf("Expected size 0 after clear, got %d", cache.Size())
	}

	// Previously set values should be gone
	if cache.Get("session-1") != "" {
		t.Error("Expected empty value after clear")
	}
}

func TestProvisionCache_ConcurrentAccess(t *testing.T) {
	cache := NewProvisionCache()
	const numGoroutines = 100
	const numOperations = 1000

	var wg sync.WaitGroup
	wg.Add(numGoroutines)

	// Launch concurrent readers and writers
	for i := 0; i < numGoroutines; i++ {
		go func(id int) {
			defer wg.Done()

			sessionID := "session-concurrent"
			path := "/path/concurrent"

			for j := 0; j < numOperations; j++ {
				// Mix of reads and writes
				if j%2 == 0 {
					cache.Set(sessionID, path)
				} else {
					cache.Get(sessionID)
				}
			}
		}(i)
	}

	wg.Wait()

	// Verify cache is still functional
	cache.Set("test", "/test-path")
	if cache.Get("test") != "/test-path" {
		t.Error("Cache corrupted after concurrent access")
	}
}

func TestProvisionCache_EmptySessionID(t *testing.T) {
	cache := NewProvisionCache()

	// Empty session ID should work (though not recommended)
	cache.Set("", "/empty-session-path")

	path := cache.Get("")
	if path != "/empty-session-path" {
		t.Errorf("Get(\"\") = %q, want %q", path, "/empty-session-path")
	}
}

func TestProvisionCache_LongSessionID(t *testing.T) {
	cache := NewProvisionCache()

	// Very long session ID (UUID + fallback format)
	longID := "session-abc123de-f456-7890-1234-567890abcdef-auto-1234567890-abcd"
	longPath := "/very/long/path/to/worktree/with/many/segments"

	cache.Set(longID, longPath)

	path := cache.Get(longID)
	if path != longPath {
		t.Errorf("Get() = %q, want %q", path, longPath)
	}
}

func TestProvisionCache_NonExistentKey(t *testing.T) {
	cache := NewProvisionCache()

	// Populate with some data
	cache.Set("existing-session", "/path")

	// Query non-existent key
	path := cache.Get("nonexistent-session")
	if path != "" {
		t.Errorf("Expected empty string for nonexistent key, got %q", path)
	}
}

// Benchmark cache get operation
func BenchmarkProvisionCache_Get(b *testing.B) {
	cache := NewProvisionCache()
	cache.Set("bench-session", "/bench/worktree")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		cache.Get("bench-session")
	}
}

// Benchmark cache set operation
func BenchmarkProvisionCache_Set(b *testing.B) {
	cache := NewProvisionCache()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		cache.Set("bench-session", "/bench/worktree")
	}
}

// Benchmark concurrent cache access
func BenchmarkProvisionCache_ConcurrentGet(b *testing.B) {
	cache := NewProvisionCache()
	cache.Set("bench-session", "/bench/worktree")

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			cache.Get("bench-session")
		}
	})
}

func BenchmarkProvisionCache_ConcurrentSet(b *testing.B) {
	cache := NewProvisionCache()

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			cache.Set("bench-session", "/bench/worktree")
		}
	})
}
