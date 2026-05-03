package ecphory

import (
	"testing"

	"github.com/vbonnet/dear-agent/internal/testutil"
)

func TestIndex_Build_ValidDirectory(t *testing.T) {
	tmpdir := testutil.CreateTestEngramDir(t)

	idx := NewIndex()
	err := idx.Build(tmpdir)

	if err != nil {
		t.Fatalf("Build() failed: %v", err)
	}

	// Verify index populated
	all := idx.All()
	if len(all) < 4 {
		t.Errorf("Index should contain at least 4 engrams, got %d", len(all))
	}
}

func TestIndex_Build_EmptyDirectory(t *testing.T) {
	tmpdir := t.TempDir()

	idx := NewIndex()
	err := idx.Build(tmpdir)

	if err != nil {
		t.Fatalf("Build() should succeed on empty directory, got: %v", err)
	}

	all := idx.All()
	if len(all) != 0 {
		t.Errorf("Empty directory should have 0 engrams, got %d", len(all))
	}
}

func TestIndex_All(t *testing.T) {
	tmpdir := testutil.CreateTestEngramDir(t)

	idx := NewIndex()
	idx.Build(tmpdir)

	all := idx.All()

	// Should have 4 test engrams
	if len(all) != 4 {
		t.Errorf("All() should return 4 engrams, got %d", len(all))
	}
}

func TestIndex_FilterByTags(t *testing.T) {
	tmpdir := testutil.CreateTestEngramDir(t)

	idx := NewIndex()
	idx.Build(tmpdir)

	tests := []struct {
		name    string
		tags    []string
		wantMin int // Minimum expected results
	}{
		{"single tag - go", []string{"go"}, 2},
		{"single tag - markdown", []string{"markdown"}, 1},
		{"multiple tags", []string{"go", "markdown"}, 3},
		{"no matches", []string{"python"}, 0},
		{"tag prefix - patterns", []string{"patterns"}, 2},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			results := idx.FilterByTags(tt.tags)
			if len(results) < tt.wantMin {
				t.Errorf("FilterByTags(%v) got %d results, want at least %d", tt.tags, len(results), tt.wantMin)
			}
		})
	}
}

func TestIndex_FilterByTags_HierarchicalMatching(t *testing.T) {
	tmpdir := testutil.CreateTestEngramDir(t)

	idx := NewIndex()
	idx.Build(tmpdir)

	// Filter by "go" should match engrams tagged with "go"
	results := idx.FilterByTags([]string{"go"})

	if len(results) < 2 {
		t.Errorf("FilterByTags(['go']) should return at least 2 results (error-handling, table-driven-tests), got %d", len(results))
	}

	// Verify hierarchical matching works
	// Note: Current implementation does hierarchical prefix matching
	// So "patterns" matches tags like "patterns" or tags that have "patterns" as prefix
	patternsResults := idx.FilterByTags([]string{"patterns"})
	if len(patternsResults) < 2 {
		t.Errorf("FilterByTags(['patterns']) should return at least 2 results, got %d", len(patternsResults))
	}
}

func TestIndex_FilterByType(t *testing.T) {
	tmpdir := testutil.CreateTestEngramDir(t)

	idx := NewIndex()
	idx.Build(tmpdir)

	tests := []struct {
		name    string
		typ     string
		wantMin int
	}{
		{"pattern type", "pattern", 2},
		{"reference type", "reference", 1},
		{"strategy type", "strategy", 1},
		{"nonexistent type", "tutorial", 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			results := idx.FilterByType(tt.typ)
			if len(results) < tt.wantMin {
				t.Errorf("FilterByType(%s) got %d results, want at least %d", tt.typ, len(results), tt.wantMin)
			}
		})
	}
}

func TestIndex_FilterByAgent(t *testing.T) {
	tmpdir := testutil.CreateTestEngramDir(t)

	idx := NewIndex()
	idx.Build(tmpdir)

	// Filter by "claude-code" should return:
	// - Agent-specific engrams (error-handling, table-driven-tests, retrieval)
	// - Agent-agnostic engrams (markdown-formatting)
	results := idx.FilterByAgent("claude-code")

	// Should return all 4 engrams (3 claude-code specific + 1 agnostic)
	if len(results) < 4 {
		t.Errorf("FilterByAgent('claude-code') should return at least 4 results, got %d", len(results))
	}

	// Filter by nonexistent agent should still return agent-agnostic engrams
	results = idx.FilterByAgent("nonexistent-agent")
	if len(results) < 1 {
		t.Errorf("FilterByAgent('nonexistent-agent') should return at least 1 agnostic engram, got %d", len(results))
	}
}
