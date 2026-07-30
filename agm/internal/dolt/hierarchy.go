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

// GetParent retrieves the parent session for a given session ID.
// Returns nil if the session has no parent (root session).
// Returns error if session not found.
func (a *Adapter) GetParent(sessionID string) (*manifest.Manifest, error) {
	if sessionID == "" {
		return nil, fmt.Errorf("session_id cannot be empty")
	}

	// Ensure migrations are applied
	if err := a.ApplyMigrations(); err != nil {
		return nil, fmt.Errorf("failed to apply migrations: %w", err)
	}

	// First get the parent_session_id for this session
	var parentID sql.NullString
	query := `SELECT parent_session_id FROM agm_sessions WHERE id = ? AND workspace = ?`

	err := a.conn.QueryRow(query, sessionID, a.workspace).Scan(&parentID) //nolint:noctx // TODO(context): plumb ctx through this layer
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("session not found: %s", sessionID)
		}
		return nil, fmt.Errorf("failed to get parent_session_id: %w", err)
	}

	// If parent_session_id is NULL or empty, this is a root session
	if !parentID.Valid || parentID.String == "" {
		return nil, nil
	}

	// Fetch the parent session
	parent, err := a.GetSession(parentID.String)
	if err != nil {
		// Parent was deleted (orphaned child) - return nil parent
		if errors.Is(err, sql.ErrNoRows) || fmt.Sprint(err) == fmt.Sprintf("session not found: %s", parentID.String) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get parent session: %w", err)
	}

	return parent, nil
}

// GetChildren retrieves all direct child sessions for a given parent session ID.
// Returns empty slice if no children found.
func (a *Adapter) GetChildren(sessionID string) ([]*manifest.Manifest, error) {
	if sessionID == "" {
		return nil, fmt.Errorf("session_id cannot be empty")
	}

	// Ensure migrations are applied
	if err := a.ApplyMigrations(); err != nil {
		return nil, fmt.Errorf("failed to apply migrations: %w", err)
	}

	query := `
		SELECT id, created_at, updated_at, status, workspace, model, name, harness,
			context_project, context_purpose, context_tags, context_notes,
			claude_uuid, tmux_session_name, tmux_session_revision, metadata,
			permission_mode, permission_mode_updated_at, permission_mode_source,
			is_test,
			context_total_tokens, context_used_tokens, context_percentage_used,
			monitors
		FROM agm_sessions
		WHERE parent_session_id = ? AND workspace = ?
		ORDER BY created_at ASC
	`

	rows, err := a.conn.Query(query, sessionID, a.workspace) //nolint:noctx // TODO(context): plumb ctx through this layer
	if err != nil {
		return nil, fmt.Errorf("failed to query child sessions: %w", err)
	}
	defer rows.Close()

	var children []*manifest.Manifest
	for rows.Next() {
		child, err := a.scanSession(rows)
		if err != nil {
			return nil, fmt.Errorf("failed to scan child session: %w", err)
		}
		children = append(children, child)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating child sessions: %w", err)
	}

	return children, nil
}

// LinkSessionParent atomically assigns a parent and, when requested, an
// inherited display name against the exact session identity revision observed
// by the caller. Administrative hierarchy repair must never report a name
// update that a concurrent full-session writer silently fenced out.
func (a *Adapter) LinkSessionParent(ctx context.Context, sessionID, observedRevision, parentSessionID string, inheritedName *string) (retErr error) {
	if sessionID == "" {
		return fmt.Errorf("session_id cannot be empty")
	}
	if parentSessionID == "" {
		return fmt.Errorf("parent_session_id cannot be empty")
	}
	if sessionID == parentSessionID {
		return fmt.Errorf("session cannot be its own parent")
	}
	if err := a.ApplyMigrations(); err != nil {
		return fmt.Errorf("failed to apply migrations: %w", err)
	}

	if err := a.ensureParentSessionExists(ctx, parentSessionID); err != nil {
		return err
	}

	reservationCreated, err := a.reserveParentLinkName(ctx, sessionID, inheritedName)
	if reservationCreated {
		defer func() {
			if err := a.ReleaseSessionNameReservation(sessionID); err != nil {
				retErr = errors.Join(retErr, err)
			}
		}()
	}
	if err != nil {
		return err
	}

	observed := nullableStringValue(sql.NullString{String: observedRevision, Valid: observedRevision != ""})
	var inheritedNameValue any
	if inheritedName != nil {
		inheritedNameValue = *inheritedName
	}
	result, err := a.conn.ExecContext(ctx, `
		UPDATE agm_sessions
		SET updated_at = ?, parent_session_id = ?,
			name = CASE WHEN ? IS NULL THEN name ELSE ? END,
			tmux_session_revision = ?
		WHERE id = ? AND workspace = ?
		  AND ((tmux_session_revision IS NULL AND ? IS NULL) OR tmux_session_revision = ?)
	`, time.Now(), parentSessionID, inheritedNameValue, inheritedNameValue, uuid.NewString(), sessionID, a.workspace, observed, observed)
	if err != nil {
		return fmt.Errorf("link session parent: %w", err)
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("get linked session rows affected: %w", err)
	}
	if rowsAffected > 0 {
		return nil
	}

	var currentRevision sql.NullString
	if err := a.conn.QueryRowContext(ctx, `SELECT tmux_session_revision FROM agm_sessions WHERE id = ? AND workspace = ?`, sessionID, a.workspace).Scan(&currentRevision); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("session not found: %s", sessionID)
		}
		return fmt.Errorf("inspect session parent conflict: %w", err)
	}
	return fmt.Errorf("session identity changed concurrently while linking parent %s", sessionID)
}

func (a *Adapter) ensureParentSessionExists(ctx context.Context, parentSessionID string) error {
	var parentExists int
	if err := a.conn.QueryRowContext(
		ctx,
		`SELECT 1 FROM agm_sessions WHERE id = ? AND workspace = ?`,
		parentSessionID,
		a.workspace,
	).Scan(&parentExists); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("parent session not found: %s", parentSessionID)
		}
		return fmt.Errorf("check parent session: %w", err)
	}
	return nil
}

func (a *Adapter) reserveParentLinkName(ctx context.Context, sessionID string, inheritedName *string) (bool, error) {
	if inheritedName == nil {
		return false, nil
	}
	var currentName, lifecycle string
	if err := a.conn.QueryRowContext(
		ctx,
		`SELECT name, status FROM agm_sessions WHERE id = ? AND workspace = ?`,
		sessionID,
		a.workspace,
	).Scan(&currentName, &lifecycle); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return false, fmt.Errorf("session not found: %s", sessionID)
		}
		return false, fmt.Errorf("inspect session before linking parent: %w", err)
	}
	if lifecycle == manifest.LifecycleArchived || currentName == *inheritedName {
		// Archived rows do not occupy the active-name set. ReactivateSession
		// uses an exact archived identity CAS, so a link that advances this
		// revision fences every stale reactivation snapshot.
		return false, nil
	}
	return a.reserveSessionName(sessionID, *inheritedName)
}

// DetachChild sets parent_session_id to NULL for a given child session.
// Returns error if the session is not found.
func (a *Adapter) DetachChild(sessionID string) error {
	if sessionID == "" {
		return fmt.Errorf("session_id cannot be empty")
	}

	// Ensure migrations are applied
	if err := a.ApplyMigrations(); err != nil {
		return fmt.Errorf("failed to apply migrations: %w", err)
	}

	query := `UPDATE agm_sessions SET parent_session_id = NULL WHERE id = ? AND workspace = ?`
	result, err := a.conn.Exec(query, sessionID, a.workspace) //nolint:noctx // TODO(context): plumb ctx through this layer
	if err != nil {
		return fmt.Errorf("failed to detach child session %s: %w", sessionID, err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected for child %s: %w", sessionID, err)
	}

	if rowsAffected == 0 {
		return fmt.Errorf("child session not found: %s", sessionID)
	}

	return nil
}

// SessionTree represents a session with its hierarchical relationships
type SessionTree struct {
	Root     *manifest.Manifest   // The session itself
	Parent   *manifest.Manifest   // Immediate parent (nil if root)
	Children []*manifest.Manifest // Direct children
	Depth    int                  // Depth in hierarchy (0 for root)
}

// GetSessionTree retrieves a session with its parent and children.
// Calculates depth by walking up the parent chain.
func (a *Adapter) GetSessionTree(sessionID string) (*SessionTree, error) {
	if sessionID == "" {
		return nil, fmt.Errorf("session_id cannot be empty")
	}

	// Ensure migrations are applied
	if err := a.ApplyMigrations(); err != nil {
		return nil, fmt.Errorf("failed to apply migrations: %w", err)
	}

	// Get the session itself
	root, err := a.GetSession(sessionID)
	if err != nil {
		return nil, fmt.Errorf("failed to get session: %w", err)
	}

	// Get parent
	parent, err := a.GetParent(sessionID)
	if err != nil {
		return nil, fmt.Errorf("failed to get parent: %w", err)
	}

	// Get children
	children, err := a.GetChildren(sessionID)
	if err != nil {
		return nil, fmt.Errorf("failed to get children: %w", err)
	}

	// Calculate depth by walking up the parent chain
	depth := 0
	visited := make(map[string]bool)
	currentID := sessionID
	visited[currentID] = true

	for {
		var parentID sql.NullString
		query := `SELECT parent_session_id FROM agm_sessions WHERE id = ? AND workspace = ?`
		err := a.conn.QueryRow(query, currentID, a.workspace).Scan(&parentID) //nolint:noctx // TODO(context): plumb ctx through this layer
		if err != nil {
			if err == sql.ErrNoRows {
				break // Session not found in chain
			}
			return nil, fmt.Errorf("failed to get parent_session_id for depth calculation: %w", err)
		}

		// No parent - reached root
		if !parentID.Valid || parentID.String == "" {
			break
		}

		// Circular reference detection
		if visited[parentID.String] {
			return nil, fmt.Errorf("circular reference detected in session hierarchy: %s", sessionID)
		}

		visited[parentID.String] = true
		currentID = parentID.String
		depth++
	}

	return &SessionTree{
		Root:     root,
		Parent:   parent,
		Children: children,
		Depth:    depth,
	}, nil
}
