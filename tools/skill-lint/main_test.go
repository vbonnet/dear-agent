package main

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/vbonnet/dear-agent/internal/gittest"
)

func TestRunRepository(t *testing.T) {
	repo := t.TempDir()
	runGit(t, repo, "init", "-q")
	path := filepath.Join(repo, "skills", "example", "SKILL.md")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	const skill = `---
name: example
description: Use when a test needs an example skill.
---

# Example

## Workflow

1. Inspect input.
2. Produce output.

## Verify

Confirm the output.
`
	if err := os.WriteFile(path, []byte(skill), 0o644); err != nil {
		t.Fatalf("write skill: %v", err)
	}
	runGit(t, repo, "add", "skills/example/SKILL.md")

	var stderr bytes.Buffer
	if code := run(context.Background(), []string{"-repo", repo}, &stderr); code != 0 {
		t.Fatalf("run returned %d: %s", code, stderr.String())
	}
}

func TestRunUsageErrors(t *testing.T) {
	tests := [][]string{
		nil,
		{"-repo", ".", "extra"},
		{"-repo", ".", "-file", "SKILL.md"},
	}
	for _, args := range tests {
		var stderr bytes.Buffer
		if code := run(context.Background(), args, &stderr); code != 2 {
			t.Errorf("run(%v) = %d, want 2; stderr=%s", args, code, stderr.String())
		}
	}
}

func TestRunHelpSucceeds(t *testing.T) {
	for _, arg := range []string{"-h", "-help"} {
		var stderr bytes.Buffer
		if code := run(context.Background(), []string{arg}, &stderr); code != 0 {
			t.Errorf("run(%q) = %d, want 0; stderr=%s", arg, code, stderr.String())
		}
	}
}

func runGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := gittest.Command(t, dir, args...)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git failed: %v\n%s", err, output)
	}
}
