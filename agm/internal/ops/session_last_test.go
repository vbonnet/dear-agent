package ops

import (
	"errors"
	"testing"
	"time"

	"github.com/vbonnet/dear-agent/agm/internal/manifest"
)

func TestLastSession_ReturnsMostRecent(t *testing.T) {
	older := newManifest("id-old", "old-session", "~/project-a")
	older.UpdatedAt = time.Date(2026, 3, 20, 10, 0, 0, 0, time.UTC)

	newer := newManifest("id-new", "new-session", "~/project-b")
	newer.UpdatedAt = time.Date(2026, 3, 25, 10, 0, 0, 0, time.UTC)

	ctx := testCtx([]*manifest.Manifest{older, newer}, "new-session")

	result, err := LastSession(ctx, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Operation != "last_session" {
		t.Errorf("expected operation last_session, got %s", result.Operation)
	}
	if result.Session.Name != "new-session" {
		t.Errorf("expected new-session, got %s", result.Session.Name)
	}
	if result.Session.ID != "id-new" {
		t.Errorf("expected id-new, got %s", result.Session.ID)
	}
}

func TestLastSession_NoSessions(t *testing.T) {
	ctx := testCtx(nil)

	_, err := LastSession(ctx, nil)
	if err == nil {
		t.Fatal("expected error when no sessions exist")
	}
	var opErr *OpError
	if !errors.As(err, &opErr) {
		t.Fatalf("expected *OpError, got %T", err)
	}
	if opErr.Code != ErrCodeSessionNotFound {
		t.Errorf("expected code %s, got %s", ErrCodeSessionNotFound, opErr.Code)
	}
}

func TestLastSession_SingleSession(t *testing.T) {
	session := newManifest("id-only", "only-session", "~/project")
	session.UpdatedAt = time.Date(2026, 3, 22, 14, 0, 0, 0, time.UTC)

	ctx := testCtx([]*manifest.Manifest{session}, "only-session")

	result, err := LastSession(ctx, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Session.Name != "only-session" {
		t.Errorf("expected only-session, got %s", result.Session.Name)
	}
	if result.Session.Status != "active" {
		t.Errorf("expected active status, got %s", result.Session.Status)
	}
}

func TestLastSession_ExcludesArchived(t *testing.T) {
	// The most recent session is archived — should be excluded.
	// The ops layer passes ExcludeArchived: true to storage.
	active := newManifest("id-active", "active-session", "~/project-a")
	active.UpdatedAt = time.Date(2026, 3, 20, 10, 0, 0, 0, time.UTC)

	archived := newManifest("id-archived", "archived-session", "~/project-b")
	archived.UpdatedAt = time.Date(2026, 3, 25, 10, 0, 0, 0, time.UTC)
	archived.Lifecycle = "archived"

	ctx := testCtx([]*manifest.Manifest{active, archived}, "active-session")

	result, err := LastSession(ctx, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Session.Name != "active-session" {
		t.Errorf("expected active-session (archived should be excluded), got %s", result.Session.Name)
	}
}

func TestLastSession_StatusReflectsTmux(t *testing.T) {
	session := newManifest("id-1", "my-session", "~/project")
	session.UpdatedAt = time.Date(2026, 3, 22, 14, 0, 0, 0, time.UTC)

	// No tmux session running
	ctx := testCtx([]*manifest.Manifest{session})

	result, err := LastSession(ctx, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Session.Status != "stopped" {
		t.Errorf("expected stopped status (no tmux), got %s", result.Session.Status)
	}
}
