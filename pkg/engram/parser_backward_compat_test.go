package engram

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParser_BackwardCompatibility_LegacyEngram(t *testing.T) {
	// Test parsing an old engram without metadata fields
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "legacy.ai.md")

	content := []byte(`---
type: pattern
title: Legacy Pattern
description: An old pattern without metadata
tags: [legacy, test]
---

# Legacy Pattern

This engram was created before metadata tracking.
`)

	if err := os.WriteFile(testFile, content, 0644); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	// Parse the engram
	parser := NewParser()
	eg, err := parser.Parse(testFile)
	if err != nil {
		t.Fatalf("failed to parse legacy engram: %v", err)
	}

	// Verify core fields parsed correctly
	if eg.Frontmatter.Type != "pattern" {
		t.Errorf("expected type=pattern, got %s", eg.Frontmatter.Type)
	}

	if eg.Frontmatter.Title != "Legacy Pattern" {
		t.Errorf("expected title=Legacy Pattern, got %s", eg.Frontmatter.Title)
	}

	// Verify defaults applied for missing metadata
	if eg.Frontmatter.EncodingStrength != 1.0 {
		t.Errorf("expected encoding_strength=1.0 (default), got %f", eg.Frontmatter.EncodingStrength)
	}

	if eg.Frontmatter.RetrievalCount != 0 {
		t.Errorf("expected retrieval_count=0 (default), got %d", eg.Frontmatter.RetrievalCount)
	}

	// CreatedAt should be set from file mtime
	if eg.Frontmatter.CreatedAt.IsZero() {
		t.Error("expected created_at to be set from file mtime")
	}

	// LastAccessed should be zero (never accessed)
	if !eg.Frontmatter.LastAccessed.IsZero() {
		t.Error("expected last_accessed to be zero for legacy engram")
	}
}

func TestParser_BackwardCompatibility_NewEngram(t *testing.T) {
	// Test parsing an engram with all metadata fields
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "new.ai.md")

	content := []byte(`---
type: pattern
title: New Pattern
description: A modern pattern with metadata
tags: [modern, test]
encoding_strength: 1.5
retrieval_count: 42
created_at: 2025-01-15T10:30:00Z
last_accessed: 2025-12-16T14:22:33Z
---

# New Pattern

This engram has full metadata tracking.
`)

	if err := os.WriteFile(testFile, content, 0644); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	// Parse the engram
	parser := NewParser()
	eg, err := parser.Parse(testFile)
	if err != nil {
		t.Fatalf("failed to parse new engram: %v", err)
	}

	// Verify metadata fields parsed correctly
	if eg.Frontmatter.EncodingStrength != 1.5 {
		t.Errorf("expected encoding_strength=1.5, got %f", eg.Frontmatter.EncodingStrength)
	}

	if eg.Frontmatter.RetrievalCount != 42 {
		t.Errorf("expected retrieval_count=42, got %d", eg.Frontmatter.RetrievalCount)
	}

	if eg.Frontmatter.CreatedAt.IsZero() {
		t.Error("expected created_at to be parsed from frontmatter")
	}

	if eg.Frontmatter.LastAccessed.IsZero() {
		t.Error("expected last_accessed to be parsed from frontmatter")
	}
}

func TestParser_BackwardCompatibility_PartialMetadata(t *testing.T) {
	// Test parsing an engram with some but not all metadata fields
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "partial.ai.md")

	content := []byte(`---
type: pattern
title: Partial Pattern
description: A pattern with partial metadata
tags: [partial, test]
retrieval_count: 10
---

# Partial Pattern

This engram has only retrieval_count set.
`)

	if err := os.WriteFile(testFile, content, 0644); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	// Parse the engram
	parser := NewParser()
	eg, err := parser.Parse(testFile)
	if err != nil {
		t.Fatalf("failed to parse partial engram: %v", err)
	}

	// Verify explicit fields preserved
	if eg.Frontmatter.RetrievalCount != 10 {
		t.Errorf("expected retrieval_count=10, got %d", eg.Frontmatter.RetrievalCount)
	}

	// Verify defaults applied for missing fields
	if eg.Frontmatter.EncodingStrength != 1.0 {
		t.Errorf("expected encoding_strength=1.0 (default), got %f", eg.Frontmatter.EncodingStrength)
	}

	if eg.Frontmatter.CreatedAt.IsZero() {
		t.Error("expected created_at to be set from file mtime")
	}
}
