package dolt

import (
	"encoding/json"
	"fmt"

	"github.com/vbonnet/dear-agent/agm/internal/logs"
)

// LogAdapter implements logs.Store using Dolt.
type LogAdapter struct {
	adapter *Adapter
}

// Compile-time check that LogAdapter implements logs.Store.
var _ logs.Store = (*LogAdapter)(nil)

// NewLogAdapter creates a LogAdapter backed by the given Dolt Adapter.
func NewLogAdapter(adapter *Adapter) *LogAdapter {
	return &LogAdapter{adapter: adapter}
}

// Append writes a new log entry.
func (la *LogAdapter) Append(entry *logs.LogEntry) error {
	if entry == nil {
		return fmt.Errorf("log entry cannot be nil")
	}
	if entry.ID == "" {
		return fmt.Errorf("log entry id cannot be empty")
	}
	if entry.SessionID == "" {
		return fmt.Errorf("log entry session_id cannot be empty")
	}

	if err := la.adapter.ApplyMigrations(); err != nil {
		return fmt.Errorf("failed to apply migrations: %w", err)
	}

	var dataJSON interface{}
	if entry.Data != nil {
		b, err := json.Marshal(entry.Data)
		if err != nil {
			return fmt.Errorf("failed to marshal log data: %w", err)
		}
		dataJSON = string(b)
	}

	query := `
		INSERT INTO agm_session_logs (id, session_id, timestamp, level, source, message, data)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`

	_, err := la.adapter.conn.Exec(query, //nolint:noctx // TODO(context): plumb ctx through this layer
		entry.ID,
		entry.SessionID,
		entry.Timestamp,
		entry.Level,
		entry.Source,
		entry.Message,
		dataJSON,
	)
	if err != nil {
		return fmt.Errorf("failed to insert log entry: %w", err)
	}

	return nil
}

// List retrieves log entries for a session.
func (la *LogAdapter) List(sessionID string, opts *logs.ListOpts) ([]*logs.LogEntry, error) {
	if sessionID == "" {
		return nil, fmt.Errorf("session_id cannot be empty")
	}

	if err := la.adapter.ApplyMigrations(); err != nil {
		return nil, fmt.Errorf("failed to apply migrations: %w", err)
	}

	query := `
		SELECT id, session_id, timestamp, level, source, message, data
		FROM agm_session_logs
		WHERE session_id = ?
	`
	args := []any{sessionID}

	if opts != nil {
		if opts.MinLevel != "" {
			query += " AND level = ?"
			args = append(args, opts.MinLevel)
		}
		if !opts.Since.IsZero() {
			query += " AND timestamp >= ?"
			args = append(args, opts.Since)
		}
	}

	query += " ORDER BY timestamp ASC"

	if opts != nil {
		if opts.Limit > 0 {
			query += " LIMIT ?"
			args = append(args, opts.Limit)
		}
		if opts.Offset > 0 {
			query += " OFFSET ?"
			args = append(args, opts.Offset)
		}
	}

	rows, err := la.adapter.conn.Query(query, args...) //nolint:noctx // TODO(context): plumb ctx through this layer
	if err != nil {
		return nil, fmt.Errorf("failed to query log entries: %w", err)
	}
	defer rows.Close()

	var entries []*logs.LogEntry
	for rows.Next() {
		var entry logs.LogEntry
		var dataJSON []byte

		err := rows.Scan(
			&entry.ID,
			&entry.SessionID,
			&entry.Timestamp,
			&entry.Level,
			&entry.Source,
			&entry.Message,
			&dataJSON,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan log entry: %w", err)
		}

		if len(dataJSON) > 0 {
			if err := json.Unmarshal(dataJSON, &entry.Data); err != nil {
				return nil, fmt.Errorf("failed to unmarshal log data: %w", err)
			}
		}

		entries = append(entries, &entry)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating log rows: %w", err)
	}

	return entries, nil
}
