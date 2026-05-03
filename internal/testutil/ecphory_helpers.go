package testutil

import (
	"os"
	"path/filepath"
	"testing"
)

// CreateTestEngramDir creates a temporary directory with sample .ai.md files for ecphory testing.
// The directory is automatically cleaned up when the test completes.
//
// Directory structure:
//
//	patterns/go/error-handling.ai.md (tags: go, errors, patterns; agent: claude-code)
//	patterns/go/table-driven-tests.ai.md (tags: go, testing, patterns; agent: claude-code)
//	references/markdown-formatting.ai.md (tags: markdown, formatting; no agent)
//	strategies/retrieval.ai.md (tags: ai, retrieval, strategies; agent: claude-code)
func CreateTestEngramDir(t *testing.T) string {
	t.Helper()
	tmpdir := t.TempDir()

	// Create directory structure
	if err := os.MkdirAll(filepath.Join(tmpdir, "patterns", "go"), 0o700); err != nil {
		t.Fatalf("mkdir patterns/go: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(tmpdir, "references"), 0o700); err != nil {
		t.Fatalf("mkdir references: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(tmpdir, "strategies"), 0o700); err != nil {
		t.Fatalf("mkdir strategies: %v", err)
	}

	// Write sample engrams
	writeTestEngram(t, filepath.Join(tmpdir, "patterns/go/error-handling.ai.md"),
		`---
type: pattern
tags: ["go", "errors", "patterns"]
agents: ["claude-code"]
title: Go Error Handling
---

# Error Handling in Go

Use errors.Is() and errors.As() for error inspection.
`)

	writeTestEngram(t, filepath.Join(tmpdir, "patterns/go/table-driven-tests.ai.md"),
		`---
type: pattern
tags: ["go", "testing", "patterns"]
agents: ["claude-code"]
title: Table-Driven Tests
---

# Table-Driven Tests in Go

Use table-driven tests for comprehensive coverage.
`)

	writeTestEngram(t, filepath.Join(tmpdir, "references/markdown-formatting.ai.md"),
		`---
type: reference
tags: ["markdown", "formatting"]
title: Markdown Formatting Guide
---

# Markdown Formatting

Use **bold** and *italic* for emphasis.
`)

	writeTestEngram(t, filepath.Join(tmpdir, "strategies/retrieval.ai.md"),
		`---
type: strategy
tags: ["ai", "retrieval", "strategies"]
agents: ["claude-code"]
title: Retrieval Strategies
---

# Retrieval Strategies

Use semantic search for relevant retrieval.
`)

	return tmpdir
}

// writeTestEngram writes an engram file to the given path
func writeTestEngram(t *testing.T, path, content string) {
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("failed to write test engram: %v", err)
	}
}
