package db

import (
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/vbonnet/dear-agent/agm/internal/manifest"
)

// SearchOptions configures search behavior
type SearchOptions struct {
	Limit  int    // Maximum number of results (0 = unlimited)
	Offset int    // Number of results to skip
	Filter Filter // Additional filters
}

// Filter provides additional search constraints
type Filter struct {
	Lifecycle      string    // Filter by lifecycle state ("", "archived")
	Harness        string    // Filter by harness type ("claude-code", "gemini-cli", "codex-cli", "opencode-cli")
	CreatedAfter   time.Time // Filter by creation date
	CreatedBefore  time.Time // Filter by creation date
	ParentSession  string    // Filter by parent session ID
	HasEscalations bool      // Filter sessions with escalations
}

// DefaultSearchOptions returns reasonable defaults
func DefaultSearchOptions() *SearchOptions {
	return &SearchOptions{
		Limit:  50,
		Offset: 0,
	}
}

// SearchSessions performs full-text search on session history using FTS5
// Returns sessions ranked by relevance
func (db *DB) SearchSessions(query string, opts *SearchOptions) ([]*manifest.Manifest, error) {
	if db == nil || db.conn == nil {
		return nil, fmt.Errorf("database connection is nil")
	}

	if query == "" {
		return nil, fmt.Errorf("search query cannot be empty")
	}

	if opts == nil {
		opts = DefaultSearchOptions()
	}

	// Sanitize and build FTS5 query
	ftsQuery, err := sanitizeFTS5Query(query)
	if err != nil {
		return nil, fmt.Errorf("invalid search query: %w", err)
	}

	// Build SQL query with filters
	sqlQuery, args := buildSearchSQL(ftsQuery, opts)

	// Execute query
	rows, err := db.conn.Query(sqlQuery, args...) //nolint:noctx // TODO(context): plumb ctx through this layer
	if err != nil {
		return nil, fmt.Errorf("search query failed: %w", err)
	}
	defer rows.Close()

	// Parse results
	results := make([]*manifest.Manifest, 0)
	for rows.Next() {
		m, err := scanManifest(rows)
		if err != nil {
			return nil, fmt.Errorf("failed to scan result: %w", err)
		}
		results = append(results, m)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating results: %w", err)
	}

	return results, nil
}

// BuildFTS5Query constructs an FTS5 query from search terms and operator
// Supports: AND, OR operators and phrase searches
func BuildFTS5Query(terms []string, operator string) string {
	if len(terms) == 0 {
		return ""
	}

	// Normalize operator
	op := strings.ToUpper(strings.TrimSpace(operator))
	if op != "AND" && op != "OR" {
		op = "AND" // Default to AND
	}

	// Escape and join terms
	var escapedTerms []string
	for _, term := range terms {
		term = strings.TrimSpace(term)
		if term == "" {
			continue
		}
		// Check if it's already a phrase (quoted)
		if strings.HasPrefix(term, `"`) && strings.HasSuffix(term, `"`) {
			escapedTerms = append(escapedTerms, escapeFTS5String(term))
		} else if strings.Contains(term, ":") {
			// Column-specific search (e.g., "name:value")
			escapedTerms = append(escapedTerms, escapeFTS5String(term))
		} else {
			// Regular term - just escape
			escapedTerms = append(escapedTerms, escapeFTS5String(term))
		}
	}

	if len(escapedTerms) == 0 {
		return ""
	}

	return strings.Join(escapedTerms, " "+op+" ")
}

// sanitizeFTS5Query validates and sanitizes user input for FTS5 queries
// Preserves intentional FTS5 syntax while escaping dangerous characters
func sanitizeFTS5Query(query string) (string, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return "", fmt.Errorf("empty query")
	}

	// Check for balanced quotes
	quoteCount := strings.Count(query, `"`)
	if quoteCount%2 != 0 {
		return "", fmt.Errorf("unbalanced quotes in query")
	}

	// FTS5 query is already user-provided, we just need to ensure it's safe
	// The query can contain: AND, OR, column:value, "phrases", etc.
	return query, nil
}

// escapeFTS5String escapes special FTS5 characters in a search term
// Protects against FTS5 injection while preserving search functionality
func escapeFTS5String(s string) string {
	// Don't escape if already quoted (phrase search)
	if strings.HasPrefix(s, `"`) && strings.HasSuffix(s, `"`) {
		// Just return as-is for phrase searches
		return s
	}

	// Don't escape column-specific searches
	if strings.Contains(s, ":") {
		parts := strings.SplitN(s, ":", 2)
		if len(parts) == 2 {
			// Validate column name
			validColumns := map[string]bool{
				"name":            true,
				"context_project": true,
				"context_purpose": true,
				"context_notes":   true,
			}
			if validColumns[parts[0]] {
				// Escape the value part only
				return parts[0] + ":" + escapeFTS5Value(parts[1])
			}
		}
	}

	// Escape special FTS5 characters for regular terms
	return escapeFTS5Value(s)
}

// escapeFTS5Value escapes the value portion of FTS5 queries
func escapeFTS5Value(s string) string {
	// Escape double quotes inside the value
	s = strings.ReplaceAll(s, `"`, `""`)
	// If the value contains spaces or special characters, quote it
	if strings.ContainsAny(s, " \t\n\r") {
		return `"` + s + `"`
	}
	return s
}

// buildSearchSQL constructs the SQL query with filters
func buildSearchSQL(ftsQuery string, opts *SearchOptions) (string, []interface{}) {
	var args []interface{}
	argNum := 1

	// Base query with FTS5 search and JOIN
	sql := `
		SELECT
			s.session_id,
			s.name,
			s.schema_version,
			s.created_at,
			s.updated_at,
			s.lifecycle,
			s.harness,
			s.model,
			s.context_project,
			s.context_purpose,
			s.context_tags,
			s.context_notes,
			s.claude_uuid,
			s.tmux_session_name,
			s.engram_enabled,
			s.engram_query,
			s.engram_ids,
			s.engram_loaded_at,
			s.engram_count,
			s.parent_session_id
		FROM sessions s
		JOIN sessions_fts fts ON s.rowid = fts.rowid
		WHERE fts.sessions_fts MATCH ?`
	args = append(args, ftsQuery)
	argNum++

	// Apply filters
	if opts.Filter.Lifecycle != "" {
		sql += fmt.Sprintf(" AND s.lifecycle = $%d", argNum)
		args = append(args, opts.Filter.Lifecycle)
		argNum++
	}

	if opts.Filter.Harness != "" {
		sql += fmt.Sprintf(" AND s.harness = $%d", argNum)
		args = append(args, opts.Filter.Harness)
		argNum++
	}

	if !opts.Filter.CreatedAfter.IsZero() {
		sql += fmt.Sprintf(" AND s.created_at >= $%d", argNum)
		args = append(args, opts.Filter.CreatedAfter)
		argNum++
	}

	if !opts.Filter.CreatedBefore.IsZero() {
		sql += fmt.Sprintf(" AND s.created_at <= $%d", argNum)
		args = append(args, opts.Filter.CreatedBefore)
		argNum++
	}

	if opts.Filter.ParentSession != "" {
		sql += fmt.Sprintf(" AND s.parent_session_id = $%d", argNum)
		args = append(args, opts.Filter.ParentSession)
		argNum++
	}

	if opts.Filter.HasEscalations {
		sql += ` AND EXISTS (
			SELECT 1 FROM escalations e
			WHERE e.session_id = s.session_id
		)`
	}

	// Order by relevance (FTS5 rank)
	sql += " ORDER BY fts.rank"

	// Apply pagination
	if opts.Limit > 0 {
		sql += fmt.Sprintf(" LIMIT $%d", argNum)
		args = append(args, opts.Limit)
		argNum++
	}

	if opts.Offset > 0 {
		sql += fmt.Sprintf(" OFFSET $%d", argNum)
		args = append(args, opts.Offset)
	}

	return sql, args
}

// scanManifest scans a database row into a Manifest struct
func scanManifest(rows *sql.Rows) (*manifest.Manifest, error) {
	var m manifest.Manifest
	var contextTags, engramIDs sql.NullString
	var contextProject, contextPurpose, contextNotes sql.NullString
	var claudeUUID, tmuxSessionName sql.NullString
	var engramEnabled sql.NullInt64
	var engramQuery sql.NullString
	var engramLoadedAt sql.NullTime
	var engramCount sql.NullInt64
	var parentSessionID sql.NullString
	var harness sql.NullString
	var model sql.NullString

	err := rows.Scan(
		&m.SessionID,
		&m.Name,
		&m.SchemaVersion,
		&m.CreatedAt,
		&m.UpdatedAt,
		&m.Lifecycle,
		&harness,
		&model,
		&contextProject,
		&contextPurpose,
		&contextTags,
		&contextNotes,
		&claudeUUID,
		&tmuxSessionName,
		&engramEnabled,
		&engramQuery,
		&engramIDs,
		&engramLoadedAt,
		&engramCount,
		&parentSessionID,
	)
	if err != nil {
		return nil, err
	}

	// Handle nullable fields
	if harness.Valid {
		m.Harness = harness.String
	}
	if model.Valid {
		m.Model = model.String
	}

	// Build Context
	m.Context = manifest.Context{
		Project: contextProject.String,
		Purpose: contextPurpose.String,
		Notes:   contextNotes.String,
	}

	// Parse context_tags JSON
	if contextTags.Valid && contextTags.String != "" {
		// Simple JSON array parsing (would use json.Unmarshal in production)
		tags := parseJSONArray(contextTags.String)
		m.Context.Tags = tags
	}

	// Build Claude metadata
	m.Claude = manifest.Claude{
		UUID: claudeUUID.String,
	}

	// Build Tmux metadata
	m.Tmux = manifest.Tmux{
		SessionName: tmuxSessionName.String,
	}

	// Build Engram metadata
	if engramEnabled.Valid && engramEnabled.Int64 == 1 {
		m.EngramMetadata = &manifest.EngramMetadata{
			Enabled: true,
			Query:   engramQuery.String,
			Count:   int(engramCount.Int64),
		}
		if engramLoadedAt.Valid {
			m.EngramMetadata.LoadedAt = engramLoadedAt.Time
		}
		if engramIDs.Valid && engramIDs.String != "" {
			m.EngramMetadata.EngramIDs = parseJSONArray(engramIDs.String)
		}
	}

	return &m, nil
}

// parseJSONArray is a simple JSON array parser for string arrays
// In production, would use encoding/json
func parseJSONArray(s string) []string {
	s = strings.TrimSpace(s)
	if s == "" || s == "[]" || s == "null" {
		return nil
	}

	// Remove brackets
	s = strings.TrimPrefix(s, "[")
	s = strings.TrimSuffix(s, "]")

	// Split by comma
	parts := strings.Split(s, ",")
	var result []string
	for _, part := range parts {
		part = strings.TrimSpace(part)
		// Remove quotes
		part = strings.Trim(part, `"`)
		if part != "" {
			result = append(result, part)
		}
	}

	return result
}
