package dolt

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/vbonnet/dear-agent/agm/internal/manifest"
)

const sessionNameReservationTTL = 2 * time.Hour

// SessionNameReservationCommitUncertainError means a reservation commit may
// have succeeded but a durable re-read could not prove its exact owner.
// Callers must compensate as though the session may own the lease.
type SessionNameReservationCommitUncertainError struct {
	Err error
}

func (e *SessionNameReservationCommitUncertainError) Error() string {
	return e.Err.Error()
}

func (e *SessionNameReservationCommitUncertainError) Unwrap() error {
	return e.Err
}

// SessionIdentityMutationCommitUncertainError means an identity write may
// still commit and its name reservation must remain leased until expiry.
type SessionIdentityMutationCommitUncertainError struct {
	Err error
}

func (e *SessionIdentityMutationCommitUncertainError) Error() string {
	return e.Err.Error()
}

func (e *SessionIdentityMutationCommitUncertainError) Unwrap() error {
	return e.Err
}

// ReserveSessionName atomically leases a workspace-scoped active session name.
// Existing duplicate session rows remain readable and actionable; only new
// creation attempts for an already active name are rejected.
func (a *Adapter) ReserveSessionName(sessionID, name string) error {
	_, err := a.reserveSessionName(sessionID, name)
	return err
}

// reserveSessionName reports whether this call created, or may have created,
// the lease. A caller that already owns the same workspace/name/session tuple
// keeps ownership, so a nested durable mutation must not release the caller's
// longer-lived lease. A true result paired with an error requires cleanup.
func (a *Adapter) reserveSessionName(sessionID, name string) (bool, error) {
	if sessionID == "" {
		return false, fmt.Errorf("session_id cannot be empty")
	}
	if name == "" {
		return false, nil
	}
	if err := a.ApplyMigrations(); err != nil {
		return false, fmt.Errorf("failed to apply migrations: %w", err)
	}

	tx, err := a.conn.Begin() //nolint:noctx // TODO(context): plumb ctx through this layer
	if err != nil {
		return false, fmt.Errorf("begin session-name reservation: %w", err)
	}
	defer tx.Rollback()

	now := time.Now()
	if _, err := tx.Exec( //nolint:noctx // TODO(context): plumb ctx through this layer
		`DELETE FROM agm_session_name_reservations WHERE expires_at <= ?`,
		now,
	); err != nil {
		return false, fmt.Errorf("reclaim expired session-name reservations: %w", err)
	}
	reservationCreated := true
	if _, err := tx.Exec( //nolint:noctx // TODO(context): plumb ctx through this layer
		`INSERT INTO agm_session_name_reservations
			(workspace, name, session_id, created_at, expires_at)
		 VALUES (?, ?, ?, ?, ?)`,
		a.workspace,
		name,
		sessionID,
		now,
		now.Add(sessionNameReservationTTL),
	); err != nil {
		if !isUniqueConstraintError(err) {
			return false, fmt.Errorf("reserve session name: %w", err)
		}
		var owner string
		lookupErr := tx.QueryRow( //nolint:noctx // TODO(context): plumb ctx through this layer
			`SELECT session_id FROM agm_session_name_reservations
			 WHERE workspace = ? AND name = ?`,
			a.workspace,
			name,
		).Scan(&owner)
		if lookupErr != nil || owner != sessionID {
			return false, &SessionNameConflictError{Name: name}
		}
		reservationCreated = false
	}

	var existingID string
	err = tx.QueryRow( //nolint:noctx // TODO(context): plumb ctx through this layer
		`SELECT id FROM agm_sessions
		 WHERE workspace = ? AND name = ? AND status != 'archived'
		 LIMIT 1`,
		a.workspace,
		name,
	).Scan(&existingID)
	switch {
	case err == nil:
		return false, &SessionNameConflictError{Name: name}
	case !errors.Is(err, sql.ErrNoRows):
		return false, fmt.Errorf("inspect active session name: %w", err)
	}

	if err := tx.Commit(); err != nil {
		owned, inspectErr := a.sessionNameReservationOwned(sessionID, name)
		return resolveSessionNameReservationCommitError(reservationCreated, err, owned, inspectErr)
	}
	return reservationCreated, nil
}

func (a *Adapter) sessionNameReservationOwned(sessionID, name string) (bool, error) {
	var owner string
	err := a.conn.QueryRow( //nolint:noctx // commit reconciliation must outlive the transaction response
		`SELECT session_id FROM agm_session_name_reservations
		 WHERE workspace = ? AND name = ?`,
		a.workspace,
		name,
	).Scan(&owner)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("re-read session-name reservation after commit error: %w", err)
	}
	return owner == sessionID, nil
}

func resolveSessionNameReservationCommitError(reservationCreated bool, commitErr error, owned bool, inspectErr error) (bool, error) {
	err := fmt.Errorf("commit session-name reservation: %w", commitErr)
	if inspectErr != nil {
		return reservationCreated, &SessionNameReservationCommitUncertainError{
			Err: errors.Join(err, inspectErr),
		}
	}
	if owned {
		return reservationCreated, nil
	}
	return false, err
}

// ReleaseSessionNameReservation releases an operation lease owned by the
// supplied session ID. Missing reservations are already released.
func (a *Adapter) ReleaseSessionNameReservation(sessionID string) error {
	if sessionID == "" {
		return nil
	}
	if err := a.ApplyMigrations(); err != nil {
		return fmt.Errorf("failed to apply migrations: %w", err)
	}
	if _, err := a.conn.Exec( //nolint:noctx // TODO(context): plumb ctx through this layer
		`DELETE FROM agm_session_name_reservations
		 WHERE workspace = ? AND session_id = ?`,
		a.workspace,
		sessionID,
	); err != nil {
		return fmt.Errorf("release session-name reservation: %w", err)
	}
	return nil
}

// RenewSessionNameReservation verifies ownership and extends the lease before
// a long preparation interval can hand the name to another creator.
func (a *Adapter) RenewSessionNameReservation(sessionID, name string) error {
	if sessionID == "" || name == "" {
		return nil
	}
	if err := a.ApplyMigrations(); err != nil {
		return fmt.Errorf("failed to apply migrations: %w", err)
	}
	now := time.Now()
	result, err := a.conn.Exec( //nolint:noctx // TODO(context): plumb ctx through this layer
		`UPDATE agm_session_name_reservations
		 SET expires_at = ?
		 WHERE workspace = ? AND name = ? AND session_id = ? AND expires_at > ?`,
		now.Add(sessionNameReservationTTL), a.workspace, name, sessionID, now,
	)
	if err != nil {
		return fmt.Errorf("renew session-name reservation: %w", err)
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("get renewed session-name reservation rows affected: %w", err)
	}
	if rowsAffected == 0 {
		// Dolt (observed on 2.2.x) can report zero rows affected for an UPDATE
		// that targets a row INSERTed by a recent, separate transaction, even
		// though a primary-key SELECT of the same row still returns it, owned and
		// unexpired. The anomaly affects UPDATEs regardless of predicate — a
		// primary-key re-UPDATE can spuriously report zero rows just the same, so
		// it cannot be the arbiter. Reads ARE consistent, so use the authoritative
		// primary-key SELECT as the source of truth: renew's contract is only
		// "this session still holds a valid lease on the name", satisfied whenever
		// the reservation exists, is owned by this session, and is unexpired.
		// Without this, session creation aborts with AGM-007 for a name the caller
		// just reserved.
		owned, ownErr := a.reservationOwnedAndUnexpired(sessionID, name, now)
		if ownErr != nil {
			return ownErr
		}
		if !owned {
			return &SessionNameConflictError{Name: name}
		}
		// Best-effort extension by primary key so a long-running caller keeps a
		// fresh lease. A spurious zero-row result here is deliberately ignored:
		// ownership and non-expiry are already proven above, and the TTL set at
		// reservation time (2h) covers the caller's remaining launch/readiness
		// work by orders of magnitude. Correctness must not depend on this UPDATE.
		_, _ = a.conn.Exec( //nolint:noctx // best-effort lease extension; see comment
			`UPDATE agm_session_name_reservations
			 SET expires_at = ?
			 WHERE workspace = ? AND name = ?`,
			now.Add(sessionNameReservationTTL), a.workspace, name,
		)
		return nil
	}
	return nil
}

// reservationOwnedAndUnexpired reports whether the workspace-scoped reservation
// for name is currently held by sessionID and has not expired as of asOf. It
// reads by primary key (workspace, name); reads are consistent even when the
// zero-row UPDATE anomaly is present, so this is the authoritative source of
// truth RenewSessionNameReservation falls back to.
func (a *Adapter) reservationOwnedAndUnexpired(sessionID, name string, asOf time.Time) (bool, error) {
	var owner string
	var expiresAt time.Time
	err := a.conn.QueryRow( //nolint:noctx // TODO(context): plumb ctx through this layer
		`SELECT session_id, expires_at FROM agm_session_name_reservations
		 WHERE workspace = ? AND name = ?`,
		a.workspace, name,
	).Scan(&owner, &expiresAt)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("re-read session-name reservation after zero-row renew: %w", err)
	}
	return owner == sessionID && expiresAt.After(asOf), nil
}

// ReactivateSessionResult distinguishes a failed lifecycle mutation from a
// committed reactivation whose reservation cleanup still needs attention.
type ReactivateSessionResult struct {
	StorageCommitted bool
}

// ReactivateSession restores an archived session only after atomically
// reserving its workspace-scoped active name. The reservation stays live until
// an identity-fenced lifecycle update finishes, so creation, parent linking,
// and concurrent restore attempts cannot install the same active name.
func (a *Adapter) ReactivateSession(session *manifest.Manifest) (result ReactivateSessionResult, retErr error) {
	if session == nil {
		return ReactivateSessionResult{}, fmt.Errorf("session cannot be nil")
	}
	if session.SessionID == "" {
		return ReactivateSessionResult{}, fmt.Errorf("session_id cannot be empty")
	}
	if session.Lifecycle != manifest.LifecycleArchived {
		return ReactivateSessionResult{}, fmt.Errorf("session is not archived: %s", session.SessionID)
	}
	reservationCreated, err := a.reserveSessionName(session.SessionID, session.Name)
	releaseReservation := reservationCreated
	if releaseReservation {
		defer func() {
			if !releaseReservation {
				return
			}
			if err := a.ReleaseSessionNameReservation(session.SessionID); err != nil {
				retErr = errors.Join(retErr, err)
			}
		}()
	}
	if err != nil {
		return ReactivateSessionResult{}, err
	}
	result, retErr = a.reactivateSessionReserved(session, session.Name)
	var uncertain *SessionIdentityMutationCommitUncertainError
	if errors.As(retErr, &uncertain) {
		releaseReservation = false
	}
	return result, retErr
}

func (a *Adapter) reactivateSessionReserved(session *manifest.Manifest, reservationName string) (ReactivateSessionResult, error) {
	observedRevision := nullableStringValue(sql.NullString{
		String: session.Tmux.SessionRevision,
		Valid:  session.Tmux.SessionRevision != "",
	})
	nextRevision := uuid.NewString()
	updatedAt := time.Now()
	updateResult, err := a.conn.Exec( //nolint:noctx // TODO(context): plumb ctx through this layer
		`UPDATE agm_sessions
		 SET status = 'active', updated_at = ?, tmux_session_revision = ?
		 WHERE id = ? AND workspace = ? AND status = 'archived'
		   AND name = ? AND tmux_session_name = ?
		   AND ((tmux_session_revision IS NULL AND ? IS NULL) OR tmux_session_revision = ?)
		   AND (? = '' OR EXISTS (
			   SELECT 1 FROM agm_session_name_reservations
			   WHERE workspace = ? AND name = ? AND session_id = ?
		   ))`,
		updatedAt,
		nextRevision,
		session.SessionID,
		a.workspace,
		session.Name,
		session.Tmux.SessionName,
		observedRevision,
		observedRevision,
		reservationName,
		a.workspace,
		reservationName,
		session.SessionID,
	)
	if err == nil {
		rowsAffected, rowsErr := updateResult.RowsAffected()
		if rowsErr == nil && rowsAffected == 1 {
			session.Lifecycle = ""
			session.UpdatedAt = updatedAt
			session.Tmux.SessionRevision = nextRevision
			return ReactivateSessionResult{StorageCommitted: true}, nil
		}
		if rowsErr != nil {
			err = fmt.Errorf("get reactivated session rows affected: %w", rowsErr)
		} else {
			err = fmt.Errorf("archived session identity changed concurrently: %s", session.SessionID)
		}
	} else {
		err = fmt.Errorf("reactivate session: %w", err)
	}
	return a.reconcileSessionReactivation(session, reservationName, observedRevision, nextRevision, err)
}

func (a *Adapter) reconcileSessionReactivation(session *manifest.Manifest, reservationName string, observedRevision any, nextRevision string, primaryErr error) (ReactivateSessionResult, error) {
	inspectCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	fenceRevision := uuid.NewString()
	fenceWon, fenceErr := a.fenceSessionReactivation(
		inspectCtx,
		session,
		observedRevision,
		fenceRevision,
	)
	if fenceErr != nil {
		primaryErr = errors.Join(primaryErr, fenceErr)
	}
	current, inspectErr := a.GetSession(session.SessionID)
	if inspectErr != nil {
		primaryErr = errors.Join(primaryErr, fmt.Errorf("inspect session after reactivation error: %w", inspectErr))
		if fenceWon {
			return ReactivateSessionResult{}, primaryErr
		}
		return ReactivateSessionResult{}, &SessionIdentityMutationCommitUncertainError{Err: primaryErr}
	}
	if sessionReactivationCommitted(current, session, nextRevision) {
		session.Lifecycle = current.Lifecycle
		session.UpdatedAt = current.UpdatedAt
		session.Tmux.SessionRevision = current.Tmux.SessionRevision
		return ReactivateSessionResult{StorageCommitted: true}, nil
	}
	if reservationName != "" {
		owned, ownershipErr := a.sessionNameReservationOwned(session.SessionID, reservationName)
		if ownershipErr != nil {
			primaryErr = errors.Join(primaryErr, ownershipErr)
		} else if !owned {
			primaryErr = &SessionNameConflictError{Name: reservationName}
		}
	}
	if sessionReactivationFenced(current, session, fenceWon) {
		return ReactivateSessionResult{}, primaryErr
	}
	return ReactivateSessionResult{}, &SessionIdentityMutationCommitUncertainError{Err: primaryErr}
}

func sessionReactivationCommitted(current, observed *manifest.Manifest, nextRevision string) bool {
	return current.Tmux.SessionRevision == nextRevision ||
		(current.Lifecycle != manifest.LifecycleArchived &&
			current.Name == observed.Name &&
			current.Tmux.SessionName == observed.Tmux.SessionName)
}

func sessionReactivationFenced(current, observed *manifest.Manifest, fenceWon bool) bool {
	return fenceWon ||
		current.Tmux.SessionRevision != observed.Tmux.SessionRevision ||
		current.Lifecycle != manifest.LifecycleArchived ||
		current.Name != observed.Name ||
		current.Tmux.SessionName != observed.Tmux.SessionName
}

func (a *Adapter) fenceSessionReactivation(ctx context.Context, session *manifest.Manifest, observedRevision any, fenceRevision string) (bool, error) {
	result, err := a.conn.ExecContext(ctx, `
		UPDATE agm_sessions
		SET tmux_session_revision = ?
		WHERE id = ? AND workspace = ? AND status = 'archived'
		  AND name = ? AND tmux_session_name = ?
		  AND ((tmux_session_revision IS NULL AND ? IS NULL) OR tmux_session_revision = ?)
	`, fenceRevision, session.SessionID, a.workspace, session.Name, session.Tmux.SessionName, observedRevision, observedRevision)
	if err != nil {
		return false, fmt.Errorf("fence pending session reactivation: %w", err)
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("get session reactivation fence rows affected: %w", err)
	}
	if rowsAffected > 1 {
		return false, fmt.Errorf("session reactivation fence changed %d rows", rowsAffected)
	}
	return rowsAffected == 1, nil
}

func reservationOwnedBy(tx *sql.Tx, workspace, sessionID, name string) error {
	var owner string
	err := tx.QueryRow( //nolint:noctx // TODO(context): plumb ctx through this layer
		`SELECT session_id FROM agm_session_name_reservations
		 WHERE workspace = ? AND name = ?`,
		workspace,
		name,
	).Scan(&owner)
	if errors.Is(err, sql.ErrNoRows) || (err == nil && owner != sessionID) {
		return &SessionNameConflictError{Name: name}
	}
	if err != nil {
		return fmt.Errorf("inspect session-name reservation: %w", err)
	}
	return nil
}
