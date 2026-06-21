package document

import (
	"context"
	"errors"
	"testing"
)

func newTestStore(t *testing.T) *FSStore {
	t.Helper()
	s, err := NewFSStore(t.TempDir())
	if err != nil {
		t.Fatalf("NewFSStore: %v", err)
	}
	return s
}

func TestPutAssignsVersionAndHash(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	ns := []string{"project", "engram"}

	got, err := s.Put(ctx, ns, Document{ID: "spec", Kind: KindSpec, Title: "Spec", Content: "hello"})
	if err != nil {
		t.Fatalf("Put: %v", err)
	}
	if got.Version != 1 {
		t.Errorf("first version = %d, want 1", got.Version)
	}
	if got.ContentHash != HashContent("hello") {
		t.Errorf("ContentHash not computed by store")
	}
	if got.SchemaVersion != SchemaVersion {
		t.Errorf("SchemaVersion = %q, want %q", got.SchemaVersion, SchemaVersion)
	}
	if got.CreatedAt.IsZero() {
		t.Errorf("CreatedAt not stamped")
	}
}

func TestPutIsImmutableAndVersioned(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	ns := []string{"project", "engram"}

	if _, err := s.Put(ctx, ns, Document{ID: "spec", Content: "v1 body"}); err != nil {
		t.Fatalf("Put v1: %v", err)
	}
	v2, err := s.Put(ctx, ns, Document{ID: "spec", Content: "v2 body"})
	if err != nil {
		t.Fatalf("Put v2: %v", err)
	}
	if v2.Version != 2 {
		t.Fatalf("second version = %d, want 2", v2.Version)
	}

	// Old version is retained, unchanged.
	v1, err := s.GetVersion(ctx, ns, "spec", 1)
	if err != nil {
		t.Fatalf("GetVersion(1): %v", err)
	}
	if v1.Content != "v1 body" {
		t.Errorf("v1 content mutated: %q", v1.Content)
	}

	// Get returns the latest.
	latest, err := s.Get(ctx, ns, "spec")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if latest.Version != 2 || latest.Content != "v2 body" {
		t.Errorf("Get returned v%d %q, want v2 \"v2 body\"", latest.Version, latest.Content)
	}
}

func TestPutIgnoresCallerVersionAndHash(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	ns := []string{"ns"}

	got, err := s.Put(ctx, ns, Document{ID: "d", Version: 99, ContentHash: "bogus", Content: "x"})
	if err != nil {
		t.Fatalf("Put: %v", err)
	}
	if got.Version != 1 {
		t.Errorf("Version = %d, want 1 (caller value ignored)", got.Version)
	}
	if got.ContentHash == "bogus" {
		t.Errorf("ContentHash = caller value, want recomputed")
	}
}

func TestListVersions(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	ns := []string{"ns"}

	for _, c := range []string{"a", "b", "c"} {
		if _, err := s.Put(ctx, ns, Document{ID: "d", Content: c}); err != nil {
			t.Fatalf("Put %q: %v", c, err)
		}
	}
	versions, err := s.ListVersions(ctx, ns, "d")
	if err != nil {
		t.Fatalf("ListVersions: %v", err)
	}
	if len(versions) != 3 {
		t.Fatalf("got %d versions, want 3", len(versions))
	}
	for i, v := range versions {
		if v.Version != i+1 {
			t.Errorf("versions[%d].Version = %d, want %d (oldest first)", i, v.Version, i+1)
		}
	}
}

func TestListVersionsMissingReturnsEmpty(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	versions, err := s.ListVersions(ctx, []string{"ns"}, "nope")
	if err != nil {
		t.Fatalf("ListVersions: %v", err)
	}
	if len(versions) != 0 {
		t.Errorf("got %d versions, want 0", len(versions))
	}
}

func TestListWithFilter(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	ns := []string{"ns"}

	mustPut(t, s, ns, Document{ID: "spec1", Kind: KindSpec, Title: "Auth Spec"})
	mustPut(t, s, ns, Document{ID: "arch1", Kind: KindArchitecture, Title: "Auth Arch"})
	mustPut(t, s, ns, Document{ID: "spec2", Kind: KindSpec, Title: "Billing Spec"})

	all, err := s.List(ctx, ns, Filter{})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(all) != 3 {
		t.Fatalf("List all = %d, want 3", len(all))
	}
	// Sorted by ID.
	if all[0].ID != "arch1" || all[1].ID != "spec1" || all[2].ID != "spec2" {
		t.Errorf("List not sorted by ID: %v", []string{all[0].ID, all[1].ID, all[2].ID})
	}

	specs, err := s.List(ctx, ns, Filter{Kind: KindSpec})
	if err != nil {
		t.Fatalf("List kind: %v", err)
	}
	if len(specs) != 2 {
		t.Errorf("List by kind = %d, want 2", len(specs))
	}

	billing, err := s.List(ctx, ns, Filter{TitleContains: "billing"})
	if err != nil {
		t.Fatalf("List title: %v", err)
	}
	if len(billing) != 1 || billing[0].ID != "spec2" {
		t.Errorf("List by title = %v, want [spec2]", billing)
	}
}

func TestListReturnsLatestVersionOnly(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	ns := []string{"ns"}

	mustPut(t, s, ns, Document{ID: "d", Content: "v1"})
	mustPut(t, s, ns, Document{ID: "d", Content: "v2"})

	docs, err := s.List(ctx, ns, Filter{})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(docs) != 1 {
		t.Fatalf("List = %d, want 1 (latest only)", len(docs))
	}
	if docs[0].Version != 2 {
		t.Errorf("List returned v%d, want latest v2", docs[0].Version)
	}
}

func TestDelete(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	ns := []string{"ns"}

	mustPut(t, s, ns, Document{ID: "d", Content: "v1"})
	mustPut(t, s, ns, Document{ID: "d", Content: "v2"})

	if err := s.Delete(ctx, ns, "d"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := s.Get(ctx, ns, "d"); !errors.Is(err, ErrNotFound) {
		t.Errorf("Get after Delete: err = %v, want ErrNotFound", err)
	}
	if err := s.Delete(ctx, ns, "d"); !errors.Is(err, ErrNotFound) {
		t.Errorf("Delete missing: err = %v, want ErrNotFound", err)
	}
}

func TestGetMissing(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	if _, err := s.Get(ctx, []string{"ns"}, "missing"); !errors.Is(err, ErrNotFound) {
		t.Errorf("Get missing: err = %v, want ErrNotFound", err)
	}
	if _, err := s.GetVersion(ctx, []string{"ns"}, "missing", 1); !errors.Is(err, ErrNotFound) {
		t.Errorf("GetVersion missing: err = %v, want ErrNotFound", err)
	}
}

func TestValidationRejectsTraversal(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)

	cases := []struct {
		name string
		ns   []string
		id   string
		want error
	}{
		{"empty ns", nil, "d", ErrInvalidNamespace},
		{"traversal ns", []string{".."}, "d", ErrInvalidNamespace},
		{"slash ns", []string{"a/b"}, "d", ErrInvalidNamespace},
		{"empty id", []string{"ns"}, "", ErrInvalidID},
		{"traversal id", []string{"ns"}, "../escape", ErrInvalidID},
		{"slash id", []string{"ns"}, "a/b", ErrInvalidID},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := s.Put(ctx, tc.ns, Document{ID: tc.id, Content: "x"})
			if !errors.Is(err, tc.want) {
				t.Errorf("Put err = %v, want %v", err, tc.want)
			}
		})
	}
}

func TestPutRejectsUnknownKind(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	_, err := s.Put(ctx, []string{"ns"}, Document{ID: "d", Kind: Kind("bogus"), Content: "x"})
	if !errors.Is(err, ErrInvalidDocument) {
		t.Errorf("Put unknown kind: err = %v, want ErrInvalidDocument", err)
	}
}

func TestContextCancellation(t *testing.T) {
	s := newTestStore(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := s.Put(ctx, []string{"ns"}, Document{ID: "d", Content: "x"}); !errors.Is(err, context.Canceled) {
		t.Errorf("Put with cancelled ctx: err = %v, want context.Canceled", err)
	}
}

func mustPut(t *testing.T, s *FSStore, ns []string, doc Document) Document {
	t.Helper()
	got, err := s.Put(context.Background(), ns, doc)
	if err != nil {
		t.Fatalf("Put %q: %v", doc.ID, err)
	}
	return got
}
