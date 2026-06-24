package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// fakeModelCaller returns a canned response for deterministic tests.
type fakeModelCaller struct {
	response string
	err      error
}

func (f *fakeModelCaller) Call(_ context.Context, _ string) (string, error) {
	return f.response, f.err
}

const validModelResponse = `SKILL_NAME: worktree-pr-flow
REPLACES: nothing
WHY_NEW: This session demonstrates the specific git worktree + safe-pr pattern used across all bead PRs.

---SKILL.md---
---
name: worktree-pr-flow
description: >
  Create a git worktree, implement a change, and open a PR via safe-pr.
  Use when implementing a bead or feature that requires an isolated branch.
---

# Worktree PR Flow

1. Create worktree: git -C ~/src/<repo> worktree add ~/worktrees/<repo>/<branch> -b <branch>
2. Implement changes in ~/worktrees/<repo>/<branch>/
3. Run preflight: make -C ~/worktrees/<repo>/<branch> preflight
4. Push: GIT_TERMINAL_PROMPT=0 gtimeout 30 git -C ~/worktrees/<repo>/<branch> push -u origin <branch>
5. Create PR: safe-pr create --title "..." --body "..."
---END---`

func TestExtract_EmptyTranscript(t *testing.T) {
	t.Parallel()

	projectsDir := t.TempDir()
	subDir := filepath.Join(projectsDir, "proj")
	if err := os.Mkdir(subDir, 0o755); err != nil {
		t.Fatal(err)
	}
	// Write an empty JSONL file.
	if err := os.WriteFile(filepath.Join(subDir, "abc123.jsonl"), []byte(""), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg := Config{
		SessionID:   "abc123",
		ProjectsDir: projectsDir,
		SkillsDir:   t.TempDir(),
		OutputDir:   t.TempDir(),
	}
	_, err := Extract(context.Background(), cfg, &fakeModelCaller{response: validModelResponse})
	if !errors.Is(err, ErrEmptyTranscript) {
		t.Errorf("expected ErrEmptyTranscript, got %v", err)
	}
}

func TestExtract_DryRunNoWrite(t *testing.T) {
	t.Parallel()

	projectsDir := t.TempDir()
	subDir := filepath.Join(projectsDir, "proj")
	if err := os.Mkdir(subDir, 0o755); err != nil {
		t.Fatal(err)
	}
	// Write a JSONL with a minimal user message.
	content := `{"type":"user","message":{"role":"user","content":"do something useful"}}` + "\n"
	if err := os.WriteFile(filepath.Join(subDir, "sess1.jsonl"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	outputDir := t.TempDir()
	cfg := Config{
		SessionID:   "sess1",
		ProjectsDir: projectsDir,
		SkillsDir:   t.TempDir(),
		OutputDir:   outputDir,
		DryRun:      true,
	}
	result, err := Extract(context.Background(), cfg, &fakeModelCaller{response: validModelResponse})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.OutputPath != "" {
		t.Errorf("dry-run should not set OutputPath, got %q", result.OutputPath)
	}

	// No files should have been written to outputDir.
	entries, err := os.ReadDir(outputDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Errorf("dry-run wrote %d files; expected 0", len(entries))
	}
}

func TestListSkillNames_Dedup(t *testing.T) {
	t.Parallel()

	skillsDir := t.TempDir()
	// Create fake skill entries (dirs, like real skills).
	for _, name := range []string{"diagnose", "caveman", "run"} {
		if err := os.Mkdir(filepath.Join(skillsDir, name), 0o755); err != nil {
			t.Fatal(err)
		}
	}

	names, err := listSkillNames(skillsDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(names) != 3 {
		t.Errorf("expected 3 skills, got %d: %v", len(names), names)
	}

	// Verify the prompt includes these names for dedup.
	prompt := buildPrompt("some transcript content", names)
	for _, name := range names {
		if !strings.Contains(prompt, name) {
			t.Errorf("prompt missing skill name %q for dedup", name)
		}
	}
}

func TestListSkillNames_EmptyDir(t *testing.T) {
	t.Parallel()

	names, err := listSkillNames(t.TempDir())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(names) != 0 {
		t.Errorf("expected empty list, got %v", names)
	}
}

func TestListSkillNames_MissingDir(t *testing.T) {
	t.Parallel()

	names, err := listSkillNames("/nonexistent/path/to/skills")
	if err != nil {
		t.Fatalf("missing skills dir should return nil error, got %v", err)
	}
	if names != nil {
		t.Errorf("missing skills dir should return nil slice, got %v", names)
	}
}

func TestParseResponse_NothingNew(t *testing.T) {
	t.Parallel()

	_, err := parseResponse("NOTHING_NEW")
	if !errors.Is(err, ErrNothingNew) {
		t.Errorf("expected ErrNothingNew, got %v", err)
	}
}

func TestParseResponse_Valid(t *testing.T) {
	t.Parallel()

	result, err := parseResponse(validModelResponse)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.SkillName != "worktree-pr-flow" {
		t.Errorf("SkillName = %q, want %q", result.SkillName, "worktree-pr-flow")
	}
	if result.Replaces != "nothing" {
		t.Errorf("Replaces = %q, want %q", result.Replaces, "nothing")
	}
	if !strings.Contains(result.Content, "name: worktree-pr-flow") {
		t.Errorf("Content missing frontmatter, got: %.100s", result.Content)
	}
}

func TestExtract_WritesFile(t *testing.T) {
	t.Parallel()

	projectsDir := t.TempDir()
	subDir := filepath.Join(projectsDir, "proj")
	if err := os.Mkdir(subDir, 0o755); err != nil {
		t.Fatal(err)
	}
	content := `{"type":"user","message":{"role":"user","content":"build a skill extractor"}}` + "\n"
	if err := os.WriteFile(filepath.Join(subDir, "sess2.jsonl"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	outputDir := t.TempDir()
	cfg := Config{
		SessionID:   "sess2",
		ProjectsDir: projectsDir,
		SkillsDir:   t.TempDir(),
		OutputDir:   outputDir,
		DryRun:      false,
	}
	result, err := Extract(context.Background(), cfg, &fakeModelCaller{response: validModelResponse})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.OutputPath == "" {
		t.Error("OutputPath should be set after write")
	}
	if _, err := os.Stat(result.OutputPath); os.IsNotExist(err) {
		t.Errorf("output file not found at %s", result.OutputPath)
	}
}

func TestSanitizeFilename(t *testing.T) {
	t.Parallel()

	cases := []struct {
		in, want string
	}{
		{"worktree-pr-flow", "worktree-pr-flow"},
		{"My Cool Skill", "my-cool-skill"},
		{"skill_name", "skill-name"},
		{"  trimmed  ", "trimmed"},
		{"UPPER CASE", "upper-case"},
	}
	for _, c := range cases {
		got := sanitizeFilename(c.in)
		if got != c.want {
			t.Errorf("sanitizeFilename(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}
