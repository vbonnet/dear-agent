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
