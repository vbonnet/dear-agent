package compaction

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGeneratePrompt_AllFields(t *testing.T) {
	input := &PromptInput{
		SessionName: "my-session",
		Project:     "~/project",
		Purpose:     "auth refactor",
		Tags:        []string{"auth", "security"},
		Notes:       "Working on JWT rotation",
		FocusText:   "Preserve middleware chain context",
	}
	result := GeneratePrompt(input)
	if !strings.HasPrefix(result, "/compact") {
		t.Error("should start with /compact")
	}
	if !strings.Contains(result, "my-session") {
		t.Error("should contain session name")
	}
	if !strings.Contains(result, "auth refactor") {
		t.Error("should contain purpose")
	}
	if !strings.Contains(result, "auth, security") {
		t.Error("should contain tags")
	}
	if !strings.Contains(result, "JWT rotation") {
		t.Error("should contain notes")
	}
	if !strings.Contains(result, "middleware chain") {
		t.Error("should contain focus text")
	}
}

func TestGeneratePrompt_EmptyFields(t *testing.T) {
	input := &PromptInput{}
	result := GeneratePrompt(input)
	if result != "/compact" {
		t.Errorf("empty input should produce plain /compact, got %q", result)
	}
}

func TestGeneratePrompt_FocusOnly(t *testing.T) {
	input := &PromptInput{
		FocusText: "preserve auth context",
	}
	result := GeneratePrompt(input)
	if !strings.Contains(result, "/compact") {
		t.Error("should contain /compact")
	}
	if !strings.Contains(result, "preserve auth context") {
		t.Error("should contain focus text")
	}
}

func TestNextPromptNumber_NoExisting(t *testing.T) {
	dir := t.TempDir()
	n, err := NextPromptNumber(dir, "my-session")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if n != 1 {
		t.Errorf("NextPromptNumber = %d, want 1", n)
	}
}

func TestNextPromptNumber_WithExisting(t *testing.T) {
	dir := t.TempDir()
	pDir := filepath.Join(dir, "compaction-prompts")
	os.MkdirAll(pDir, 0o755)
	os.WriteFile(filepath.Join(pDir, "my-session-compact-1.md"), []byte("test"), 0o644)
	os.WriteFile(filepath.Join(pDir, "my-session-compact-3.md"), []byte("test"), 0o644)

	n, err := NextPromptNumber(dir, "my-session")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if n != 4 {
		t.Errorf("NextPromptNumber = %d, want 4", n)
	}
}

func TestSavePrompt(t *testing.T) {
	dir := t.TempDir()
	path, err := SavePrompt(dir, "my-session", 1, "/compact test content")
	if err != nil {
		t.Fatalf("SavePrompt: %v", err)
	}
	expected := filepath.Join(dir, "compaction-prompts", "my-session-compact-1.md")
	if path != expected {
		t.Errorf("path = %q, want %q", path, expected)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read saved prompt: %v", err)
	}
	if string(data) != "/compact test content" {
		t.Errorf("content = %q, want %q", string(data), "/compact test content")
	}
}
