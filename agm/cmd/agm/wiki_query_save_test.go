package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestResolveWikiTextInputReadsFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "answer.md")
	want := "answer with `shell` characters: $() ' \""
	if err := os.WriteFile(path, []byte(want), 0o600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	got, err := resolveWikiTextInput("answer", "", path)
	if err != nil {
		t.Fatalf("resolveWikiTextInput: %v", err)
	}
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestResolveWikiTextInputRejectsConflictingInputs(t *testing.T) {
	_, err := resolveWikiTextInput("query", "inline", "query.txt")
	if err == nil || !strings.Contains(err.Error(), "mutually exclusive") {
		t.Fatalf("expected mutually-exclusive error, got %v", err)
	}
}

func TestResolveWikiTextInputRejectsEmptyInput(t *testing.T) {
	_, err := resolveWikiTextInput("answer", "  \n", "")
	if err == nil || !strings.Contains(err.Error(), "is required") {
		t.Fatalf("expected required-input error, got %v", err)
	}
}

func TestResolveWikiTextInputBoundsFileReads(t *testing.T) {
	path := filepath.Join(t.TempDir(), "oversized.txt")
	data := strings.Repeat("x", maxWikiQueryInputBytes+1)
	if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	if _, err := resolveWikiTextInput("answer", "", path); err == nil || !strings.Contains(err.Error(), "exceeds") {
		t.Fatalf("expected bounded-input error, got %v", err)
	}
}

func TestResolveWikiOutputPathRejectsTraversal(t *testing.T) {
	kb := t.TempDir()
	for _, output := range []string{"../outside.md", "nested/../../outside.md", filepath.Join(string(filepath.Separator), "tmp", "outside.md"), "."} {
		if _, err := resolveWikiOutputPath(kb, output); err == nil {
			t.Errorf("resolveWikiOutputPath accepted %q", output)
		}
	}
	got, err := resolveWikiOutputPath(kb, "02-research-index/topic.md")
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(kb, "02-research-index", "topic.md")
	if got != want {
		t.Fatalf("resolved output = %q, want %q", got, want)
	}
}

func TestResolveWikiOutputPathRejectsSymlinkTraversal(t *testing.T) {
	kb := t.TempDir()
	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(kb, "linked")); err != nil {
		t.Fatalf("create symlink fixture: %v", err)
	}
	if _, err := resolveWikiOutputPath(kb, "linked/nested/topic.md"); err == nil {
		t.Fatal("resolveWikiOutputPath accepted a path through an escaping symlink")
	}
}
