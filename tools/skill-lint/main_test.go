package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/vbonnet/dear-agent/pkg/skilllint"
)

func TestSkillLint_CleanFile(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "SKILL.md")
	// A minimal valid skill file with required frontmatter.
	content := `---
model: sonnet
effort: high
---
# My Skill

This skill does something.
`
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	vs, err := skilllint.CheckFile(path)
	if err != nil {
		t.Fatalf("CheckFile: %v", err)
	}
	if len(vs) != 0 {
		t.Errorf("CheckFile(valid skill) = %v, want no violations", vs)
	}
}

func TestSkillLint_MissingFrontmatter(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "SKILL.md")
	if err := os.WriteFile(path, []byte("# No frontmatter\n"), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	vs, err := skilllint.CheckFile(path)
	if err != nil {
		t.Fatalf("CheckFile: %v", err)
	}
	if len(vs) == 0 {
		t.Error("CheckFile(missing frontmatter): expected violations, got none")
	}
}

func TestSkillLint_CheckDir_Empty(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	vs, err := skilllint.CheckDir(dir)
	if err != nil {
		t.Fatalf("CheckDir(empty): %v", err)
	}
	if len(vs) != 0 {
		t.Errorf("CheckDir(empty dir) = %v, want no violations", vs)
	}
}
