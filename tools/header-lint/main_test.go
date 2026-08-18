package main

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/vbonnet/dear-agent/internal/gittest"
)

func TestRunRepository_Clean(t *testing.T) {
	repo := t.TempDir()
	runGit(t, repo, "init", "-q")
	path := filepath.Join(repo, "README.md")
	const doc = "# Repo\n\n- **Status:** authoritative\n- **Last updated:** 2026-06-11\n\n## Overview\n"
	if err := os.WriteFile(path, []byte(doc), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	runGit(t, repo, "add", "README.md")

	var stderr bytes.Buffer
	if code := run(context.Background(), []string{"-repo", repo}, &stderr); code != 0 {
		t.Fatalf("run returned %d: %s", code, stderr.String())
	}
}

func TestRunRepository_Violation(t *testing.T) {
	repo := t.TempDir()
	runGit(t, repo, "init", "-q")
	path := filepath.Join(repo, "REVIEW.md")
	const doc = "# REVIEW.md\n\n**Status:** authoritative · **Last updated:** 2026-06-11\n\n## Section\n"
	if err := os.WriteFile(path, []byte(doc), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	runGit(t, repo, "add", "REVIEW.md")

	var stderr bytes.Buffer
	code := run(context.Background(), []string{"-repo", repo}, &stderr)
	if code != 1 {
		t.Fatalf("run returned %d, want 1: %s", code, stderr.String())
	}
	if !strings.Contains(stderr.String(), "REVIEW.md:3") {
		t.Fatalf("stderr missing REVIEW.md:3 location: %s", stderr.String())
	}
}

func TestRunFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "doc.md")
	const doc = "# Doc\n\n**A:** 1 · **B:** 2\n"
	if err := os.WriteFile(path, []byte(doc), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	var stderr bytes.Buffer
	if code := run(context.Background(), []string{"-file", path}, &stderr); code != 1 {
		t.Fatalf("run returned %d, want 1: %s", code, stderr.String())
	}
}

func TestRunDir(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "doc.md")
	const doc = "# Doc\n\nfine\n"
	if err := os.WriteFile(path, []byte(doc), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	var stderr bytes.Buffer
	if code := run(context.Background(), []string{dir}, &stderr); code != 0 {
		t.Fatalf("run returned %d: %s", code, stderr.String())
	}
}

func TestRunUsageErrors(t *testing.T) {
	tests := [][]string{
		nil,
		{"-repo", ".", "extra"},
		{"-repo", ".", "-file", "README.md"},
	}
	for _, args := range tests {
		var stderr bytes.Buffer
		if code := run(context.Background(), args, &stderr); code != 2 {
			t.Errorf("run(%v) = %d, want 2; stderr=%s", args, code, stderr.String())
		}
	}
}

func TestRunOperationalError(t *testing.T) {
	var stderr bytes.Buffer
	code := run(context.Background(), []string{"-file", filepath.Join(t.TempDir(), "missing.md")}, &stderr)
	if code != 2 {
		t.Fatalf("run returned %d, want 2: %s", code, stderr.String())
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
	gittest.Run(t, dir, args...)
}
