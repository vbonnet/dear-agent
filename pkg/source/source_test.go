package source_test

import (
	"testing"
	"time"

	"github.com/vbonnet/dear-agent/pkg/source"
)

func TestErrNotFound_IsNonNil(t *testing.T) {
	t.Parallel()
	if source.ErrNotFound == nil {
		t.Error("ErrNotFound is nil, want non-nil sentinel")
	}
}

func TestFetchQuery_ZeroValue(t *testing.T) {
	t.Parallel()
	q := source.FetchQuery{}
	if q.K != 0 {
		t.Errorf("FetchQuery zero K = %d, want 0", q.K)
	}
	if q.Rerank {
		t.Error("FetchQuery zero Rerank = true, want false")
	}
}

func TestSource_Construction(t *testing.T) {
	t.Parallel()
	now := time.Now()
	s := source.Source{
		URI:       "engram://test/source",
		Title:     "Test Source",
		Content:   []byte("hello"),
		IndexedAt: now,
		Metadata: source.Metadata{
			Cues:     []string{"test", "unit"},
			WorkItem: "ce-1234",
			Role:     "research",
		},
	}
	if s.URI != "engram://test/source" {
		t.Errorf("URI = %q, want engram://test/source", s.URI)
	}
	if len(s.Metadata.Cues) != 2 {
		t.Errorf("Cues len = %d, want 2", len(s.Metadata.Cues))
	}
}

func TestRef_Fields(t *testing.T) {
	t.Parallel()
	now := time.Now()
	ref := source.Ref{
		URI:       "engram://test/ref",
		Backend:   "sqlite",
		IndexedAt: now,
	}
	if ref.Backend != "sqlite" {
		t.Errorf("Backend = %q, want sqlite", ref.Backend)
	}
}
