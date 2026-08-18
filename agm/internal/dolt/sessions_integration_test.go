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
	stale.Name = "stale-production-display-name"
	stale.Context.Notes = "stale production writer"
	if err := adapter.UpdateSession(stale); err != nil {
		t.Fatalf("UpdateSession() from pre-revision snapshot: %v", err)
	}
	preserved, err := adapter.GetSession(sessionID)
	if err != nil {
		t.Fatalf("GetSession() after stale full update: %v", err)
	}
	if preserved.Name != m.Name || preserved.Tmux.SessionName != change.CurrentName || preserved.Tmux.SessionRevision == "" || preserved.Tmux.SessionRevision == change.CurrentRevision || preserved.Context.Notes != stale.Context.Notes {
		t.Fatalf("stale full update state = (display=%q tmux=%q revision=%q notes=%q), want original display name, canonical tmux name, advanced revision, and unrelated note", preserved.Name, preserved.Tmux.SessionName, preserved.Tmux.SessionRevision, preserved.Context.Notes)
	}
	firstSupersedingRevision := preserved.Tmux.SessionRevision
	secondStale.Name = "second-stale-production-display-name"
	secondStale.Context.Notes = "second stale production writer"
	if err := adapter.UpdateSession(secondStale); err != nil {
		t.Fatalf("UpdateSession() from second pre-revision snapshot: %v", err)
	}
	preserved, err = adapter.GetSession(sessionID)
	if err != nil {
		t.Fatalf("GetSession() after second stale full update: %v", err)
	}
	if preserved.Name != m.Name || preserved.Tmux.SessionName != change.CurrentName || preserved.Tmux.SessionRevision == "" || preserved.Tmux.SessionRevision == firstSupersedingRevision || preserved.Context.Notes != secondStale.Context.Notes {
		t.Fatalf("second stale full update state = (display=%q tmux=%q revision=%q notes=%q), want original display name, canonical tmux name, another advanced revision, and second unrelated note", preserved.Name, preserved.Tmux.SessionName, preserved.Tmux.SessionRevision, preserved.Context.Notes)
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
	renameStale, err := adapter.GetSession(sessionID)
	if err != nil {
		t.Fatalf("GetSession() before authoritative rename conflict: %v", err)
	}
	renameConcurrent, err := adapter.GetSession(sessionID)
	if err != nil {
		t.Fatalf("second GetSession() before authoritative rename conflict: %v", err)
	}
	renameConcurrent.Context.Notes = "concurrent update before authoritative rename"
	if err := adapter.UpdateSession(renameConcurrent); err != nil {
		t.Fatalf("UpdateSession() before authoritative rename: %v", err)
	}
	if _, err := adapter.RenameSessionIdentity(t.Context(), sessionID, renameStale.Name, renameStale.Tmux.SessionName, renameStale.Tmux.SessionRevision, sessionID+"-stale-rename"); err == nil {
		t.Fatal("RenameSessionIdentity() from stale revision succeeded")
	}
	renameCurrent, err := adapter.GetSession(sessionID)
	if err != nil {
		t.Fatalf("GetSession() after authoritative rename conflict: %v", err)
	}
	authoritativeName := sessionID + "-renamed"
	if _, err := adapter.RenameSessionIdentity(t.Context(), sessionID, renameCurrent.Name, renameCurrent.Tmux.SessionName, renameCurrent.Tmux.SessionRevision, authoritativeName); err != nil {
		t.Fatalf("RenameSessionIdentity() current revision: %v", err)
	}
	renamed, err := adapter.GetSession(sessionID)
	if err != nil {
		t.Fatalf("GetSession() after authoritative rename: %v", err)
	}
	if renamed.Name != authoritativeName || renamed.Tmux.SessionName != authoritativeName || renamed.Context.Notes != renameConcurrent.Context.Notes || renamed.Tmux.SessionRevision == renameCurrent.Tmux.SessionRevision {
		t.Fatalf("authoritative rename state = (name=%q tmux=%q notes=%q revision=%q), want atomic names, preserved note, and advanced revision", renamed.Name, renamed.Tmux.SessionName, renamed.Context.Notes, renamed.Tmux.SessionRevision)
	}

	parentID := sessionID + "-parent"
	parent := &manifest.Manifest{
		SchemaVersion: manifest.SchemaVersion,
		SessionID:     parentID,
		Name:          "integration-parent",
		Workspace:     adapter.Workspace(),
		Harness:       "codex-cli",
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
		Context:       manifest.Context{Project: t.TempDir()},
		Tmux:          manifest.Tmux{SessionName: "integration-parent"},
	}
	if err := adapter.CreateSession(parent); err != nil {
		t.Fatalf("CreateSession() parent: %v", err)
	}
	t.Cleanup(func() { _ = adapter.DeleteSession(parentID) })
	inheritedName := parent.Name + "-exec"
	if err := adapter.LinkSessionParent(t.Context(), sessionID, renamed.Tmux.SessionRevision, parentID, &inheritedName); err != nil {
		t.Fatalf("LinkSessionParent() current revision: %v", err)
	}
	staleAfterLink := *renamed
	staleAfterLink.Name = "stale-after-parent-link"
	staleAfterLink.Context.Notes = "unrelated update after parent link"
	if err := adapter.UpdateSession(&staleAfterLink); err != nil {
		t.Fatalf("UpdateSession() stale after parent link: %v", err)
	}
	var linkedParentID, linkedName string
	if err := adapter.Conn().QueryRowContext(t.Context(), `SELECT parent_session_id, name FROM agm_sessions WHERE id = ? AND workspace = ?`, sessionID, adapter.Workspace()).Scan(&linkedParentID, &linkedName); err != nil {
		t.Fatalf("query linked production session: %v", err)
	}
	if linkedParentID != parentID || linkedName != inheritedName {
		t.Fatalf("linked production identity = (parent=%q name=%q), want (%q, %q)", linkedParentID, linkedName, parentID, inheritedName)
	}
}
