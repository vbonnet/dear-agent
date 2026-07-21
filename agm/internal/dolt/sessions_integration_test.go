package dolt

import (
	"database/sql"
	"fmt"
	"testing"
	"time"

	"github.com/vbonnet/dear-agent/agm/internal/manifest"
)

func TestDoltTmuxSessionNameChangeUsesCrossDialectOwnership(t *testing.T) {
	adapter := setupIntegrationTest(t)

	sessionID := fmt.Sprintf("tmux-revision-integration-%d", time.Now().UnixNano())
	m := &manifest.Manifest{
		SchemaVersion: manifest.SchemaVersion,
		SessionID:     sessionID,
		Name:          sessionID,
		Workspace:     adapter.Workspace(),
		Harness:       "codex-cli",
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
		Context:       manifest.Context{Project: t.TempDir()},
		Tmux:          manifest.Tmux{SessionName: "legacy-name"},
	}
	if err := adapter.CreateSession(m); err != nil {
		t.Fatalf("CreateSession() error: %v", err)
	}
	t.Cleanup(func() { _ = adapter.DeleteSession(sessionID) })
	stale, err := adapter.GetSession(sessionID)
	if err != nil {
		t.Fatalf("GetSession() before provisional write: %v", err)
	}
	secondStale, err := adapter.GetSession(sessionID)
	if err != nil {
		t.Fatalf("second GetSession() before provisional write: %v", err)
	}

	change, err := adapter.BeginTmuxSessionNameChange(t.Context(), sessionID, "canonical-name")
	if err != nil || change == nil {
		t.Fatalf("BeginTmuxSessionNameChange() = (%v, %v), want non-nil change", change, err)
	}
	var provisionalRevision sql.NullString
	if err := adapter.Conn().QueryRowContext(t.Context(), `SELECT tmux_session_revision FROM agm_sessions WHERE id = ? AND workspace = ?`, sessionID, adapter.Workspace()).Scan(&provisionalRevision); err != nil {
		t.Fatalf("query provisional revision: %v", err)
	}
	if !provisionalRevision.Valid || provisionalRevision.String != change.CurrentRevision {
		t.Fatalf("provisional revision = %#v, want %q", provisionalRevision, change.CurrentRevision)
	}
	stale.Context.Notes = "stale production writer"
	if err := adapter.UpdateSession(stale); err != nil {
		t.Fatalf("UpdateSession() from pre-revision snapshot: %v", err)
	}
	preserved, err := adapter.GetSession(sessionID)
	if err != nil {
		t.Fatalf("GetSession() after stale full update: %v", err)
	}
	if preserved.Tmux.SessionName != change.CurrentName || preserved.Tmux.SessionRevision == "" || preserved.Tmux.SessionRevision == change.CurrentRevision || preserved.Context.Notes != stale.Context.Notes {
		t.Fatalf("stale full update state = (name=%q revision=%q notes=%q), want canonical name, advanced revision, and unrelated note", preserved.Tmux.SessionName, preserved.Tmux.SessionRevision, preserved.Context.Notes)
	}
	firstSupersedingRevision := preserved.Tmux.SessionRevision
	secondStale.Context.Notes = "second stale production writer"
	if err := adapter.UpdateSession(secondStale); err != nil {
		t.Fatalf("UpdateSession() from second pre-revision snapshot: %v", err)
	}
	preserved, err = adapter.GetSession(sessionID)
	if err != nil {
		t.Fatalf("GetSession() after second stale full update: %v", err)
	}
	if preserved.Tmux.SessionName != change.CurrentName || preserved.Tmux.SessionRevision == "" || preserved.Tmux.SessionRevision == firstSupersedingRevision || preserved.Context.Notes != secondStale.Context.Notes {
		t.Fatalf("second stale full update state = (name=%q revision=%q notes=%q), want canonical name, another advanced revision, and second unrelated note", preserved.Tmux.SessionName, preserved.Tmux.SessionRevision, preserved.Context.Notes)
	}
	completed, err := adapter.CompleteTmuxSessionNameChange(t.Context(), *change)
	if err != nil || completed {
		t.Fatalf("CompleteTmuxSessionNameChange() = (%v, %v), want (false, nil) after superseding writer", completed, err)
	}

	secondChange, err := adapter.BeginTmuxSessionNameChange(t.Context(), sessionID, "second-canonical-name")
	if err != nil || secondChange == nil {
		t.Fatalf("second BeginTmuxSessionNameChange() = (%v, %v), want non-nil change", secondChange, err)
	}
	latest, err := adapter.GetSession(sessionID)
	if err != nil {
		t.Fatalf("GetSession() after provisional write: %v", err)
	}
	latest.Context.Notes = "newer production writer"
	if err := adapter.UpdateSession(latest); err != nil {
		t.Fatalf("UpdateSession() after provisional write: %v", err)
	}
	restored, err := adapter.RestoreTmuxSessionNameChange(t.Context(), *secondChange)
	if err != nil || restored {
		t.Fatalf("RestoreTmuxSessionNameChange() = (%v, %v), want (false, nil)", restored, err)
	}
	stored, err := adapter.GetSession(sessionID)
	if err != nil {
		t.Fatalf("GetSession() after rejected compensation: %v", err)
	}
	if stored.Tmux.SessionName != "second-canonical-name" || stored.Context.Notes != "newer production writer" {
		t.Fatalf("newer production state was overwritten: name=%q notes=%q", stored.Tmux.SessionName, stored.Context.Notes)
	}
	previousUpdatedAt := stored.UpdatedAt
	thirdChange, err := adapter.BeginTmuxSessionNameChange(t.Context(), sessionID, "third-canonical-name")
	if err != nil || thirdChange == nil {
		t.Fatalf("third BeginTmuxSessionNameChange() = (%v, %v), want non-nil change", thirdChange, err)
	}
	restored, err = adapter.RestoreTmuxSessionNameChange(t.Context(), *thirdChange)
	if err != nil || !restored {
		t.Fatalf("third RestoreTmuxSessionNameChange() = (%v, %v), want (true, nil)", restored, err)
	}
	final, err := adapter.GetSession(sessionID)
	if err != nil {
		t.Fatalf("GetSession() after production compensation: %v", err)
	}
	if final.Tmux.SessionName != "second-canonical-name" || !final.UpdatedAt.Equal(previousUpdatedAt) {
		t.Fatalf("production compensation = (%q, %v), want (second-canonical-name, %v)", final.Tmux.SessionName, final.UpdatedAt, previousUpdatedAt)
	}
}
