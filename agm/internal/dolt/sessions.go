package dolt

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/vbonnet/dear-agent/agm/internal/manifest"
)

// defaultIfEmpty returns dflt when s is empty, otherwise s.
func defaultIfEmpty(s, dflt string) string {
	if s == "" {
		return dflt
	}
	return s
}

// marshalCreateSessionJSON serializes the JSON-typed fields needed by the
// agm_sessions INSERT (context tags, engram metadata, monitors).
func marshalCreateSessionJSON(session *manifest.Manifest) ([]byte, []byte, interface{}, error) {
	contextTags, err := json.Marshal(session.Context.Tags)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to marshal context tags: %w", err)
	}
	metadata := make(map[string]any)
	if session.EngramMetadata != nil {
		metadata["engram_enabled"] = session.EngramMetadata.Enabled
		metadata["engram_query"] = session.EngramMetadata.Query
		metadata["engram_ids"] = session.EngramMetadata.EngramIDs
		metadata["engram_loaded_at"] = session.EngramMetadata.LoadedAt
		metadata["engram_count"] = session.EngramMetadata.Count
	}
	metadataJSON, err := json.Marshal(metadata)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to marshal metadata: %w", err)
	}
	var monitorsJSON interface{}
	if len(session.Monitors) > 0 {
		monitorsData, err := json.Marshal(session.Monitors)
		if err != nil {
			return nil, nil, nil, fmt.Errorf("failed to marshal monitors: %w", err)
		}
		monitorsJSON = string(monitorsData)
	}
	return contextTags, metadataJSON, monitorsJSON, nil
}

// CreateSession inserts a new session into the database
func (a *Adapter) CreateSession(session *manifest.Manifest) error {
	if session == nil {
		return fmt.Errorf("session cannot be nil")
	}
	if session.SessionID == "" {
		return fmt.Errorf("session_id cannot be empty")
	}

	// Ensure migrations are applied
	if err := a.ApplyMigrations(); err != nil {
		return fmt.Errorf("failed to apply migrations: %w", err)
	}

	contextTags, metadataJSON, monitorsJSON, err := marshalCreateSessionJSON(session)
	if err != nil {
		return err
	}
	harness := defaultIfEmpty(session.Harness, "claude-code")
	status := "active"
	if session.Lifecycle == manifest.LifecycleArchived {
		status = "archived"
	}
	permissionMode := defaultIfEmpty(session.PermissionMode, "default")
	permissionModeSource := defaultIfEmpty(session.PermissionModeSource, "init")

	query := `
		INSERT INTO agm_sessions (
			id, created_at, updated_at, status, workspace, model, name, harness,
			context_project, context_purpose, context_tags, context_notes,
			claude_uuid, tmux_session_name, metadata,
			permission_mode, permission_mode_updated_at, permission_mode_source,
			parent_session_id, is_test,
			context_total_tokens, context_used_tokens, context_percentage_used,
			monitors
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`

	// Handle parent_session_id (nil pointer -> NULL)
	var parentSessionID interface{}
	if session.ParentSessionID != nil {
		parentSessionID = *session.ParentSessionID
	}

	// Handle context usage (nil pointer -> NULL)
	var ctxTotalTokens, ctxUsedTokens interface{}
	var ctxPercentageUsed interface{}
	if session.ContextUsage != nil {
		ctxTotalTokens = session.ContextUsage.TotalTokens
		ctxUsedTokens = session.ContextUsage.UsedTokens
		ctxPercentageUsed = session.ContextUsage.PercentageUsed
	}

	// Determine model (use session model if set, otherwise default)
	model := session.Model
	if model == "" {
		model = "claude-sonnet-4-5"
	}

	_, err = a.conn.Exec(query, //nolint:noctx // TODO(context): plumb ctx through this layer
		session.SessionID,
		session.CreatedAt,
		session.UpdatedAt,
		status,
		a.workspace, // Auto-set workspace from adapter
		model,
		session.Name,
		harness,
		session.Context.Project,
		session.Context.Purpose,
		contextTags,
		session.Context.Notes,
		session.Claude.UUID,
		session.Tmux.SessionName,
		metadataJSON,
		permissionMode,
		session.PermissionModeUpdatedAt,
		permissionModeSource,
		parentSessionID,
		session.IsTest,
		ctxTotalTokens,
		ctxUsedTokens,
		ctxPercentageUsed,
		monitorsJSON,
	)

	if err != nil {
		return fmt.Errorf("failed to insert session: %w", err)
	}

	return nil
}

// GetSession retrieves a session by ID
func (a *Adapter) GetSession(sessionID string) (*manifest.Manifest, error) {
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
			claude_uuid, tmux_session_name, metadata,
			permission_mode, permission_mode_updated_at, permission_mode_source,
			is_test,
			context_total_tokens, context_used_tokens, context_percentage_used,
			monitors
		FROM agm_sessions
		WHERE id = ? AND workspace = ?
	`

	row := a.conn.QueryRow(query, sessionID, a.workspace) //nolint:noctx // TODO(context): plumb ctx through this layer
	return a.scanSession(row)
}

// UpdateSession updates an existing session in the database
func (a *Adapter) UpdateSession(session *manifest.Manifest) error {
	if session == nil {
		return fmt.Errorf("session cannot be nil")
	}
	if session.SessionID == "" {
		return fmt.Errorf("session_id cannot be empty")
	}

	// Ensure migrations are applied
	if err := a.ApplyMigrations(); err != nil {
		return fmt.Errorf("failed to apply migrations: %w", err)
	}

	// Marshal context tags to JSON
	contextTags, err := json.Marshal(session.Context.Tags)
	if err != nil {
		return fmt.Errorf("failed to marshal context tags: %w", err)
	}

	// Build metadata JSON from EngramMetadata
	metadata := make(map[string]any)
	if session.EngramMetadata != nil {
		metadata["engram_enabled"] = session.EngramMetadata.Enabled
		metadata["engram_query"] = session.EngramMetadata.Query
		metadata["engram_ids"] = session.EngramMetadata.EngramIDs
		metadata["engram_loaded_at"] = session.EngramMetadata.LoadedAt
		metadata["engram_count"] = session.EngramMetadata.Count
	}
	metadataJSON, err := json.Marshal(metadata)
	if err != nil {
		return fmt.Errorf("failed to marshal metadata: %w", err)
	}

	// Determine harness (default to claude-code for backward compatibility)
	harness := session.Harness
	if harness == "" {
		harness = "claude-code"
	}

	// Determine status from lifecycle
	status := "active"
	if session.Lifecycle == manifest.LifecycleArchived {
		status = "archived"
	}

	// Update timestamp
	session.UpdatedAt = time.Now()

	// Handle context usage (nil pointer -> NULL)
	var ctxTotalTokens, ctxUsedTokens interface{}
	var ctxPercentageUsed interface{}
	if session.ContextUsage != nil {
		ctxTotalTokens = session.ContextUsage.TotalTokens
		ctxUsedTokens = session.ContextUsage.UsedTokens
		ctxPercentageUsed = session.ContextUsage.PercentageUsed
	}

	// Marshal monitors to JSON
	var monitorsJSON interface{}
	if len(session.Monitors) > 0 {
		monitorsData, err := json.Marshal(session.Monitors)
		if err != nil {
			return fmt.Errorf("failed to marshal monitors: %w", err)
		}
		monitorsJSON = string(monitorsData)
	}

	query := `
		UPDATE agm_sessions
		SET updated_at = ?, status = ?, name = ?, harness = ?,
			context_project = ?, context_purpose = ?, context_tags = ?,
			context_notes = ?, claude_uuid = ?, tmux_session_name = ?,
			metadata = ?,
			permission_mode = ?, permission_mode_updated_at = ?, permission_mode_source = ?,
			is_test = ?,
			context_total_tokens = ?, context_used_tokens = ?, context_percentage_used = ?,
			monitors = ?
		WHERE id = ? AND workspace = ?
	`

	result, err := a.conn.Exec(query, //nolint:noctx // TODO(context): plumb ctx through this layer
		session.UpdatedAt,
		status,
		session.Name,
		harness,
		session.Context.Project,
		session.Context.Purpose,
		contextTags,
		session.Context.Notes,
		session.Claude.UUID,
		session.Tmux.SessionName,
		metadataJSON,
		session.PermissionMode,
		session.PermissionModeUpdatedAt,
		session.PermissionModeSource,
		session.IsTest,
		ctxTotalTokens,
		ctxUsedTokens,
		ctxPercentageUsed,
		monitorsJSON,
		session.SessionID,
		a.workspace,
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
func (a *Adapter) DeleteSession(sessionID string) error {
	if sessionID == "" {
		return fmt.Errorf("session_id cannot be empty")
	}

	// Ensure migrations are applied
	if err := a.ApplyMigrations(); err != nil {
		return fmt.Errorf("failed to apply migrations: %w", err)
	}

	query := `DELETE FROM agm_sessions WHERE id = ? AND workspace = ?`

	result, err := a.conn.Exec(query, sessionID, a.workspace) //nolint:noctx // TODO(context): plumb ctx through this layer
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

// ListActiveSessions returns the names of all non-archived sessions.
// Used for lightweight cross-reference (e.g., audit commands) without loading full manifests.
func (a *Adapter) ListActiveSessions(ctx context.Context) ([]string, error) {
	if err := a.ApplyMigrations(); err != nil {
		return nil, fmt.Errorf("failed to apply migrations: %w", err)
	}
	query := `SELECT name FROM agm_sessions WHERE workspace = ? AND status != 'archived' ORDER BY updated_at DESC`
	rows, err := a.conn.QueryContext(ctx, query, a.workspace)
	if err != nil {
		return nil, fmt.Errorf("failed to list active sessions: %w", err)
	}
	defer rows.Close()

	var names []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, fmt.Errorf("failed to scan session name: %w", err)
		}
		names = append(names, name)
	}
	return names, rows.Err()
}

// applyListSessionsFilter appends the WHERE clauses for the optional filter
// (lifecycle/test/harness/tags) and returns the updated query+args.
func applyListSessionsFilter(query string, args []any, filter *SessionFilter) (string, []any) {
	if filter == nil {
		return query, args
	}
	if filter.ExcludeArchived {
		query += " AND status != 'archived'"
	} else if filter.Lifecycle != "" {
		if filter.Lifecycle == manifest.LifecycleArchived {
			query += " AND status = 'archived'"
		} else {
			query += " AND status = 'active'"
		}
	}
	if filter.ExcludeTest {
		query += " AND (is_test = FALSE OR is_test IS NULL)"
	}
	if filter.Harness != "" {
		query += " AND harness = ?"
		args = append(args, filter.Harness)
	}
	for _, tag := range filter.Tags {
		query += " AND JSON_CONTAINS(context_tags, ?)"
		args = append(args, fmt.Sprintf("%q", tag))
	}
	return query, args
}

// applyListSessionsLimit appends LIMIT/OFFSET clauses from filter.
func applyListSessionsLimit(query string, args []any, filter *SessionFilter) (string, []any) {
	if filter == nil {
		return query, args
	}
	if filter.Limit > 0 {
		query += " LIMIT ?"
		args = append(args, filter.Limit)
	}
	if filter.Offset > 0 {
		query += " OFFSET ?"
		args = append(args, filter.Offset)
	}
	return query, args
}

// ListSessions returns a list of sessions matching the filter criteria
func (a *Adapter) ListSessions(filter *SessionFilter) ([]*manifest.Manifest, error) {
	// Ensure migrations are applied
	if err := a.ApplyMigrations(); err != nil {
		return nil, fmt.Errorf("failed to apply migrations: %w", err)
	}

	query := `
		SELECT id, created_at, updated_at, status, workspace, model, name, harness,
			context_project, context_purpose, context_tags, context_notes,
			claude_uuid, tmux_session_name, metadata,
			permission_mode, permission_mode_updated_at, permission_mode_source,
			is_test,
			context_total_tokens, context_used_tokens, context_percentage_used,
			monitors
		FROM agm_sessions
		WHERE workspace = ?
	`

	args := []any{a.workspace}

	query, args = applyListSessionsFilter(query, args, filter)
	query += " ORDER BY updated_at DESC"
	query, args = applyListSessionsLimit(query, args, filter)

	rows, err := a.conn.Query(query, args...) //nolint:noctx // TODO(context): plumb ctx through this layer
	if err != nil {
		return nil, fmt.Errorf("failed to query sessions: %w", err)
	}
	defer rows.Close()

	var sessions []*manifest.Manifest
	for rows.Next() {
		session, err := a.scanSession(rows)
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

// ResolveIdentifier finds a session by session ID, tmux name, or manifest name
// This replaces the filesystem-based session.ResolveIdentifier() for Dolt storage
func (a *Adapter) ResolveIdentifier(identifier string) (*manifest.Manifest, error) {
	if identifier == "" {
		return nil, fmt.Errorf("identifier cannot be empty")
	}

	// Ensure migrations are applied
	if err := a.ApplyMigrations(); err != nil {
		return nil, fmt.Errorf("failed to apply migrations: %w", err)
	}

	// Try matching by session ID, tmux session name, OR manifest name
	// Exclude archived sessions (status != 'archived')
	query := `
		SELECT id, created_at, updated_at, status, workspace, model, name, harness,
			context_project, context_purpose, context_tags, context_notes,
			claude_uuid, tmux_session_name, metadata,
			permission_mode, permission_mode_updated_at, permission_mode_source,
			is_test,
			context_total_tokens, context_used_tokens, context_percentage_used,
			monitors
		FROM agm_sessions
		WHERE workspace = ?
		  AND (id = ? OR tmux_session_name = ? OR name = ?)
		  AND status != 'archived'
		LIMIT 1
	`

	row := a.conn.QueryRow(query, a.workspace, identifier, identifier, identifier) //nolint:noctx // TODO(context): plumb ctx through this layer
	m, err := a.scanSession(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) || strings.Contains(err.Error(), "session not found") {
			return nil, fmt.Errorf("session not found: %s", identifier)
		}
		return nil, err
	}

	return m, nil
}

// GetSessionByName returns the first non-archived session matching the given name
func (a *Adapter) GetSessionByName(name string) (*manifest.Manifest, error) {
	if name == "" {
		return nil, fmt.Errorf("name cannot be empty")
	}

	// Ensure migrations are applied
	if err := a.ApplyMigrations(); err != nil {
		return nil, fmt.Errorf("failed to apply migrations: %w", err)
	}

	query := `
		SELECT id, created_at, updated_at, status, workspace, model, name, harness,
			context_project, context_purpose, context_tags, context_notes,
			claude_uuid, tmux_session_name, metadata,
			permission_mode, permission_mode_updated_at, permission_mode_source,
			is_test,
			context_total_tokens, context_used_tokens, context_percentage_used,
			monitors
		FROM agm_sessions
		WHERE workspace = ? AND name = ? AND status != 'archived'
		LIMIT 1
	`

	row := a.conn.QueryRow(query, a.workspace, name) //nolint:noctx // TODO(context): plumb ctx through this layer
	m, err := a.scanSession(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) || strings.Contains(err.Error(), "session not found") {
			return nil, nil
		}
		return nil, err
	}

	return m, nil
}

// scanSession is a helper that scans a row into a Manifest struct
type scanner interface {
	Scan(dest ...any) error
}

func (a *Adapter) scanSession(row scanner) (*manifest.Manifest, error) {
	var session manifest.Manifest
	var contextTagsJSON []byte
	var metadataJSON []byte
	var status string
	var workspace string
	var model string
	var permissionMode sql.NullString
	var permissionModeUpdatedAt sql.NullTime
	var permissionModeSource sql.NullString
	var isTest sql.NullBool
	var ctxTotalTokens sql.NullInt64
	var ctxUsedTokens sql.NullInt64
	var ctxPercentageUsed sql.NullFloat64
	var monitorsJSON sql.NullString

	err := row.Scan(
		&session.SessionID,
		&session.CreatedAt,
		&session.UpdatedAt,
		&status,
		&workspace,
		&model,
		&session.Name,
		&session.Harness,
		&session.Context.Project,
		&session.Context.Purpose,
		&contextTagsJSON,
		&session.Context.Notes,
		&session.Claude.UUID,
		&session.Tmux.SessionName,
		&metadataJSON,
		&permissionMode,
		&permissionModeUpdatedAt,
		&permissionModeSource,
		&isTest,
		&ctxTotalTokens,
		&ctxUsedTokens,
		&ctxPercentageUsed,
		&monitorsJSON,
	)

	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("session not found")
		}
		return nil, fmt.Errorf("failed to scan session: %w", err)
	}

	// Set lifecycle from status
	if status == "archived" {
		session.Lifecycle = manifest.LifecycleArchived
	} else {
		session.Lifecycle = "" // Active or stopped
	}

	// Set workspace and model
	session.Workspace = workspace
	session.Model = model

	// Set schema version
	session.SchemaVersion = "2.0"

	// Note: parent_session_id temporarily removed from SELECT due to schema migration issues
	// Will be re-added once migration 007 is reliably applied across all environments

	applyNullableScanFields(&session, permissionMode, permissionModeUpdatedAt, permissionModeSource, isTest, ctxTotalTokens, ctxUsedTokens, ctxPercentageUsed)
	if err := unmarshalContextTags(&session, contextTagsJSON); err != nil {
		return nil, err
	}
	if err := unmarshalMonitors(&session, monitorsJSON); err != nil {
		return nil, err
	}
	if err := unmarshalEngramMetadata(&session, metadataJSON); err != nil {
		return nil, err
	}
	return &session, nil
}

// applyNullableScanFields copies the nullable sql.Null* values from a Scan call
// into the corresponding manifest fields when valid.
func applyNullableScanFields(session *manifest.Manifest, permissionMode sql.NullString, permissionModeUpdatedAt sql.NullTime, permissionModeSource sql.NullString, isTest sql.NullBool, ctxTotalTokens, ctxUsedTokens sql.NullInt64, ctxPercentageUsed sql.NullFloat64) {
	if permissionMode.Valid {
		session.PermissionMode = permissionMode.String
	}
	if permissionModeUpdatedAt.Valid {
		session.PermissionModeUpdatedAt = &permissionModeUpdatedAt.Time
	}
	if permissionModeSource.Valid {
		session.PermissionModeSource = permissionModeSource.String
	}
	if isTest.Valid {
		session.IsTest = isTest.Bool
	}
	if ctxTotalTokens.Valid || ctxUsedTokens.Valid || ctxPercentageUsed.Valid {
		session.ContextUsage = &manifest.ContextUsage{}
		if ctxTotalTokens.Valid {
			session.ContextUsage.TotalTokens = int(ctxTotalTokens.Int64)
		}
		if ctxUsedTokens.Valid {
			session.ContextUsage.UsedTokens = int(ctxUsedTokens.Int64)
		}
		if ctxPercentageUsed.Valid {
			session.ContextUsage.PercentageUsed = ctxPercentageUsed.Float64
		}
	}
}

func unmarshalContextTags(session *manifest.Manifest, contextTagsJSON []byte) error {
	if len(contextTagsJSON) == 0 {
		return nil
	}
	if err := json.Unmarshal(contextTagsJSON, &session.Context.Tags); err != nil {
		return fmt.Errorf("failed to unmarshal context tags: %w", err)
	}
	return nil
}

func unmarshalMonitors(session *manifest.Manifest, monitorsJSON sql.NullString) error {
	if !monitorsJSON.Valid || monitorsJSON.String == "" {
		return nil
	}
	if err := json.Unmarshal([]byte(monitorsJSON.String), &session.Monitors); err != nil {
		return fmt.Errorf("failed to unmarshal monitors: %w", err)
	}
	return nil
}

func unmarshalEngramMetadata(session *manifest.Manifest, metadataJSON []byte) error {
	if len(metadataJSON) == 0 {
		return nil
	}
	var metadata map[string]any
	if err := json.Unmarshal(metadataJSON, &metadata); err != nil {
		return fmt.Errorf("failed to unmarshal metadata: %w", err)
	}
	enabled, ok := metadata["engram_enabled"].(bool)
	if !ok || !enabled {
		return nil
	}
	session.EngramMetadata = &manifest.EngramMetadata{Enabled: enabled}
	if query, ok := metadata["engram_query"].(string); ok {
		session.EngramMetadata.Query = query
	}
	if count, ok := metadata["engram_count"].(float64); ok {
		session.EngramMetadata.Count = int(count)
	}
	if ids, ok := metadata["engram_ids"].([]any); ok {
		session.EngramMetadata.EngramIDs = make([]string, len(ids))
		for i, id := range ids {
			if idStr, ok := id.(string); ok {
				session.EngramMetadata.EngramIDs[i] = idStr
			}
		}
	}
	if loadedAtStr, ok := metadata["engram_loaded_at"].(string); ok {
		if loadedAt, err := time.Parse(time.RFC3339, loadedAtStr); err == nil {
			session.EngramMetadata.LoadedAt = loadedAt
		}
	}
	return nil
}

// --- manifest.Store implementation (delegates to legacy methods) ---

// Create implements manifest.Store.
func (a *Adapter) Create(m *manifest.Manifest) error {
	return a.CreateSession(m)
}

// Get implements manifest.Store.
func (a *Adapter) Get(sessionID string) (*manifest.Manifest, error) {
	return a.GetSession(sessionID)
}

// Update implements manifest.Store.
func (a *Adapter) Update(m *manifest.Manifest) error {
	return a.UpdateSession(m)
}

// Delete implements manifest.Store.
func (a *Adapter) Delete(sessionID string) error {
	return a.DeleteSession(sessionID)
}

// List implements manifest.Store by converting manifest.Filter to SessionFilter.
func (a *Adapter) List(filter *manifest.Filter) ([]*manifest.Manifest, error) {
	if filter == nil {
		return a.ListSessions(nil)
	}
	sf := &SessionFilter{
		Workspace: filter.Workspace,
		Harness:   filter.Harness,
		Tags:      filter.Tags,
		Limit:     filter.Limit,
		Offset:    filter.Offset,
	}
	// Map Status to Lifecycle filter
	switch filter.Status {
	case "archived":
		sf.Lifecycle = manifest.LifecycleArchived
	case "active":
		sf.ExcludeArchived = true
	}
	return a.ListSessions(sf)
}
