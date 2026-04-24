// Package testutil provides test utilities for retrieval package tests (B4.1).
// This file is partitioned per the test strategy to avoid conflicts with other
// B4.* sub-projects running in parallel.
package testutil

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// CreateTestEngram creates a test .ai.md file with specified frontmatter.
// Returns the full path to the created file.
func CreateTestEngram(t *testing.T, dir, filename, typ string, tags []string) string {
	t.Helper()

	// Build tags string for YAML
	var tagsStr string
	if len(tags) > 0 {
		tagsStr = "[" + strings.Join(tags, ", ") + "]"
	} else {
		tagsStr = "[]"
	}

	content := fmt.Sprintf(`---
title: %s
type: %s
tags: %s
version: v0.1.0-prototype
status: Prototype
---

# %s

Test content for %s.

This is a test engram used for unit testing the retrieval package.
`, filename, typ, tagsStr, filename, filename)

	path := filepath.Join(dir, filename)
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("failed to create test engram %s: %v", filename, err)
	}

	return path
}

// SetupTestEngrams creates a temporary directory with sample test engrams.
// Automatically registers cleanup via t.Cleanup().
// Returns the path to the temporary directory.
func SetupTestEngrams(t *testing.T) string {
	t.Helper()

	tmpdir, err := os.MkdirTemp("", "retrieval-test-*")
	if err != nil {
		t.Fatalf("failed to create temp directory: %v", err)
	}

	// Auto-cleanup on test completion
	t.Cleanup(func() {
		os.RemoveAll(tmpdir)
	})

	// Create diverse test engrams
	CreateTestEngram(t, tmpdir, "pattern1.ai.md", "pattern", []string{"go", "testing"})
	CreateTestEngram(t, tmpdir, "pattern2.ai.md", "pattern", []string{"go", "errors"})
	CreateTestEngram(t, tmpdir, "workflow1.ai.md", "workflow", []string{"python"})
	CreateTestEngram(t, tmpdir, "strategy1.ai.md", "strategy", []string{"go"})
	CreateTestEngram(t, tmpdir, "pattern3.ai.md", "pattern", []string{"rust"})

	return tmpdir
}
