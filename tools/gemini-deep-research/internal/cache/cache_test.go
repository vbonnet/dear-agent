package cache

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCheck(t *testing.T) {
	// Create temp cache directory
	tmpDir := t.TempDir()

	// Create a test cached research
	research := &Research{
		URL:         "https://youtube.com/watch?v=abc123",
		ContentHash: "sha256:test",
		Topics:      []string{"AI", "ML"},
		Content:     "# Test Report\n\nSome content",
		ContentType: "youtube",
	}

	// Write to cache
	cachePath, err := Write(research, tmpDir, false)
	if err != nil {
		t.Fatalf("Failed to write to cache: %v", err)
	}

	// Test cache hit
	t.Run("CacheHit", func(t *testing.T) {
		foundPath, exists, err := Check("https://youtube.com/watch?v=abc123", tmpDir, "youtube")
		if err != nil {
			t.Fatalf("Check failed: %v", err)
		}
		if !exists {
			t.Error("Expected cache hit, got miss")
		}
		if foundPath == "" {
			t.Error("Expected non-empty path for cache hit")
		}
		// Verify path matches what we wrote
		if foundPath != cachePath {
			t.Errorf("Expected path %s, got %s", cachePath, foundPath)
		}
	})

	// Test cache miss
	t.Run("CacheMiss", func(t *testing.T) {
		_, exists, err := Check("https://youtube.com/watch?v=xyz789", tmpDir, "youtube")
		if err != nil {
			t.Fatalf("Check failed: %v", err)
		}
		if exists {
			t.Error("Expected cache miss, got hit")
		}
	})

	// Test URL normalization (different URLs, same normalized form)
	t.Run("NormalizedCacheHit", func(t *testing.T) {
		// URL with tracking params should normalize to same key
		urlWithParams := "https://www.youtube.com/watch?v=abc123&utm_source=test"
		foundPath, exists, err := Check(urlWithParams, tmpDir, "youtube")
		if err != nil {
			t.Fatalf("Check failed: %v", err)
		}
		if !exists {
			t.Error("Expected cache hit with normalized URL, got miss")
		}
		if foundPath == "" {
			t.Error("Expected non-empty path for cache hit")
		}
		if foundPath != cachePath {
			t.Errorf("Expected path %s, got %s", cachePath, foundPath)
		}
	})
}

func TestWrite(t *testing.T) {
	tmpDir := t.TempDir()

	research := &Research{
		URL:         "https://arxiv.org/abs/2601.20802",
		ContentHash: "sha256:abcd1234",
		Topics:      []string{"Machine Learning", "NLP"},
		Content:     "# Research Report\n\nFindings...",
		ContentType: "arxiv",
	}

	t.Run("FirstWrite", func(t *testing.T) {
		path, err := Write(research, tmpDir, false)
		if err != nil {
			t.Fatalf("Write failed: %v", err)
		}

		// Verify directory structure
		reportPath := filepath.Join(path, "report.md")
		if _, err := os.Stat(reportPath); os.IsNotExist(err) {
			t.Errorf("Expected report.md to exist at %s", reportPath)
		}

		// Verify frontmatter
		fm, err := readFrontmatter(reportPath)
		if err != nil {
			t.Fatalf("Failed to read frontmatter: %v", err)
		}

		if fm.URL != "https://arxiv.org/abs/2601.20802" {
			t.Errorf("Expected URL https://arxiv.org/abs/2601.20802, got %s", fm.URL)
		}
		if fm.Version != 1 {
			t.Errorf("Expected version 1, got %d", fm.Version)
		}
	})

	t.Run("ForceWrite", func(t *testing.T) {
		// First write
		path1, err := Write(research, tmpDir, false)
		if err != nil {
			t.Fatalf("First write failed: %v", err)
		}

		// Force write (should create versioned directory)
		path2, err := Write(research, tmpDir, true)
		if err != nil {
			t.Fatalf("Force write failed: %v", err)
		}

		if path1 == path2 {
			t.Error("Expected different paths for force write")
		}

		// Verify version 2
		reportPath2 := filepath.Join(path2, "report.md")
		fm2, err := readFrontmatter(reportPath2)
		if err != nil {
			t.Fatalf("Failed to read frontmatter from version 2: %v", err)
		}

		if fm2.Version != 2 {
			t.Errorf("Expected version 2, got %d", fm2.Version)
		}
	})
}

func TestReadFrontmatter(t *testing.T) {
	tmpDir := t.TempDir()

	// Create test file with frontmatter
	content := `---
url: https://example.com
content_hash: sha256:test123
researched_at: 2026-02-03T00:00:00Z
topics:
  - Topic 1
  - Topic 2
version: 1
---

# Report Content

Some content here.
`

	testFile := filepath.Join(tmpDir, "test.md")
	if err := os.WriteFile(testFile, []byte(content), 0644); err != nil {
		t.Fatalf("Failed to write test file: %v", err)
	}

	fm, err := readFrontmatter(testFile)
	if err != nil {
		t.Fatalf("readFrontmatter failed: %v", err)
	}

	if fm.URL != "https://example.com" {
		t.Errorf("Expected URL https://example.com, got %s", fm.URL)
	}
	if fm.ContentHash != "sha256:test123" {
		t.Errorf("Expected hash sha256:test123, got %s", fm.ContentHash)
	}
	if len(fm.Topics) != 2 {
		t.Errorf("Expected 2 topics, got %d", len(fm.Topics))
	}
	if fm.Version != 1 {
		t.Errorf("Expected version 1, got %d", fm.Version)
	}
}
