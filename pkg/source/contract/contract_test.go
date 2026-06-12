package contract

import (
	"bytes"
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/vbonnet/dear-agent/pkg/source"
)

// memAdapter is an in-memory source.Adapter that satisfies the contract.
type memAdapter struct {
	mu      sync.RWMutex
	sources []source.Source
}

func newMemAdapter(_ *testing.T) source.Adapter { return &memAdapter{} }

func (m *memAdapter) Name() string { return "mock" }

func (m *memAdapter) HealthCheck(_ context.Context) error { return nil }

func (m *memAdapter) Close() error { return nil }

func (m *memAdapter) Add(_ context.Context, s source.Source) (source.Ref, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	// Upsert by URI.
	for i, existing := range m.sources {
		if existing.URI == s.URI {
			m.sources[i] = s
			return source.Ref{URI: s.URI, Backend: "mock", IndexedAt: s.IndexedAt}, nil
		}
	}
	m.sources = append(m.sources, s)
	return source.Ref{URI: s.URI, Backend: "mock", IndexedAt: s.IndexedAt}, nil
}

func (m *memAdapter) Fetch(_ context.Context, q source.FetchQuery) ([]source.Source, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var out []source.Source
	for _, s := range m.sources {
		if !matchesFilters(s, q.Filters) {
			continue
		}
		if q.Query != "" && !containsText(s, q.Query) {
			continue
		}
		out = append(out, s)
	}
	return out, nil
}

func matchesFilters(s source.Source, f source.Filters) bool {
	// Cue filter: source must have ALL requested cues.
	for _, cue := range f.Cues {
		found := false
		for _, c := range s.Metadata.Cues {
			if c == cue {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	// WorkItem filter: exact or prefix match.
	if f.WorkItem != "" {
		if s.Metadata.WorkItem != f.WorkItem &&
			!strings.HasPrefix(s.Metadata.WorkItem, f.WorkItem+"/") {
			return false
		}
	}
	// Time filters.
	if f.After != nil && !s.IndexedAt.After(*f.After) {
		return false
	}
	if f.Before != nil && !s.IndexedAt.Before(*f.Before) {
		return false
	}
	return true
}

func containsText(s source.Source, q string) bool {
	ql := strings.ToLower(q)
	return strings.Contains(strings.ToLower(s.URI), ql) ||
		strings.Contains(strings.ToLower(s.Title), ql) ||
		bytes.Contains(bytes.ToLower(s.Content), []byte(ql))
}

func TestMemAdapter_ContractSuite(t *testing.T) {
	RunSuite(t, newMemAdapter)
}

func TestRunSuite_RejectsNoHealthCheck(t *testing.T) {
	// Verify RunSuite can be invoked and reports subtests via t.Run.
	// This test exercises the RunSuite dispatch path itself.
	called := 0
	factory := func(t *testing.T) source.Adapter {
		called++
		return newMemAdapter(t)
	}
	RunSuite(t, factory)
	if called == 0 {
		t.Error("factory was never called — RunSuite may not have dispatched")
	}
}

func TestFilters_WorkItemPrefixMatch(t *testing.T) {
	t.Parallel()
	a := &memAdapter{}
	ctx := context.Background()
	now := time.Now()

	for _, s := range []source.Source{
		{URI: "u1", Title: "u1", Content: []byte("c"), IndexedAt: now, Metadata: source.Metadata{WorkItem: "run-X/n1"}},
		{URI: "u2", Title: "u2", Content: []byte("c"), IndexedAt: now, Metadata: source.Metadata{WorkItem: "run-X/n2"}},
		{URI: "u3", Title: "u3", Content: []byte("c"), IndexedAt: now, Metadata: source.Metadata{WorkItem: "run-Y/n1"}},
	} {
		if _, err := a.Add(ctx, s); err != nil {
			t.Fatalf("Add %s: %v", s.URI, err)
		}
	}

	got, err := a.Fetch(ctx, source.FetchQuery{Filters: source.Filters{WorkItem: "run-X"}})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(got) != 2 {
		t.Errorf("prefix match returned %d results, want 2; URIs=%v", len(got), uris(got))
	}
}
