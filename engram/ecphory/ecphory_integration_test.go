package ecphory

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/vbonnet/dear-agent/pkg/engram"
)

// TestRetrievalWorkflow verifies the full ecphory retrieval pipeline
// This is an integration test covering tier 1 (index), tier 2 (ranking), tier 3 (budget)
// testEngramData holds engram file data for testing
type testEngramData struct {
	filename string
	content  string
}

// getTestEngrams returns standard test engram data
func getTestEngrams() []testEngramData {
	return []testEngramData{
		{
			filename: "go-error-handling.ai.md",
			content: `---
type: pattern
title: Go Error Handling
description: Best practices for error handling in Go
tags:
  - go
  - errors
  - best-practices
agents:
  - claude-code
  - cursor
---
# Go Error Handling

Always wrap errors with context:

` + "```go" + `
if err != nil {
    return fmt.Errorf("failed to process: %w", err)
}
` + "```" + `
`,
		},
		{
			filename: "python-testing.ai.md",
			content: `---
type: pattern
title: Python Testing with Pytest
description: How to write tests using pytest
tags:
  - python
  - testing
  - pytest
agents:
  - claude-code
---
# Python Testing

Use pytest for all tests:

` + "```python" + `
def test_example():
    assert 1 + 1 == 2
` + "```" + `
`,
		},
		{
			filename: "go-concurrency.ai.md",
			content: `---
type: pattern
title: Go Concurrency Patterns
description: Common concurrency patterns in Go
tags:
  - go
  - concurrency
  - goroutines
agents:
  - claude-code
---
# Concurrency Patterns

Use channels for communication:

` + "```go" + `
ch := make(chan int)
go func() {
    ch <- 42
}()
result := <-ch
` + "```" + `
`,
		},
		{
			filename: "typescript-types.ai.md",
			content: `---
type: pattern
title: TypeScript Type Safety
description: Advanced TypeScript type patterns
tags:
  - typescript
  - types
agents:
  - cursor
---
# TypeScript Types

Use strict null checks:

` + "```typescript" + `
type Maybe<T> = T | null
` + "```" + `
`,
		},
	}
}

// setupTestEcphory creates temp dir with test engrams and returns ecphory instance
func setupTestEcphory(t *testing.T) (*Ecphory, string, func()) {
	t.Helper()

	tmpDir := t.TempDir()

	// Write test engrams
	for _, te := range getTestEngrams() {
		path := filepath.Join(tmpDir, te.filename)
		if err := os.WriteFile(path, []byte(te.content), 0644); err != nil {
			t.Fatalf("failed to write test engram %s: %v", te.filename, err)
		}
	}

	// Create ecphory instance
	ecphory, err := NewEcphory(tmpDir, 10000)
	if err != nil {
		t.Fatalf("NewEcphory() failed: %v", err)
	}

	cleanup := func() { os.RemoveAll(tmpDir) }
	return ecphory, tmpDir, cleanup
}

// assertAllHaveTag checks if all results have the given tag
func assertAllHaveTag(t *testing.T, results []*engram.Engram, tag string) {
	t.Helper()
	for _, r := range results {
		hasTag := false
		for _, resultTag := range r.Frontmatter.Tags {
			if resultTag == tag {
				hasTag = true
				break
			}
		}
		if !hasTag {
			t.Errorf("Result %q does not have tag %q", r.Frontmatter.Title, tag)
		}
	}
}

// assertAllHaveAgent checks if all results have the given agent
func assertAllHaveAgent(t *testing.T, results []*engram.Engram, agent string) {
	t.Helper()
	for _, r := range results {
		hasAgent := false
		for _, resultAgent := range r.Frontmatter.Agents {
			if resultAgent == agent {
				hasAgent = true
				break
			}
		}
		if !hasAgent {
			t.Errorf("Result %q does not have agent %q", r.Frontmatter.Title, agent)
		}
	}
}

func TestRetrievalWorkflow(t *testing.T) {
	// Skip if no API key
	if os.Getenv("ANTHROPIC_API_KEY") == "" {
		t.Skip("Skipping integration test: ANTHROPIC_API_KEY not set")
	}

	ecphory, _, cleanup := setupTestEcphory(t)
	defer cleanup()

	t.Run("query_with_tags", func(t *testing.T) {
		results, err := ecphory.Query(context.Background(), "error handling", "test-session-query-tags", "test transcript", []string{"go"}, "")
		if err != nil {
			t.Fatalf("Query() failed: %v", err)
		}
		if len(results) == 0 {
			t.Fatal("Query() returned no results, expected at least 1")
		}
		assertAllHaveTag(t, results, "go")
	})

	t.Run("query_with_agent", func(t *testing.T) {
		results, err := ecphory.Query(context.Background(), "testing", "test-session-query-agent", "test transcript", []string{}, "claude-code")
		if err != nil {
			t.Fatalf("Query() failed: %v", err)
		}
		if len(results) == 0 {
			t.Fatal("Query() returned no results for claude-code agent")
		}
		assertAllHaveAgent(t, results, "claude-code")
	})

	t.Run("query_with_tags_and_agent", func(t *testing.T) {
		results, err := ecphory.Query(context.Background(), "concurrency", "test-session-query-both", "test transcript", []string{"go"}, "claude-code")
		if err != nil {
			t.Fatalf("Query() failed: %v", err)
		}
		if len(results) == 0 {
			t.Fatal("Query() returned no results for go + claude-code filter")
		}
		assertAllHaveTag(t, results, "go")
		assertAllHaveAgent(t, results, "claude-code")
	})

	t.Run("query_no_filters", func(t *testing.T) {
		results, err := ecphory.Query(context.Background(), "patterns", "test-session-query-no-filters", "test transcript", []string{}, "")
		if err != nil {
			t.Fatalf("Query() failed: %v", err)
		}
		if len(results) < 2 {
			t.Errorf("Query() returned %d results, expected at least 2", len(results))
		}
	})

	t.Run("verify_content_loaded", func(t *testing.T) {
		results, err := ecphory.Query(context.Background(), "error handling", "test-session-verify-content", "test transcript", []string{"go"}, "")
		if err != nil {
			t.Fatalf("Query() failed: %v", err)
		}
		if len(results) == 0 {
			t.Fatal("Query() returned no results")
		}
		if results[0].Content == "" || results[0].Path == "" || results[0].Frontmatter.Title == "" {
			t.Error("Query() returned engram with empty content/path/title")
		}
	})
}

// TestRetrievalWorkflow_NoAPIKey verifies that NewEcphory requires API key
// (as ranker initialization needs it upfront)
func TestRetrievalWorkflow_NoAPIKey(t *testing.T) {
	// Temporarily unset API key
	oldAPIKey := os.Getenv("ANTHROPIC_API_KEY")
	os.Unsetenv("ANTHROPIC_API_KEY")
	defer func() {
		if oldAPIKey != "" {
			t.Setenv("ANTHROPIC_API_KEY", oldAPIKey)
		}
	}()

	// Create temporary directory
	tmpDir := t.TempDir()

	// NewEcphory should fail without API key (ranker requirement)
	_, err := NewEcphory(tmpDir, 10000)
	if err == nil {
		t.Fatal("NewEcphory() succeeded without API key, expected error")
	}

	// Error should mention API key
	errMsg := err.Error()
	if errMsg != "failed to create ranker: neither GOOGLE_CLOUD_PROJECT nor ANTHROPIC_API_KEY environment variable set" {
		t.Errorf("NewEcphory() error = %q, want API key error", errMsg)
	}
}

// TestRetrievalWorkflow_EmptyDirectory verifies handling of empty engram directories
func TestRetrievalWorkflow_EmptyDirectory(t *testing.T) {
	// Skip if no API key
	apiKey := os.Getenv("ANTHROPIC_API_KEY")
	if apiKey == "" {
		t.Skip("Skipping integration test: ANTHROPIC_API_KEY not set")
	}

	tmpDir := t.TempDir()

	// Create ecphory with empty directory
	ecphory, err := NewEcphory(tmpDir, 10000)
	if err != nil {
		t.Fatalf("NewEcphory() failed with empty directory: %v", err)
	}

	// Query should return no results, not error
	results, err := ecphory.Query(context.Background(), "test", "test-session-empty-dir", "test transcript", []string{}, "")
	if err != nil {
		t.Fatalf("Query() failed on empty directory: %v", err)
	}

	if len(results) > 0 {
		t.Errorf("Query() returned %d results on empty directory, want 0", len(results))
	}
}
