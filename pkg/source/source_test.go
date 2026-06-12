package source_test

import (
	"errors"
	"testing"

	"github.com/vbonnet/dear-agent/pkg/source"
)

func TestErrNotFound(t *testing.T) {
	if source.ErrNotFound == nil {
		t.Fatal("ErrNotFound must be non-nil")
	}
	// Sentinel errors should wrap cleanly with errors.Is.
	wrapped := errors.Join(source.ErrNotFound, errors.New("extra context"))
	if !errors.Is(wrapped, source.ErrNotFound) {
		t.Error("errors.Is failed for wrapped ErrNotFound")
	}
}

func TestFetchQueryDefaults(t *testing.T) {
	var q source.FetchQuery
	// K == 0 means "adapter default" per the package doc.
	if q.K != 0 {
		t.Errorf("FetchQuery zero value K = %d, want 0", q.K)
	}
	// Rerank off by default.
	if q.Rerank {
		t.Error("FetchQuery zero value Rerank = true, want false")
	}
}

func TestFiltersZeroValue(t *testing.T) {
	var f source.Filters
	// An empty Filters must not have non-nil Cues so callers can check len.
	if f.Cues != nil {
		t.Errorf("Filters zero value Cues = %v, want nil", f.Cues)
	}
	if f.Backend != "" {
		t.Errorf("Filters zero value Backend = %q, want empty", f.Backend)
	}
	if f.After != nil || f.Before != nil {
		t.Error("Filters zero value time pointers should be nil")
	}
}

func TestSourceZeroValue(t *testing.T) {
	var s source.Source
	if s.Score != 0 {
		t.Errorf("Source zero value Score = %v, want 0", s.Score)
	}
}
