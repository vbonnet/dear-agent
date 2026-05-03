package engram

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestNewParser verifies parser initialization
func TestNewParser(t *testing.T) {
	parser := NewParser()
	if parser == nil {
		t.Fatal("NewParser() returned nil")
	}
}

// TestParseBytes_ValidEngram verifies parsing a valid engram
func TestParseBytes_ValidEngram(t *testing.T) {
	content := []byte(`---
type: pattern
title: Test Pattern
description: A test pattern for unit tests
tags:
  - test
  - example
agents:
  - claude-code
---
# Test Content

This is the markdown content of the engram.
`)

	parser := NewParser()
	engram, err := parser.ParseBytes("/test/path.ai.md", content)
	if err != nil {
		t.Fatalf("ParseBytes() failed: %v", err)
	}

	// Verify frontmatter
	if engram.Frontmatter.Type != "pattern" {
		t.Errorf("Frontmatter.Type = %q, want %q", engram.Frontmatter.Type, "pattern")
	}
	if engram.Frontmatter.Title != "Test Pattern" {
		t.Errorf("Frontmatter.Title = %q, want %q", engram.Frontmatter.Title, "Test Pattern")
	}
	if engram.Frontmatter.Description != "A test pattern for unit tests" {
		t.Errorf("Frontmatter.Description = %q, want %q", engram.Frontmatter.Description, "A test pattern for unit tests")
	}
	if len(engram.Frontmatter.Tags) != 2 {
		t.Errorf("len(Frontmatter.Tags) = %d, want 2", len(engram.Frontmatter.Tags))
	}
	if len(engram.Frontmatter.Agents) != 1 || engram.Frontmatter.Agents[0] != "claude-code" {
		t.Errorf("Frontmatter.Agents = %v, want [claude-code]", engram.Frontmatter.Agents)
	}

	// Verify content
	expectedContent := "# Test Content\n\nThis is the markdown content of the engram.\n"
	if engram.Content != expectedContent {
		t.Errorf("Content = %q, want %q", engram.Content, expectedContent)
	}

	// Verify path
	if engram.Path != "/test/path.ai.md" {
		t.Errorf("Path = %q, want %q", engram.Path, "/test/path.ai.md")
	}
}

// TestParseBytes_MinimalEngram verifies parsing an engram with minimal frontmatter
func TestParseBytes_MinimalEngram(t *testing.T) {
	content := []byte(`---
type: strategy
title: Minimal Strategy
description: Just the basics
---
Minimal content.
`)

	parser := NewParser()
	engram, err := parser.ParseBytes("/minimal.ai.md", content)
	if err != nil {
		t.Fatalf("ParseBytes() failed: %v", err)
	}

	if engram.Frontmatter.Type != "strategy" {
		t.Errorf("Frontmatter.Type = %q, want %q", engram.Frontmatter.Type, "strategy")
	}
	if len(engram.Frontmatter.Tags) != 0 {
		t.Errorf("len(Frontmatter.Tags) = %d, want 0 (empty)", len(engram.Frontmatter.Tags))
	}
	if len(engram.Frontmatter.Agents) != 0 {
		t.Errorf("len(Frontmatter.Agents) = %d, want 0 (omitempty)", len(engram.Frontmatter.Agents))
	}
}

// TestParseBytes_WithModifiedTime verifies parsing frontmatter with modified timestamp
func TestParseBytes_WithModifiedTime(t *testing.T) {
	content := []byte(`---
type: workflow
title: Timestamped Workflow
description: Has a modified timestamp
modified: 2024-11-27T10:30:00Z
---
Content with timestamp.
`)

	parser := NewParser()
	engram, err := parser.ParseBytes("/timestamped.ai.md", content)
	if err != nil {
		t.Fatalf("ParseBytes() failed: %v", err)
	}

	expected := time.Date(2024, 11, 27, 10, 30, 0, 0, time.UTC)
	if !engram.Frontmatter.Modified.Equal(expected) {
		t.Errorf("Frontmatter.Modified = %v, want %v", engram.Frontmatter.Modified, expected)
	}
}

// TestParseBytes_MissingFrontmatter verifies error when frontmatter is missing
func TestParseBytes_MissingFrontmatter(t *testing.T) {
	content := []byte("# Just content, no frontmatter\n\nThis should fail.")

	parser := NewParser()
	_, err := parser.ParseBytes("/invalid.md", content)
	if err == nil {
		t.Fatal("ParseBytes() succeeded with missing frontmatter, want error")
	}
}

// TestParseBytes_UnclosedFrontmatter verifies error when closing delimiter is missing
func TestParseBytes_UnclosedFrontmatter(t *testing.T) {
	content := []byte(`---
type: pattern
title: Unclosed
description: Missing closing delimiter
This should fail
`)

	parser := NewParser()
	_, err := parser.ParseBytes("/unclosed.md", content)
	if err == nil {
		t.Fatal("ParseBytes() succeeded with unclosed frontmatter, want error")
	}
}

// TestParseBytes_InvalidYAML verifies error when frontmatter YAML is invalid
func TestParseBytes_InvalidYAML(t *testing.T) {
	content := []byte(`---
type: pattern
title: "unclosed quote
description: Invalid YAML
---
Content here.
`)

	parser := NewParser()
	_, err := parser.ParseBytes("/invalidyaml.md", content)
	if err == nil {
		t.Fatal("ParseBytes() succeeded with invalid YAML, want error")
	}
}

// TestParseBytes_InvalidYAML_UnquotedArray verifies error when unquoted array syntax is used
// Regression test for template.ai.md fix (2026-02-19)
// YAML interprets [a|b|c] as a sequence, not a string, causing unmarshal error
func TestParseBytes_InvalidYAML_UnquotedArray(t *testing.T) {
	content := []byte(`---
type: [prompt|instruction|pattern]
title: "Template"
description: "Invalid array syntax"
tags: [tag1, tag2]
---
Content here.
`)

	parser := NewParser()
	_, err := parser.ParseBytes("/template.ai.md", content)
	if err == nil {
		t.Fatal("ParseBytes() succeeded with unquoted array syntax in type field, want error")
	}
	// Verify error message mentions type mismatch or unmarshal
	if err != nil && !contains(err.Error(), "unmarshal") {
		t.Logf("Expected error about unmarshal, got: %v", err)
	}
}

// helper function for error message checking
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 ||
		(len(s) > 0 && (s[0:len(substr)] == substr || contains(s[1:], substr))))
}

// TestParseBytes_EmptyFrontmatter verifies that consecutive delimiters fail
// (parser requires at least one newline between --- delimiters)
func TestParseBytes_EmptyFrontmatter(t *testing.T) {
	content := []byte(`---
---
Content with empty frontmatter.
`)

	parser := NewParser()
	_, err := parser.ParseBytes("/empty-frontmatter.md", content)
	// This should fail because the parser looks for \n---\n
	// and consecutive delimiters don't have content between them
	if err == nil {
		t.Fatal("ParseBytes() succeeded with empty frontmatter (consecutive delimiters), want error")
	}
}

// TestParse_FileNotFound verifies error when file doesn't exist
func TestParse_FileNotFound(t *testing.T) {
	parser := NewParser()
	_, err := parser.Parse("/nonexistent/file.ai.md")
	if err == nil {
		t.Fatal("Parse() succeeded with nonexistent file, want error")
	}
}

// TestParse_ValidFile verifies parsing from a real file
func TestParse_ValidFile(t *testing.T) {
	// Create temporary file
	tmpDir := t.TempDir()

	content := []byte(`---
type: pattern
title: File Test Pattern
description: Testing file-based parsing
tags:
  - file-test
---
# Pattern Content

This pattern is loaded from a file.
`)

	testFile := filepath.Join(tmpDir, "test-pattern.ai.md")
	if err := os.WriteFile(testFile, content, 0644); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	parser := NewParser()
	engram, err := parser.Parse(testFile)
	if err != nil {
		t.Fatalf("Parse() failed: %v", err)
	}

	if engram.Frontmatter.Title != "File Test Pattern" {
		t.Errorf("Frontmatter.Title = %q, want %q", engram.Frontmatter.Title, "File Test Pattern")
	}
	if engram.Path != testFile {
		t.Errorf("Path = %q, want %q", engram.Path, testFile)
	}
}

// TestSplitFrontmatter_VariousFormats verifies frontmatter splitting edge cases
func TestSplitFrontmatter_VariousFormats(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		wantFM      string
		wantContent string
		wantErr     bool
	}{
		{
			name: "basic",
			input: `---
key: value
---
content`,
			wantFM:      "key: value",
			wantContent: "content",
			wantErr:     false,
		},
		{
			name: "multiline frontmatter",
			input: `---
key1: value1
key2:
  - item1
  - item2
---
content line 1
content line 2`,
			wantFM:      "key1: value1\nkey2:\n  - item1\n  - item2",
			wantContent: "content line 1\ncontent line 2",
			wantErr:     false,
		},
		{
			name: "content with triple dashes",
			input: `---
key: value
---
Some content

---

More content with dashes in the middle`,
			wantFM:      "key: value",
			wantContent: "Some content\n\n---\n\nMore content with dashes in the middle",
			wantErr:     false,
		},
		{
			name: "no opening delimiter",
			input: `key: value
---
content`,
			wantErr: true,
		},
		{
			name: "no closing delimiter",
			input: `---
key: value
content`,
			wantErr: true,
		},
	}

	parser := NewParser()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fm, content, err := parser.splitFrontmatter([]byte(tt.input))

			if (err != nil) != tt.wantErr {
				t.Errorf("splitFrontmatter() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr {
				if string(fm) != tt.wantFM {
					t.Errorf("frontmatter = %q, want %q", string(fm), tt.wantFM)
				}
				if string(content) != tt.wantContent {
					t.Errorf("content = %q, want %q", string(content), tt.wantContent)
				}
			}
		})
	}
}
