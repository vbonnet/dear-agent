package aggregator

import (
	"context"
	"path/filepath"
	"testing"
	"time"
)

func newTestStore(t *testing.T) *SQLiteStore {
	t.Helper()
	dir := t.TempDir()
	store, err := OpenSQLiteStore(filepath.Join(dir, "signals.db"))
	if err != nil {
		t.Fatalf("OpenSQLiteStore: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	return store
}

func TestSQLiteStoreInsertAndRecent(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)
	ctx := context.Background()
	at := time.Date(2026, 5, 3, 10, 0, 0, 0, time.UTC)

	sigs := []Signal{
		{ID: "a", Kind: KindLintTrend, Subject: "pkg/a.go", Value: 3, CollectedAt: at},
		{ID: "b", Kind: KindLintTrend, Subject: "pkg/b.go", Value: 5, CollectedAt: at.Add(time.Minute)},
		{ID: "c", Kind: KindGitActivity, Subject: "/repo", Value: 12, CollectedAt: at},
	}
	if err := store.Insert(ctx, sigs); err != nil {
		t.Fatalf("Insert: %v", err)
	}

	got, err := store.Recent(ctx, KindLintTrend, 10)
	if err != nil {
		t.Fatalf("Recent: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("Recent returned %d signals, want 2", len(got))
	}
	if got[0].Subject != "pkg/b.go" {
		t.Errorf("Recent[0].Subject = %q, want pkg/b.go (most recent first)", got[0].Subject)
	}

	gitGot, err := store.Recent(ctx, KindGitActivity, 0)
	if err != nil {
		t.Fatalf("Recent(git): %v", err)
	}
	if len(gitGot) != 1 || gitGot[0].Value != 12 {
		t.Errorf("Recent(git) = %+v, want one row with value 12", gitGot)
	}
}

func TestSQLiteStoreInsertEmpty(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)
	if err := store.Insert(context.Background(), nil); err != nil {
		t.Fatalf("Insert(nil) should be no-op, got %v", err)
	}
}

func TestSQLiteStoreInsertRejectsInvalid(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)
	bad := []Signal{
		{ID: "", Kind: KindLintTrend, Subject: "x", Value: 1,
			CollectedAt: time.Now()},
	}
	if err := store.Insert(context.Background(), bad); err == nil {
		t.Error("Insert with empty ID should fail")
	}
}

func TestSQLiteStoreInsertRejectsBadJSON(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)
	bad := []Signal{
		{
			ID: "id1", Kind: KindLintTrend, Subject: "x", Value: 1,
			Metadata:    "not json",
			CollectedAt: time.Now(),
		},
	}
	if err := store.Insert(context.Background(), bad); err == nil {
		t.Error("Insert with non-JSON metadata should fail")
	}
}

func TestSQLiteStoreRange(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)
	ctx := context.Background()
	base := time.Date(2026, 5, 3, 10, 0, 0, 0, time.UTC)

	for i := 0; i < 5; i++ {
		s := Signal{
			ID:          string(rune('a' + i)),
			Kind:        KindLintTrend,
			Subject:     "pkg/x.go",
			Value:       float64(i),
			CollectedAt: base.Add(time.Duration(i) * time.Hour),
		}
		if err := store.Insert(ctx, []Signal{s}); err != nil {
			t.Fatalf("Insert[%d]: %v", i, err)
		}
	}
	got, err := store.Range(ctx, KindLintTrend, base.Add(2*time.Hour))
	if err != nil {
		t.Fatalf("Range: %v", err)
	}
	if len(got) != 3 {
		t.Errorf("Range returned %d signals, want 3", len(got))
	}
}

func TestSQLiteStoreKinds(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)
	ctx := context.Background()
	at := time.Date(2026, 5, 3, 10, 0, 0, 0, time.UTC)

	if kinds, err := store.Kinds(ctx); err != nil {
		t.Fatalf("Kinds(empty): %v", err)
	} else if len(kinds) != 0 {
		t.Errorf("Kinds(empty) = %v, want []", kinds)
	}

	sigs := []Signal{
		{ID: "1", Kind: KindLintTrend, Subject: "a", Value: 1, CollectedAt: at},
		{ID: "2", Kind: KindGitActivity, Subject: "/repo", Value: 1, CollectedAt: at},
	}
	if err := store.Insert(ctx, sigs); err != nil {
		t.Fatalf("Insert: %v", err)
	}
	kinds, err := store.Kinds(ctx)
	if err != nil {
		t.Fatalf("Kinds: %v", err)
	}
	if len(kinds) != 2 {
		t.Errorf("Kinds = %v, want 2 distinct", kinds)
	}
}

func TestSQLiteStoreCloseIsIdempotent(t *testing.T) {
	t.Parallel()
	store, err := OpenSQLiteStore(filepath.Join(t.TempDir(), "s.db"))
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("first Close: %v", err)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("second Close: %v", err)
	}
}

func TestOpenSQLiteStoreEmptyPath(t *testing.T) {
	t.Parallel()
	if _, err := OpenSQLiteStore(""); err == nil {
		t.Error("OpenSQLiteStore(\"\") should fail")
	}
}
