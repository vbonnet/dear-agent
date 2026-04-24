package ops

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestAppendAndRead(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "errors.jsonl")
	el := NewErrorLog(logPath)

	// Append 3 errors
	if err := el.AppendError("hook-error", "pre-commit failed", "git-hook"); err != nil {
		t.Fatalf("AppendError #1: %v", err)
	}
	if err := el.AppendError("test-failure", "TestFoo failed", "go-test"); err != nil {
		t.Fatalf("AppendError #2: %v", err)
	}
	if err := el.AppendError("build-failure", "compile error in main.go", "go-build"); err != nil {
		t.Fatalf("AppendError #3: %v", err)
	}

	// Read back all
	entries, err := el.ReadErrors(time.Time{})
	if err != nil {
		t.Fatalf("ReadErrors: %v", err)
	}

	if got := len(entries); got != 3 {
		t.Fatalf("got %d entries, want 3", got)
	}

	// Verify fields
	if entries[0].Category != CategoryHookError {
		t.Errorf("entry[0].Category = %q, want %q", entries[0].Category, CategoryHookError)
	}
	if entries[0].Message != "pre-commit failed" {
		t.Errorf("entry[0].Message = %q, want %q", entries[0].Message, "pre-commit failed")
	}
	if entries[0].Source != "git-hook" {
		t.Errorf("entry[0].Source = %q, want %q", entries[0].Source, "git-hook")
	}
	if entries[1].Category != CategoryTestFailure {
		t.Errorf("entry[1].Category = %q, want %q", entries[1].Category, CategoryTestFailure)
	}
	if entries[2].Category != CategoryBuildFailure {
		t.Errorf("entry[2].Category = %q, want %q", entries[2].Category, CategoryBuildFailure)
	}
}

func TestReadSince(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "errors.jsonl")
	el := NewErrorLog(logPath)

	// Write entries with controlled timestamps via appendEntry directly
	now := time.Now()
	old := ErrorEntry{
		Timestamp: now.Add(-2 * time.Hour),
		Category:  CategoryHookError,
		Message:   "old error",
		Source:    "test",
	}
	recent := ErrorEntry{
		Timestamp: now.Add(-30 * time.Minute),
		Category:  CategoryTestFailure,
		Message:   "recent error",
		Source:    "test",
	}
	newest := ErrorEntry{
		Timestamp: now.Add(-5 * time.Minute),
		Category:  CategoryBuildFailure,
		Message:   "newest error",
		Source:    "test",
	}

	for _, e := range []ErrorEntry{old, recent, newest} {
		if err := el.appendEntry(e); err != nil {
			t.Fatalf("appendEntry: %v", err)
		}
	}

	// Read since 1 hour ago — should get 2 entries
	since := now.Add(-1 * time.Hour)
	entries, err := el.ReadErrors(since)
	if err != nil {
		t.Fatalf("ReadErrors: %v", err)
	}

	if got := len(entries); got != 2 {
		t.Fatalf("got %d entries since 1h ago, want 2", got)
	}

	if entries[0].Message != "recent error" {
		t.Errorf("entries[0].Message = %q, want %q", entries[0].Message, "recent error")
	}
	if entries[1].Message != "newest error" {
		t.Errorf("entries[1].Message = %q, want %q", entries[1].Message, "newest error")
	}
}

func TestCategories(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "errors.jsonl")
	el := NewErrorLog(logPath)

	// Write entries of different categories
	entries := []ErrorEntry{
		{Timestamp: time.Now(), Category: CategoryHookError, Message: "hook1", Source: "s1"},
		{Timestamp: time.Now(), Category: CategoryTestFailure, Message: "test1", Source: "s2"},
		{Timestamp: time.Now(), Category: CategoryHookError, Message: "hook2", Source: "s3"},
		{Timestamp: time.Now(), Category: CategoryMergeConflict, Message: "merge1", Source: "s4"},
		{Timestamp: time.Now(), Category: CategoryTestFailure, Message: "test2", Source: "s5"},
	}

	for _, e := range entries {
		if err := el.appendEntry(e); err != nil {
			t.Fatalf("appendEntry: %v", err)
		}
	}

	// Filter by hook-error
	hookErrors, err := el.ReadErrorsByCategory("hook-error", time.Time{})
	if err != nil {
		t.Fatalf("ReadErrorsByCategory(hook-error): %v", err)
	}
	if got := len(hookErrors); got != 2 {
		t.Fatalf("hook-error count = %d, want 2", got)
	}

	// Filter by test-failure
	testFailures, err := el.ReadErrorsByCategory("test-failure", time.Time{})
	if err != nil {
		t.Fatalf("ReadErrorsByCategory(test-failure): %v", err)
	}
	if got := len(testFailures); got != 2 {
		t.Fatalf("test-failure count = %d, want 2", got)
	}

	// Filter by merge-conflict
	mergeConflicts, err := el.ReadErrorsByCategory("merge-conflict", time.Time{})
	if err != nil {
		t.Fatalf("ReadErrorsByCategory(merge-conflict): %v", err)
	}
	if got := len(mergeConflicts); got != 1 {
		t.Fatalf("merge-conflict count = %d, want 1", got)
	}

	// Filter by nonexistent category
	none, err := el.ReadErrorsByCategory("nonexistent", time.Time{})
	if err != nil {
		t.Fatalf("ReadErrorsByCategory(nonexistent): %v", err)
	}
	if got := len(none); got != 0 {
		t.Fatalf("nonexistent count = %d, want 0", got)
	}
}

func TestReadErrors_NoFile(t *testing.T) {
	el := NewErrorLog(filepath.Join(t.TempDir(), "does-not-exist.jsonl"))

	entries, err := el.ReadErrors(time.Time{})
	if err != nil {
		t.Fatalf("ReadErrors on missing file: %v", err)
	}
	if len(entries) != 0 {
		t.Errorf("got %d entries from missing file, want 0", len(entries))
	}
}

func TestIsValidCategory(t *testing.T) {
	for _, cat := range ValidCategories() {
		if !IsValidCategory(string(cat)) {
			t.Errorf("IsValidCategory(%q) = false, want true", cat)
		}
	}

	if IsValidCategory("bogus") {
		t.Error("IsValidCategory(bogus) = true, want false")
	}
}

func TestDefaultErrorLogPath(t *testing.T) {
	path := DefaultErrorLogPath()
	home, _ := os.UserHomeDir()
	want := filepath.Join(home, ".agm", "hook-errors.jsonl")
	if path != want {
		t.Errorf("DefaultErrorLogPath() = %q, want %q", path, want)
	}
}
