package wikibrain_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/vbonnet/dear-agent/pkg/wikibrain"
)

func TestScanKB_BasicPage(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "01-decisions/ADR-001-go-language.md", `# ADR-001: Go as Primary Language

- **Last updated:** 2026-01-15
- **Related ADRs:** [[ADR-002]]

## Summary

Go is the primary language.

See also [topic-foo](../02-research-index/topic-foo.md).
`)

	pages, err := wikibrain.ScanKB(dir)
	if err != nil {
		t.Fatalf("ScanKB: %v", err)
	}
	if len(pages) != 1 {
		t.Fatalf("expected 1 page, got %d", len(pages))
	}

	p := pages[0]
	if p.Title != "ADR-001: Go as Primary Language" {
		t.Errorf("title = %q, want ADR-001: Go as Primary Language", p.Title)
	}
	if !p.HasLastUpdated {
		t.Error("expected HasLastUpdated=true")
	}
	want := time.Date(2026, 1, 15, 0, 0, 0, 0, time.UTC)
	if !p.LastUpdated.Equal(want) {
		t.Errorf("LastUpdated = %v, want %v", p.LastUpdated, want)
	}
	if len(p.WikiLinks) != 1 || p.WikiLinks[0] != "ADR-002" {
		t.Errorf("WikiLinks = %v, want [ADR-002]", p.WikiLinks)
	}
	if len(p.MarkdownLinks) != 1 {
		t.Errorf("MarkdownLinks = %v, want 1 entry", p.MarkdownLinks)
	}
}

func TestScanKB_SkipsPrivateAndDotDirs(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "00-private/secret.md", "# Secret\n")
	writeFile(t, dir, ".obsidian/app.json", "{}") // not .md but just in case
	writeFile(t, dir, "01-decisions/ADR-001.md", "# ADR\n")

	pages, err := wikibrain.ScanKB(dir)
	if err != nil {
		t.Fatalf("ScanKB: %v", err)
	}
	for _, p := range pages {
		if filepath.Dir(p.RelPath) == "00-private" {
			t.Errorf("private page leaked: %s", p.RelPath)
		}
	}
	if len(pages) != 1 {
		t.Errorf("expected 1 public page, got %d", len(pages))
	}
}

func TestScanKB_TitleFallsBackToFilename(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "02-research-index/topic-foo-bar.md", "No heading here.\n")

	pages, err := wikibrain.ScanKB(dir)
	if err != nil {
		t.Fatalf("ScanKB: %v", err)
	}
	if len(pages) != 1 {
		t.Fatalf("expected 1 page")
	}
	// Title derived from filename with hyphens replaced by spaces
	if pages[0].Title != "topic foo bar" {
		t.Errorf("title = %q", pages[0].Title)
	}
}

func writeFile(t *testing.T, root, rel, content string) {
	t.Helper()
	abs := filepath.Join(root, rel)
	if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(abs, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
