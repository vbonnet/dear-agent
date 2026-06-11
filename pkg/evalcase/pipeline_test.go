package evalcase

import (
	"context"
	"testing"
	"time"
)

func fixedClock() func() time.Time {
	now := time.Date(2026, 6, 9, 8, 0, 0, 0, time.UTC)
	return func() time.Time { return now }
}

func TestPipeline_GeneratesOnlyProblematic(t *testing.T) {
	store := NewFileStore(t.TempDir())
	p := NewPipeline(store)
	p.Now = fixedClock()

	traces := []Trace{
		{TraceID: "good", Outcome: OutcomeSuccess, Spans: []Span{toolSpan("ok", "", 0)}},
		{TraceID: "bad1", Outcome: OutcomeError, Spans: []Span{toolSpan("boom", "err", 0)}},
		{TraceID: "bad2", Outcome: OutcomeStalled},
	}
	res, err := p.Run(context.Background(), traces)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if res.Scanned != 3 {
		t.Errorf("scanned = %d, want 3", res.Scanned)
	}
	if res.Problematic != 2 || res.Generated != 2 {
		t.Errorf("problematic=%d generated=%d, want 2/2", res.Problematic, res.Generated)
	}

	stored, _ := store.List()
	if len(stored) != 2 {
		t.Fatalf("stored = %d, want 2", len(stored))
	}
	for _, c := range stored {
		if c.ID == "good" {
			t.Fatal("clean trace produced an eval case")
		}
	}
}

func TestPipeline_SkipsAlreadyStored(t *testing.T) {
	store := NewFileStore(t.TempDir())
	p := NewPipeline(store)
	p.Now = fixedClock()

	traces := []Trace{{TraceID: "dup", Outcome: OutcomeError, Spans: []Span{toolSpan("x", "e", 0)}}}

	first, err := p.Run(context.Background(), traces)
	if err != nil {
		t.Fatalf("first run: %v", err)
	}
	if first.Generated != 1 || first.Skipped != 0 {
		t.Fatalf("first run generated=%d skipped=%d", first.Generated, first.Skipped)
	}

	second, err := p.Run(context.Background(), traces)
	if err != nil {
		t.Fatalf("second run: %v", err)
	}
	if second.Generated != 0 || second.Skipped != 1 {
		t.Fatalf("second run generated=%d skipped=%d, want 0/1", second.Generated, second.Skipped)
	}
	if second.Problematic != 1 {
		t.Errorf("second run problematic=%d, want 1", second.Problematic)
	}
}

func TestPipeline_NoStoreErrors(t *testing.T) {
	p := &Pipeline{Classifier: DefaultClassifierConfig()}
	if _, err := p.Run(context.Background(), []Trace{{TraceID: "x"}}); err == nil {
		t.Fatal("expected error with nil store")
	}
}

func TestPipeline_EmptyTraceIDStillStored(t *testing.T) {
	store := NewFileStore(t.TempDir())
	p := NewPipeline(store)
	p.Now = fixedClock()
	res, err := p.Run(context.Background(), []Trace{{TraceID: "", Outcome: OutcomeError}})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if res.Generated != 1 {
		t.Fatalf("generated = %d, want 1", res.Generated)
	}
	if !store.Has("unknown") {
		t.Fatal("empty-trace-id case not stored under 'unknown'")
	}
}
