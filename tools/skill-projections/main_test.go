package main

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const canonicalSkill = `---
name: %s
description: Use when a test needs %s.
---

# Canonical
`

func TestCheckDelegatesAcceptsExactProjection(t *testing.T) {
	repo := t.TempDir()
	writeCanonicalSkills(t, repo)
	if err := writeDelegates(repo); err != nil {
		t.Fatalf("writeDelegates: %v", err)
	}
	var stdout, stderr bytes.Buffer
	if code := run([]string{"-repo", repo}, &stdout, &stderr); code != 0 {
		t.Fatalf("run = %d, stderr=%s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "2 canonical skill discovery delegates") {
		t.Fatalf("stdout = %q", stdout.String())
	}
}

func TestCheckDelegatesRejectsDrift(t *testing.T) {
	repo := t.TempDir()
	writeCanonicalSkills(t, repo)
	if err := writeDelegates(repo); err != nil {
		t.Fatalf("writeDelegates: %v", err)
	}
	path := filepath.Join(repo, ".agents", "skills", "write-spec", "SKILL.md")
	if err := os.WriteFile(path, []byte("stale\n"), 0o644); err != nil {
		t.Fatalf("write stale delegate: %v", err)
	}
	var stdout, stderr bytes.Buffer
	if code := run([]string{"-repo", repo}, &stdout, &stderr); code != 1 {
		t.Fatalf("run = %d, stderr=%s", code, stderr.String())
	}
	if !strings.Contains(stderr.String(), "generated delegate is stale") {
		t.Fatalf("stderr = %q", stderr.String())
	}
}

func TestCheckModeNeverWritesMissingDelegate(t *testing.T) {
	repo := t.TempDir()
	writeCanonicalSkills(t, repo)
	var stdout, stderr bytes.Buffer
	if code := run([]string{"-repo", repo}, &stdout, &stderr); code != 1 {
		t.Fatalf("run = %d, stderr=%s", code, stderr.String())
	}
	path := filepath.Join(repo, ".agents", "skills", "write-spec", "SKILL.md")
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("check mode wrote %s: stat error=%v", path, err)
	}
}

func TestCheckDelegatesRejectsUnexpectedMarker(t *testing.T) {
	repo := t.TempDir()
	writeCanonicalSkills(t, repo)
	if err := writeDelegates(repo); err != nil {
		t.Fatalf("writeDelegates: %v", err)
	}
	path := filepath.Join(repo, "other", "SKILL.md")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(path, []byte(generatedMarker), 0o644); err != nil {
		t.Fatalf("write unexpected delegate: %v", err)
	}
	var stdout, stderr bytes.Buffer
	if code := run([]string{"-repo", repo}, &stdout, &stderr); code != 1 {
		t.Fatalf("run = %d, stderr=%s", code, stderr.String())
	}
	if !strings.Contains(stderr.String(), "unexpected generated delegate marker") {
		t.Fatalf("stderr = %q", stderr.String())
	}
}

func TestWriteDelegatesRefusesSymlinkedParent(t *testing.T) {
	repo := t.TempDir()
	writeCanonicalSkills(t, repo)
	outside := t.TempDir()
	if err := os.MkdirAll(filepath.Join(repo, ".agents"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.Symlink(outside, filepath.Join(repo, ".agents", "skills")); err != nil {
		t.Fatalf("symlink: %v", err)
	}
	if err := writeDelegates(repo); err == nil || !strings.Contains(err.Error(), "refusing to traverse symlinked parent") {
		t.Fatalf("writeDelegates error = %v", err)
	}
}

func TestRunUsage(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if code := run(nil, &stdout, &stderr); code != 2 {
		t.Fatalf("run = %d, want 2", code)
	}
}

func writeCanonicalSkills(t *testing.T, repo string) {
	t.Helper()
	for _, item := range projections {
		path := filepath.Join(repo, filepath.FromSlash(item.canonical))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		name := filepath.Base(filepath.Dir(path))
		if err := os.WriteFile(path, fmt.Appendf(nil, canonicalSkill, name, name), 0o644); err != nil {
			t.Fatalf("write canonical: %v", err)
		}
	}
}
