package ecphory

import (
	"context"
	"testing"
	"time"

	"github.com/vbonnet/dear-agent/internal/testutil"
)

// TestEcphory_Query_ContextCancellation tests context cancellation handling
func TestEcphory_Query_ContextCancellation(t *testing.T) {
	tmpdir := testutil.CreateTestEngramDir(t)

	// Create minimal ecphory for testing (without ranker to avoid API key requirement)
	idx := NewIndex()
	idx.Build(tmpdir)

	ecphory := &Ecphory{
		index:       idx,
		ranker:      nil, // Will cause Query to fail gracefully
		parser:      nil,
		tokenBudget: 10000,
	}

	// Test with already-cancelled context
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	_, err := ecphory.Query(ctx, "test query", "test-session-1", "test transcript", []string{"go"}, "claude-code")
	if err == nil {
		t.Error("Query should fail with cancelled context")
	}
	if err != nil && err.Error() != "context cancelled: context canceled" {
		t.Logf("Got error: %v", err)
	}
}

// TestEcphory_Query_ContextTimeout tests context timeout handling
func TestEcphory_Query_ContextTimeout(t *testing.T) {
	tmpdir := testutil.CreateTestEngramDir(t)

	idx := NewIndex()
	idx.Build(tmpdir)

	ecphory := &Ecphory{
		index:       idx,
		ranker:      nil,
		parser:      nil,
		tokenBudget: 10000,
	}

	// Test with very short timeout
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Nanosecond)
	defer cancel()

	time.Sleep(10 * time.Millisecond) // Ensure timeout has passed

	_, err := ecphory.Query(ctx, "test query", "test-session-1", "test transcript", []string{"go"}, "claude-code")
	if err == nil {
		t.Error("Query should fail with timeout context")
	}
}

// TestRanker_Rank_ContextCancellation tests that Rank respects context cancellation
func TestRanker_Rank_ContextCancellation(t *testing.T) {
	// Skip if no API key
	if !testutil.HasAnthropicAPIKey() {
		t.Skip("ANTHROPIC_API_KEY not set")
	}

	ranker, err := NewRanker()
	if err != nil {
		t.Fatalf("Failed to create ranker: %v", err)
	}

	// Test with cancelled context
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	candidates := []string{"test1.ai.md", "test2.ai.md"}
	_, err = ranker.Rank(ctx, "test query", candidates)
	if err == nil {
		t.Error("Rank should fail with cancelled context")
	}
}
