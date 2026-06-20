package escalation

import (
	"context"
	"errors"
	"testing"
	"time"
)

func testStores(t *testing.T) map[string]Store {
	t.Helper()
	fs, err := NewFileStore(t.TempDir())
	if err != nil {
		t.Fatalf("NewFileStore: %v", err)
	}
	return map[string]Store{"mem": NewMemStore(), "file": fs}
}

func TestStore_PutGetListRoundTrip(t *testing.T) {
	ctx := context.Background()
	for name, s := range testStores(t) {
		t.Run(name, func(t *testing.T) {
			pending := &Escalation{
				ID: "esc-1", Kind: KindQuestion, Phase: PhaseRouted,
				OriginSessionID: "w1", CurrentSessionID: "sup",
				Chain: []string{"w1", "sup"}, CreatedAt: time.Unix(1, 0),
			}
			resolved := &Escalation{
				ID: "esc-2", Kind: KindDecision, Phase: PhaseAnswered,
				OriginSessionID: "w2", CurrentSessionID: "sup", Answer: "ok",
				Chain: []string{"w2", "sup"}, CreatedAt: time.Unix(2, 0),
			}
			if err := s.Put(ctx, pending); err != nil {
				t.Fatalf("Put pending: %v", err)
			}
			if err := s.Put(ctx, resolved); err != nil {
				t.Fatalf("Put resolved: %v", err)
			}

			got, err := s.Get(ctx, "esc-1")
			if err != nil {
				t.Fatalf("Get: %v", err)
			}
			if got.Question != pending.Question || got.Phase != PhaseRouted {
				t.Errorf("round-trip mismatch: %+v", got)
			}

			if _, err := s.Get(ctx, "missing"); !errors.Is(err, ErrNotFound) {
				t.Errorf("want ErrNotFound, got %v", err)
			}

			all, err := s.List(ctx, Filter{})
			if err != nil || len(all) != 2 {
				t.Fatalf("List all: n=%d err=%v", len(all), err)
			}
			// Sorted by CreatedAt.
			if all[0].ID != "esc-1" || all[1].ID != "esc-2" {
				t.Errorf("List order wrong: %s, %s", all[0].ID, all[1].ID)
			}

			pendingOnly, err := s.List(ctx, Filter{Pending: true})
			if err != nil || len(pendingOnly) != 1 || pendingOnly[0].ID != "esc-1" {
				t.Errorf("pending filter wrong: %v / %v", pendingOnly, err)
			}

			byHolder, err := s.List(ctx, Filter{CurrentSessionID: "sup", Pending: true})
			if err != nil || len(byHolder) != 1 {
				t.Errorf("holder filter wrong: n=%d err=%v", len(byHolder), err)
			}
		})
	}
}

func TestFileStore_RejectsBadID(t *testing.T) {
	fs, _ := NewFileStore(t.TempDir())
	if err := fs.Put(context.Background(), &Escalation{ID: "../escape"}); err == nil {
		t.Errorf("expected rejection of path-traversal id")
	}
}

func TestFileStore_Persists(t *testing.T) {
	dir := t.TempDir()
	ctx := context.Background()
	fs1, _ := NewFileStore(dir)
	if err := fs1.Put(ctx, &Escalation{ID: "p1", Phase: PhaseRouted, CreatedAt: time.Unix(1, 0)}); err != nil {
		t.Fatal(err)
	}
	// A fresh FileStore over the same dir sees it (cross-process shape).
	fs2, _ := NewFileStore(dir)
	if _, err := fs2.Get(ctx, "p1"); err != nil {
		t.Errorf("expected persisted escalation, got %v", err)
	}
}
