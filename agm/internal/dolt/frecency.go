package dolt

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"math"
	"time"

	"github.com/vbonnet/dear-agent/agm/internal/manifest"
)

// FrecencyScore computes a zoxide-style frecency score from access count and
// last access time. The score is: frequency * recency_weight, where
// recency_weight decays based on elapsed time since last access.
//
// Decay brackets (matching zoxide):
//
//	< 1 hour:   weight = 4
//	< 1 day:    weight = 2
//	< 1 week:   weight = 1
//	< 1 month:  weight = 0.5
//	>= 1 month: weight = 0.25
func FrecencyScore(accessCount int, lastAccessedAt *time.Time, now time.Time) float64 {
	if accessCount == 0 || lastAccessedAt == nil {
		return 0
	}

	hours := now.Sub(*lastAccessedAt).Hours()

	var weight float64
	switch {
	case hours < 1:
		weight = 4.0
	case hours < 24:
		weight = 2.0
	case hours < 24*7:
		weight = 1.0
	case hours < 24*30:
		weight = 0.5
	default:
		weight = 0.25
	}

	return float64(accessCount) * weight
}

// UpdateAccess increments the access count and sets last_accessed_at to now
// for the given session.
func (a *Adapter) UpdateAccess(sessionID string) error {
	if sessionID == "" {
		return fmt.Errorf("session_id cannot be empty")
	}

	if err := a.ApplyMigrations(); err != nil {
		return fmt.Errorf("failed to apply migrations: %w", err)
	}

	query := `
		UPDATE agm_sessions
		SET access_count = access_count + 1,
		    last_accessed_at = ?
		WHERE id = ? AND workspace = ?
	`

	result, err := a.conn.Exec(query, time.Now(), sessionID, a.workspace)
	if err != nil {
		return fmt.Errorf("failed to update access: %w", err)
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

// FrecencyResult holds a session with its computed frecency score.
type FrecencyResult struct {
	Session *manifest.Manifest
	Score   float64
}

// GetByFrecency returns non-archived sessions ranked by frecency score
// (descending). Sessions that have never been accessed sort last.
// If limit <= 0, all matching sessions are returned.
func (a *Adapter) GetByFrecency(limit int) ([]FrecencyResult, error) {
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
			access_count, last_accessed_at
		FROM agm_sessions
		WHERE workspace = ? AND status != 'archived'
	`

	args := []any{a.workspace}

	if limit > 0 {
		query += " LIMIT ?"
		args = append(args, limit)
	}

	rows, err := a.conn.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query sessions by frecency: %w", err)
	}
	defer rows.Close()

	now := time.Now()
	var results []FrecencyResult

	for rows.Next() {
		session, accessCount, lastAccessedAt, err := scanSessionFrecency(rows)
		if err != nil {
			return nil, err
		}

		score := FrecencyScore(accessCount, lastAccessedAt, now)
		results = append(results, FrecencyResult{Session: session, Score: score})
	}

	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating rows: %w", err)
	}

	// Sort by score descending
	sortFrecencyResults(results)

	return results, nil
}

// sortFrecencyResults sorts by score descending using insertion sort.
func sortFrecencyResults(results []FrecencyResult) {
	for i := 1; i < len(results); i++ {
		for j := i; j > 0 && results[j].Score > results[j-1].Score; j-- {
			results[j], results[j-1] = results[j-1], results[j]
		}
	}
}

// scanSessionFrecency scans a row with the standard session columns plus
// access_count and last_accessed_at.
func scanSessionFrecency(row scanner) (*manifest.Manifest, int, *time.Time, error) {
	var m manifest.Manifest
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
	var accessCount int
	var lastAccessedAt sql.NullTime

	err := row.Scan(
		&m.SessionID,
		&m.CreatedAt,
		&m.UpdatedAt,
		&status,
		&workspace,
		&model,
		&m.Name,
		&m.Harness,
		&m.Context.Project,
		&m.Context.Purpose,
		&contextTagsJSON,
		&m.Context.Notes,
		&m.Claude.UUID,
		&m.Tmux.SessionName,
		&metadataJSON,
		&permissionMode,
		&permissionModeUpdatedAt,
		&permissionModeSource,
		&isTest,
		&ctxTotalTokens,
		&ctxUsedTokens,
		&ctxPercentageUsed,
		&accessCount,
		&lastAccessedAt,
	)
	if err != nil {
		return nil, 0, nil, fmt.Errorf("failed to scan session with frecency: %w", err)
	}

	if status == "archived" {
		m.Lifecycle = manifest.LifecycleArchived
	}
	m.Workspace = workspace
	m.Model = model
	m.SchemaVersion = "2.0"

	if permissionMode.Valid {
		m.PermissionMode = permissionMode.String
	}
	if permissionModeUpdatedAt.Valid {
		m.PermissionModeUpdatedAt = &permissionModeUpdatedAt.Time
	}
	if permissionModeSource.Valid {
		m.PermissionModeSource = permissionModeSource.String
	}
	if isTest.Valid {
		m.IsTest = isTest.Bool
	}
	if ctxTotalTokens.Valid || ctxUsedTokens.Valid || ctxPercentageUsed.Valid {
		m.ContextUsage = &manifest.ContextUsage{}
		if ctxTotalTokens.Valid {
			m.ContextUsage.TotalTokens = int(ctxTotalTokens.Int64)
		}
		if ctxUsedTokens.Valid {
			m.ContextUsage.UsedTokens = int(ctxUsedTokens.Int64)
		}
		if ctxPercentageUsed.Valid {
			m.ContextUsage.PercentageUsed = ctxPercentageUsed.Float64
		}
	}

	if len(contextTagsJSON) > 0 {
		if err := json.Unmarshal(contextTagsJSON, &m.Context.Tags); err != nil {
			return nil, 0, nil, fmt.Errorf("failed to unmarshal context tags: %w", err)
		}
	}
	if len(metadataJSON) > 0 {
		var metadata map[string]any
		if err := json.Unmarshal(metadataJSON, &metadata); err != nil {
			return nil, 0, nil, fmt.Errorf("failed to unmarshal metadata: %w", err)
		}
		if enabled, ok := metadata["engram_enabled"].(bool); ok && enabled {
			m.EngramMetadata = &manifest.EngramMetadata{Enabled: enabled}
			if query, ok := metadata["engram_query"].(string); ok {
				m.EngramMetadata.Query = query
			}
			if count, ok := metadata["engram_count"].(float64); ok {
				m.EngramMetadata.Count = int(count)
			}
			if ids, ok := metadata["engram_ids"].([]any); ok {
				m.EngramMetadata.EngramIDs = make([]string, len(ids))
				for i, id := range ids {
					if idStr, ok := id.(string); ok {
						m.EngramMetadata.EngramIDs[i] = idStr
					}
				}
			}
			if loadedAtStr, ok := metadata["engram_loaded_at"].(string); ok {
				if loadedAt, err := time.Parse(time.RFC3339, loadedAtStr); err == nil {
					m.EngramMetadata.LoadedAt = loadedAt
				}
			}
		}
	}

	var lap *time.Time
	if lastAccessedAt.Valid {
		lap = &lastAccessedAt.Time
	}

	return &m, accessCount, lap, nil
}

// RoundScore rounds a frecency score to the given number of decimal places.
func RoundScore(score float64, decimals int) float64 {
	pow := math.Pow(10, float64(decimals))
	return math.Round(score*pow) / pow
}
