package compaction

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
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

func TestAllocatePromptExclusiveRetriesWithoutTruncatingExistingPrompt(t *testing.T) {
	baseDir := t.TempDir()
	existingPath, err := SavePrompt(baseDir, "stable-id", 1, "first delivery")
	if err != nil {
		t.Fatal(err)
	}

	allocation, err := AllocatePromptExclusive(baseDir, "stable-id", "second delivery")
	if err != nil {
		t.Fatalf("AllocatePromptExclusive() error = %v", err)
	}
	if allocation.Number != 2 {
		t.Fatalf("allocation number = %d, want 2", allocation.Number)
	}
	first, err := os.ReadFile(existingPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(first) != "first delivery" {
		t.Fatalf("existing prompt was truncated: %q", first)
	}
	second, err := os.ReadFile(allocation.Path)
	if err != nil {
		t.Fatal(err)
	}
	if string(second) != "second delivery" {
		t.Fatalf("allocated content = %q", second)
	}
}

func TestAllocatePromptExclusiveConcurrentWritersPreserveEveryPrompt(t *testing.T) {
	baseDir := t.TempDir()
	const writers = 48
	type result struct {
		allocation PromptAllocation
		content    string
		err        error
	}
	start := make(chan struct{})
	results := make(chan result, writers)
	var wg sync.WaitGroup
	for i := range writers {
		wg.Go(func() {
			content := fmt.Sprintf("delivery-%d", i)
			<-start
			allocation, err := AllocatePromptExclusive(baseDir, "stable-id", content)
			results <- result{allocation: allocation, content: content, err: err}
		})
	}
	close(start)
	wg.Wait()
	close(results)

	seenPaths := make(map[string]struct{}, writers)
	seenNumbers := make(map[int]struct{}, writers)
	for result := range results {
		if result.err != nil {
			t.Errorf("AllocatePromptExclusive() error = %v", result.err)
			continue
		}
		if _, exists := seenPaths[result.allocation.Path]; exists {
			t.Errorf("duplicate allocation path %q", result.allocation.Path)
		}
		seenPaths[result.allocation.Path] = struct{}{}
		if _, exists := seenNumbers[result.allocation.Number]; exists {
			t.Errorf("duplicate allocation number %d", result.allocation.Number)
		}
		seenNumbers[result.allocation.Number] = struct{}{}
		data, err := os.ReadFile(result.allocation.Path)
		if err != nil {
			t.Errorf("read allocation %q: %v", result.allocation.Path, err)
			continue
		}
		if string(data) != result.content {
			t.Errorf("allocation %q content = %q, want %q", result.allocation.Path, data, result.content)
		}
	}
	if len(seenPaths) != writers {
		t.Fatalf("unique prompt files = %d, want %d", len(seenPaths), writers)
	}
}

func TestAllocatePromptExclusiveUsesStableSessionIDAsAuditKey(t *testing.T) {
	baseDir := t.TempDir()
	allocation, err := AllocatePromptExclusive(baseDir, "2e6e8c25-7bca-4da7-8bc5-d7ce5c22d19c", "prompt")
	if err != nil {
		t.Fatal(err)
	}
	if got := filepath.Base(allocation.Path); got != "2e6e8c25-7bca-4da7-8bc5-d7ce5c22d19c-compact-1.md" {
		t.Fatalf("prompt filename = %q", got)
	}
	info, err := os.Stat(allocation.Path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("prompt permissions = %o, want 600", info.Mode().Perm())
	}
}

func TestAllocatePromptExclusiveTightensExistingPromptDirectoryPermissions(t *testing.T) {
	baseDir := t.TempDir()
	dir := promptDir(baseDir)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	// Make the starting mode independent of the process umask.
	if err := os.Chmod(dir, 0o755); err != nil {
		t.Fatal(err)
	}

	if _, err := AllocatePromptExclusive(baseDir, "stable-id", "sensitive prompt"); err != nil {
		t.Fatalf("AllocatePromptExclusive() error = %v", err)
	}
	info, err := os.Stat(dir)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != 0o700 {
		t.Fatalf("existing prompt directory permissions = %o, want 700", got)
	}
}

func TestAllocatePromptExclusiveRejectsPathTraversal(t *testing.T) {
	root := t.TempDir()
	baseDir := filepath.Join(root, "base")
	if _, err := AllocatePromptExclusive(baseDir, "../../outside", "prompt"); err == nil {
		t.Fatal("AllocatePromptExclusive() accepted a path-traversing stable ID")
	}
	if _, err := os.Stat(filepath.Join(root, "outside-compact-1.md")); !os.IsNotExist(err) {
		t.Fatalf("outside path was created: %v", err)
	}
}

func TestWriteExclusivePromptSyncsParentAfterClosingFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "prompt.md")
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	wantErr := errors.New("directory sync failed")
	called := false

	err = writeExclusivePromptWithDirSync(file, path, "prompt", func(gotDir string) error {
		called = true
		if gotDir != dir {
			t.Fatalf("sync directory = %q, want %q", gotDir, dir)
		}
		if _, statErr := file.Stat(); statErr == nil {
			t.Fatal("prompt file remained open when directory sync began")
		}
		data, readErr := os.ReadFile(path)
		if readErr != nil || string(data) != "prompt" {
			t.Fatalf("published prompt = %q, %v", data, readErr)
		}
		return wantErr
	})
	if !called {
		t.Fatal("prompt creation did not sync the containing directory")
	}
	if !errors.Is(err, wantErr) || !strings.Contains(err.Error(), "sync prompt directory") {
		t.Fatalf("writeExclusivePrompt error = %v, want directory-sync failure", err)
	}
	if _, statErr := os.Stat(path); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("unconfirmed prompt survived sync failure: %v", statErr)
	}
}
