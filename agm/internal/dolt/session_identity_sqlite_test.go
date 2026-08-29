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

func TestSessionIdentityRenameInspectionErrorRetainsReservationOnlyWhenFenceFails(t *testing.T) {
	primaryErr := errors.New("rename inspection failed")
	fenceErr := errors.New("rename fence failed")

	err := sessionIdentityRenameInspectionError(primaryErr, fenceErr)
	var uncertain *SessionIdentityMutationCommitUncertainError
	if !errors.As(err, &uncertain) || !errors.Is(err, primaryErr) {
		t.Fatalf("inspection error with failed fence = %v, want typed mutation uncertainty", err)
	}

	err = sessionIdentityRenameInspectionError(primaryErr, nil)
	uncertain = nil
	if errors.As(err, &uncertain) || !errors.Is(err, primaryErr) {
		t.Fatalf("inspection error after successful fence = %v, want ordinary inspection error", err)
	}
}

func TestSQLiteArchivedParentLinkFencesStaleReactivation(t *testing.T) {
	adapter, err := NewSQLiteAdapter(filepath.Join(t.TempDir(), "agm.db"))
	if err != nil {
		t.Fatalf("NewSQLiteAdapter() error: %v", err)
	}
	t.Cleanup(func() { _ = adapter.Close() })

	now := time.Now()
	parent := &manifest.Manifest{
		SessionID: "archived-link-parent",
		Name:      "planning",
		CreatedAt: now,
		UpdatedAt: now,
	}
	child := &manifest.Manifest{
		SessionID: "archived-link-child",
		Name:      "old-archived-name",
		Lifecycle: manifest.LifecycleArchived,
		CreatedAt: now,
		UpdatedAt: now,
		Tmux:      manifest.Tmux{SessionName: "archived-child-tmux"},
	}
	for _, session := range []*manifest.Manifest{parent, child} {
		if err := adapter.CreateSession(session); err != nil {
			t.Fatalf("CreateSession(%s) error: %v", session.SessionID, err)
		}
	}
	staleReactivation, err := adapter.GetSession(child.SessionID)
	if err != nil {
		t.Fatalf("GetSession() stale reactivation: %v", err)
	}
	inheritedName := "planning-exec"
	if err := adapter.LinkSessionParent(
		t.Context(),
		child.SessionID,
		staleReactivation.Tmux.SessionRevision,
		parent.SessionID,
		&inheritedName,
	); err != nil {
		t.Fatalf("LinkSessionParent() archived child: %v", err)
	}
	if err := adapter.ReserveSessionName("concurrent-creator", inheritedName); err != nil {
		t.Fatalf("ReserveSessionName() concurrent creator: %v", err)
	}

	result, err := adapter.ReactivateSession(staleReactivation)
	if err == nil || result.StorageCommitted {
		t.Fatalf("ReactivateSession(stale identity) = (%+v, %v), want uncommitted conflict", result, err)
	}
	current, err := adapter.GetSession(child.SessionID)
	if err != nil {
		t.Fatalf("GetSession() after stale reactivation: %v", err)
	}
	if current.Lifecycle != manifest.LifecycleArchived || current.Name != inheritedName {
		t.Fatalf("linked child after stale reactivation = lifecycle %q name %q, want archived/%q", current.Lifecycle, current.Name, inheritedName)
	}
	if err := adapter.CreateSession(&manifest.Manifest{
		SessionID: "concurrent-creator",
		Name:      inheritedName,
		CreatedAt: now,
		UpdatedAt: now,
	}); err != nil {
		t.Fatalf("CreateSession() concurrent creator: %v", err)
	}
	var activeNames int
	if err := adapter.Conn().QueryRow(
		`SELECT COUNT(*) FROM agm_sessions
		 WHERE workspace = ? AND name = ? AND status != 'archived'`,
		adapter.Workspace(),
		inheritedName,
	).Scan(&activeNames); err != nil {
		t.Fatalf("count active inherited names: %v", err)
	}
	if activeNames != 1 {
		t.Fatalf("active inherited names = %d, want 1", activeNames)
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
	if err := adapter.ReserveSessionName("competing-parent-link", inheritedName); err != nil {
		t.Fatalf("ReserveSessionName() competing parent link: %v", err)
	}
	err = adapter.LinkSessionParent(t.Context(), child.SessionID, current.Tmux.SessionRevision, parent.SessionID, &inheritedName)
	if _, ok := errors.AsType[*SessionNameConflictError](err); !ok {
		t.Fatalf("LinkSessionParent() with competing target lease = %v, want SessionNameConflictError", err)
	}
	if got, err := adapter.GetParent(child.SessionID); err != nil || got != nil {
		t.Fatalf("parent after competing target lease = (%#v, %v), want (nil, nil)", got, err)
	}
	if err := adapter.ReleaseSessionNameReservation("competing-parent-link"); err != nil {
		t.Fatalf("ReleaseSessionNameReservation() competing parent link: %v", err)
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
	var reservations int
	if err := adapter.Conn().QueryRowContext(
		t.Context(),
		`SELECT COUNT(*) FROM agm_session_name_reservations WHERE session_id = ?`,
		child.SessionID,
	).Scan(&reservations); err != nil {
		t.Fatalf("count inherited-name reservations: %v", err)
	}
	if reservations != 0 {
		t.Fatalf("inherited-name reservations after parent link = %d, want 0", reservations)
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

func TestSQLiteLinkSessionParentRejectsReservedInheritedName(t *testing.T) {
	adapter, err := NewSQLiteAdapter(filepath.Join(t.TempDir(), "agm.db"))
	if err != nil {
		t.Fatalf("NewSQLiteAdapter() error: %v", err)
	}
	t.Cleanup(func() { _ = adapter.Close() })
	createdAt := time.Now().UTC().Truncate(time.Second)
	parent := &manifest.Manifest{
		SchemaVersion: manifest.SchemaVersion,
		SessionID:     "reserved-link-parent",
		Name:          "planning-session",
		Workspace:     adapter.Workspace(),
		CreatedAt:     createdAt,
		UpdatedAt:     createdAt,
		Context:       manifest.Context{Project: t.TempDir()},
		Tmux:          manifest.Tmux{SessionName: "planning-session"},
	}
	child := &manifest.Manifest{
		SchemaVersion: manifest.SchemaVersion,
		SessionID:     "reserved-link-child",
		Name:          "Unknown",
		Workspace:     adapter.Workspace(),
		CreatedAt:     createdAt,
		UpdatedAt:     createdAt,
		Context:       manifest.Context{Project: t.TempDir()},
		Tmux:          manifest.Tmux{SessionName: "child-tmux"},
	}
	conflicting := &manifest.Manifest{
		SchemaVersion: manifest.SchemaVersion,
		SessionID:     "reserved-link-conflict",
		Name:          "planning-session-exec",
		Workspace:     adapter.Workspace(),
		CreatedAt:     createdAt,
		UpdatedAt:     createdAt,
		Context:       manifest.Context{Project: t.TempDir()},
		Tmux:          manifest.Tmux{SessionName: "planning-session-exec"},
	}
	for _, session := range []*manifest.Manifest{parent, child, conflicting} {
		if err := adapter.CreateSession(session); err != nil {
			t.Fatalf("CreateSession(%s) error: %v", session.SessionID, err)
		}
	}
	observed, err := adapter.GetSession(child.SessionID)
	if err != nil {
		t.Fatalf("GetSession() child: %v", err)
	}
	inheritedName := parent.Name + "-exec"
	err = adapter.LinkSessionParent(
		t.Context(),
		child.SessionID,
		observed.Tmux.SessionRevision,
		parent.SessionID,
		&inheritedName,
	)
	var conflict *SessionNameConflictError
	if !errors.As(err, &conflict) || conflict.Name != inheritedName {
		t.Fatalf("LinkSessionParent() error = %v, want conflict for %q", err, inheritedName)
	}
	unchanged, err := adapter.GetSession(child.SessionID)
	if err != nil {
		t.Fatalf("GetSession() after conflict: %v", err)
	}
	if unchanged.Name != child.Name || unchanged.ParentSessionID != nil {
		t.Fatalf("child after inherited-name conflict = name %q parent %#v, want %q and nil", unchanged.Name, unchanged.ParentSessionID, child.Name)
	}
}

func TestSQLiteRenameSessionIdentityUsesTargetNameReservation(t *testing.T) {
	adapter, err := NewSQLiteAdapter(filepath.Join(t.TempDir(), "agm.db"))
	if err != nil {
		t.Fatalf("NewSQLiteAdapter() error: %v", err)
	}
	t.Cleanup(func() { _ = adapter.Close() })
	m := &manifest.Manifest{
		SchemaVersion: manifest.SchemaVersion,
		SessionID:     "sqlite-reserved-rename",
		Name:          "old-name",
		Workspace:     adapter.Workspace(),
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
		Context:       manifest.Context{Project: t.TempDir()},
		Tmux:          manifest.Tmux{SessionName: "old-name"},
	}
	if err := adapter.CreateSession(m); err != nil {
		t.Fatalf("CreateSession() error: %v", err)
	}
	observed, err := adapter.GetSession(m.SessionID)
	if err != nil {
		t.Fatalf("GetSession() error: %v", err)
	}
	if err := adapter.ReserveSessionName("concurrent-creator", "target-name"); err != nil {
		t.Fatalf("ReserveSessionName(concurrent creator) error: %v", err)
	}

	_, err = adapter.RenameSessionIdentity(
		t.Context(),
		observed.SessionID,
		observed.Name,
		observed.Tmux.SessionName,
		observed.Tmux.SessionRevision,
		"target-name",
	)
	var conflict *SessionNameConflictError
	if !errors.As(err, &conflict) || conflict.Name != "target-name" {
		t.Fatalf("RenameSessionIdentity() error = %v, want target-name conflict", err)
	}
	unchanged, err := adapter.GetSession(m.SessionID)
	if err != nil {
		t.Fatalf("GetSession() after conflict error: %v", err)
	}
	if unchanged.Name != "old-name" || unchanged.Tmux.SessionName != "old-name" {
		t.Fatalf("identity after reservation conflict = (%q, %q), want unchanged", unchanged.Name, unchanged.Tmux.SessionName)
	}

	if err := adapter.ReleaseSessionNameReservation("concurrent-creator"); err != nil {
		t.Fatalf("ReleaseSessionNameReservation() error: %v", err)
	}
	if err := adapter.ReserveSessionName(unchanged.SessionID, "target-name"); err != nil {
		t.Fatalf("ReserveSessionName(rename owner) error: %v", err)
	}
	if _, err := adapter.RenameSessionIdentity(
		t.Context(),
		unchanged.SessionID,
		unchanged.Name,
		unchanged.Tmux.SessionName,
		unchanged.Tmux.SessionRevision,
		"target-name",
	); err != nil {
		t.Fatalf("RenameSessionIdentity() after release error: %v", err)
	}
	var reservations int
	if err := adapter.conn.QueryRow(
		`SELECT COUNT(*) FROM agm_session_name_reservations WHERE session_id = ?`,
		unchanged.SessionID,
	).Scan(&reservations); err != nil {
		t.Fatalf("count caller-owned rename reservations: %v", err)
	}
	if reservations != 1 {
		t.Fatalf("caller-owned rename reservations = %d, want 1", reservations)
	}
	if err := adapter.ReleaseSessionNameReservation(unchanged.SessionID); err != nil {
		t.Fatalf("ReleaseSessionNameReservation(rename owner) error: %v", err)
	}

	renamed, err := adapter.GetSession(unchanged.SessionID)
	if err != nil {
		t.Fatalf("GetSession() after caller-owned rename error: %v", err)
	}
	if _, err := adapter.RenameSessionIdentity(
		t.Context(),
		renamed.SessionID,
		renamed.Name,
		renamed.Tmux.SessionName,
		renamed.Tmux.SessionRevision,
		"final-name",
	); err != nil {
		t.Fatalf("RenameSessionIdentity() with self-owned lease error: %v", err)
	}
	if err := adapter.conn.QueryRow(
		`SELECT COUNT(*) FROM agm_session_name_reservations WHERE session_id = ?`,
		renamed.SessionID,
	).Scan(&reservations); err != nil {
		t.Fatalf("count self-owned rename reservations: %v", err)
	}
	if reservations != 0 {
		t.Fatalf("self-owned rename reservations = %d, want 0", reservations)
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
		name          string
		currentName   string
		currentTmux   string
		currentRev    string
		wantSuccess   bool
		wantRollback  bool
		wantUncertain bool
	}{
		{name: "exact generated revision committed", currentName: "new-name", currentTmux: "new-name", currentRev: "next-revision", wantSuccess: true},
		{name: "later writer superseded committed revision", currentName: "new-name", currentTmux: "new-name", currentRev: "later-revision", wantSuccess: true},
		{name: "exact fence revision proves rollback safe", currentName: "old-name", currentTmux: "old-tmux", currentRev: "fence-revision", wantRollback: true},
		{name: "later revision also fences pending write", currentName: "old-name", currentTmux: "old-tmux", currentRev: "later-unrelated-revision", wantRollback: true},
		{name: "unchanged observed revision remains ambiguous", currentName: "old-name", currentTmux: "old-tmux", currentRev: "observed-revision", wantUncertain: true},
		{name: "different identity makes rollback unsafe", currentName: "other-name", currentTmux: "other-tmux", currentRev: "other-revision"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := classifySessionIdentityRenameAfterError("old-name", "old-tmux", "observed-revision", "new-name", "next-revision", "fence-revision", tt.currentName, tt.currentTmux, tt.currentRev, primary)
			if (err == nil) != tt.wantSuccess || result.TmuxRollbackSafe != tt.wantRollback {
				t.Fatalf("classification = (result=%+v err=%v), want success=%v rollback=%v", result, err, tt.wantSuccess, tt.wantRollback)
			}
			var uncertain *SessionIdentityMutationCommitUncertainError
			if errors.As(err, &uncertain) != tt.wantUncertain {
				t.Fatalf("classification error = %v, uncertain=%v, want %v", err, uncertain != nil, tt.wantUncertain)
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
