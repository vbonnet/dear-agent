package dolt

import (
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
		return true, &SessionNameReservationCommitUncertainError{
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
	if reservationCreated {
		defer func() {
			if err := a.ReleaseSessionNameReservation(session.SessionID); err != nil {
				retErr = errors.Join(retErr, err)
			}
		}()
	}
	if err != nil {
		return ReactivateSessionResult{}, err
	}
	result, retErr = a.reactivateSessionReserved(session, session.Name)
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

	current, inspectErr := a.GetSession(session.SessionID)
	if inspectErr != nil {
		return ReactivateSessionResult{}, errors.Join(err, fmt.Errorf("inspect session after reactivation error: %w", inspectErr))
	}
	if current.Lifecycle != manifest.LifecycleArchived &&
		current.Name == session.Name &&
		current.Tmux.SessionName == session.Tmux.SessionName {
		session.Lifecycle = current.Lifecycle
		session.UpdatedAt = current.UpdatedAt
		session.Tmux.SessionRevision = current.Tmux.SessionRevision
		return ReactivateSessionResult{StorageCommitted: true}, nil
	}
	if reservationName != "" {
		owned, ownershipErr := a.sessionNameReservationOwned(session.SessionID, reservationName)
		if ownershipErr != nil {
			return ReactivateSessionResult{}, errors.Join(err, ownershipErr)
		}
		if !owned {
			return ReactivateSessionResult{}, &SessionNameConflictError{Name: reservationName}
		}
	}
	return ReactivateSessionResult{}, err
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
