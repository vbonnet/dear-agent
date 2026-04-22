// Package db provides db functionality.
package db

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/vbonnet/dear-agent/agm/internal/manifest"
)

// SessionFilter provides options for filtering sessions in queries
type SessionFilter struct {
	Lifecycle       string    // Filter by lifecycle state ("", "archived")
	Harness         string    // Filter by harness type ("claude-code", "gemini-cli", "codex-cli", "opencode-cli")
	ParentSessionID *string   // Filter by parent session ID (nil = no filter)
	UpdatedAfter    time.Time // Filter by updated_at >= this time
	Limit           int       // Max number of results (0 = no limit)
	Offset          int       // Number of results to skip
}

// CreateSession inserts a new session into the database
func (db *DB) CreateSession(session *manifest.Manifest) error {
	if session == nil {
		return fmt.Errorf("session cannot be nil")
	}
	if session.SessionID == "" {
		return fmt.Errorf("session_id cannot be empty")
	}

	// Marshal nested structs to JSON
	contextTags, err := json.Marshal(session.Context.Tags)
	if err != nil {
		return fmt.Errorf("failed to marshal context tags: %w", err)
	}

	var engramEnabled int
	var engramIDs []byte
	var engramLoadedAt *time.Time
	var engramCount int
	var engramQuery string

	if session.EngramMetadata != nil {
		if session.EngramMetadata.Enabled {
			engramEnabled = 1
		}
		engramQuery = session.EngramMetadata.Query
		engramIDs, err = json.Marshal(session.EngramMetadata.EngramIDs)
		if err != nil {
			return fmt.Errorf("failed to marshal engram IDs: %w", err)
		}
		if !session.EngramMetadata.LoadedAt.IsZero() {
			engramLoadedAt = &session.EngramMetadata.LoadedAt
		}
		engramCount = session.EngramMetadata.Count
	}

	query := `
		INSERT INTO sessions (
			session_id, name, schema_version, created_at, updated_at, lifecycle,
			harness, model, context_project, context_purpose, context_tags, context_notes,
			claude_uuid, tmux_session_name, engram_enabled, engram_query,
			engram_ids, engram_loaded_at, engram_count, parent_session_id
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`

	_, err = db.conn.Exec(query,
		session.SessionID,
		session.Name,
		session.SchemaVersion,
		session.CreatedAt,
		session.UpdatedAt,
		session.Lifecycle,
		session.Harness,
		session.Model,
		session.Context.Project,
		session.Context.Purpose,
		contextTags,
		session.Context.Notes,
		session.Claude.UUID,
		session.Tmux.SessionName,
		engramEnabled,
		engramQuery,
		engramIDs,
		engramLoadedAt,
		engramCount,
		nil, // parent_session_id - not used yet
	)

	if err != nil {
		return fmt.Errorf("failed to insert session: %w", err)
	}

	return nil
}

// GetSession retrieves a session by ID
func (db *DB) GetSession(sessionID string) (*manifest.Manifest, error) {
	if sessionID == "" {
		return nil, fmt.Errorf("session_id cannot be empty")
	}

	query := `
		SELECT session_id, name, schema_version, created_at, updated_at, lifecycle,
			harness, model, context_project, context_purpose, context_tags, context_notes,
			claude_uuid, tmux_session_name, engram_enabled, engram_query,
			engram_ids, engram_loaded_at, engram_count, parent_session_id
		FROM sessions
		WHERE session_id = ?
	`

	row := db.conn.QueryRow(query, sessionID)
	return scanSession(row)
}

// UpdateSession updates an existing session in the database
func (db *DB) UpdateSession(session *manifest.Manifest) error {
	if session == nil {
		return fmt.Errorf("session cannot be nil")
	}
	if session.SessionID == "" {
		return fmt.Errorf("session_id cannot be empty")
	}

	// Marshal nested structs to JSON
	contextTags, err := json.Marshal(session.Context.Tags)
	if err != nil {
		return fmt.Errorf("failed to marshal context tags: %w", err)
	}

	var engramEnabled int
	var engramIDs []byte
	var engramLoadedAt *time.Time
	var engramCount int
	var engramQuery string

	if session.EngramMetadata != nil {
		if session.EngramMetadata.Enabled {
			engramEnabled = 1
		}
		engramQuery = session.EngramMetadata.Query
		engramIDs, err = json.Marshal(session.EngramMetadata.EngramIDs)
		if err != nil {
			return fmt.Errorf("failed to marshal engram IDs: %w", err)
		}
		if !session.EngramMetadata.LoadedAt.IsZero() {
			engramLoadedAt = &session.EngramMetadata.LoadedAt
		}
		engramCount = session.EngramMetadata.Count
	}

	query := `
		UPDATE sessions
		SET name = ?, schema_version = ?, updated_at = ?, lifecycle = ?,
			harness = ?, model = ?, context_project = ?, context_purpose = ?, context_tags = ?,
			context_notes = ?, claude_uuid = ?, tmux_session_name = ?,
			engram_enabled = ?, engram_query = ?, engram_ids = ?,
			engram_loaded_at = ?, engram_count = ?, parent_session_id = ?
		WHERE session_id = ?
	`

	result, err := db.conn.Exec(query,
		session.Name,
		session.SchemaVersion,
		session.UpdatedAt,
		session.Lifecycle,
		session.Harness,
		session.Model,
		session.Context.Project,
		session.Context.Purpose,
		contextTags,
		session.Context.Notes,
		session.Claude.UUID,
		session.Tmux.SessionName,
		engramEnabled,
		engramQuery,
		engramIDs,
		engramLoadedAt,
		engramCount,
		nil, // parent_session_id - not used yet
		session.SessionID,
	)

	if err != nil {
		return fmt.Errorf("failed to update session: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rowsAffected == 0 {
		return fmt.Errorf("session not found: %s", session.SessionID)
	}

	return nil
}

// DeleteSession deletes a session from the database
func (db *DB) DeleteSession(sessionID string) error {
	if sessionID == "" {
		return fmt.Errorf("session_id cannot be empty")
	}

	query := `DELETE FROM sessions WHERE session_id = ?`

	result, err := db.conn.Exec(query, sessionID)
	if err != nil {
		return fmt.Errorf("failed to delete session: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rowsAffected == 0 {
		return fmt.Errorf("session not found: %s", sessionID)
	}

	return nil
}

// ListSessions returns a list of sessions matching the filter criteria
func (db *DB) ListSessions(filter *SessionFilter) ([]*manifest.Manifest, error) {
	query := `
		SELECT session_id, name, schema_version, created_at, updated_at, lifecycle,
			harness, model, context_project, context_purpose, context_tags, context_notes,
			claude_uuid, tmux_session_name, engram_enabled, engram_query,
			engram_ids, engram_loaded_at, engram_count, parent_session_id
		FROM sessions
		WHERE 1=1
	`

	args := []interface{}{}

	// Apply filters
	if filter != nil {
		if filter.Lifecycle != "" {
			query += " AND lifecycle = ?"
			args = append(args, filter.Lifecycle)
		}

		if filter.Harness != "" {
			query += " AND harness = ?"
			args = append(args, filter.Harness)
		}

		if filter.ParentSessionID != nil {
			if *filter.ParentSessionID == "" {
				query += " AND parent_session_id IS NULL"
			} else {
				query += " AND parent_session_id = ?"
				args = append(args, *filter.ParentSessionID)
			}
		}

		if !filter.UpdatedAfter.IsZero() {
			query += " AND updated_at >= ?"
			args = append(args, filter.UpdatedAfter)
		}
	}

	// Order by updated_at descending
	query += " ORDER BY updated_at DESC"

	// Apply limit and offset
	if filter != nil {
		if filter.Limit > 0 {
			query += " LIMIT ?"
			args = append(args, filter.Limit)
		}

		if filter.Offset > 0 {
			query += " OFFSET ?"
			args = append(args, filter.Offset)
		}
	}

	rows, err := db.conn.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query sessions: %w", err)
	}
	defer rows.Close()

	var sessions []*manifest.Manifest
	for rows.Next() {
		session, err := scanSession(rows)
		if err != nil {
			return nil, err
		}
		sessions = append(sessions, session)
	}

	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating rows: %w", err)
	}

	return sessions, nil
}

// scanSession is a helper that scans a row into a Manifest struct
type scanner interface {
	Scan(dest ...interface{}) error
}

func scanSession(row scanner) (*manifest.Manifest, error) {
	var session manifest.Manifest
	var contextTagsJSON []byte
	var engramEnabled int
	var engramQuery string
	var engramIDsJSON []byte
	var engramLoadedAt sql.NullTime
	var engramCount int
	var parentSessionID sql.NullString
	var model sql.NullString

	err := row.Scan(
		&session.SessionID,
		&session.Name,
		&session.SchemaVersion,
		&session.CreatedAt,
		&session.UpdatedAt,
		&session.Lifecycle,
		&session.Harness,
		&model,
		&session.Context.Project,
		&session.Context.Purpose,
		&contextTagsJSON,
		&session.Context.Notes,
		&session.Claude.UUID,
		&session.Tmux.SessionName,
		&engramEnabled,
		&engramQuery,
		&engramIDsJSON,
		&engramLoadedAt,
		&engramCount,
		&parentSessionID,
	)

	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("session not found")
		}
		return nil, fmt.Errorf("failed to scan session: %w", err)
	}

	// Set nullable model field
	if model.Valid {
		session.Model = model.String
	}

	// Unmarshal context tags
	if len(contextTagsJSON) > 0 {
		if err := json.Unmarshal(contextTagsJSON, &session.Context.Tags); err != nil {
			return nil, fmt.Errorf("failed to unmarshal context tags: %w", err)
		}
	}

	// Reconstruct engram metadata if enabled
	if engramEnabled == 1 || len(engramIDsJSON) > 0 {
		session.EngramMetadata = &manifest.EngramMetadata{
			Enabled: engramEnabled == 1,
			Query:   engramQuery,
			Count:   engramCount,
		}

		if len(engramIDsJSON) > 0 {
			if err := json.Unmarshal(engramIDsJSON, &session.EngramMetadata.EngramIDs); err != nil {
				return nil, fmt.Errorf("failed to unmarshal engram IDs: %w", err)
			}
		}

		if engramLoadedAt.Valid {
			session.EngramMetadata.LoadedAt = engramLoadedAt.Time
		}
	}

	return &session, nil
}
