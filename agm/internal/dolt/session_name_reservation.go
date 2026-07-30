package dolt

import (
	"database/sql"
	"errors"
	"fmt"
	"time"
)

const sessionNameReservationTTL = 2 * time.Hour

// ReserveSessionName atomically leases a workspace-scoped active session name.
// Existing duplicate session rows remain readable and actionable; only new
// creation attempts for an already active name are rejected.
func (a *Adapter) ReserveSessionName(sessionID, name string) error {
	if sessionID == "" {
		return fmt.Errorf("session_id cannot be empty")
	}
	if name == "" {
		return nil
	}
	if err := a.ApplyMigrations(); err != nil {
		return fmt.Errorf("failed to apply migrations: %w", err)
	}

	tx, err := a.conn.Begin() //nolint:noctx // TODO(context): plumb ctx through this layer
	if err != nil {
		return fmt.Errorf("begin session-name reservation: %w", err)
	}
	defer tx.Rollback()

	now := time.Now()
	if _, err := tx.Exec( //nolint:noctx // TODO(context): plumb ctx through this layer
		`DELETE FROM agm_session_name_reservations WHERE expires_at <= ?`,
		now,
	); err != nil {
		return fmt.Errorf("reclaim expired session-name reservations: %w", err)
	}
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
			return fmt.Errorf("reserve session name: %w", err)
		}
		var owner string
		lookupErr := tx.QueryRow( //nolint:noctx // TODO(context): plumb ctx through this layer
			`SELECT session_id FROM agm_session_name_reservations
			 WHERE workspace = ? AND name = ?`,
			a.workspace,
			name,
		).Scan(&owner)
		if lookupErr != nil || owner != sessionID {
			return &SessionNameConflictError{Name: name}
		}
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
		return &SessionNameConflictError{Name: name}
	case !errors.Is(err, sql.ErrNoRows):
		return fmt.Errorf("inspect active session name: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit session-name reservation: %w", err)
	}
	return nil
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
