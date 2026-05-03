package workflow

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

// fakeIndexer is a SourceIndexer test double that captures every Add
// call so the wiring assertions can inspect what would have been
// pushed into pkg/source.
type fakeIndexer struct {
	name string
	got  []SourceArtifact
	err  error
}

func (f *fakeIndexer) Name() string { return f.name }
func (f *fakeIndexer) Add(_ context.Context, s SourceArtifact) error {
	if f.err != nil {
		return f.err
	}
	f.got = append(f.got, s)
	return nil
}

func TestOutputWriter_EngramIndexed_CallsAdd(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "report.md")
	if err := os.WriteFile(path, []byte("# Report\n\nFirst paragraph.\n"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	idx := &fakeIndexer{name: "sqlite"}
	w := &OutputWriter{WorkflowDir: dir, SourceIndexer: idx}
	specs := map[string]OutputSpec{
		"report": {Path: "report.md", Durability: DurabilityEngramIndexed, ContentType: "text/markdown"},
	}
	err := w.MaterialiseOutputs(context.Background(), "run-1", "research", specs, nil)
	if err != nil {
		t.Fatalf("MaterialiseOutputs: %v", err)
	}
	if len(idx.got) != 1 {
		t.Fatalf("indexer received %d artifacts, want 1", len(idx.got))
	}
	got := idx.got[0]
	wantURI := "workflow://run-1/research/report"
	if got.URI != wantURI {
		t.Errorf("URI = %q, want %q", got.URI, wantURI)
	}
	if got.WorkItem != "run-1/research" {
		t.Errorf("WorkItem = %q, want run-1/research", got.WorkItem)
	}
	if got.Snippet != "# Report" {
		t.Errorf("Snippet = %q, want '# Report'", got.Snippet)
	}
	if string(got.Content) == "" {
		t.Errorf("Content empty")
	}
	if got.ContentType != "text/markdown" {
		t.Errorf("ContentType = %q, want text/markdown", got.ContentType)
	}
}

func TestOutputWriter_EngramIndexed_NoIndexerIsOK(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "report.md")
	if err := os.WriteFile(path, []byte("hello"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	w := &OutputWriter{WorkflowDir: dir} // SourceIndexer is nil
	err := w.MaterialiseOutputs(context.Background(), "run-1", "n", map[string]OutputSpec{
		"r": {Path: "report.md", Durability: DurabilityEngramIndexed},
	}, nil)
	if err != nil {
		t.Fatalf("MaterialiseOutputs without indexer: %v", err)
	}
}

func TestOutputWriter_EngramIndexed_RecordsBeforeIndexing(t *testing.T) {
	// Sanity check: even when the indexer fails, the node_outputs row
	// for the artifact has already been written. This matches the
	// substrate principle: the work-item ledger is authoritative; the
	// knowledge-store side is best-effort.
	dir := t.TempDir()
	path := filepath.Join(dir, "report.md")
	if err := os.WriteFile(path, []byte("hello"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	rec := &captureRecorder{}
	idx := &fakeIndexer{name: "sqlite", err: errBoom}
	w := &OutputWriter{WorkflowDir: dir, Recorder: rec, SourceIndexer: idx}
	err := w.MaterialiseOutputs(context.Background(), "run-1", "n",
		map[string]OutputSpec{"r": {Path: "report.md", Durability: DurabilityEngramIndexed}}, nil)
	if err == nil {
		t.Fatal("expected indexer error to surface")
	}
	if len(rec.records) != 1 {
		t.Fatalf("recorder got %d, want 1 (record happens before indexing)", len(rec.records))
	}
	if rec.records[0].Durability != DurabilityEngramIndexed {
		t.Errorf("recorder durability = %q", rec.records[0].Durability)
	}
}

type captureRecorder struct{ records []OutputRecord }

func (c *captureRecorder) RecordOutput(_ context.Context, r OutputRecord) error {
	c.records = append(c.records, r)
	return nil
}

var errBoom = &boomErr{}

type boomErr struct{}

func (*boomErr) Error() string { return "boom" }
