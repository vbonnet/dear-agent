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
	// updated_at is a bare TIMESTAMP and carries second precision on Dolt, so
	// a detach in the same second as the preceding update writes an identical
	// value. The rotated revision is the guarantee that does not collide.
	if detached.Tmux.SessionRevision == linked.Tmux.SessionRevision {
		t.Errorf("tmux_session_revision unchanged at %q: updated_at alone collides "+
			"within a second, so the comparison can still accept a stale snapshot",
			detached.Tmux.SessionRevision)
	}
	if detached.Tmux.SessionRevision == "" {
		t.Error("detach left an empty revision, which cannot distinguish anything")
	}

	gotParent, err := adapter.GetParent(child.SessionID)
	if err != nil {
		t.Fatalf("GetParent() error: %v", err)
	}
	if gotParent != nil {
		t.Errorf("GetParent() = %v, want nil after detach", gotParent.SessionID)
	}
}

// Deleting a parent must version its children too.
//
// Migration 007 declares ON DELETE SET NULL, so the cascade detaches children
// without going through DetachChild and therefore without advancing their
// updated_at. Combined with GetSession omitting parent_session_id, a caller
// comparing two manifest snapshots across a confirmation prompt would see no
// change at all and could archive or remove a child whose hierarchy moved.
func TestDeleteSessionVersionsDetachedChildren(t *testing.T) {
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
	parent := newSession("cascade-parent")
	child := newSession("cascade-child")
	bystander := newSession("cascade-bystander")
	for _, m := range []*manifest.Manifest{parent, child, bystander} {
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
		t.Fatalf("GetSession() before delete error: %v", err)
	}
	untouched, err := adapter.GetSession(bystander.SessionID)
	if err != nil {
		t.Fatalf("GetSession(bystander) error: %v", err)
	}

	if err := adapter.DeleteSession(parent.SessionID); err != nil {
		t.Fatalf("DeleteSession() error: %v", err)
	}

	after, err := adapter.GetSession(child.SessionID)
	if err != nil {
		t.Fatalf("GetSession() after delete error: %v", err)
	}
	if !after.UpdatedAt.After(linked.UpdatedAt) {
		t.Errorf("child updated_at = %v, want it advanced past %v: the cascade detach "+
			"must be visible to a caller revalidating a snapshot", after.UpdatedAt, linked.UpdatedAt)
	}
	if after.Tmux.SessionRevision == linked.Tmux.SessionRevision {
		t.Errorf("cascade left tmux_session_revision at %q; second-precision updated_at "+
			"is not enough on its own", after.Tmux.SessionRevision)
	}

	// Only the parent's own children are versioned; an unrelated session is
	// left alone, so this does not invalidate every snapshot in the workspace.
	stillUntouched, err := adapter.GetSession(bystander.SessionID)
	if err != nil {
		t.Fatalf("GetSession(bystander) after delete error: %v", err)
	}
	if stillUntouched.UpdatedAt.After(untouched.UpdatedAt) {
		t.Error("an unrelated session was versioned by the delete")
	}
}
