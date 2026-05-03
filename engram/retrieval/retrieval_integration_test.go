package retrieval

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

// TestRetrieval_WithConfig_Integration verifies retrieval service respects config settings
// This is an integration test covering retrieval + config interaction
func TestRetrieval_WithConfig_Integration(t *testing.T) {
	// Skip if no API key (API ranking optional)
	apiKey := os.Getenv("ANTHROPIC_API_KEY")
	if apiKey == "" {
		t.Skip("Skipping integration test: ANTHROPIC_API_KEY not set")
	}

	// Create temporary directory with test engrams
	tmpDir := t.TempDir()

	// Create test engrams
	testEngrams := []struct {
		filename string
		content  string
	}{
		{
			filename: "go-testing.ai.md",
			content: `---
type: pattern
title: Go Testing Best Practices
description: Best practices for testing in Go
tags:
  - go
  - testing
agents:
  - claude-code
---
# Go Testing

Use table-driven tests for comprehensive coverage.
`,
		},
		{
			filename: "python-testing.ai.md",
			content: `---
type: pattern
title: Python Testing with Pytest
description: Testing patterns for Python
tags:
  - python
  - testing
agents:
  - claude-code
---
# Python Testing

Use pytest fixtures for test setup.
`,
		},
		{
			filename: "go-errors.ai.md",
			content: `---
type: pattern
title: Go Error Handling
description: Error handling patterns in Go
tags:
  - go
  - errors
agents:
  - cursor
---
# Error Handling

Always wrap errors with context.
`,
		},
	}

	// Write test engrams to temp directory
	for _, te := range testEngrams {
		path := filepath.Join(tmpDir, te.filename)
		if err := os.WriteFile(path, []byte(te.content), 0644); err != nil {
			t.Fatalf("failed to write test engram %s: %v", te.filename, err)
		}
	}

	// Test 1: Custom search paths
	t.Run("custom_search_paths", func(t *testing.T) {
		service := NewService()

		// Search with custom path (absolute)
		results, err := service.Search(context.Background(), SearchOptions{
			EngramPath: tmpDir,
			Query:      "testing",
			UseAPI:     false,
			Limit:      10,
		})

		if err != nil {
			t.Fatalf("Search() failed: %v", err)
		}

		if len(results) == 0 {
			t.Fatal("Search() returned no results from custom path")
		}

		// Verify results are from our temp directory
		for _, r := range results {
			if !filepath.IsAbs(r.Path) {
				t.Errorf("Result path %q is not absolute", r.Path)
			}
			if !filepath.HasPrefix(r.Path, tmpDir) {
				t.Errorf("Result path %q is not in temp directory %q", r.Path, tmpDir)
			}
		}
	})

	// Test 2: Token budget limits (via result limit)
	t.Run("token_budget_limits", func(t *testing.T) {
		service := NewService()

		// Search with strict limit
		results, err := service.Search(context.Background(), SearchOptions{
			EngramPath: tmpDir,
			Query:      "testing",
			UseAPI:     false,
			Limit:      1, // Only 1 result
		})

		if err != nil {
			t.Fatalf("Search() failed: %v", err)
		}

		if len(results) != 1 {
			t.Errorf("Search() returned %d results, want 1 (limit respected)", len(results))
		}

		// Search with higher limit
		results2, err := service.Search(context.Background(), SearchOptions{
			EngramPath: tmpDir,
			Query:      "testing",
			UseAPI:     false,
			Limit:      10,
		})

		if err != nil {
			t.Fatalf("Search() failed: %v", err)
		}

		if len(results2) <= len(results) {
			t.Errorf("Search() with higher limit returned %d results, want > %d", len(results2), len(results))
		}
	})

	// Test 3: Config overrides (tag filtering)
	t.Run("config_overrides", func(t *testing.T) {
		service := NewService()

		// Search without tag filter
		resultsAll, err := service.Search(context.Background(), SearchOptions{
			EngramPath: tmpDir,
			Query:      "testing",
			UseAPI:     false,
		})

		if err != nil {
			t.Fatalf("Search() without filter failed: %v", err)
		}

		// Search with tag filter (go only)
		resultsGo, err := service.Search(context.Background(), SearchOptions{
			EngramPath: tmpDir,
			Query:      "testing",
			Tags:       []string{"go"},
			UseAPI:     false,
		})

		if err != nil {
			t.Fatalf("Search() with tag filter failed: %v", err)
		}

		// Go filter should return fewer results
		if len(resultsGo) >= len(resultsAll) {
			t.Errorf("Tag filter returned %d results, want < %d (unfiltered)", len(resultsGo), len(resultsAll))
		}

		// Verify all results have go tag
		for _, r := range resultsGo {
			hasGoTag := false
			for _, tag := range r.Engram.Frontmatter.Tags {
				if tag == "go" {
					hasGoTag = true
					break
				}
			}
			if !hasGoTag {
				t.Errorf("Result %q does not have 'go' tag", r.Engram.Frontmatter.Title)
			}
		}
	})
}
