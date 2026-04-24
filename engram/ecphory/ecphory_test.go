package ecphory

import (
	"testing"

	"github.com/vbonnet/dear-agent/internal/testutil"
)

func TestNewEcphory_ValidPath(t *testing.T) {
	tmpdir := testutil.CreateTestEngramDir(t)

	// Note: NewEcphory requires ANTHROPIC_API_KEY for ranker
	// This test will fail if API key is not set
	// For unit testing, we'd ideally inject a mock ranker, but keeping this simple
	ecphory, err := NewEcphory(tmpdir, 10000)

	if err != nil {
		// If ANTHROPIC_API_KEY or GOOGLE_CLOUD_PROJECT not set, test is expected to fail
		// This is acceptable for unit test - we're testing the path validation logic
		if err.Error() == "failed to create ranker: neither GOOGLE_CLOUD_PROJECT nor ANTHROPIC_API_KEY environment variable set" {
			t.Skip("Skipping test - API key not set (expected for unit tests)")
		}
		t.Fatalf("NewEcphory() failed: %v", err)
	}

	if ecphory == nil {
		t.Fatal("NewEcphory() should return non-nil ecphory")
	}

	if ecphory.tokenBudget != 10000 {
		t.Errorf("token budget should be 10000, got %d", ecphory.tokenBudget)
	}
}

func TestIntersect(t *testing.T) {
	tests := []struct {
		name string
		a    []string
		b    []string
		want []string
	}{
		{
			name: "no intersection",
			a:    []string{"a", "b"},
			b:    []string{"c", "d"},
			want: []string{},
		},
		{
			name: "full intersection",
			a:    []string{"a", "b"},
			b:    []string{"a", "b"},
			want: []string{"a", "b"},
		},
		{
			name: "partial intersection",
			a:    []string{"a", "b", "c"},
			b:    []string{"b", "c", "d"},
			want: []string{"b", "c"},
		},
		{
			name: "empty slices",
			a:    []string{},
			b:    []string{},
			want: []string{},
		},
		{
			name: "first empty",
			a:    []string{},
			b:    []string{"a", "b"},
			want: []string{},
		},
		{
			name: "second empty",
			a:    []string{"a", "b"},
			b:    []string{},
			want: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := intersect(tt.a, tt.b)

			// Check length
			if len(got) != len(tt.want) {
				t.Errorf("intersect() length = %d, want %d", len(got), len(tt.want))
				return
			}

			// For empty results, we're done
			if len(tt.want) == 0 {
				return
			}

			// Check elements (order-independent comparison)
			if !equalSets(got, tt.want) {
				t.Errorf("intersect() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestEcphory_fastFilter(t *testing.T) {
	tmpdir := testutil.CreateTestEngramDir(t)

	// Create minimal ecphory instance without requiring ranker
	idx := NewIndex()
	idx.Build(tmpdir)

	ecphory := &Ecphory{
		index:       idx,
		ranker:      nil, // Don't need ranker for fastFilter test
		parser:      nil,
		tokenBudget: 10000,
	}

	tests := []struct {
		name    string
		tags    []string
		agent   string
		wantMin int // Minimum expected results
	}{
		{"no filters", []string{}, "", 4},                             // All 4 engrams
		{"filter by go tag", []string{"go"}, "", 2},                   // error-handling, table-driven-tests
		{"filter by claude-code agent", []string{}, "claude-code", 4}, // 3 agent-specific + 1 agnostic
		{"filter by tag and agent", []string{"go"}, "claude-code", 2}, // error-handling, table-driven-tests
		{"no matches", []string{"python"}, "", 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			results := ecphory.fastFilter(tt.tags, tt.agent)
			if len(results) < tt.wantMin {
				t.Errorf("fastFilter(tags=%v, agent=%s) got %d results, want at least %d",
					tt.tags, tt.agent, len(results), tt.wantMin)
			}
		})
	}
}

func TestEcphory_fastFilter_EdgeCases(t *testing.T) {
	tmpdir := testutil.CreateTestEngramDir(t)

	idx := NewIndex()
	idx.Build(tmpdir)

	ecphory := &Ecphory{
		index:       idx,
		ranker:      nil,
		parser:      nil,
		tokenBudget: 10000,
	}

	// Test empty filters - should return all
	results := ecphory.fastFilter([]string{}, "")
	if len(results) != 4 {
		t.Errorf("fastFilter with no filters should return all 4 engrams, got %d", len(results))
	}

	// Test non-existent tag - should return empty
	results = ecphory.fastFilter([]string{"nonexistent-tag"}, "")
	if len(results) != 0 {
		t.Errorf("fastFilter with non-existent tag should return 0 results, got %d", len(results))
	}

	// Test non-existent agent - should return only agnostic engrams
	results = ecphory.fastFilter([]string{}, "nonexistent-agent")
	if len(results) < 1 {
		t.Errorf("fastFilter with non-existent agent should return at least 1 agnostic engram, got %d", len(results))
	}
}

func TestEcphory_NewEcphory_WithInvalidPath(t *testing.T) {
	// Test with non-existent directory
	_, err := NewEcphory("/nonexistent/path", 10000)

	if err == nil {
		t.Error("NewEcphory() should return error for invalid path")
	}

	// Error could be either "failed to build index" or "failed to create ranker"
	// Both are acceptable
}

// Helper to compare slices as sets (order-independent)
func equalSets(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	m := make(map[string]bool)
	for _, s := range a {
		m[s] = true
	}
	for _, s := range b {
		if !m[s] {
			return false
		}
	}
	return true
}
