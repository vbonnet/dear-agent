// Package contract houses the substrate test suite that every
// pkg/source.Adapter implementation must pass. New adapters wire
// these tests via a thin wrapper:
//
//	func TestMyAdapter(t *testing.T) {
//	    contract.RunSuite(t, func(t *testing.T) source.Adapter {
//	        // construct a fresh, empty adapter for the test
//	    })
//	}
//
// The suite covers four named scenarios that map directly to the
// "Adapter contract" block in the workflow engineering research doc:
// AddFetch_RoundTrip, FetchByCue_Filters, FetchByWorkItem_Filters,
// HealthCheck. Tests are intentionally framework-agnostic — they use
// only stdlib testing and pkg/source — so an adapter that lives in a
// downstream module can pull this in and run it.
package contract

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/vbonnet/dear-agent/pkg/source"
)

// New is the factory the suite calls before every test. It must
// return a freshly-initialised adapter with no pre-existing rows.
type New func(t *testing.T) source.Adapter

// RunSuite executes the four contract tests against the adapter
// returned by mk. The caller registers a single Test* function in
// their package and delegates to RunSuite.
func RunSuite(t *testing.T, mk New) {
	t.Helper()
	t.Run("AddFetch_RoundTrip", func(t *testing.T) { testAddFetchRoundTrip(t, mk(t)) })
	t.Run("FetchByCue_Filters", func(t *testing.T) { testFetchByCueFilters(t, mk(t)) })
	t.Run("FetchByWorkItem_Filters", func(t *testing.T) { testFetchByWorkItemFilters(t, mk(t)) })
	t.Run("HealthCheck", func(t *testing.T) { testHealthCheck(t, mk(t)) })
}

func testAddFetchRoundTrip(t *testing.T, a source.Adapter) {
	t.Helper()
	defer func() { _ = a.Close() }()
	ctx := context.Background()
	now := time.Date(2026, 5, 2, 10, 0, 0, 0, time.UTC)

	src := source.Source{
		URI:       "https://example.com/research/llm-routing",
		Title:     "LLM routing strategies",
		Snippet:   "An overview of routing requests across model tiers.",
		Content:   []byte("Routing across primary/secondary/tertiary tiers..."),
		IndexedAt: now,
		Metadata: source.Metadata{
			Cues:       []string{"routing", "llm"},
			WorkItem:   "run-abc/research",
			Role:       "research",
			Confidence: 0.9,
			Source:     "test",
		},
	}
	ref, err := a.Add(ctx, src)
	if err != nil {
		t.Fatalf("Add: %v", err)
	}
	if ref.URI != src.URI {
		t.Fatalf("Ref.URI = %q, want %q", ref.URI, src.URI)
	}
	if ref.Backend != a.Name() {
		t.Fatalf("Ref.Backend = %q, want %q", ref.Backend, a.Name())
	}

	got, err := a.Fetch(ctx, source.FetchQuery{Query: "routing"})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("Fetch returned %d sources, want 1", len(got))
	}
	if got[0].URI != src.URI {
		t.Fatalf("Fetch URI = %q, want %q", got[0].URI, src.URI)
	}
	if got[0].Title != src.Title {
		t.Fatalf("Fetch Title = %q, want %q", got[0].Title, src.Title)
	}
	if !equalStrings(got[0].Metadata.Cues, src.Metadata.Cues) {
		t.Fatalf("Fetch Cues = %v, want %v", got[0].Metadata.Cues, src.Metadata.Cues)
	}
	if got[0].Metadata.WorkItem != src.Metadata.WorkItem {
		t.Fatalf("Fetch WorkItem = %q, want %q", got[0].Metadata.WorkItem, src.Metadata.WorkItem)
	}
}

func testFetchByCueFilters(t *testing.T, a source.Adapter) {
	t.Helper()
	defer func() { _ = a.Close() }()
	ctx := context.Background()

	mk := func(uri, title string, cues []string) source.Source {
		return source.Source{
			URI: uri, Title: title,
			Content:   []byte(title),
			IndexedAt: time.Now().UTC(),
			Metadata:  source.Metadata{Cues: cues, WorkItem: "run-1/n"},
		}
	}
	for _, s := range []source.Source{
		mk("u1", "alpha doc", []string{"alpha", "shared"}),
		mk("u2", "beta doc", []string{"beta", "shared"}),
		mk("u3", "alpha-beta doc", []string{"alpha", "beta", "shared"}),
	} {
		if _, err := a.Add(ctx, s); err != nil {
			t.Fatalf("Add %s: %v", s.URI, err)
		}
	}

	got, err := a.Fetch(ctx, source.FetchQuery{Filters: source.Filters{Cues: []string{"alpha"}}})
	if err != nil {
		t.Fatalf("Fetch alpha: %v", err)
	}
	if !uriSetEquals(got, []string{"u1", "u3"}) {
		t.Fatalf("Cues=alpha returned %v, want u1+u3", uris(got))
	}

	got, err = a.Fetch(ctx, source.FetchQuery{Filters: source.Filters{Cues: []string{"alpha", "beta"}}})
	if err != nil {
		t.Fatalf("Fetch alpha+beta: %v", err)
	}
	if !uriSetEquals(got, []string{"u3"}) {
		t.Fatalf("Cues=alpha+beta returned %v, want u3", uris(got))
	}
}

func testFetchByWorkItemFilters(t *testing.T, a source.Adapter) {
	t.Helper()
	defer func() { _ = a.Close() }()
	ctx := context.Background()

	mk := func(uri, work string) source.Source {
		return source.Source{
			URI: uri, Title: uri,
			Content:   []byte("c"),
			IndexedAt: time.Now().UTC(),
			Metadata:  source.Metadata{WorkItem: work},
		}
	}
	for _, s := range []source.Source{
		mk("u1", "run-A/n1"),
		mk("u2", "run-A/n2"),
		mk("u3", "run-B/n1"),
	} {
		if _, err := a.Add(ctx, s); err != nil {
			t.Fatalf("Add %s: %v", s.URI, err)
		}
	}

	got, err := a.Fetch(ctx, source.FetchQuery{Filters: source.Filters{WorkItem: "run-A/n1"}})
	if err != nil {
		t.Fatalf("Fetch run-A/n1: %v", err)
	}
	if !uriSetEquals(got, []string{"u1"}) {
		t.Fatalf("WorkItem exact returned %v, want u1", uris(got))
	}

	got, err = a.Fetch(ctx, source.FetchQuery{Filters: source.Filters{WorkItem: "run-A"}})
	if err != nil {
		t.Fatalf("Fetch run-A prefix: %v", err)
	}
	if !uriSetEquals(got, []string{"u1", "u2"}) {
		t.Fatalf("WorkItem prefix returned %v, want u1+u2", uris(got))
	}
}

func testHealthCheck(t *testing.T, a source.Adapter) {
	t.Helper()
	defer func() { _ = a.Close() }()
	if err := a.HealthCheck(context.Background()); err != nil {
		t.Fatalf("HealthCheck on a fresh adapter: %v", err)
	}
}

func uris(sources []source.Source) []string {
	out := make([]string, len(sources))
	for i, s := range sources {
		out[i] = s.URI
	}
	return out
}

func uriSetEquals(sources []source.Source, want []string) bool {
	if len(sources) != len(want) {
		return false
	}
	seen := make(map[string]struct{}, len(want))
	for _, w := range want {
		seen[w] = struct{}{}
	}
	for _, s := range sources {
		if _, ok := seen[s.URI]; !ok {
			return false
		}
	}
	return true
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if strings.TrimSpace(a[i]) != strings.TrimSpace(b[i]) {
			return false
		}
	}
	return true
}
