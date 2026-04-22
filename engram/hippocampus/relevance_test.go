package hippocampus

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestScanMemoryFiles(t *testing.T) {
	dir := t.TempDir()

	// Create memory files with frontmatter
	writeMemFile(t, dir, "user_role.md", `---
name: user role
description: User is a senior Go developer
type: user
---
Senior Go developer working on engram.
`)

	writeMemFile(t, dir, "feedback_testing.md", `---
name: testing feedback
description: Prefers integration tests over mocks
type: feedback
---
Use real database in tests, not mocks.
`)

	// MEMORY.md should be skipped
	writeMemFile(t, dir, "MEMORY.md", "# Memory Index\n")

	// Non-.md files should be skipped
	writeMemFile(t, dir, "notes.txt", "some notes")

	// Directory should be skipped
	os.MkdirAll(filepath.Join(dir, "subdir"), 0o755)

	files, err := ScanMemoryFiles(dir)
	if err != nil {
		t.Fatalf("ScanMemoryFiles: %v", err)
	}

	if len(files) != 2 {
		t.Fatalf("expected 2 files, got %d", len(files))
	}

	// Check frontmatter parsing
	found := map[string]MemoryFile{}
	for _, f := range files {
		found[f.Name] = f
	}

	if f, ok := found["user_role.md"]; !ok {
		t.Error("missing user_role.md")
	} else {
		if f.Description != "User is a senior Go developer" {
			t.Errorf("description = %q, want %q", f.Description, "User is a senior Go developer")
		}
		if f.Type != "user" {
			t.Errorf("type = %q, want %q", f.Type, "user")
		}
	}

	if f, ok := found["feedback_testing.md"]; !ok {
		t.Error("missing feedback_testing.md")
	} else {
		if f.Type != "feedback" {
			t.Errorf("type = %q, want %q", f.Type, "feedback")
		}
	}
}

func TestScanMemoryFiles_SortedByMtime(t *testing.T) {
	dir := t.TempDir()

	// Create files with different mtimes
	writeMemFile(t, dir, "old.md", "---\ndescription: old\ntype: project\n---\nold content")
	time.Sleep(10 * time.Millisecond)
	writeMemFile(t, dir, "new.md", "---\ndescription: new\ntype: project\n---\nnew content")

	files, err := ScanMemoryFiles(dir)
	if err != nil {
		t.Fatalf("ScanMemoryFiles: %v", err)
	}

	if len(files) != 2 {
		t.Fatalf("expected 2 files, got %d", len(files))
	}

	// Newest first
	if files[0].Name != "new.md" {
		t.Errorf("expected newest first, got %s", files[0].Name)
	}
}

func TestScanMemoryFiles_Cap200(t *testing.T) {
	dir := t.TempDir()

	// Create 210 files
	for i := 0; i < 210; i++ {
		name := filepath.Join(dir, fileNameN(i))
		os.WriteFile(name, []byte("content"), 0o644)
	}

	files, err := ScanMemoryFiles(dir)
	if err != nil {
		t.Fatalf("ScanMemoryFiles: %v", err)
	}

	if len(files) > 200 {
		t.Errorf("expected cap at 200, got %d", len(files))
	}
}

func TestScanMemoryFiles_EmptyDir(t *testing.T) {
	dir := t.TempDir()
	files, err := ScanMemoryFiles(dir)
	if err != nil {
		t.Fatalf("ScanMemoryFiles: %v", err)
	}
	if len(files) != 0 {
		t.Errorf("expected 0 files, got %d", len(files))
	}
}

func TestScanMemoryFiles_NonexistentDir(t *testing.T) {
	files, err := ScanMemoryFiles("/nonexistent/path")
	if err != nil {
		t.Fatalf("expected nil error for nonexistent dir, got %v", err)
	}
	if files != nil {
		t.Errorf("expected nil files, got %v", files)
	}
}

func TestFindRelevantMemories(t *testing.T) {
	dir := t.TempDir()

	writeMemFile(t, dir, "go_prefs.md", "---\ndescription: Go coding preferences\ntype: feedback\n---\nPrefer table-driven tests.")
	writeMemFile(t, dir, "python_setup.md", "---\ndescription: Python environment setup\ntype: reference\n---\nUse pyenv.")
	writeMemFile(t, dir, "api_design.md", "---\ndescription: API design decisions\ntype: project\n---\nREST over gRPC.")

	files, err := ScanMemoryFiles(dir)
	if err != nil {
		t.Fatalf("ScanMemoryFiles: %v", err)
	}

	// Mock sideQuery that returns indices 0 and 2 as relevant
	mockSideQuery := func(_ context.Context, _, _ string, _ int) (string, error) {
		return `{"selected": [{"index": 0, "score": 0.9}, {"index": 2, "score": 0.7}]}`, nil
	}

	results, err := FindRelevantMemories(context.Background(), "Go testing", files, mockSideQuery, nil)
	if err != nil {
		t.Fatalf("FindRelevantMemories: %v", err)
	}

	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}

	// Should be sorted by score descending
	if results[0].Score < results[1].Score {
		t.Error("results not sorted by score descending")
	}
}

func TestFindRelevantMemories_ExcludePaths(t *testing.T) {
	dir := t.TempDir()

	writeMemFile(t, dir, "a.md", "---\ndescription: file a\ntype: user\n---\ncontent a")
	writeMemFile(t, dir, "b.md", "---\ndescription: file b\ntype: user\n---\ncontent b")

	files, err := ScanMemoryFiles(dir)
	if err != nil {
		t.Fatalf("ScanMemoryFiles: %v", err)
	}

	// Exclude all files
	exclude := map[string]bool{}
	for _, f := range files {
		exclude[f.Path] = true
	}

	mockSideQuery := func(_ context.Context, _, _ string, _ int) (string, error) {
		return `{"selected": []}`, nil
	}

	results, err := FindRelevantMemories(context.Background(), "test", files, mockSideQuery, exclude)
	if err != nil {
		t.Fatalf("FindRelevantMemories: %v", err)
	}

	if len(results) != 0 {
		t.Errorf("expected 0 results with all excluded, got %d", len(results))
	}
}

func TestFindRelevantMemories_NilSideQuery(t *testing.T) {
	files := []MemoryFile{{Path: "/tmp/test.md", Name: "test.md"}}
	results, err := FindRelevantMemories(context.Background(), "query", files, nil, nil)
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if results != nil {
		t.Errorf("expected nil results, got %v", results)
	}
}

func TestFindRelevantMemories_Cap5(t *testing.T) {
	dir := t.TempDir()
	for i := 0; i < 10; i++ {
		writeMemFile(t, dir, fileNameN(i), "---\ndescription: file\ntype: user\n---\ncontent")
	}

	files, err := ScanMemoryFiles(dir)
	if err != nil {
		t.Fatalf("ScanMemoryFiles: %v", err)
	}

	// Return all 10 as relevant
	mockSideQuery := func(_ context.Context, _, _ string, _ int) (string, error) {
		return `{"selected": [
			{"index": 0, "score": 0.9}, {"index": 1, "score": 0.85},
			{"index": 2, "score": 0.8}, {"index": 3, "score": 0.75},
			{"index": 4, "score": 0.7}, {"index": 5, "score": 0.65},
			{"index": 6, "score": 0.6}, {"index": 7, "score": 0.55}
		]}`, nil
	}

	results, err := FindRelevantMemories(context.Background(), "test", files, mockSideQuery, nil)
	if err != nil {
		t.Fatalf("FindRelevantMemories: %v", err)
	}

	if len(results) > 5 {
		t.Errorf("expected cap at 5, got %d", len(results))
	}
}

func TestFindRelevantMemories_MarkdownWrappedJSON(t *testing.T) {
	dir := t.TempDir()
	writeMemFile(t, dir, "test.md", "---\ndescription: test\ntype: user\n---\ncontent")

	files, err := ScanMemoryFiles(dir)
	if err != nil {
		t.Fatalf("ScanMemoryFiles: %v", err)
	}

	// LLM wraps response in markdown code fence
	mockSideQuery := func(_ context.Context, _, _ string, _ int) (string, error) {
		return "```json\n{\"selected\": [{\"index\": 0, \"score\": 0.8}]}\n```", nil
	}

	results, err := FindRelevantMemories(context.Background(), "test", files, mockSideQuery, nil)
	if err != nil {
		t.Fatalf("FindRelevantMemories: %v", err)
	}

	if len(results) != 1 {
		t.Fatalf("expected 1 result from markdown-wrapped JSON, got %d", len(results))
	}
}

func TestExtractJSON(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{`{"selected": []}`, `{"selected": []}`},
		{"```json\n{\"selected\": []}\n```", `{"selected": []}`},
		{"Here is the result: {\"selected\": [{\"index\": 0}]}", `{"selected": [{"index": 0}]}`},
		{"no json here", ""},
	}

	for _, tt := range tests {
		got := extractJSON(tt.input)
		if got != tt.want {
			t.Errorf("extractJSON(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

// --- helpers ---

func writeMemFile(t *testing.T, dir, name, content string) {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
}

func fileNameN(i int) string {
	return filepath.Base(filepath.Join(".", filepath.Clean(
		"mem-"+padInt(i)+".md",
	)))
}

func padInt(i int) string {
	s := ""
	if i < 100 {
		s += "0"
	}
	if i < 10 {
		s += "0"
	}
	return s + itoa(i)
}

func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	s := ""
	for i > 0 {
		s = string(rune('0'+i%10)) + s
		i /= 10
	}
	return s
}
