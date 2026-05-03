package llmwiki_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/vbonnet/dear-agent/pkg/source"
	"github.com/vbonnet/dear-agent/pkg/source/contract"
	"github.com/vbonnet/dear-agent/pkg/source/llmwiki"
)

func TestAdapter_Contract(t *testing.T) {
	contract.RunSuite(t, func(t *testing.T) source.Adapter {
		t.Helper()
		dir := t.TempDir()
		a, err := llmwiki.Open(dir)
		if err != nil {
			t.Fatalf("Open: %v", err)
		}
		return a
	})
}

func TestOpen_RejectsFilePath(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "wiki")
	_ = os.WriteFile(f, []byte{}, 0o600)
	if _, err := llmwiki.Open(f); err == nil {
		t.Fatal("expected error when path is a file")
	}
}

func TestAddFetch_WikiSchemeRoundTrip(t *testing.T) {
	dir := t.TempDir()
	a, _ := llmwiki.Open(dir)
	defer a.Close()
	ctx := context.Background()
	now := time.Date(2026, 5, 3, 10, 0, 0, 0, time.UTC)
	src := source.Source{
		URI:       "wiki:///pages/research.md",
		Title:     "Research",
		Content:   []byte("Body"),
		IndexedAt: now,
		Metadata: source.Metadata{
			Cues:     []string{"x"},
			WorkItem: "run-1/n",
		},
	}
	ref, err := a.Add(ctx, src)
	if err != nil {
		t.Fatalf("Add: %v", err)
	}
	if ref.URI != src.URI {
		t.Errorf("Ref.URI = %q", ref.URI)
	}
	res, err := a.Fetch(ctx, source.FetchQuery{Filters: source.Filters{Cues: []string{"x"}}})
	if err != nil || len(res) != 1 || res[0].URI != src.URI {
		t.Errorf("round-trip failed: res=%v err=%v", res, err)
	}
}

func TestAdd_AutoCommitInRealRepo(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not on PATH")
	}
	dir := t.TempDir()
	for _, args := range [][]string{
		{"init", "-q"},
		{"config", "user.email", "test@example.com"},
		{"config", "user.name", "Test"},
		{"commit", "--allow-empty", "-m", "init"},
	} {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v (%s)", args, err, string(out))
		}
	}
	a, err := llmwiki.Open(dir)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if !a.AutoCommit {
		t.Fatal("expected AutoCommit=true in a git repo")
	}
	defer a.Close()
	_, err = a.Add(context.Background(), source.Source{
		URI:     "wiki:///x.md",
		Title:   "X",
		Content: []byte("contents"),
	})
	if err != nil {
		t.Fatalf("Add: %v", err)
	}
	// Two commits: init + the source add.
	cmd := exec.Command("git", "log", "--oneline")
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git log: %v", err)
	}
	if string(out) == "" {
		t.Errorf("git log empty")
	}
}

func TestHealthCheck_FailsWhenDirGone(t *testing.T) {
	dir := t.TempDir()
	a, _ := llmwiki.Open(dir)
	defer a.Close()
	if err := a.HealthCheck(context.Background()); err != nil {
		t.Errorf("HealthCheck initial: %v", err)
	}
	_ = os.RemoveAll(dir)
	if err := a.HealthCheck(context.Background()); err == nil {
		t.Error("expected HealthCheck to fail")
	}
}

func TestEncodeDecode_Symmetry(t *testing.T) {
	dir := t.TempDir()
	a, _ := llmwiki.Open(dir)
	defer a.Close()
	ctx := context.Background()
	src := source.Source{
		URI:     "wiki:///nested/page.md",
		Title:   "Nested",
		Snippet: "Snip",
		Content: []byte("body lines\nmore body"),
		Metadata: source.Metadata{
			Cues:       []string{"a", "b"},
			Role:       "research",
			Confidence: 0.5,
		},
	}
	if _, err := a.Add(ctx, src); err != nil {
		t.Fatalf("Add: %v", err)
	}
	got, err := a.Fetch(ctx, source.FetchQuery{Query: "body lines"})
	if err != nil || len(got) != 1 {
		t.Fatalf("Fetch: %v %v", got, err)
	}
	if got[0].Title != "Nested" {
		t.Errorf("Title = %q", got[0].Title)
	}
	if string(got[0].Content) != "body lines\nmore body\n" && string(got[0].Content) != "body lines\nmore body" {
		t.Errorf("Content round-trip lost data: %q", string(got[0].Content))
	}
}
