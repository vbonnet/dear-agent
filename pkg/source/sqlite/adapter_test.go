package sqlite_test

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/vbonnet/dear-agent/pkg/source"
	"github.com/vbonnet/dear-agent/pkg/source/contract"
	sqliteadapter "github.com/vbonnet/dear-agent/pkg/source/sqlite"
)

// TestContractSuite runs the full pkg/source/contract suite against the
// SQLite + FTS5 adapter. New contract scenarios automatically apply
// here without touching this file.
func TestContractSuite(t *testing.T) {
	contract.RunSuite(t, func(t *testing.T) source.Adapter {
		return openTempAdapter(t)
	})
}

func TestAdapter_Add_OverwritesByURI(t *testing.T) {
	a := openTempAdapter(t)
	defer func() { _ = a.Close() }()
	ctx := context.Background()

	src := source.Source{URI: "u", Title: "v1", Content: []byte("first"),
		IndexedAt: time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)}
	if _, err := a.Add(ctx, src); err != nil {
		t.Fatalf("Add v1: %v", err)
	}
	src.Title = "v2"
	src.Content = []byte("second")
	src.IndexedAt = time.Date(2026, 5, 2, 0, 0, 0, 0, time.UTC)
	if _, err := a.Add(ctx, src); err != nil {
		t.Fatalf("Add v2: %v", err)
	}

	got, err := a.Fetch(ctx, source.FetchQuery{Query: "second"})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(got) != 1 || got[0].Title != "v2" {
		t.Fatalf("Fetch returned %+v, expected v2 only", got)
	}
	got, err = a.Fetch(ctx, source.FetchQuery{Query: "first"})
	if err != nil {
		t.Fatalf("Fetch first: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("Fetch first returned %d, expected 0 after overwrite", len(got))
	}
}

func TestAdapter_Fetch_TimeFilters(t *testing.T) {
	a := openTempAdapter(t)
	defer func() { _ = a.Close() }()
	ctx := context.Background()

	mk := func(uri string, ts time.Time) source.Source {
		return source.Source{URI: uri, Title: uri, Content: []byte("c"), IndexedAt: ts}
	}
	t1 := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	t2 := time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC)
	t3 := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
	for _, s := range []source.Source{mk("u1", t1), mk("u2", t2), mk("u3", t3)} {
		if _, err := a.Add(ctx, s); err != nil {
			t.Fatalf("Add %s: %v", s.URI, err)
		}
	}

	cutoff := time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC)
	got, err := a.Fetch(ctx, source.FetchQuery{Filters: source.Filters{After: &cutoff}})
	if err != nil {
		t.Fatalf("Fetch After: %v", err)
	}
	if len(got) != 2 || got[0].URI != "u3" || got[1].URI != "u2" {
		t.Fatalf("After=Feb returned %v, expected u3 then u2", uris(got))
	}

	end := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
	got, err = a.Fetch(ctx, source.FetchQuery{Filters: source.Filters{After: &t1, Before: &end}})
	if err != nil {
		t.Fatalf("Fetch After+Before: %v", err)
	}
	if len(got) != 2 || got[0].URI != "u2" || got[1].URI != "u1" {
		t.Fatalf("range returned %v, expected u2 then u1", uris(got))
	}
}

func TestAdapter_Fetch_EmptyQuery_OrdersByIndexedAtDesc(t *testing.T) {
	a := openTempAdapter(t)
	defer func() { _ = a.Close() }()
	ctx := context.Background()

	mk := func(uri string, ts time.Time) source.Source {
		return source.Source{URI: uri, Title: uri, Content: []byte("c"), IndexedAt: ts}
	}
	t1 := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	t2 := time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC)
	t3 := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
	for _, s := range []source.Source{mk("u1", t1), mk("u3", t3), mk("u2", t2)} {
		if _, err := a.Add(ctx, s); err != nil {
			t.Fatalf("Add: %v", err)
		}
	}
	got, err := a.Fetch(ctx, source.FetchQuery{})
	if err != nil {
		t.Fatalf("Fetch empty: %v", err)
	}
	if len(got) != 3 || got[0].URI != "u3" || got[1].URI != "u2" || got[2].URI != "u1" {
		t.Fatalf("ordering = %v, expected u3 u2 u1", uris(got))
	}
}

func TestAdapter_Fetch_KCap(t *testing.T) {
	a := openTempAdapter(t)
	defer func() { _ = a.Close() }()
	ctx := context.Background()
	for i := 0; i < 25; i++ {
		_, err := a.Add(ctx, source.Source{
			URI: pad("u", i), Title: "t", Content: []byte("hello"),
			IndexedAt: time.Date(2026, 1, 1, 0, i, 0, 0, time.UTC),
		})
		if err != nil {
			t.Fatalf("Add %d: %v", i, err)
		}
	}
	got, err := a.Fetch(ctx, source.FetchQuery{Query: "hello", K: 5})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(got) != 5 {
		t.Fatalf("K=5 returned %d", len(got))
	}
	got, err = a.Fetch(ctx, source.FetchQuery{Query: "hello"})
	if err != nil {
		t.Fatalf("Fetch default: %v", err)
	}
	if len(got) != 10 {
		t.Fatalf("default K returned %d, expected 10", len(got))
	}
}

func TestAdapter_FetchByURI(t *testing.T) {
	a := openTempAdapter(t)
	defer func() { _ = a.Close() }()
	ctx := context.Background()
	if _, err := a.Add(ctx, source.Source{URI: "u1", Title: "t", Content: []byte("c"),
		IndexedAt: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)}); err != nil {
		t.Fatalf("Add: %v", err)
	}
	got, err := a.FetchByURI(ctx, "u1")
	if err != nil {
		t.Fatalf("FetchByURI: %v", err)
	}
	if got.URI != "u1" {
		t.Fatalf("FetchByURI URI = %q, want u1", got.URI)
	}
	if _, err := a.FetchByURI(ctx, "missing"); err == nil {
		t.Fatalf("FetchByURI(missing): expected error")
	}
}

func openTempAdapter(t *testing.T) *sqliteadapter.Adapter {
	t.Helper()
	dir := t.TempDir()
	a, err := sqliteadapter.Open(filepath.Join(dir, "sources.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = a.Close() })
	return a
}

func uris(srcs []source.Source) []string {
	out := make([]string, len(srcs))
	for i, s := range srcs {
		out[i] = s.URI
	}
	return out
}

func pad(prefix string, n int) string {
	return prefix + itoa(n)
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b [20]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	return string(b[i:])
}
