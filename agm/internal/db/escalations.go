package db

import (
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// EscalationEvent represents an escalation detected during session monitoring
type EscalationEvent struct {
	Type        string    // Type of escalation
	Pattern     string    // Pattern that matched
	Line        string    // The actual line that triggered escalation
	LineNumber  int       // Line number in the output
	DetectedAt  time.Time // When the escalation was detected
	Description string    // Description of the escalation
}

// CreateEscalation inserts a new escalation event into the database
func (db *DB) CreateEscalation(sessionID string, event *EscalationEvent) (int64, error) {
	if sessionID == "" {
		return 0, fmt.Errorf("session_id cannot be empty")
	}
	if event == nil {
		return 0, fmt.Errorf("escalation event cannot be nil")
	}
	if event.Type == "" {
		return 0, fmt.Errorf("escalation type cannot be empty")
	}
	if event.Pattern == "" {
		return 0, fmt.Errorf("escalation pattern cannot be empty")
	}

	query := `
		INSERT INTO escalations (
			session_id, type, pattern, line, line_number,
			detected_at, description, resolved, resolved_at, resolution_note
		) VALUES (?, ?, ?, ?, ?, ?, ?, 0, NULL, NULL)
	`

	result, err := db.conn.Exec(query, //nolint:noctx // TODO(context): plumb ctx through this layer
		sessionID,
		event.Type,
		event.Pattern,
		event.Line,
		event.LineNumber,
		event.DetectedAt,
		event.Description,
	)

	if err != nil {
		return 0, fmt.Errorf("failed to insert escalation: %w", err)
	}

	escalationID, err := result.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("failed to get last insert ID: %w", err)
	}

	return escalationID, nil
}

// GetEscalations retrieves all escalations for a session
func (db *DB) GetEscalations(sessionID string) ([]*EscalationEvent, error) {
	if sessionID == "" {
		return nil, fmt.Errorf("session_id cannot be empty")
	}

	query := `
		SELECT escalation_id, type, pattern, line, line_number,
			detected_at, description, resolved, resolved_at, resolution_note
		FROM escalations
		WHERE session_id = ?
		ORDER BY detected_at DESC
	`

	rows, err := db.conn.Query(query, sessionID) //nolint:noctx // TODO(context): plumb ctx through this layer
	if err != nil {
		return nil, fmt.Errorf("failed to query escalations: %w", err)
	}
	defer rows.Close()

	var escalations []*EscalationEvent
	for rows.Next() {
		escalation, err := scanEscalation(rows)
		if err != nil {
			return nil, err
		}
		escalations = append(escalations, escalation)
	}

	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating rows: %w", err)
	}

	return escalations, nil
}

// GetUnresolvedEscalations retrieves all unresolved escalations for a session
func (db *DB) GetUnresolvedEscalations(sessionID string) ([]*EscalationEvent, error) {
	if sessionID == "" {
		return nil, fmt.Errorf("session_id cannot be empty")
	}

	query := `
		SELECT escalation_id, type, pattern, line, line_number,
			detected_at, description, resolved, resolved_at, resolution_note
		FROM escalations
		WHERE session_id = ? AND resolved = 0
		ORDER BY detected_at DESC
	`

	rows, err := db.conn.Query(query, sessionID) //nolint:noctx // TODO(context): plumb ctx through this layer
	if err != nil {
		return nil, fmt.Errorf("failed to query unresolved escalations: %w", err)
	}
	defer rows.Close()

	var escalations []*EscalationEvent
	for rows.Next() {
		escalation, err := scanEscalation(rows)
		if err != nil {
			return nil, err
		}
		escalations = append(escalations, escalation)
	}

	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating rows: %w", err)
	}

	return escalations, nil
}

// ResolveEscalation marks an escalation as resolved
func (db *DB) ResolveEscalation(escalationID int64, note string) error {
	if escalationID <= 0 {
		return fmt.Errorf("escalation_id must be positive")
	}

	query := `
		UPDATE escalations
		SET resolved = 1, resolved_at = ?, resolution_note = ?
		WHERE escalation_id = ?
	`

	result, err := db.conn.Exec(query, time.Now(), note, escalationID) //nolint:noctx // TODO(context): plumb ctx through this layer
	if err != nil {
		return fmt.Errorf("failed to resolve escalation: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rowsAffected == 0 {
		return fmt.Errorf("escalation not found: %d", escalationID)
	}

	return nil
}

// DeleteEscalations deletes all escalations for a session
func (db *DB) DeleteEscalations(sessionID string) error {
	if sessionID == "" {
		return fmt.Errorf("session_id cannot be empty")
	}

	query := `DELETE FROM escalations WHERE session_id = ?`

	_, err := db.conn.Exec(query, sessionID) //nolint:noctx // TODO(context): plumb ctx through this layer
	if err != nil {
		return fmt.Errorf("failed to delete escalations: %w", err)
	}

	return nil
}

// scanEscalation scans a row into an EscalationEvent struct
func scanEscalation(row scanner) (*EscalationEvent, error) {
	var escalation EscalationEvent
	var escalationID int64
	var resolved int
	var resolvedAt sql.NullTime
	var resolutionNote sql.NullString

	err := row.Scan(
		&escalationID,
		&escalation.Type,
		&escalation.Pattern,
		&escalation.Line,
		&escalation.LineNumber,
		&escalation.DetectedAt,
		&escalation.Description,
		&resolved,
		&resolvedAt,
		&resolutionNote,
	)

	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("escalation not found")
		}
		return nil, fmt.Errorf("failed to scan escalation: %w", err)
	}

	// Note: EscalationEvent doesn't have resolved fields, but we can use them
	// if we extend the struct in the future. For now, we just ignore them.
	// The database tracks resolution status even if the struct doesn't.

	return &escalation, nil
}
