package obsidian_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/vbonnet/dear-agent/pkg/source"
	"github.com/vbonnet/dear-agent/pkg/source/contract"
	"github.com/vbonnet/dear-agent/pkg/source/obsidian"
)

func TestAdapter_Contract(t *testing.T) {
	contract.RunSuite(t, func(t *testing.T) source.Adapter {
		t.Helper()
		dir := t.TempDir()
		a, err := obsidian.Open(dir)
		if err != nil {
			t.Fatalf("Open: %v", err)
		}
		return a
	})
}

func TestOpen_RejectsFileMasqueradingAsDir(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "vault")
	if err := os.WriteFile(f, []byte("not a dir"), 0o600); err != nil {
		t.Fatalf("seed: %v", err)
	}
	if _, err := obsidian.Open(f); err == nil {
		t.Fatal("expected error when vault path is a file")
	}
}

func TestOpen_CreatesMissingVault(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "fresh", "vault")
	a, err := obsidian.Open(dir)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer a.Close()
	if _, err := os.Stat(dir); err != nil {
		t.Errorf("vault dir not created: %v", err)
	}
}

func TestAddFetch_PreservesFrontmatter(t *testing.T) {
	dir := t.TempDir()
	a, err := obsidian.Open(dir)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer a.Close()

	ctx := context.Background()
	now := time.Date(2026, 5, 3, 10, 0, 0, 0, time.UTC)
	src := source.Source{
		URI:       "obsidian:///notes/research.md",
		Title:     "Research notes",
		Snippet:   "About routing",
		Content:   []byte("Body of the note."),
		IndexedAt: now,
		Metadata: source.Metadata{
			Cues:       []string{"routing", "llm"},
			WorkItem:   "run-1",
			Role:       "research",
			Confidence: 0.85,
			Source:     "test",
		},
	}
	ref, err := a.Add(ctx, src)
	if err != nil {
		t.Fatalf("Add: %v", err)
	}
	if ref.URI != src.URI {
		t.Errorf("Ref.URI = %q", ref.URI)
	}

	// File on disk should round-trip frontmatter and body.
	body, err := os.ReadFile(filepath.Join(dir, "notes", "research.md"))
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	got := string(body)
	if !contains(got, "title: Research notes") {
		t.Errorf("missing title in frontmatter:\n%s", got)
	}
	if !contains(got, "- routing") {
		t.Errorf("missing cues in frontmatter:\n%s", got)
	}

	res, err := a.Fetch(ctx, source.FetchQuery{Query: "routing"})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(res) != 1 {
		t.Fatalf("Fetch returned %d, want 1", len(res))
	}
	if res[0].Title != "Research notes" {
		t.Errorf("Title = %q", res[0].Title)
	}
}

func TestFetch_FilterByCue(t *testing.T) {
	dir := t.TempDir()
	a, _ := obsidian.Open(dir)
	defer a.Close()
	ctx := context.Background()
	_, _ = a.Add(ctx, source.Source{URI: "obsidian:///a.md", Title: "A", Content: []byte("alpha"), Metadata: source.Metadata{Cues: []string{"x"}}})
	_, _ = a.Add(ctx, source.Source{URI: "obsidian:///b.md", Title: "B", Content: []byte("beta"), Metadata: source.Metadata{Cues: []string{"y"}}})
	res, _ := a.Fetch(ctx, source.FetchQuery{Filters: source.Filters{Cues: []string{"x"}}})
	if len(res) != 1 || res[0].Title != "A" {
		t.Errorf("expected only A, got %+v", res)
	}
}

func TestFetch_KCap(t *testing.T) {
	dir := t.TempDir()
	a, _ := obsidian.Open(dir)
	defer a.Close()
	ctx := context.Background()
	for i := 0; i < 5; i++ {
		_, _ = a.Add(ctx, source.Source{URI: "obsidian:///" + string(rune('a'+i)) + ".md", Content: []byte("hit")})
	}
	res, _ := a.Fetch(ctx, source.FetchQuery{Query: "hit", K: 2})
	if len(res) != 2 {
		t.Errorf("len=%d, want 2", len(res))
	}
}

func TestFetch_TimeWindow(t *testing.T) {
	dir := t.TempDir()
	a, _ := obsidian.Open(dir)
	defer a.Close()
	ctx := context.Background()
	old := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	new_ := time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)
	_, _ = a.Add(ctx, source.Source{URI: "obsidian:///old.md", IndexedAt: old})
	_, _ = a.Add(ctx, source.Source{URI: "obsidian:///new.md", IndexedAt: new_})
	cutoff := time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC)
	res, _ := a.Fetch(ctx, source.FetchQuery{Filters: source.Filters{After: &cutoff}})
	if len(res) != 1 {
		t.Errorf("after-filter returned %d, want 1", len(res))
	}
}

func TestHealthCheck(t *testing.T) {
	dir := t.TempDir()
	a, _ := obsidian.Open(dir)
	if err := a.HealthCheck(context.Background()); err != nil {
		t.Errorf("HealthCheck: %v", err)
	}
	a.Close()
	if err := os.RemoveAll(dir); err != nil {
		t.Fatalf("rm: %v", err)
	}
	if err := a.HealthCheck(context.Background()); err == nil {
		t.Error("expected HealthCheck to fail after vault removed")
	}
}

func TestAdd_ExternalURIPreservedRoundTrip(t *testing.T) {
	dir := t.TempDir()
	a, _ := obsidian.Open(dir)
	defer a.Close()
	ctx := context.Background()
	const uri = "https://example.com/some/page.html"
	ref, err := a.Add(ctx, source.Source{URI: uri, Content: []byte("hello")})
	if err != nil {
		t.Fatalf("Add: %v", err)
	}
	if ref.URI != uri {
		t.Errorf("Ref.URI = %q, want %q", ref.URI, uri)
	}
	got, err := a.Fetch(ctx, source.FetchQuery{Query: "hello"})
	if err != nil || len(got) != 1 || got[0].URI != uri {
		t.Errorf("round-trip failed: got=%v err=%v", got, err)
	}
}

func contains(s, sub string) bool { return len(s) >= len(sub) && indexOf(s, sub) >= 0 }
func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
