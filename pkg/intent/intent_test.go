package intent

import (
	"errors"
	"sort"
	"testing"
	"time"
)

func newTestBoard(t *testing.T) *FileBoard {
	t.Helper()
	b, err := NewFileBoard(t.TempDir())
	if err != nil {
		t.Fatalf("NewFileBoard: %v", err)
	}
	return b
}

func TestDeclareRequiresSessionID(t *testing.T) {
	b := newTestBoard(t)
	if _, err := b.Declare(DeclareOpts{Files: []string{"a.go"}}); err == nil {
		t.Error("missing session_id should fail")
	}
}

func TestDeclareRequiresFilesOrPackages(t *testing.T) {
	b := newTestBoard(t)
	if _, err := b.Declare(DeclareOpts{SessionID: "s1"}); err == nil {
		t.Error("empty scope should fail")
	}
}

func TestDeclarePersistsAndReturnsIntent(t *testing.T) {
	b := newTestBoard(t)
	intent, err := b.Declare(DeclareOpts{
		SessionID:   "worker-1",
		Files:       []string{"a.go", "b.go"},
		Description: "refactor",
	})
	if err != nil {
		t.Fatalf("Declare: %v", err)
	}
	if intent.ID == "" {
		t.Error("ID should be generated")
	}
	if intent.DeclaredAt.IsZero() {
		t.Error("DeclaredAt should be set")
	}
	if !intent.ExpiresAt.After(intent.DeclaredAt) {
		t.Error("ExpiresAt should be after DeclaredAt")
	}

	got, err := b.Get(intent.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.SessionID != "worker-1" {
		t.Errorf("session = %q, want worker-1", got.SessionID)
	}
	if len(got.Files) != 2 {
		t.Errorf("files = %v", got.Files)
	}
}

func TestDeclareDeduplicatesFiles(t *testing.T) {
	b := newTestBoard(t)
	intent, err := b.Declare(DeclareOpts{
		SessionID: "s1",
		Files:     []string{"a.go", "a.go", " ", "b.go"},
	})
	if err != nil {
		t.Fatalf("Declare: %v", err)
	}
	if len(intent.Files) != 2 {
		t.Errorf("files = %v, want 2 unique entries", intent.Files)
	}
	if !sort.StringsAreSorted(intent.Files) {
		t.Errorf("files should be sorted: %v", intent.Files)
	}
}

func TestDeclareUsesDefaultTTLWhenZero(t *testing.T) {
	b := newTestBoard(t)
	intent, err := b.Declare(DeclareOpts{SessionID: "s", Files: []string{"x.go"}})
	if err != nil {
		t.Fatalf("Declare: %v", err)
	}
	got := intent.ExpiresAt.Sub(intent.DeclaredAt)
	if got != DefaultTTL {
		t.Errorf("ttl = %v, want %v", got, DefaultTTL)
	}
}

func TestDeclareHonoursExplicitTTL(t *testing.T) {
	b := newTestBoard(t)
	intent, err := b.Declare(DeclareOpts{
		SessionID: "s",
		Files:     []string{"x.go"},
		TTL:       5 * time.Minute,
	})
	if err != nil {
		t.Fatalf("Declare: %v", err)
	}
	got := intent.ExpiresAt.Sub(intent.DeclaredAt)
	if got != 5*time.Minute {
		t.Errorf("ttl = %v, want 5m", got)
	}
}

func TestGetUnknownIDReturnsErrNotFound(t *testing.T) {
	b := newTestBoard(t)
	_, err := b.Get("nonexistent")
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("err = %v, want ErrNotFound", err)
	}
}

func TestRemoveUnknownIDReturnsErrNotFound(t *testing.T) {
	b := newTestBoard(t)
	if err := b.Remove("nonexistent"); !errors.Is(err, ErrNotFound) {
		t.Errorf("err = %v, want ErrNotFound", err)
	}
}

func TestListFiltersBySession(t *testing.T) {
	b := newTestBoard(t)
	if _, err := b.Declare(DeclareOpts{SessionID: "alpha", Files: []string{"a.go"}}); err != nil {
		t.Fatal(err)
	}
	if _, err := b.Declare(DeclareOpts{SessionID: "beta", Files: []string{"b.go"}}); err != nil {
		t.Fatal(err)
	}
	got, err := b.List(ListFilter{SessionID: "alpha"})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(got) != 1 || got[0].SessionID != "alpha" {
		t.Errorf("filtered list = %+v", got)
	}
}

func TestListExcludesExpiredByDefault(t *testing.T) {
	dir := t.TempDir()
	b, err := NewFileBoard(dir)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	b.Now = func() time.Time { return now }

	// Declare A while clock = now, then advance and declare B.
	a, err := b.Declare(DeclareOpts{SessionID: "s", Files: []string{"a.go"}, TTL: time.Minute})
	if err != nil {
		t.Fatal(err)
	}
	b.Now = func() time.Time { return now.Add(2 * time.Minute) }
	bIntent, err := b.Declare(DeclareOpts{SessionID: "s", Files: []string{"b.go"}, TTL: time.Minute})
	if err != nil {
		t.Fatal(err)
	}

	got, err := b.List(ListFilter{})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(got) != 1 || got[0].ID != bIntent.ID {
		t.Errorf("list = %+v, want only B (A=%s expired)", got, a.ID)
	}

	all, err := b.List(ListFilter{IncludeExpired: true})
	if err != nil {
		t.Fatalf("List include-expired: %v", err)
	}
	if len(all) != 2 {
		t.Errorf("include-expired list = %d, want 2", len(all))
	}
}

func TestExpireRemovesExpiredRows(t *testing.T) {
	dir := t.TempDir()
	b, err := NewFileBoard(dir)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	b.Now = func() time.Time { return now }
	for _, f := range []string{"a.go", "b.go", "c.go"} {
		if _, err := b.Declare(DeclareOpts{SessionID: "s", Files: []string{f}, TTL: time.Minute}); err != nil {
			t.Fatal(err)
		}
	}
	b.Now = func() time.Time { return now.Add(2 * time.Minute) }

	n, err := b.Expire()
	if err != nil {
		t.Fatalf("Expire: %v", err)
	}
	if n != 3 {
		t.Errorf("removed = %d, want 3", n)
	}
	left, err := b.List(ListFilter{IncludeExpired: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(left) != 0 {
		t.Errorf("after Expire: %d rows remain", len(left))
	}
}

func TestOverlapsDetectsFileSharing(t *testing.T) {
	b := newTestBoard(t)
	a, err := b.Declare(DeclareOpts{SessionID: "alpha", Files: []string{"shared.go", "a.go"}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := b.Declare(DeclareOpts{SessionID: "beta", Files: []string{"shared.go", "b.go"}}); err != nil {
		t.Fatal(err)
	}
	if _, err := b.Declare(DeclareOpts{SessionID: "gamma", Files: []string{"unrelated.go"}}); err != nil {
		t.Fatal(err)
	}

	overlaps, err := b.Overlaps(a)
	if err != nil {
		t.Fatalf("Overlaps: %v", err)
	}
	if len(overlaps) != 1 || overlaps[0].SessionID != "beta" {
		t.Errorf("overlaps = %+v", overlaps)
	}
}

func TestOverlapsIgnoresSameSession(t *testing.T) {
	b := newTestBoard(t)
	first, err := b.Declare(DeclareOpts{SessionID: "alpha", Files: []string{"x.go"}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := b.Declare(DeclareOpts{SessionID: "alpha", Files: []string{"x.go"}}); err != nil {
		t.Fatal(err)
	}
	overlaps, err := b.Overlaps(first)
	if err != nil {
		t.Fatalf("Overlaps: %v", err)
	}
	if len(overlaps) != 0 {
		t.Errorf("same-session overlaps should be 0, got %+v", overlaps)
	}
}

func TestOverlapsByPackage(t *testing.T) {
	b := newTestBoard(t)
	a, err := b.Declare(DeclareOpts{SessionID: "alpha", Packages: []string{"pkg/api", "pkg/db"}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := b.Declare(DeclareOpts{SessionID: "beta", Packages: []string{"pkg/db"}}); err != nil {
		t.Fatal(err)
	}
	overlaps, err := b.Overlaps(a)
	if err != nil {
		t.Fatalf("Overlaps: %v", err)
	}
	if len(overlaps) != 1 {
		t.Errorf("expected 1 package overlap, got %+v", overlaps)
	}
}

func TestIntentActive(t *testing.T) {
	now := time.Now()
	i := Intent{ExpiresAt: now.Add(time.Minute)}
	if !i.Active(now) {
		t.Error("future expiry should be active")
	}
	if i.Active(now.Add(2 * time.Minute)) {
		t.Error("past expiry should not be active")
	}
}

func TestNewFileBoardCreatesDir(t *testing.T) {
	dir := t.TempDir() + "/nested/path"
	b, err := NewFileBoard(dir)
	if err != nil {
		t.Fatalf("NewFileBoard: %v", err)
	}
	if _, err := b.Declare(DeclareOpts{SessionID: "s", Files: []string{"x.go"}}); err != nil {
		t.Errorf("Declare into newly-created dir: %v", err)
	}
}
