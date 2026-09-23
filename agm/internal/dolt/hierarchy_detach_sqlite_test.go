package dolt

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/vbonnet/dear-agent/agm/internal/manifest"
)

// Detaching a child must advance updated_at, exactly as linking one does.
//
// LinkSessionParent writes `SET updated_at = ?, parent_session_id = ?`, while
// DetachChild wrote only `SET parent_session_id = NULL`. Every revalidation
// that asks "did this session change since I snapshotted it?" compares
// updated_at, and GetSession deliberately omits parent_session_id pending a
// migration, so both sides of the comparison carried a nil parent and matched.
// A child detached while a confirmation prompt was open therefore looked
// unchanged, and cleanup went on to archive or delete it.
func TestDetachChildAdvancesUpdatedAt(t *testing.T) {
	adapter, err := NewSQLiteAdapter(filepath.Join(t.TempDir(), "agm.db"))
	if err != nil {
		t.Fatalf("NewSQLiteAdapter() error: %v", err)
	}
	t.Cleanup(func() { _ = adapter.Close() })

	newSession := func(id string) *manifest.Manifest {
		return &manifest.Manifest{
			SchemaVersion: manifest.SchemaVersion,
			SessionID:     id,
			Name:          id,
			Harness:       "claude-code",
			CreatedAt:     time.Now().Add(-time.Hour),
			UpdatedAt:     time.Now().Add(-time.Hour),
			Context:       manifest.Context{Project: t.TempDir()},
			Tmux:          manifest.Tmux{SessionName: id},
		}
	}
	parent := newSession("detach-parent")
	child := newSession("detach-child")
	for _, m := range []*manifest.Manifest{parent, child} {
		if err := adapter.CreateSession(m); err != nil {
			t.Fatalf("CreateSession(%s) error: %v", m.SessionID, err)
		}
	}

	if err := adapter.LinkSessionParent(context.Background(),
		child.SessionID, "", parent.SessionID, nil); err != nil {
		t.Fatalf("LinkSessionParent() error: %v", err)
	}
	linked, err := adapter.GetSession(child.SessionID)
	if err != nil {
		t.Fatalf("GetSession() after link error: %v", err)
	}

	if err := adapter.DetachChild(child.SessionID); err != nil {
		t.Fatalf("DetachChild() error: %v", err)
	}
	detached, err := adapter.GetSession(child.SessionID)
	if err != nil {
		t.Fatalf("GetSession() after detach error: %v", err)
	}

	if !detached.UpdatedAt.After(linked.UpdatedAt) {
		t.Errorf("updated_at after detach = %v, want it advanced past %v: "+
			"a snapshot taken before the detach compares equal and cleanup treats "+
			"the session as unchanged", detached.UpdatedAt, linked.UpdatedAt)
	}

	gotParent, err := adapter.GetParent(child.SessionID)
	if err != nil {
		t.Fatalf("GetParent() error: %v", err)
	}
	if gotParent != nil {
		t.Errorf("GetParent() = %v, want nil after detach", gotParent.SessionID)
	}
}
