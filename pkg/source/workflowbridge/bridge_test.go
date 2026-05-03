package workflowbridge_test

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/vbonnet/dear-agent/pkg/source"
	sqliteadapter "github.com/vbonnet/dear-agent/pkg/source/sqlite"
	"github.com/vbonnet/dear-agent/pkg/source/workflowbridge"
	"github.com/vbonnet/dear-agent/pkg/workflow"
)

func TestBridge_Add_RoundTrip(t *testing.T) {
	dir := t.TempDir()
	a, err := sqliteadapter.Open(filepath.Join(dir, "sources.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer func() { _ = a.Close() }()

	idx := workflowbridge.New(a)
	if idx.Name() != "sqlite" {
		t.Fatalf("Name = %q", idx.Name())
	}

	now := time.Date(2026, 5, 2, 12, 0, 0, 0, time.UTC)
	if err := idx.Add(context.Background(), workflow.SourceArtifact{
		URI:         "workflow://run-1/research/report",
		Title:       "report.md",
		Snippet:     "first line",
		Content:     []byte("hello world"),
		ContentType: "text/markdown",
		WorkItem:    "run-1/research",
		Cues:        []string{"research", "report"},
		IndexedAt:   now,
	}); err != nil {
		t.Fatalf("Add: %v", err)
	}

	got, err := a.Fetch(context.Background(), source.FetchQuery{Filters: source.Filters{WorkItem: "run-1"}})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("Fetch returned %d, want 1", len(got))
	}
	if got[0].URI != "workflow://run-1/research/report" {
		t.Errorf("URI = %q", got[0].URI)
	}
	if got[0].Metadata.Source != "workflow" {
		t.Errorf("Metadata.Source = %q, want workflow", got[0].Metadata.Source)
	}
	if v, ok := got[0].Metadata.Custom["content_type"]; !ok || v != "text/markdown" {
		t.Errorf("Custom.content_type = %v, want text/markdown", v)
	}
}

func TestBridge_NilAdapter_ReturnsNil(t *testing.T) {
	if workflowbridge.New(nil) != nil {
		t.Fatal("expected nil bridge for nil adapter")
	}
}
