package ecphory

import (
	"sync"
	"testing"

	"github.com/vbonnet/dear-agent/internal/testutil"
)

// TestIndex_ConcurrentAccess tests that Index is safe for concurrent access
func TestIndex_ConcurrentAccess(t *testing.T) {
	tmpdir := testutil.CreateTestEngramDir(t)

	idx := NewIndex()
	err := idx.Build(tmpdir)
	if err != nil {
		t.Fatalf("Build() failed: %v", err)
	}

	// Run concurrent operations
	var wg sync.WaitGroup
	iterations := 100

	// Concurrent FilterByTags
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < iterations; j++ {
				_ = idx.FilterByTags([]string{"go"})
			}
		}()
	}

	// Concurrent FilterByAgent
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < iterations; j++ {
				_ = idx.FilterByAgent("claude-code")
			}
		}()
	}

	// Concurrent FilterByType
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < iterations; j++ {
				_ = idx.FilterByType("pattern")
			}
		}()
	}

	// Concurrent All()
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < iterations; j++ {
				_ = idx.All()
			}
		}()
	}

	wg.Wait()
	// If we get here without data races, test passes
}

// TestIndex_FilterByAgent_Performance tests that FilterByAgent is fast (P0-5)
func TestIndex_FilterByAgent_Performance(t *testing.T) {
	tmpdir := testutil.CreateTestEngramDir(t)

	idx := NewIndex()
	err := idx.Build(tmpdir)
	if err != nil {
		t.Fatalf("Build() failed: %v", err)
	}

	// Run multiple iterations to get average
	iterations := 1000
	var totalDuration int64

	for i := 0; i < iterations; i++ {
		start := testutil.NowNano()
		_ = idx.FilterByAgent("claude-code")
		duration := testutil.NowNano() - start
		totalDuration += duration
	}

	avgDurationMs := float64(totalDuration) / float64(iterations) / 1e6

	// Should be well under 50ms (target from P0-5)
	// With pre-indexed agent-agnostic engrams, should be <1ms
	if avgDurationMs > 50.0 {
		t.Errorf("FilterByAgent average duration %.2fms exceeds 50ms target", avgDurationMs)
	}

	// Log performance for visibility
	t.Logf("FilterByAgent average duration: %.3fms", avgDurationMs)
}

// TestIndex_AgentAgnostic_Cached tests that agent-agnostic engrams are cached
func TestIndex_AgentAgnostic_Cached(t *testing.T) {
	tmpdir := testutil.CreateTestEngramDir(t)

	idx := NewIndex()
	err := idx.Build(tmpdir)
	if err != nil {
		t.Fatalf("Build() failed: %v", err)
	}

	// Verify agentAgnostic cache is populated
	idx.mu.RLock()
	agnosticCount := len(idx.agentAgnostic)
	idx.mu.RUnlock()

	if agnosticCount < 1 {
		t.Errorf("Expected at least 1 agent-agnostic engram, got %d", agnosticCount)
	}

	// FilterByAgent should include cached agnostic engrams
	results := idx.FilterByAgent("claude-code")

	// Should have agent-specific + agent-agnostic
	if len(results) < agnosticCount {
		t.Errorf("FilterByAgent should include at least %d agnostic engrams, got %d total",
			agnosticCount, len(results))
	}
}

// TestIndex_ConcurrentBuild tests building index while reading
// This should NOT be done in practice, but we test defensive behavior
func TestIndex_ConcurrentReadDuringBuild(t *testing.T) {
	tmpdir := testutil.CreateTestEngramDir(t)

	idx := NewIndex()

	var wg sync.WaitGroup

	// Start build in background
	wg.Add(1)
	go func() {
		defer wg.Done()
		idx.Build(tmpdir)
	}()

	// Try to read while building (should not panic)
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 100; i++ {
			_ = idx.All()
			_ = idx.FilterByTags([]string{"go"})
		}
	}()

	wg.Wait()
	// Test passes if no panic/deadlock occurs
}
