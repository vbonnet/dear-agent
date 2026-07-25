package dolt

import (
	"database/sql"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/vbonnet/dear-agent/agm/internal/manifest"
)

func TestSQLiteGetSessionByUUID_ClaudeUUID(t *testing.T) {
	adapter, err := NewSQLiteAdapter(filepath.Join(t.TempDir(), "agm.db"))
	if err != nil {
		t.Fatalf("NewSQLiteAdapter() error: %v", err)
	}
	t.Cleanup(func() { _ = adapter.Close() })

	m := &manifest.Manifest{
		SchemaVersion: manifest.SchemaVersion,
		SessionID:     "sqlite-uuid-session",
		Name:          "sqlite-uuid-session",
		Harness:       "claude-code",
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
		Context:       manifest.Context{Project: t.TempDir()},
		Claude:        manifest.Claude{UUID: "claude-conversation-uuid"},
		Tmux:          manifest.Tmux{SessionName: "sqlite-uuid-session"},
	}
	if err := adapter.CreateSession(m); err != nil {
		t.Fatalf("CreateSession() error: %v", err)
	}

	found, err := adapter.GetSessionByUUID(m.Claude.UUID)
	if err != nil {
		t.Fatalf("GetSessionByUUID() error: %v", err)
	}
	if found == nil || found.SessionID != m.SessionID {
		t.Fatalf("GetSessionByUUID() = %#v, want session %q", found, m.SessionID)
	}

	codex := &manifest.Manifest{
		SchemaVersion: manifest.SchemaVersion,
		SessionID:     "sqlite-codex-session",
		Name:          "sqlite-codex-session",
		Harness:       "codex-cli",
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
		Context:       manifest.Context{Project: t.TempDir()},
		Codex:         &manifest.Codex{SessionID: "codex-conversation-uuid"},
		Tmux:          manifest.Tmux{SessionName: "sqlite-codex-session"},
	}
	if err := adapter.CreateSession(codex); err != nil {
		t.Fatalf("CreateSession(codex) error: %v", err)
	}

	found, err = adapter.GetSessionByUUID(codex.Codex.SessionID)
	if err != nil {
		t.Fatalf("GetSessionByUUID(codex) error: %v", err)
	}
	if found == nil || found.SessionID != codex.SessionID {
		t.Fatalf("GetSessionByUUID(codex) = %#v, want session %q", found, codex.SessionID)
	}
}

func TestSQLiteAdapterUpgradesLegacySessionRevisionColumn(t *testing.T) {
	databasePath := filepath.Join(t.TempDir(), "agm.db")
	legacySchema := strings.Replace(sqliteSessionSchema, "  tmux_session_revision TEXT,\n", "", 1)
	if legacySchema == sqliteSessionSchema {
		t.Fatal("legacy schema fixture still contains tmux_session_revision")
	}
	legacyConn, err := sql.Open("sqlite", databasePath)
	if err != nil {
		t.Fatalf("open legacy SQLite store: %v", err)
	}
	if _, err := legacyConn.Exec(legacySchema); err != nil {
		_ = legacyConn.Close()
		t.Fatalf("initialize legacy SQLite schema: %v", err)
	}
	now := time.Now().Add(-time.Hour).UTC().Truncate(time.Second)
	m := &manifest.Manifest{
		SchemaVersion: manifest.SchemaVersion,
		SessionID:     "legacy-sqlite-revision-session",
		Name:          "legacy-sqlite-revision-session",
		Harness:       "codex-cli",
		CreatedAt:     now,
		UpdatedAt:     now,
		Context:       manifest.Context{Project: t.TempDir(), Notes: "preserve on upgrade"},
		Tmux:          manifest.Tmux{SessionName: "legacy-name"},
	}
	legacyAdapter := &Adapter{
		conn:              legacyConn,
		workspace:         "test",
		migrationsApplied: true,
		testStore:         true,
	}
	if err := legacyAdapter.CreateSession(m); err != nil {
		_ = legacyConn.Close()
		t.Fatalf("seed legacy SQLite session: %v", err)
	}
	if err := legacyConn.Close(); err != nil {
		t.Fatalf("close legacy SQLite store: %v", err)
	}

	adapter, err := NewSQLiteAdapter(databasePath)
	if err != nil {
		t.Fatalf("NewSQLiteAdapter() legacy reopen error: %v", err)
	}
	t.Cleanup(func() { _ = adapter.Close() })
	upgraded, err := sqliteSessionColumnExists(adapter.Conn(), "tmux_session_revision")
	if err != nil || !upgraded {
		t.Fatalf("SQLite ownership revision after reopen = (%v, %v), want (true, nil)", upgraded, err)
	}
	secondAdapter, err := NewSQLiteAdapter(databasePath)
	if err != nil {
		t.Fatalf("NewSQLiteAdapter() idempotent second reopen error: %v", err)
	}
	if err := secondAdapter.Close(); err != nil {
		t.Fatalf("close second upgraded SQLite adapter: %v", err)
	}
	stored, err := adapter.GetSession(m.SessionID)
	if err != nil {
		t.Fatalf("GetSession() after schema upgrade: %v", err)
	}
	if stored.Tmux.SessionName != "legacy-name" || stored.Context.Notes != "preserve on upgrade" {
		t.Fatalf("legacy row changed during schema upgrade: name=%q notes=%q", stored.Tmux.SessionName, stored.Context.Notes)
	}
	stored.Context.Notes = "updated after upgrade"
	if err := adapter.UpdateSession(stored); err != nil {
		t.Fatalf("UpdateSession() after schema upgrade: %v", err)
	}
	change, err := adapter.BeginTmuxSessionNameChange(t.Context(), m.SessionID, "canonical-name")
	if err != nil || change == nil {
		t.Fatalf("BeginTmuxSessionNameChange() after schema upgrade = (%v, %v), want non-nil change", change, err)
	}
	restored, err := adapter.RestoreTmuxSessionNameChange(t.Context(), *change)
	if err != nil || !restored {
		t.Fatalf("RestoreTmuxSessionNameChange() after schema upgrade = (%v, %v), want (true, nil)", restored, err)
	}
	final, err := adapter.GetSession(m.SessionID)
	if err != nil {
		t.Fatalf("GetSession() after upgraded lifecycle mutations: %v", err)
	}
	if final.Tmux.SessionName != "legacy-name" || final.Context.Notes != "updated after upgrade" || !final.UpdatedAt.Equal(stored.UpdatedAt) {
		t.Fatalf("upgraded lifecycle state = (%q, %q, %v), want (legacy-name, updated after upgrade, %v)", final.Tmux.SessionName, final.Context.Notes, final.UpdatedAt, stored.UpdatedAt)
	}
}

func TestSQLiteSessionLifecycle_RoundTripsReapingTombstone(t *testing.T) {
	adapter, err := NewSQLiteAdapter(filepath.Join(t.TempDir(), "agm.db"))
	if err != nil {
		t.Fatalf("NewSQLiteAdapter() error: %v", err)
	}
	t.Cleanup(func() { _ = adapter.Close() })

	m := &manifest.Manifest{
		SchemaVersion: manifest.SchemaVersion,
		SessionID:     "sqlite-reaping-session",
		Name:          "sqlite-reaping-session",
		Harness:       "agy",
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
		Context:       manifest.Context{Project: t.TempDir()},
		Tmux:          manifest.Tmux{SessionName: "sqlite-reaping-session"},
	}
	if err := adapter.CreateSession(m); err != nil {
		t.Fatalf("CreateSession() error: %v", err)
	}
	m.Lifecycle = manifest.LifecycleReaping
	if err := adapter.UpdateSession(m); err != nil {
		t.Fatalf("UpdateSession() error: %v", err)
	}

	stored, err := adapter.GetSession(m.SessionID)
	if err != nil {
		t.Fatalf("GetSession() error: %v", err)
	}
	if stored.Lifecycle != manifest.LifecycleReaping {
		t.Fatalf("Lifecycle = %q, want %q", stored.Lifecycle, manifest.LifecycleReaping)
	}
}

func TestSQLiteUpdateTmuxSessionNamePreservesOtherColumns(t *testing.T) {
	adapter, err := NewSQLiteAdapter(filepath.Join(t.TempDir(), "agm.db"))
	if err != nil {
		t.Fatalf("NewSQLiteAdapter() error: %v", err)
	}
	t.Cleanup(func() { _ = adapter.Close() })

	m := &manifest.Manifest{
		SchemaVersion:  manifest.SchemaVersion,
		SessionID:      "sqlite-tmux-name-session",
		Name:           "sqlite-tmux-name-session",
		Harness:        "codex-cli",
		Model:          "gpt-5.6-codex",
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
		Context:        manifest.Context{Project: t.TempDir(), Notes: "concurrent notes"},
		Claude:         manifest.Claude{UUID: "concurrent-hook-uuid"},
		Tmux:           manifest.Tmux{SessionName: "historical.name"},
		PermissionMode: "plan",
	}
	if err := adapter.CreateSession(m); err != nil {
		t.Fatalf("CreateSession() error: %v", err)
	}
	if err := adapter.UpdateTmuxSessionName(t.Context(), m.SessionID, "canonical-name"); err != nil {
		t.Fatalf("UpdateTmuxSessionName() error: %v", err)
	}

	stored, err := adapter.GetSession(m.SessionID)
	if err != nil {
		t.Fatalf("GetSession() error: %v", err)
	}
	if stored.Tmux.SessionName != "canonical-name" {
		t.Fatalf("Tmux.SessionName = %q, want canonical-name", stored.Tmux.SessionName)
	}
	if stored.Tmux.SessionRevision == "" {
		t.Fatal("Tmux.SessionRevision is empty after authoritative name update")
	}
	if stored.Claude.UUID != m.Claude.UUID || stored.Context.Notes != m.Context.Notes || stored.Model != m.Model || stored.PermissionMode != m.PermissionMode {
		t.Fatalf("narrow tmux update changed other columns: %#v", stored)
	}
}

func TestSQLiteTouchSessionActivityPreservesProvisionalTmuxRevision(t *testing.T) {
	adapter, err := NewSQLiteAdapter(filepath.Join(t.TempDir(), "agm.db"))
	if err != nil {
		t.Fatalf("NewSQLiteAdapter() error: %v", err)
	}
	t.Cleanup(func() { _ = adapter.Close() })

	initial := time.Now().Add(-time.Hour).UTC().Truncate(time.Second)
	m := &manifest.Manifest{
		SchemaVersion: manifest.SchemaVersion,
		SessionID:     "sqlite-activity-touch-session",
		Name:          "sqlite-activity-touch-session",
		Harness:       "codex-cli",
		CreatedAt:     initial,
		UpdatedAt:     initial,
		Context:       manifest.Context{Project: t.TempDir(), Notes: "preserve me"},
		Tmux:          manifest.Tmux{SessionName: "historical-name"},
	}
	if err := adapter.CreateSession(m); err != nil {
		t.Fatalf("CreateSession() error: %v", err)
	}
	change, err := adapter.BeginTmuxSessionNameChange(t.Context(), m.SessionID, "canonical-name")
	if err != nil || change == nil {
		t.Fatalf("BeginTmuxSessionNameChange() = (%v, %v), want provisional change", change, err)
	}
	if err := adapter.TouchSessionActivity(t.Context(), m.SessionID); err != nil {
		t.Fatalf("TouchSessionActivity() error: %v", err)
	}
	stored, err := adapter.GetSession(m.SessionID)
	if err != nil {
		t.Fatalf("GetSession() error: %v", err)
	}
	if stored.Tmux.SessionName != change.CurrentName || stored.Tmux.SessionRevision != change.CurrentRevision {
		t.Fatalf("tmux identity after activity touch = (%q, %q), want (%q, %q)", stored.Tmux.SessionName, stored.Tmux.SessionRevision, change.CurrentName, change.CurrentRevision)
	}
	if !stored.UpdatedAt.After(initial) || stored.Context.Notes != m.Context.Notes {
		t.Fatalf("activity touch state = (updated=%v notes=%q), want updated after %v with preserved notes", stored.UpdatedAt, stored.Context.Notes, initial)
	}
	frecency, err := adapter.GetByFrecency(0)
	if err != nil {
		t.Fatalf("GetByFrecency() error: %v", err)
	}
	if len(frecency) != 1 || frecency[0].Session.Tmux.SessionRevision != change.CurrentRevision {
		t.Fatalf("frecency read lost tmux revision: %#v", frecency)
	}
}

func TestSQLiteLinkSessionParentUsesExplicitIdentityCAS(t *testing.T) {
	adapter, err := NewSQLiteAdapter(filepath.Join(t.TempDir(), "agm.db"))
	if err != nil {
		t.Fatalf("NewSQLiteAdapter() error: %v", err)
	}
	t.Cleanup(func() { _ = adapter.Close() })
	createdAt := time.Now().UTC().Truncate(time.Second)
	parent := &manifest.Manifest{
		SchemaVersion: manifest.SchemaVersion,
		SessionID:     "parent-link-parent",
		Name:          "planning-session",
		Workspace:     adapter.Workspace(),
		CreatedAt:     createdAt,
		UpdatedAt:     createdAt,
		Context:       manifest.Context{Project: t.TempDir()},
		Tmux:          manifest.Tmux{SessionName: "planning-session"},
	}
	child := &manifest.Manifest{
		SchemaVersion: manifest.SchemaVersion,
		SessionID:     "parent-link-child",
		Name:          "Unknown",
		Workspace:     adapter.Workspace(),
		CreatedAt:     createdAt,
		UpdatedAt:     createdAt,
		Context:       manifest.Context{Project: t.TempDir()},
		Tmux:          manifest.Tmux{SessionName: "child-tmux"},
	}
	for _, session := range []*manifest.Manifest{parent, child} {
		if err := adapter.CreateSession(session); err != nil {
			t.Fatalf("CreateSession(%s) error: %v", session.SessionID, err)
		}
	}
	stale, err := adapter.GetSession(child.SessionID)
	if err != nil {
		t.Fatalf("GetSession() stale snapshot: %v", err)
	}
	current, err := adapter.GetSession(child.SessionID)
	if err != nil {
		t.Fatalf("GetSession() current snapshot: %v", err)
	}
	current.Context.Notes = "concurrent unrelated update"
	if err := adapter.UpdateSession(current); err != nil {
		t.Fatalf("UpdateSession() concurrent metadata: %v", err)
	}
	inheritedName := parent.Name + "-exec"
	if err := adapter.LinkSessionParent(t.Context(), child.SessionID, stale.Tmux.SessionRevision, parent.SessionID, &inheritedName); err == nil {
		t.Fatal("LinkSessionParent() from stale identity revision succeeded")
	}
	if got, err := adapter.GetParent(child.SessionID); err != nil || got != nil {
		t.Fatalf("parent after rejected link = (%#v, %v), want (nil, nil)", got, err)
	}

	current, err = adapter.GetSession(child.SessionID)
	if err != nil {
		t.Fatalf("GetSession() after rejected link: %v", err)
	}
	if err := adapter.LinkSessionParent(t.Context(), child.SessionID, current.Tmux.SessionRevision, parent.SessionID, &inheritedName); err != nil {
		t.Fatalf("LinkSessionParent() current identity: %v", err)
	}
	var storedParentID, storedName, storedRevision string
	if err := adapter.Conn().QueryRowContext(t.Context(), `SELECT parent_session_id, name, tmux_session_revision FROM agm_sessions WHERE id = ? AND workspace = ?`, child.SessionID, adapter.Workspace()).Scan(&storedParentID, &storedName, &storedRevision); err != nil {
		t.Fatalf("query linked child: %v", err)
	}
	if storedParentID != parent.SessionID || storedName != inheritedName || storedRevision == "" || storedRevision == current.Tmux.SessionRevision {
		t.Fatalf("linked child = (parent=%q name=%q revision=%q), want (%q, %q, advanced)", storedParentID, storedName, storedRevision, parent.SessionID, inheritedName)
	}
	if got, err := adapter.GetParent(child.SessionID); err != nil || got == nil || got.SessionID != parent.SessionID {
		t.Fatalf("GetParent() after link = (%#v, %v), want %s", got, err, parent.SessionID)
	}

	stale.Name = "stale-name-writer"
	stale.Context.Notes = "later stale metadata"
	if err := adapter.UpdateSession(stale); err != nil {
		t.Fatalf("UpdateSession() after link: %v", err)
	}
	if err := adapter.Conn().QueryRowContext(t.Context(), `SELECT parent_session_id, name FROM agm_sessions WHERE id = ? AND workspace = ?`, child.SessionID, adapter.Workspace()).Scan(&storedParentID, &storedName); err != nil {
		t.Fatalf("query child after stale writer: %v", err)
	}
	if storedParentID != parent.SessionID || storedName != inheritedName {
		t.Fatalf("stale writer changed linked identity = (parent=%q name=%q)", storedParentID, storedName)
	}
}

func TestSQLiteRenameSessionIdentityRejectsStaleRevision(t *testing.T) {
	adapter, err := NewSQLiteAdapter(filepath.Join(t.TempDir(), "agm.db"))
	if err != nil {
		t.Fatalf("NewSQLiteAdapter() error: %v", err)
	}
	t.Cleanup(func() { _ = adapter.Close() })
	m := &manifest.Manifest{
		SchemaVersion: manifest.SchemaVersion,
		SessionID:     "sqlite-authoritative-rename",
		Name:          "old-name",
		Workspace:     adapter.Workspace(),
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
		Context:       manifest.Context{Project: t.TempDir()},
		Tmux:          manifest.Tmux{SessionName: "old-tmux"},
	}
	if err := adapter.CreateSession(m); err != nil {
		t.Fatalf("CreateSession() error: %v", err)
	}
	stale, err := adapter.GetSession(m.SessionID)
	if err != nil {
		t.Fatalf("GetSession() stale snapshot error: %v", err)
	}
	concurrent, err := adapter.GetSession(m.SessionID)
	if err != nil {
		t.Fatalf("GetSession() concurrent snapshot error: %v", err)
	}
	concurrent.Context.Notes = "concurrent unrelated update"
	if err := adapter.UpdateSession(concurrent); err != nil {
		t.Fatalf("UpdateSession() concurrent update error: %v", err)
	}
	conflict, err := adapter.RenameSessionIdentity(t.Context(), stale.SessionID, stale.Name, stale.Tmux.SessionName, stale.Tmux.SessionRevision, "new-name")
	if err == nil || !strings.Contains(err.Error(), "changed concurrently") || !conflict.TmuxRollbackSafe {
		t.Fatalf("RenameSessionIdentity() stale error = %v, want explicit conflict", err)
	}
	current, err := adapter.GetSession(m.SessionID)
	if err != nil {
		t.Fatalf("GetSession() after conflict error: %v", err)
	}
	if current.Name != "old-name" || current.Tmux.SessionName != "old-tmux" || current.Context.Notes != concurrent.Context.Notes {
		t.Fatalf("identity after conflict = (name=%q tmux=%q notes=%q), want unchanged identity plus concurrent note", current.Name, current.Tmux.SessionName, current.Context.Notes)
	}
	observedRevision := current.Tmux.SessionRevision
	if _, err := adapter.RenameSessionIdentity(t.Context(), current.SessionID, current.Name, current.Tmux.SessionName, observedRevision, "new-name"); err != nil {
		t.Fatalf("RenameSessionIdentity() current error: %v", err)
	}
	renamed, err := adapter.GetSession(m.SessionID)
	if err != nil {
		t.Fatalf("GetSession() after rename error: %v", err)
	}
	if renamed.Name != "new-name" || renamed.Tmux.SessionName != "new-name" || renamed.Tmux.SessionRevision == "" || renamed.Tmux.SessionRevision == observedRevision || renamed.Context.Notes != concurrent.Context.Notes {
		t.Fatalf("renamed identity = (name=%q tmux=%q revision=%q notes=%q), want atomic renamed identity, advanced revision, and preserved note", renamed.Name, renamed.Tmux.SessionName, renamed.Tmux.SessionRevision, renamed.Context.Notes)
	}
}

func TestClassifySessionIdentityRenameAfterError(t *testing.T) {
	primary := errors.New("autocommit reply lost")
	tests := []struct {
		name         string
		currentName  string
		currentTmux  string
		currentRev   string
		wantSuccess  bool
		wantRollback bool
	}{
		{name: "exact generated revision committed", currentName: "new-name", currentTmux: "new-name", currentRev: "next-revision", wantSuccess: true},
		{name: "later writer superseded committed revision", currentName: "new-name", currentTmux: "new-name", currentRev: "later-revision", wantSuccess: true},
		{name: "exact fence revision proves rollback safe", currentName: "old-name", currentTmux: "old-tmux", currentRev: "fence-revision", wantRollback: true},
		{name: "later revision also fences pending write", currentName: "old-name", currentTmux: "old-tmux", currentRev: "later-unrelated-revision", wantRollback: true},
		{name: "unchanged observed revision remains ambiguous", currentName: "old-name", currentTmux: "old-tmux", currentRev: "observed-revision"},
		{name: "different identity makes rollback unsafe", currentName: "other-name", currentTmux: "other-tmux", currentRev: "other-revision"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := classifySessionIdentityRenameAfterError("old-name", "old-tmux", "observed-revision", "new-name", "next-revision", "fence-revision", tt.currentName, tt.currentTmux, tt.currentRev, primary)
			if (err == nil) != tt.wantSuccess || result.TmuxRollbackSafe != tt.wantRollback {
				t.Fatalf("classification = (result=%+v err=%v), want success=%v rollback=%v", result, err, tt.wantSuccess, tt.wantRollback)
			}
		})
	}
}

func TestSQLiteSessionIdentityRenameFenceRejectsObservedRevision(t *testing.T) {
	adapter, err := NewSQLiteAdapter(filepath.Join(t.TempDir(), "agm.db"))
	if err != nil {
		t.Fatalf("NewSQLiteAdapter() error: %v", err)
	}
	t.Cleanup(func() { _ = adapter.Close() })
	m := &manifest.Manifest{
		SchemaVersion: manifest.SchemaVersion,
		SessionID:     "sqlite-rename-fence",
		Name:          "old-name",
		Workspace:     adapter.Workspace(),
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
		Context:       manifest.Context{Project: t.TempDir()},
		Tmux:          manifest.Tmux{SessionName: "old-tmux"},
	}
	if err := adapter.CreateSession(m); err != nil {
		t.Fatalf("CreateSession() error: %v", err)
	}
	observed, err := adapter.GetSession(m.SessionID)
	if err != nil {
		t.Fatalf("GetSession() error: %v", err)
	}
	observedValue := nullableStringValue(sql.NullString{String: observed.Tmux.SessionRevision, Valid: observed.Tmux.SessionRevision != ""})
	if err := adapter.fenceSessionIdentityRename(t.Context(), observed.SessionID, observed.Name, observed.Tmux.SessionName, observedValue, "fence-revision"); err != nil {
		t.Fatalf("fenceSessionIdentityRename() error: %v", err)
	}
	result, err := adapter.RenameSessionIdentity(t.Context(), observed.SessionID, observed.Name, observed.Tmux.SessionName, observed.Tmux.SessionRevision, "new-name")
	if err == nil || !result.TmuxRollbackSafe {
		t.Fatalf("RenameSessionIdentity() after fence = (result=%+v err=%v), want fenced conflict with safe rollback", result, err)
	}
	current, err := adapter.GetSession(m.SessionID)
	if err != nil {
		t.Fatalf("GetSession() after fence error: %v", err)
	}
	if current.Name != "old-name" || current.Tmux.SessionName != "old-tmux" || current.Tmux.SessionRevision != "fence-revision" {
		t.Fatalf("identity after fence = (name=%q tmux=%q revision=%q)", current.Name, current.Tmux.SessionName, current.Tmux.SessionRevision)
	}
}

func TestSQLiteTmuxSessionNameChangeOwnsAndRestoresExactWrite(t *testing.T) {
	adapter, err := NewSQLiteAdapter(filepath.Join(t.TempDir(), "agm.db"))
	if err != nil {
		t.Fatalf("NewSQLiteAdapter() error: %v", err)
	}
	t.Cleanup(func() { _ = adapter.Close() })

	previousUpdatedAt := time.Now().Add(-time.Hour).UTC().Truncate(time.Microsecond)
	m := &manifest.Manifest{
		SchemaVersion: manifest.SchemaVersion,
		SessionID:     "tmux-cas-session",
		Name:          "tmux-cas-session",
		Workspace:     adapter.Workspace(),
		CreatedAt:     previousUpdatedAt,
		UpdatedAt:     previousUpdatedAt,
		Context:       manifest.Context{Project: t.TempDir()},
		Tmux:          manifest.Tmux{SessionName: "legacy-name"},
	}
	if err := adapter.CreateSession(m); err != nil {
		t.Fatalf("CreateSession() error: %v", err)
	}
	stored, err := adapter.GetSession(m.SessionID)
	if err != nil {
		t.Fatalf("GetSession() error: %v", err)
	}
	stored.Context.Notes = "concurrent metadata retained"
	if err := adapter.UpdateSession(stored); err != nil {
		t.Fatalf("UpdateSession() error: %v", err)
	}
	change, err := adapter.BeginTmuxSessionNameChange(t.Context(), m.SessionID, "canonical-name")
	if err != nil || change == nil {
		t.Fatalf("BeginTmuxSessionNameChange() = (%v, %v), want non-nil change", change, err)
	}
	if state, err := adapter.inspectTmuxSessionNameChange(t.Context(), *change); err != nil || state != tmuxSessionNameChangeCurrent {
		t.Fatalf("inspect current tmux-name change = (%v, %v), want current", state, err)
	}
	restored, err := adapter.RestoreTmuxSessionNameChange(t.Context(), *change)
	if err != nil || !restored {
		t.Fatalf("RestoreTmuxSessionNameChange() = (%v, %v), want (true, nil)", restored, err)
	}
	if state, err := adapter.inspectTmuxSessionNameChange(t.Context(), *change); err != nil || state != tmuxSessionNameChangePrevious {
		t.Fatalf("inspect restored tmux-name change = (%v, %v), want previous", state, err)
	}
	final, err := adapter.GetSession(m.SessionID)
	if err != nil {
		t.Fatalf("GetSession() after restore error: %v", err)
	}
	if final.Tmux.SessionName != stored.Tmux.SessionName || final.Context.Notes != "concurrent metadata retained" || !final.UpdatedAt.Equal(stored.UpdatedAt) {
		t.Fatalf("restored state = (%q, %q, %v), want (%q, concurrent metadata retained, %v)", final.Tmux.SessionName, final.Context.Notes, final.UpdatedAt, stored.Tmux.SessionName, stored.UpdatedAt)
	}
}

func TestResolveTmuxSessionNameChangeCommitErrorPreservesUncertainOwnership(t *testing.T) {
	commitErr := errors.New("commit acknowledgement lost")
	inspectErr := errors.New("re-read unavailable")
	change := &TmuxSessionNameChange{SessionID: "session-id", CurrentRevision: "owned-revision"}
	tests := []struct {
		name        string
		state       tmuxSessionNameChangeState
		inspectErr  error
		wantPending bool
	}{
		{name: "previous revision proves no commit", state: tmuxSessionNameChangePrevious},
		{name: "current revision proves commit", state: tmuxSessionNameChangeCurrent, wantPending: true},
		{name: "superseded revision is uncertain", state: tmuxSessionNameChangeSuperseded, wantPending: true},
		{name: "failed inspection is uncertain", state: tmuxSessionNameChangeUnknown, inspectErr: inspectErr, wantPending: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := resolveTmuxSessionNameChangeCommitError(change, commitErr, tt.state, tt.inspectErr)
			if !errors.Is(err, commitErr) {
				t.Fatalf("resolve error = %v, want commit error", err)
			}
			if tt.inspectErr != nil && !errors.Is(err, tt.inspectErr) {
				t.Fatalf("resolve error = %v, want inspect error", err)
			}
			if (got != nil) != tt.wantPending {
				t.Fatalf("pending change = %#v, want pending=%v", got, tt.wantPending)
			}
		})
	}
}

func TestSQLiteTmuxSessionNameStaleFullUpdatePreservesOwnership(t *testing.T) {
	adapter, err := NewSQLiteAdapter(filepath.Join(t.TempDir(), "agm.db"))
	if err != nil {
		t.Fatalf("NewSQLiteAdapter() error: %v", err)
	}
	t.Cleanup(func() { _ = adapter.Close() })
	m := &manifest.Manifest{
		SchemaVersion: manifest.SchemaVersion,
		SessionID:     "tmux-cas-stale-full-update",
		Name:          "tmux-cas-stale-full-update",
		Workspace:     adapter.Workspace(),
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
		Context:       manifest.Context{Project: t.TempDir()},
		Tmux:          manifest.Tmux{SessionName: "legacy-name"},
	}
	if err := adapter.CreateSession(m); err != nil {
		t.Fatalf("CreateSession() error: %v", err)
	}
	stale, err := adapter.GetSession(m.SessionID)
	if err != nil {
		t.Fatalf("GetSession() before provisional change: %v", err)
	}
	secondStale, err := adapter.GetSession(m.SessionID)
	if err != nil {
		t.Fatalf("second GetSession() before provisional change: %v", err)
	}
	change, err := adapter.BeginTmuxSessionNameChange(t.Context(), m.SessionID, "canonical-name")
	if err != nil || change == nil {
		t.Fatalf("BeginTmuxSessionNameChange() = (%v, %v), want non-nil change", change, err)
	}
	stale.Name = "stale-display-name"
	stale.Context.Notes = "unrelated stale-writer update"
	if err := adapter.UpdateSession(stale); err != nil {
		t.Fatalf("UpdateSession() from stale snapshot: %v", err)
	}
	stored, err := adapter.GetSession(m.SessionID)
	if err != nil {
		t.Fatalf("GetSession() after stale update: %v", err)
	}
	if stored.Name != m.Name || stored.Tmux.SessionName != change.CurrentName || stored.Tmux.SessionRevision == "" || stored.Tmux.SessionRevision == change.CurrentRevision {
		t.Fatalf("identity after stale update = (%q, %q, %q), want original display name, canonical tmux name, and advanced revision", stored.Name, stored.Tmux.SessionName, stored.Tmux.SessionRevision)
	}
	if stored.Context.Notes != stale.Context.Notes {
		t.Fatalf("unrelated notes = %q, want %q", stored.Context.Notes, stale.Context.Notes)
	}
	firstSupersedingRevision := stored.Tmux.SessionRevision
	secondStale.Name = "second-stale-display-name"
	secondStale.Context.Notes = "second unrelated stale-writer update"
	if err := adapter.UpdateSession(secondStale); err != nil {
		t.Fatalf("UpdateSession() from second stale snapshot: %v", err)
	}
	stored, err = adapter.GetSession(m.SessionID)
	if err != nil {
		t.Fatalf("GetSession() after second stale update: %v", err)
	}
	if stored.Name != m.Name || stored.Tmux.SessionName != change.CurrentName || stored.Tmux.SessionRevision == "" || stored.Tmux.SessionRevision == firstSupersedingRevision {
		t.Fatalf("identity after second stale update = (%q, %q, %q), want original display name, canonical tmux name, and another advanced revision", stored.Name, stored.Tmux.SessionName, stored.Tmux.SessionRevision)
	}
	if stored.Context.Notes != secondStale.Context.Notes {
		t.Fatalf("second unrelated notes = %q, want %q", stored.Context.Notes, secondStale.Context.Notes)
	}
	newerUpdatedAt := secondStale.UpdatedAt
	restored, err := adapter.RestoreTmuxSessionNameChange(t.Context(), *change)
	if err != nil || restored {
		t.Fatalf("RestoreTmuxSessionNameChange() = (%v, %v), want (false, nil)", restored, err)
	}
	final, err := adapter.GetSession(m.SessionID)
	if err != nil {
		t.Fatalf("GetSession() after restore: %v", err)
	}
	if final.Tmux.SessionName != change.CurrentName || final.Context.Notes != secondStale.Context.Notes || !final.UpdatedAt.Equal(newerUpdatedAt) {
		t.Fatalf("superseded state = (name=%q notes=%q updated=%v), want canonical name plus second unrelated update timestamp %v", final.Tmux.SessionName, final.Context.Notes, final.UpdatedAt, newerUpdatedAt)
	}
}

func TestSQLiteTmuxSessionNameCompensationRejectsNewerMetadata(t *testing.T) {
	adapter, err := NewSQLiteAdapter(filepath.Join(t.TempDir(), "agm.db"))
	if err != nil {
		t.Fatalf("NewSQLiteAdapter() error: %v", err)
	}
	t.Cleanup(func() { _ = adapter.Close() })
	m := &manifest.Manifest{
		SchemaVersion: manifest.SchemaVersion,
		SessionID:     "tmux-cas-newer-session",
		Name:          "tmux-cas-newer-session",
		Workspace:     adapter.Workspace(),
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
		Context:       manifest.Context{Project: t.TempDir()},
		Tmux:          manifest.Tmux{SessionName: "legacy-name"},
	}
	if err := adapter.CreateSession(m); err != nil {
		t.Fatalf("CreateSession() error: %v", err)
	}
	change, err := adapter.BeginTmuxSessionNameChange(t.Context(), m.SessionID, "canonical-name")
	if err != nil || change == nil {
		t.Fatalf("BeginTmuxSessionNameChange() = (%v, %v), want non-nil change", change, err)
	}
	latest, err := adapter.GetSession(m.SessionID)
	if err != nil {
		t.Fatalf("GetSession() error: %v", err)
	}
	latest.Context.Notes = "newer writer"
	if err := adapter.UpdateSession(latest); err != nil {
		t.Fatalf("UpdateSession() error: %v", err)
	}
	if state, err := adapter.inspectTmuxSessionNameChange(t.Context(), *change); err != nil || state != tmuxSessionNameChangeSuperseded {
		t.Fatalf("inspect superseded tmux-name change = (%v, %v), want superseded", state, err)
	}
	restored, err := adapter.RestoreTmuxSessionNameChange(t.Context(), *change)
	if err != nil || restored {
		t.Fatalf("RestoreTmuxSessionNameChange() = (%v, %v), want (false, nil)", restored, err)
	}
	final, err := adapter.GetSession(m.SessionID)
	if err != nil {
		t.Fatalf("GetSession() after rejected restore error: %v", err)
	}
	if final.Tmux.SessionName != "canonical-name" || final.Context.Notes != "newer writer" {
		t.Fatalf("newer state was overwritten: name=%q notes=%q", final.Tmux.SessionName, final.Context.Notes)
	}
}

func TestSQLiteTmuxSessionNameChangeCompletesOwnershipToken(t *testing.T) {
	adapter, err := NewSQLiteAdapter(filepath.Join(t.TempDir(), "agm.db"))
	if err != nil {
		t.Fatalf("NewSQLiteAdapter() error: %v", err)
	}
	t.Cleanup(func() { _ = adapter.Close() })
	m := &manifest.Manifest{
		SchemaVersion: manifest.SchemaVersion,
		SessionID:     "tmux-cas-complete-session",
		Name:          "tmux-cas-complete-session",
		Workspace:     adapter.Workspace(),
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
		Context:       manifest.Context{Project: t.TempDir()},
		Tmux:          manifest.Tmux{SessionName: "legacy-name"},
	}
	if err := adapter.CreateSession(m); err != nil {
		t.Fatalf("CreateSession() error: %v", err)
	}
	change, err := adapter.BeginTmuxSessionNameChange(t.Context(), m.SessionID, "canonical-name")
	if err != nil || change == nil {
		t.Fatalf("BeginTmuxSessionNameChange() = (%v, %v), want non-nil change", change, err)
	}
	completed, err := adapter.CompleteTmuxSessionNameChange(t.Context(), *change)
	if err != nil || !completed {
		t.Fatalf("CompleteTmuxSessionNameChange() = (%v, %v), want (true, nil)", completed, err)
	}
	var revision sql.NullString
	if err := adapter.Conn().QueryRowContext(t.Context(), `SELECT tmux_session_revision FROM agm_sessions WHERE id = ?`, m.SessionID).Scan(&revision); err != nil {
		t.Fatalf("query completed tmux revision: %v", err)
	}
	if !revision.Valid || revision.String == "" || revision.String == change.CurrentRevision {
		t.Fatalf("completed tmux revision = %#v, want a new non-provisional revision", revision)
	}
}

func TestSQLiteUpdateSessionRoundTripsModel(t *testing.T) {
	adapter, err := NewSQLiteAdapter(filepath.Join(t.TempDir(), "agm.db"))
	if err != nil {
		t.Fatalf("NewSQLiteAdapter() error: %v", err)
	}
	t.Cleanup(func() { _ = adapter.Close() })

	m := &manifest.Manifest{
		SchemaVersion: manifest.SchemaVersion,
		SessionID:     "sqlite-model-session",
		Name:          "sqlite-model-session",
		Harness:       "agy",
		Model:         "Gemini 3.5 Flash (Medium)",
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
		Context:       manifest.Context{Project: t.TempDir()},
		Tmux:          manifest.Tmux{SessionName: "sqlite-model-session"},
	}
	if err := adapter.CreateSession(m); err != nil {
		t.Fatalf("CreateSession() error: %v", err)
	}
	m.Model = ""
	if err := adapter.UpdateSession(m); err != nil {
		t.Fatalf("UpdateSession() error: %v", err)
	}

	stored, err := adapter.GetSession(m.SessionID)
	if err != nil {
		t.Fatalf("GetSession() error: %v", err)
	}
	if stored.Model != "" {
		t.Fatalf("Model = %q, want cleared unknown provenance", stored.Model)
	}
}

func TestSQLiteSandboxOwnershipMetadataRoundTripsForArchive(t *testing.T) {
	adapter, err := NewSQLiteAdapter(filepath.Join(t.TempDir(), "agm.db"))
	if err != nil {
		t.Fatalf("NewSQLiteAdapter() error: %v", err)
	}
	t.Cleanup(func() { _ = adapter.Close() })

	createdAt := time.Now().UTC().Truncate(time.Microsecond)
	wantSandbox := &manifest.SandboxConfig{
		Enabled:    true,
		ID:         "sandbox-roundtrip-session",
		Provider:   "apfs-reflink",
		MergedPath: "/tmp/agm-sandbox/merged",
		WorkingDir: "/tmp/agm-sandbox/merged/repo0",
		CreatedAt:  createdAt,
	}
	m := &manifest.Manifest{
		SchemaVersion: manifest.SchemaVersion,
		SessionID:     "sandbox-roundtrip-session",
		Name:          "sandbox-roundtrip-session",
		Harness:       "codex-cli",
		CreatedAt:     createdAt,
		UpdatedAt:     createdAt,
		Context:       manifest.Context{Project: wantSandbox.WorkingDir},
		Tmux:          manifest.Tmux{SessionName: "sandbox-roundtrip-session"},
		Sandbox:       wantSandbox,
	}
	if err := adapter.CreateSession(m); err != nil {
		t.Fatalf("CreateSession() error: %v", err)
	}

	assertSandbox := func(t *testing.T, got, want *manifest.SandboxConfig) {
		t.Helper()
		if got == nil {
			t.Fatal("Sandbox = nil, want persisted ownership metadata")
		}
		if got.Enabled != want.Enabled ||
			got.ID != want.ID ||
			got.Provider != want.Provider ||
			got.MergedPath != want.MergedPath ||
			got.WorkingDir != want.WorkingDir ||
			!got.CreatedAt.Equal(want.CreatedAt) {
			t.Fatalf("Sandbox = %#v, want %#v", got, want)
		}
	}

	stored, err := adapter.GetSession(m.SessionID)
	if err != nil {
		t.Fatalf("GetSession() after create error: %v", err)
	}
	assertSandbox(t, stored.Sandbox, wantSandbox)

	wantSandbox = &manifest.SandboxConfig{
		Enabled:    true,
		ID:         m.SessionID,
		Provider:   "mock-updated",
		MergedPath: "/tmp/agm-sandbox-updated/merged",
		WorkingDir: "/tmp/agm-sandbox-updated/merged/repo0",
		CreatedAt:  createdAt.Add(time.Second),
	}
	stored.Sandbox = wantSandbox
	if err := adapter.UpdateSession(stored); err != nil {
		t.Fatalf("UpdateSession() error: %v", err)
	}
	updated, err := adapter.GetSession(m.SessionID)
	if err != nil {
		t.Fatalf("GetSession() after update error: %v", err)
	}
	assertSandbox(t, updated.Sandbox, wantSandbox)
}

func TestSQLiteMissingSandboxMetadataDoesNotInferOwnership(t *testing.T) {
	adapter, err := NewSQLiteAdapter(filepath.Join(t.TempDir(), "agm.db"))
	if err != nil {
		t.Fatalf("NewSQLiteAdapter() error: %v", err)
	}
	t.Cleanup(func() { _ = adapter.Close() })

	now := time.Now()
	m := &manifest.Manifest{
		SchemaVersion: manifest.SchemaVersion,
		SessionID:     "legacy-session-without-sandbox-metadata",
		Name:          "legacy-session-without-sandbox-metadata",
		Harness:       "codex-cli",
		CreatedAt:     now,
		UpdatedAt:     now,
		Context: manifest.Context{
			Project: "/Users/example/.agm/sandboxes/unowned/merged/repo0",
		},
		Tmux: manifest.Tmux{SessionName: "legacy-session-without-sandbox-metadata"},
	}
	if err := adapter.CreateSession(m); err != nil {
		t.Fatalf("CreateSession() error: %v", err)
	}
	stored, err := adapter.GetSession(m.SessionID)
	if err != nil {
		t.Fatalf("GetSession() error: %v", err)
	}
	if stored.Sandbox != nil {
		t.Fatalf("Sandbox = %#v, want nil without persisted ownership", stored.Sandbox)
	}
}

func TestSQLiteCreateSessionDefaultsModelOnlyForClaude(t *testing.T) {
	for _, tc := range []struct {
		name        string
		harness     string
		wantHarness string
		wantModel   string
	}{
		{name: "legacy manifest", wantHarness: "claude-code", wantModel: "claude-sonnet-4-5"},
		{name: "Claude Code", harness: "claude-code", wantHarness: "claude-code", wantModel: "claude-sonnet-4-5"},
		{name: "Antigravity", harness: "agy", wantHarness: "agy"},
		{name: "Codex", harness: "codex-cli", wantHarness: "codex-cli"},
		{name: "OpenCode", harness: "opencode-cli", wantHarness: "opencode-cli"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			adapter, err := NewSQLiteAdapter(filepath.Join(t.TempDir(), "agm.db"))
			if err != nil {
				t.Fatalf("NewSQLiteAdapter() error: %v", err)
			}
			t.Cleanup(func() { _ = adapter.Close() })

			m := &manifest.Manifest{
				SchemaVersion: manifest.SchemaVersion,
				SessionID:     "sqlite-create-model-session",
				Name:          "sqlite-create-model-session",
				Harness:       tc.harness,
				CreatedAt:     time.Now(),
				UpdatedAt:     time.Now(),
				Context:       manifest.Context{Project: t.TempDir()},
				Tmux:          manifest.Tmux{SessionName: "sqlite-create-model-session"},
			}
			if err := adapter.CreateSession(m); err != nil {
				t.Fatalf("CreateSession() error: %v", err)
			}

			stored, err := adapter.GetSession(m.SessionID)
			if err != nil {
				t.Fatalf("GetSession() error: %v", err)
			}
			if stored.Harness != tc.wantHarness || stored.Model != tc.wantModel {
				t.Fatalf("Harness/Model = %q/%q, want %q/%q", stored.Harness, stored.Model, tc.wantHarness, tc.wantModel)
			}
		})
	}
}
