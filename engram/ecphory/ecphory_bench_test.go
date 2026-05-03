package ecphory

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

// ============================================================================
// Performance Benchmarks: Validate ADR-009 performance targets
// ============================================================================

// createTestEngrams creates a temporary directory with test engram files
func createTestEngrams(b *testing.B, count int) string {
	b.Helper()

	tmpDir := b.TempDir()

	categories := []string{"patterns", "workflows", "references"}
	tags := [][]string{
		{"go", "error", "testing"},
		{"python", "async", "performance"},
		{"javascript", "react", "hooks"},
	}

	for i := 0; i < count; i++ {
		category := categories[i%len(categories)]
		tagSet := tags[i%len(tags)]
		tagStr := fmt.Sprintf("[%s]", tagSet[0])
		for j := 1; j < len(tagSet); j++ {
			tagStr = fmt.Sprintf("%s, %s", tagStr[:len(tagStr)-1], tagSet[j]) + "]"
		}

		content := fmt.Sprintf(`---
type: pattern
category: %s
tags: %s
applies_to: [claude-code]
token_budget: 500
---

# Test Engram %d

This is example content for benchmarking purposes.
It contains multiple lines to simulate realistic engram size.

## Example Section

- Bullet point 1
- Bullet point 2
- Bullet point 3

## Code Example

`+"```go\nfunc example() {\n    // code here\n}\n```"+`

Total size approximately 300-400 bytes to match typical engrams.
`, category, tagStr, i)

		path := filepath.Join(tmpDir, fmt.Sprintf("engram-%03d.ai.md", i))
		if err := os.WriteFile(path, []byte(content), 0644); err != nil {
			b.Fatal(err)
		}
	}

	return tmpDir
}

// BenchmarkIndexBuild_100 benchmarks building index with 100 engrams
// Target: <50ms (from ADR-009: <500ms for 1000 → ~50ms for 100)
func BenchmarkIndexBuild_100(b *testing.B) {
	tmpDir := createTestEngrams(b, 100)
	defer os.RemoveAll(tmpDir)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		idx := NewIndex()
		if err := idx.Build(tmpDir); err != nil {
			b.Fatalf("Build() failed: %v", err)
		}
	}
}

// BenchmarkIndexBuild_1000 benchmarks building index with 1000 engrams
// Target: <500ms (from ADR-009)
func BenchmarkIndexBuild_1000(b *testing.B) {
	tmpDir := createTestEngrams(b, 1000)
	defer os.RemoveAll(tmpDir)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		idx := NewIndex()
		if err := idx.Build(tmpDir); err != nil {
			b.Fatalf("Build() failed: %v", err)
		}
	}
}

// BenchmarkFastFilter_25Candidates benchmarks Tier 1 filtering
// Target: <50ms (from ADR-009)
func BenchmarkFastFilter_25Candidates(b *testing.B) {
	tmpDir := createTestEngrams(b, 1000)
	defer os.RemoveAll(tmpDir)

	idx := NewIndex()
	if err := idx.Build(tmpDir); err != nil {
		b.Fatalf("Build() failed: %v", err)
	}

	tags := []string{"go", "error"}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		candidates := idx.FilterByTags(tags)
		// Verify we got some candidates (at least a few should match)
		if len(candidates) == 0 {
			b.Fatal("Expected some candidates")
		}
	}
}

// BenchmarkFilterByAgent_1000Engrams benchmarks FilterByAgent performance (P0-5 fix)
// Target: <50ms for 1000 engrams (from P0-5 requirements)
func BenchmarkFilterByAgent_1000Engrams(b *testing.B) {
	tmpDir := createTestEngrams(b, 1000)
	defer os.RemoveAll(tmpDir)

	idx := NewIndex()
	if err := idx.Build(tmpDir); err != nil {
		b.Fatalf("Build() failed: %v", err)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		results := idx.FilterByAgent("claude-code")
		if len(results) == 0 {
			b.Fatal("Expected some results")
		}
	}
}

// BenchmarkFilterByAgent_Concurrent tests concurrent FilterByAgent calls
func BenchmarkFilterByAgent_Concurrent(b *testing.B) {
	tmpDir := createTestEngrams(b, 1000)
	defer os.RemoveAll(tmpDir)

	idx := NewIndex()
	if err := idx.Build(tmpDir); err != nil {
		b.Fatalf("Build() failed: %v", err)
	}

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			results := idx.FilterByAgent("claude-code")
			if len(results) == 0 {
				b.Fatal("Expected some results")
			}
		}
	})
}

// NOTE: BenchmarkRetrieval_WithoutAPI skipped
// Reason: Requires ANTHROPIC_API_KEY even for fallback mode
// Future: Create mock ranker interface for testing without API
