package dolt

import (
	"errors"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/vbonnet/dear-agent/agm/internal/manifest"
)

func TestSQLiteCreateSessionAtomicallyRejectsDuplicateNonArchivedName(t *testing.T) {
	adapter, err := NewSQLiteAdapter(filepath.Join(t.TempDir(), "agm.db"))
	if err != nil {
		t.Fatalf("NewSQLiteAdapter() error: %v", err)
	}
	t.Cleanup(func() { _ = adapter.Close() })

	start := make(chan struct{})
	results := make(chan error, 2)
	var ready sync.WaitGroup
	ready.Add(2)
	for _, id := range []string{"concurrent-a", "concurrent-b"} {
		go func(sessionID string) {
			ready.Done()
			<-start
			now := time.Now()
			results <- adapter.CreateSession(&manifest.Manifest{
				SessionID: sessionID,
				Name:      "shared-name",
				CreatedAt: now,
				UpdatedAt: now,
			})
		}(id)
	}
	ready.Wait()
	close(start)

	var successes, conflicts int
	for range 2 {
		createErr := <-results
		if createErr == nil {
			successes++
			continue
		}
		var conflict *SessionNameConflictError
		if errors.As(createErr, &conflict) && conflict.Name == "shared-name" {
			conflicts++
			continue
		}
		t.Fatalf("concurrent CreateSession() error = %v", createErr)
	}
	if successes != 1 || conflicts != 1 {
		t.Fatalf("concurrent creates = %d success, %d conflict; want 1 and 1", successes, conflicts)
	}

	sessions, err := adapter.ListSessions(&SessionFilter{ExcludeArchived: true})
	if err != nil {
		t.Fatalf("ListSessions() error: %v", err)
	}
	if len(sessions) != 1 {
		t.Fatalf("non-archived sessions = %d, want 1", len(sessions))
	}
	sessions[0].Lifecycle = manifest.LifecycleArchived
	if err := adapter.UpdateSession(sessions[0]); err != nil {
		t.Fatalf("archive winning session: %v", err)
	}
	now := time.Now()
	if err := adapter.CreateSession(&manifest.Manifest{
		SessionID: "replacement",
		Name:      "shared-name",
		CreatedAt: now,
		UpdatedAt: now,
	}); err != nil {
		t.Fatalf("reuse archived session name: %v", err)
	}
}

func TestSQLiteNameReservationsPreserveLegacyDuplicates(t *testing.T) {
	adapter, err := NewSQLiteAdapter(filepath.Join(t.TempDir(), "agm.db"))
	if err != nil {
		t.Fatalf("NewSQLiteAdapter() error: %v", err)
	}
	t.Cleanup(func() { _ = adapter.Close() })

	for _, id := range []string{"legacy-a", "legacy-b"} {
		if _, err := adapter.Conn().Exec(
			`INSERT INTO agm_sessions (id, status, workspace, name, is_test)
			 VALUES (?, 'active', 'test', 'legacy-duplicate', TRUE)`,
			id,
		); err != nil {
			t.Fatalf("seed legacy duplicate %q: %v", id, err)
		}
	}

	now := time.Now()
	if err := adapter.CreateSession(&manifest.Manifest{
		SessionID: "unrelated",
		Name:      "unrelated-name",
		CreatedAt: now,
		UpdatedAt: now,
	}); err != nil {
		t.Fatalf("unrelated registration with legacy duplicates: %v", err)
	}
	if err := adapter.CreateSession(&manifest.Manifest{
		SessionID: "third-duplicate",
		Name:      "legacy-duplicate",
		CreatedAt: now,
		UpdatedAt: now,
	}); err == nil {
		t.Fatal("CreateSession() accepted a new legacy-duplicate row")
	} else {
		if _, ok := errors.AsType[*SessionNameConflictError](err); !ok {
			t.Fatalf("CreateSession() duplicate error = %v, want SessionNameConflictError", err)
		}
	}

	var duplicateCount int
	if err := adapter.Conn().QueryRow(
		`SELECT COUNT(*) FROM agm_sessions
		 WHERE workspace = 'test' AND name = 'legacy-duplicate'`,
	).Scan(&duplicateCount); err != nil {
		t.Fatalf("count preserved legacy duplicates: %v", err)
	}
	if duplicateCount != 2 {
		t.Fatalf("legacy duplicate rows = %d, want 2 preserved", duplicateCount)
	}
}

func TestSQLiteSessionNameReservationPrecedesAndIsConsumedByRegistration(t *testing.T) {
	adapter, err := NewSQLiteAdapter(filepath.Join(t.TempDir(), "agm.db"))
	if err != nil {
		t.Fatalf("NewSQLiteAdapter() error: %v", err)
	}
	t.Cleanup(func() { _ = adapter.Close() })

	if err := adapter.ReserveSessionName("winner", "reserved-name"); err != nil {
		t.Fatalf("ReserveSessionName(winner) error: %v", err)
	}
	err = adapter.ReserveSessionName("loser", "reserved-name")
	var conflict *SessionNameConflictError
	if !errors.As(err, &conflict) {
		t.Fatalf("ReserveSessionName(loser) error = %v, want SessionNameConflictError", err)
	}

	now := time.Now()
	if err := adapter.CreateSession(&manifest.Manifest{
		SessionID: "winner",
		Name:      "reserved-name",
		CreatedAt: now,
		UpdatedAt: now,
	}); err != nil {
		t.Fatalf("CreateSession(winner) error: %v", err)
	}
	var reservations int
	if err := adapter.conn.QueryRow(
		`SELECT COUNT(*) FROM agm_session_name_reservations WHERE session_id = ?`,
		"winner",
	).Scan(&reservations); err != nil {
		t.Fatalf("count consumed reservations: %v", err)
	}
	if reservations != 0 {
		t.Fatalf("winner reservations after registration = %d, want 0", reservations)
	}
	if err := adapter.ReserveSessionName("loser", "reserved-name"); !errors.As(err, &conflict) {
		t.Fatalf("ReserveSessionName(loser after registration) error = %v, want conflict", err)
	}
	committed, err := adapter.sessionRegistrationCommitted(
		&manifest.Manifest{
			SessionID: "winner",
			Name:      "reserved-name",
			Tmux:      manifest.Tmux{SessionName: ""},
		},
		"active",
		"claude-code",
	)
	if err != nil || !committed {
		t.Fatalf("sessionRegistrationCommitted() = (%t, %v), want (true, nil)", committed, err)
	}
	_, err = adapter.sessionRegistrationCommitted(
		&manifest.Manifest{SessionID: "winner", Name: "different-name"},
		"active",
		"claude-code",
	)
	if err == nil {
		t.Fatal("sessionRegistrationCommitted() accepted a mismatched durable identity")
	}
}

func TestSQLiteSessionNameReservationCanBeRenewedByOwner(t *testing.T) {
	adapter, err := NewSQLiteAdapter(filepath.Join(t.TempDir(), "agm.db"))
	if err != nil {
		t.Fatalf("NewSQLiteAdapter() error: %v", err)
	}
	t.Cleanup(func() { _ = adapter.Close() })

	if err := adapter.ReserveSessionName("owner", "renewable-name"); err != nil {
		t.Fatalf("ReserveSessionName() error: %v", err)
	}
	if err := adapter.RenewSessionNameReservation("owner", "renewable-name"); err != nil {
		t.Fatalf("RenewSessionNameReservation(owner) error: %v", err)
	}
	var conflict *SessionNameConflictError
	if err := adapter.RenewSessionNameReservation("other", "renewable-name"); !errors.As(err, &conflict) {
		t.Fatalf("RenewSessionNameReservation(other) error = %v, want SessionNameConflictError", err)
	}
}

// TestSQLiteReservationOwnedAndUnexpiredBacksZeroRowRenewFallback covers the
// authoritative primary-key ownership read that RenewSessionNameReservation
// falls back to when its UPDATE reports zero rows affected. On Dolt that
// zero-row result can be spurious for a just-reserved name (a real bug that
// aborted every session creation with AGM-007), and it affects UPDATEs
// regardless of predicate — so the consistent primary-key SELECT, not a
// re-UPDATE, is the arbiter. It must recognise a still-valid, still-owned lease
// while still rejecting foreign, expired, and missing reservations.
func TestSQLiteReservationOwnedAndUnexpiredBacksZeroRowRenewFallback(t *testing.T) {
	adapter, err := NewSQLiteAdapter(filepath.Join(t.TempDir(), "agm.db"))
	if err != nil {
		t.Fatalf("NewSQLiteAdapter() error: %v", err)
	}
	t.Cleanup(func() { _ = adapter.Close() })

	if err := adapter.ReserveSessionName("owner", "leased-name"); err != nil {
		t.Fatalf("ReserveSessionName() error: %v", err)
	}
	now := time.Now()

	owned, err := adapter.reservationOwnedAndUnexpired("owner", "leased-name", now)
	if err != nil {
		t.Fatalf("reservationOwnedAndUnexpired(owner) error: %v", err)
	}
	if !owned {
		t.Fatal("owner must be reported as holding an unexpired reservation")
	}

	owned, err = adapter.reservationOwnedAndUnexpired("other", "leased-name", now)
	if err != nil {
		t.Fatalf("reservationOwnedAndUnexpired(other) error: %v", err)
	}
	if owned {
		t.Fatal("a non-owner must not be reported as holding the reservation")
	}

	owned, err = adapter.reservationOwnedAndUnexpired("owner", "leased-name", now.Add(sessionNameReservationTTL+time.Minute))
	if err != nil {
		t.Fatalf("reservationOwnedAndUnexpired(expired) error: %v", err)
	}
	if owned {
		t.Fatal("an expired reservation must not be reported as owned")
	}

	owned, err = adapter.reservationOwnedAndUnexpired("owner", "no-such-name", now)
	if err != nil {
		t.Fatalf("reservationOwnedAndUnexpired(missing) error: %v", err)
	}
	if owned {
		t.Fatal("a missing reservation must not be reported as owned")
	}

	// The full renew path returns success for the owner (its UPDATE applies
	// normally on SQLite) and a conflict for a non-owner.
	if err := adapter.RenewSessionNameReservation("owner", "leased-name"); err != nil {
		t.Fatalf("RenewSessionNameReservation(owner) error: %v", err)
	}
	var conflict *SessionNameConflictError
	if err := adapter.RenewSessionNameReservation("other", "leased-name"); !errors.As(err, &conflict) {
		t.Fatalf("RenewSessionNameReservation(other) error = %v, want SessionNameConflictError", err)
	}
}

func TestSQLiteUpdateSessionCannotReactivateArchivedRowFromStaleManifest(t *testing.T) {
	adapter, err := NewSQLiteAdapter(filepath.Join(t.TempDir(), "agm.db"))
	if err != nil {
		t.Fatalf("NewSQLiteAdapter() error: %v", err)
	}
	t.Cleanup(func() { _ = adapter.Close() })

	now := time.Now()
	original := &manifest.Manifest{
		SessionID: "stale-archived-session",
		Name:      "reused-after-archive",
		CreatedAt: now,
		UpdatedAt: now,
	}
	if err := adapter.CreateSession(original); err != nil {
		t.Fatalf("CreateSession() error: %v", err)
	}
	stale, err := adapter.GetSession(original.SessionID)
	if err != nil {
		t.Fatalf("GetSession(stale) error: %v", err)
	}
	current, err := adapter.GetSession(original.SessionID)
	if err != nil {
		t.Fatalf("GetSession(current) error: %v", err)
	}
	current.Lifecycle = manifest.LifecycleArchived
	if err := adapter.UpdateSession(current); err != nil {
		t.Fatalf("archive current session: %v", err)
	}
	if err := adapter.ReserveSessionName("replacement-owner", original.Name); err != nil {
		t.Fatalf("ReserveSessionName(replacement-owner) error: %v", err)
	}
	t.Cleanup(func() { _ = adapter.ReleaseSessionNameReservation("replacement-owner") })

	stale.Context.Notes = "stale writer"
	if err := adapter.UpdateSession(stale); err != nil {
		t.Fatalf("UpdateSession(stale) error: %v", err)
	}
	stored, err := adapter.GetSession(original.SessionID)
	if err != nil {
		t.Fatalf("GetSession(stored) error: %v", err)
	}
	if stored.Lifecycle != manifest.LifecycleArchived {
		t.Fatalf("stale UpdateSession lifecycle = %q, want archived", stored.Lifecycle)
	}
}

func TestSQLiteCreateSessionCleansOnlyItsOwnReservationAfterRegistrationFailure(t *testing.T) {
	adapter, err := NewSQLiteAdapter(filepath.Join(t.TempDir(), "agm.db"))
	if err != nil {
		t.Fatalf("NewSQLiteAdapter() error: %v", err)
	}
	t.Cleanup(func() { _ = adapter.Close() })

	now := time.Now()
	if err := adapter.CreateSession(&manifest.Manifest{
		SessionID: "duplicate-id",
		Name:      "existing-name",
		CreatedAt: now,
		UpdatedAt: now,
	}); err != nil {
		t.Fatalf("CreateSession(existing) error: %v", err)
	}

	failedRegistration := &manifest.Manifest{
		SessionID: "duplicate-id",
		Name:      "internally-reserved-name",
		CreatedAt: now,
		UpdatedAt: now,
	}
	if err := adapter.CreateSession(failedRegistration); err == nil {
		t.Fatal("CreateSession(duplicate ID) succeeded")
	}
	var reservations int
	if err := adapter.conn.QueryRow(
		`SELECT COUNT(*) FROM agm_session_name_reservations
		 WHERE workspace = ? AND name = ? AND session_id = ?`,
		adapter.workspace,
		failedRegistration.Name,
		failedRegistration.SessionID,
	).Scan(&reservations); err != nil {
		t.Fatalf("count internally created reservation: %v", err)
	}
	if reservations != 0 {
		t.Fatalf("internally created reservations after registration failure = %d, want 0", reservations)
	}

	const callerReservedName = "caller-reserved-name"
	if err := adapter.ReserveSessionName(failedRegistration.SessionID, callerReservedName); err != nil {
		t.Fatalf("ReserveSessionName(caller) error: %v", err)
	}
	failedRegistration.Name = callerReservedName
	if err := adapter.CreateSession(failedRegistration); err == nil {
		t.Fatal("CreateSession(duplicate ID with caller reservation) succeeded")
	}
	if err := adapter.conn.QueryRow(
		`SELECT COUNT(*) FROM agm_session_name_reservations
		 WHERE workspace = ? AND name = ? AND session_id = ?`,
		adapter.workspace,
		callerReservedName,
		failedRegistration.SessionID,
	).Scan(&reservations); err != nil {
		t.Fatalf("count caller-owned reservation: %v", err)
	}
	if reservations != 1 {
		t.Fatalf("caller-owned reservations after registration failure = %d, want 1", reservations)
	}
}

func TestResolveSessionNameReservationCommitErrorReconcilesOwnership(t *testing.T) {
	commitErr := errors.New("reservation commit acknowledgement lost")
	inspectErr := errors.New("reservation re-read unavailable")
	tests := []struct {
		name               string
		reservationCreated bool
		owned              bool
		inspectErr         error
		wantOwned          bool
		wantErr            bool
		wantUncertain      bool
	}{
		{
			name:               "new reservation committed",
			reservationCreated: true,
			owned:              true,
			wantOwned:          true,
		},
		{
			name:               "preexisting owned reservation remains owned",
			reservationCreated: false,
			owned:              true,
		},
		{
			name:               "missing reservation proves no commit",
			reservationCreated: true,
			wantErr:            true,
		},
		{
			name:               "failed inspection requires compensation",
			reservationCreated: true,
			inspectErr:         inspectErr,
			wantOwned:          true,
			wantErr:            true,
			wantUncertain:      true,
		},
		{
			name:               "failed inspection preserves caller-owned reservation",
			reservationCreated: false,
			inspectErr:         inspectErr,
			wantErr:            true,
			wantUncertain:      true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			owned, err := resolveSessionNameReservationCommitError(
				tt.reservationCreated,
				commitErr,
				tt.owned,
				tt.inspectErr,
			)
			if owned != tt.wantOwned {
				t.Fatalf("owned = %v, want %v", owned, tt.wantOwned)
			}
			if (err != nil) != tt.wantErr {
				t.Fatalf("error = %v, want error=%v", err, tt.wantErr)
			}
			if err != nil && !errors.Is(err, commitErr) {
				t.Fatalf("error = %v, want commit error", err)
			}
			var uncertain *SessionNameReservationCommitUncertainError
			if errors.As(err, &uncertain) != tt.wantUncertain {
				t.Fatalf("uncertain error = %v, want %v", err, tt.wantUncertain)
			}
			if tt.inspectErr != nil && !errors.Is(err, tt.inspectErr) {
				t.Fatalf("error = %v, want inspect error", err)
			}
		})
	}
}

func TestSQLiteExpiredReservationCannotAuthorizeRename(t *testing.T) {
	adapter, err := NewSQLiteAdapter(filepath.Join(t.TempDir(), "agm.db"))
	if err != nil {
		t.Fatalf("NewSQLiteAdapter() error: %v", err)
	}
	t.Cleanup(func() { _ = adapter.Close() })

	session := &manifest.Manifest{
		SessionID: "expired-rename-owner",
		Name:      "old-name",
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
		Tmux:      manifest.Tmux{SessionName: "old-name"},
	}
	if err := adapter.CreateSession(session); err != nil {
		t.Fatalf("CreateSession() error: %v", err)
	}
	observed, err := adapter.GetSession(session.SessionID)
	if err != nil {
		t.Fatalf("GetSession() error: %v", err)
	}
	if err := adapter.ReserveSessionName(session.SessionID, "target-name"); err != nil {
		t.Fatalf("ReserveSessionName() error: %v", err)
	}
	replaceExpiredSessionNameReservation(t, adapter, session.SessionID, "replacement-owner", "target-name")

	_, err = adapter.renameSessionIdentityReserved(
		t.Context(),
		observed.SessionID,
		observed.Name,
		observed.Tmux.SessionName,
		observed.Tmux.SessionRevision,
		"target-name",
		"target-name",
	)
	var conflict *SessionNameConflictError
	if !errors.As(err, &conflict) || conflict.Name != "target-name" {
		t.Fatalf("renameSessionIdentityReserved() error = %v, want target-name conflict", err)
	}
	current, err := adapter.GetSession(session.SessionID)
	if err != nil {
		t.Fatalf("GetSession() after rejected rename: %v", err)
	}
	if current.Name != "old-name" || current.Tmux.SessionName != "old-name" {
		t.Fatalf("identity after rejected rename = (%q, %q), want old-name", current.Name, current.Tmux.SessionName)
	}
}

func TestSQLiteExpiredReservationCannotAuthorizeReactivation(t *testing.T) {
	adapter, err := NewSQLiteAdapter(filepath.Join(t.TempDir(), "agm.db"))
	if err != nil {
		t.Fatalf("NewSQLiteAdapter() error: %v", err)
	}
	t.Cleanup(func() { _ = adapter.Close() })

	session := &manifest.Manifest{
		SessionID: "expired-reactivation-owner",
		Name:      "restored-name",
		Lifecycle: manifest.LifecycleArchived,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
		Tmux:      manifest.Tmux{SessionName: "archived-tmux"},
	}
	if err := adapter.CreateSession(session); err != nil {
		t.Fatalf("CreateSession() error: %v", err)
	}
	observed, err := adapter.GetSession(session.SessionID)
	if err != nil {
		t.Fatalf("GetSession() error: %v", err)
	}
	if err := adapter.ReserveSessionName(session.SessionID, session.Name); err != nil {
		t.Fatalf("ReserveSessionName() error: %v", err)
	}
	replaceExpiredSessionNameReservation(t, adapter, session.SessionID, "replacement-owner", session.Name)

	result, err := adapter.reactivateSessionReserved(observed, session.Name)
	var conflict *SessionNameConflictError
	if !errors.As(err, &conflict) || conflict.Name != session.Name || result.StorageCommitted {
		t.Fatalf("reactivateSessionReserved() = (%+v, %v), want uncommitted conflict", result, err)
	}
	current, err := adapter.GetSession(session.SessionID)
	if err != nil {
		t.Fatalf("GetSession() after rejected reactivation: %v", err)
	}
	if current.Lifecycle != manifest.LifecycleArchived {
		t.Fatalf("lifecycle after rejected reactivation = %q, want archived", current.Lifecycle)
	}
}

func TestSQLiteExpiredReservationCannotAuthorizeParentLink(t *testing.T) {
	adapter, err := NewSQLiteAdapter(filepath.Join(t.TempDir(), "agm.db"))
	if err != nil {
		t.Fatalf("NewSQLiteAdapter() error: %v", err)
	}
	t.Cleanup(func() { _ = adapter.Close() })

	now := time.Now()
	parent := &manifest.Manifest{
		SessionID: "expired-link-parent",
		Name:      "planning",
		CreatedAt: now,
		UpdatedAt: now,
	}
	child := &manifest.Manifest{
		SessionID: "expired-link-owner",
		Name:      "old-child-name",
		CreatedAt: now,
		UpdatedAt: now,
		Tmux:      manifest.Tmux{SessionName: "child-tmux"},
	}
	for _, session := range []*manifest.Manifest{parent, child} {
		if err := adapter.CreateSession(session); err != nil {
			t.Fatalf("CreateSession(%s) error: %v", session.SessionID, err)
		}
	}
	observed, err := adapter.GetSession(child.SessionID)
	if err != nil {
		t.Fatalf("GetSession() error: %v", err)
	}
	inheritedName := "planning-exec"
	created, reservationName, err := adapter.reserveParentLinkName(t.Context(), child.SessionID, &inheritedName)
	if err != nil || !created || reservationName != inheritedName {
		t.Fatalf("reserveParentLinkName() = (%t, %q, %v), want created %q", created, reservationName, err, inheritedName)
	}
	replaceExpiredSessionNameReservation(t, adapter, child.SessionID, "replacement-owner", inheritedName)

	err = adapter.linkSessionParentReserved(
		t.Context(),
		child.SessionID,
		observed.Tmux.SessionRevision,
		parent.SessionID,
		&inheritedName,
		reservationName,
	)
	var conflict *SessionNameConflictError
	if !errors.As(err, &conflict) || conflict.Name != inheritedName {
		t.Fatalf("linkSessionParentReserved() error = %v, want conflict for %q", err, inheritedName)
	}
	current, err := adapter.GetSession(child.SessionID)
	if err != nil {
		t.Fatalf("GetSession() after rejected parent link: %v", err)
	}
	if current.ParentSessionID != nil || current.Name != child.Name {
		t.Fatalf("child after rejected parent link = parent %#v name %q, want nil/%q", current.ParentSessionID, current.Name, child.Name)
	}
}

func replaceExpiredSessionNameReservation(t *testing.T, adapter *Adapter, owner, replacement, name string) {
	t.Helper()
	result, err := adapter.Conn().Exec(
		`UPDATE agm_session_name_reservations
		 SET expires_at = ?
		 WHERE workspace = ? AND name = ? AND session_id = ?`,
		time.Now().Add(-time.Minute),
		adapter.Workspace(),
		name,
		owner,
	)
	if err != nil {
		t.Fatalf("expire reservation %q: %v", name, err)
	}
	rows, err := result.RowsAffected()
	if err != nil || rows != 1 {
		t.Fatalf("expired reservation rows = %d, %v; want 1", rows, err)
	}
	if err := adapter.ReserveSessionName(replacement, name); err != nil {
		t.Fatalf("ReserveSessionName(%s replacement) error: %v", replacement, err)
	}
}

func TestSQLiteSessionNameReservationsPreserveLegacyDuplicateRows(t *testing.T) {
	adapter, err := NewSQLiteAdapter(filepath.Join(t.TempDir(), "agm.db"))
	if err != nil {
		t.Fatalf("NewSQLiteAdapter() error: %v", err)
	}
	t.Cleanup(func() { _ = adapter.Close() })

	now := time.Now()
	for _, id := range []string{"legacy-a", "legacy-b"} {
		if _, err := adapter.conn.Exec(
			`INSERT INTO agm_sessions
			 (id, created_at, updated_at, status, workspace, name)
			 VALUES (?, ?, ?, 'active', 'test', 'legacy-duplicate')`,
			id,
			now,
			now,
		); err != nil {
			t.Fatalf("insert legacy duplicate %s: %v", id, err)
		}
	}

	err = adapter.ReserveSessionName("new-session", "legacy-duplicate")
	if _, ok := errors.AsType[*SessionNameConflictError](err); !ok {
		t.Fatalf("ReserveSessionName() error = %v, want SessionNameConflictError", err)
	}
	var activeRows, reservations int
	if err := adapter.conn.QueryRow(
		`SELECT COUNT(*) FROM agm_sessions
		 WHERE workspace = 'test' AND name = 'legacy-duplicate' AND status != 'archived'`,
	).Scan(&activeRows); err != nil {
		t.Fatalf("count legacy duplicates: %v", err)
	}
	if err := adapter.conn.QueryRow(
		`SELECT COUNT(*) FROM agm_session_name_reservations WHERE name = 'legacy-duplicate'`,
	).Scan(&reservations); err != nil {
		t.Fatalf("count rolled-back reservations: %v", err)
	}
	if activeRows != 2 || reservations != 0 {
		t.Fatalf("legacy duplicate state = active:%d reservations:%d, want 2/0", activeRows, reservations)
	}
}

func TestSQLiteReactivateSessionRejectsReusedActiveName(t *testing.T) {
	adapter, err := NewSQLiteAdapter(filepath.Join(t.TempDir(), "agm.db"))
	if err != nil {
		t.Fatalf("NewSQLiteAdapter() error: %v", err)
	}
	t.Cleanup(func() { _ = adapter.Close() })

	now := time.Now()
	archived := &manifest.Manifest{
		SessionID: "archived-original",
		Name:      "reused-name",
		Lifecycle: manifest.LifecycleArchived,
		CreatedAt: now,
		UpdatedAt: now,
	}
	if err := adapter.CreateSession(archived); err != nil {
		t.Fatalf("CreateSession(archived) error: %v", err)
	}
	replacement := &manifest.Manifest{
		SessionID: "active-replacement",
		Name:      "reused-name",
		CreatedAt: now,
		UpdatedAt: now,
	}
	if err := adapter.CreateSession(replacement); err != nil {
		t.Fatalf("CreateSession(replacement) error: %v", err)
	}

	_, err = adapter.ReactivateSession(archived)
	var conflict *SessionNameConflictError
	if !errors.As(err, &conflict) || conflict.Name != archived.Name {
		t.Fatalf("ReactivateSession() error = %v, want conflict for %q", err, archived.Name)
	}
	if archived.Lifecycle != manifest.LifecycleArchived {
		t.Fatalf("rejected manifest lifecycle = %q, want archived", archived.Lifecycle)
	}
	stored, err := adapter.GetSession(archived.SessionID)
	if err != nil {
		t.Fatalf("GetSession(archived) error: %v", err)
	}
	if stored.Lifecycle != manifest.LifecycleArchived {
		t.Fatalf("rejected stored lifecycle = %q, want archived", stored.Lifecycle)
	}

	replacement.Lifecycle = manifest.LifecycleArchived
	if err := adapter.UpdateSession(replacement); err != nil {
		t.Fatalf("archive replacement: %v", err)
	}
	result, err := adapter.ReactivateSession(archived)
	if err != nil {
		t.Fatalf("ReactivateSession(after archive) error: %v", err)
	}
	if !result.StorageCommitted {
		t.Fatal("ReactivateSession(after archive) did not report committed storage")
	}
	stored, err = adapter.GetSession(archived.SessionID)
	if err != nil {
		t.Fatalf("GetSession(reactivated) error: %v", err)
	}
	if stored.Lifecycle == manifest.LifecycleArchived {
		t.Fatal("session remained archived after successful reactivation")
	}
	var reservations int
	if err := adapter.conn.QueryRow(
		`SELECT COUNT(*) FROM agm_session_name_reservations WHERE session_id = ?`,
		archived.SessionID,
	).Scan(&reservations); err != nil {
		t.Fatalf("count reactivation reservations: %v", err)
	}
	if reservations != 0 {
		t.Fatalf("reactivation reservations = %d, want 0", reservations)
	}
}
