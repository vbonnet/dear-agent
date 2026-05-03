// Package logs provides logs-related functionality.
package logs

import "time"

// Store is the interface for session log persistence.
// Logs are append-only — no Update or Delete.
type Store interface {
	// Append writes a new log entry.
	Append(entry *LogEntry) error

	// List retrieves log entries for a session.
	List(sessionID string, opts *ListOpts) ([]*LogEntry, error)
}

// LogEntry represents a single log record tied to a session.
type LogEntry struct {
	ID        string                 `json:"id"`
	SessionID string                 `json:"session_id"`
	Timestamp time.Time              `json:"timestamp"`
	Level     string                 `json:"level"`          // INFO, WARN, ERROR, CRITICAL
	Source    string                 `json:"source"`         // emitting subsystem
	Message   string                 `json:"message"`
	Data      map[string]interface{} `json:"data,omitempty"`
}

// ListOpts controls log listing behavior.
type ListOpts struct {
	MinLevel string    // Filter by minimum level
	Since    time.Time // Filter by start time
	Limit    int       // Max entries to return
	Offset   int       // Pagination offset
}
