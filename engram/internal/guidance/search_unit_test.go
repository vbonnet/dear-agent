package guidance

import (
	"os"
	"path/filepath"
	"testing"
)

func TestNewSearchService_NotNil(t *testing.T) {
	t.Parallel()
	svc := NewSearchService("/some/path")
	if svc == nil {
		t.Fatal("NewSearchService() returned nil")
	}
}

func TestSearch_EmptyDir_Error(t *testing.T) {
	t.Parallel()
	// An empty dir has no .ai.md files — collectGuidanceFiles returns an error.
	svc := NewSearchService(t.TempDir())
	_, err := svc.Search(SearchOptions{Query: "test"})
	if err == nil {
		t.Error("Search(empty dir): expected error for no guidance files, got nil")
	}
}

func TestSearch_FindsMatchingFile(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	content := `---
title: My Guidance Doc
description: A document about routing
domain: routing
type: tutorial
tags:
  - llm
  - routing
---
# Content here
`
	path := filepath.Join(dir, "my-doc.ai.md")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	svc := NewSearchService(dir)
	results, err := svc.Search(SearchOptions{Query: "routing"})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(results) != 1 {
		t.Errorf("Search(routing): got %d results, want 1", len(results))
	}
}

func TestSearch_FilterByDomain(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	mkFile := func(name, domain string) {
		content := "---\ntitle: " + name + "\ndomain: " + domain + "\n---\ntext\n"
		if err := os.WriteFile(filepath.Join(dir, name+".ai.md"), []byte(content), 0o600); err != nil {
			t.Fatalf("WriteFile %s: %v", name, err)
		}
	}
	mkFile("alpha", "routing")
	mkFile("beta", "security")

	svc := NewSearchService(dir)
	results, err := svc.Search(SearchOptions{Domain: "routing"})
	if err != nil {
		t.Fatalf("Search(domain=routing): %v", err)
	}
	if len(results) != 1 {
		t.Errorf("Search(domain=routing): got %d results, want 1", len(results))
	}
}

func TestCalculateScore_QueryMatch(t *testing.T) {
	t.Parallel()
	fm := &Frontmatter{
		Title:       "LLM Routing",
		Description: "about routing",
		Tags:        []string{"routing"},
	}
	scoreWithMatch := calculateScore(fm, "routing")
	scoreNoMatch := calculateScore(fm, "xxxxxxxx")
	if scoreWithMatch <= scoreNoMatch {
		t.Errorf("score with match (%d) should be > score without match (%d)", scoreWithMatch, scoreNoMatch)
	}
}
