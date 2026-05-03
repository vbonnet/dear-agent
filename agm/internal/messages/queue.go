// Package messages provides messages functionality.
package messages

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"time"

	_ "github.com/mattn/go-sqlite3" // sqlite3 driver
)

// QueueEntry represents a queued message waiting for delivery
type QueueEntry struct {
	ID           int64
	MessageID    string
	From         string
	To           string
	Message      string
	Priority     string
	QueuedAt     time.Time
	AttemptCount int
	LastAttempt  *time.Time
	Status       string // queued|delivered|failed
	AckRequired  bool
	AckReceived  bool
	AckTimeout   *time.Time
}

// MessageQueue manages queued messages in SQLite database
type MessageQueue struct {
	db *sql.DB
}

// Priority constants
const (
	PriorityCritical = "CRITICAL"
	PriorityHigh     = "HIGH"
	PriorityMedium   = "MEDIUM"
	PriorityLow      = "LOW"
)

// Status constants
const (
	StatusQueued    = "queued"
	StatusDelivered = "delivered"
	StatusFailed    = "failed"
)

// NewMessageQueue creates or opens the message queue database
// Database location: ~/.config/agm/message_queue.db
func NewMessageQueue() (*MessageQueue, error) {
	// Get config directory
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("failed to get home directory: %w", err)
	}

	configDir := filepath.Join(homeDir, ".config", "agm")
	if err := os.MkdirAll(configDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create config directory: %w", err)
	}

	dbPath := filepath.Join(configDir, "message_queue.db")

	// Open database with WAL mode for concurrent access
	db, err := sql.Open("sqlite3", fmt.Sprintf("%s?_journal_mode=WAL&_timeout=5000", dbPath))
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	// Create base table schema (without ack columns for backwards compatibility)
	schema := `
	CREATE TABLE IF NOT EXISTS message_queue (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		message_id TEXT UNIQUE NOT NULL,
		from_session TEXT NOT NULL,
		to_session TEXT NOT NULL,
		message TEXT NOT NULL,
		priority TEXT NOT NULL DEFAULT 'MEDIUM',
		queued_at TIMESTAMP NOT NULL,
		attempt_count INTEGER NOT NULL DEFAULT 0,
		last_attempt TIMESTAMP,
		status TEXT NOT NULL DEFAULT 'queued',
		created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
	);

	CREATE INDEX IF NOT EXISTS idx_to_session_status ON message_queue(to_session, status);
	CREATE INDEX IF NOT EXISTS idx_status ON message_queue(status);
	CREATE INDEX IF NOT EXISTS idx_priority ON message_queue(priority);
	CREATE INDEX IF NOT EXISTS idx_queued_at ON message_queue(queued_at);
	`

	if _, err := db.Exec(schema); err != nil {
		return nil, fmt.Errorf("failed to create schema: %w", err)
	}

	// Migrate existing databases to add ack columns if they don't exist
	// This is safe because we're only adding columns with default values
	// We ignore errors since the column might already exist
	db.Exec(`ALTER TABLE message_queue ADD COLUMN ack_required INTEGER NOT NULL DEFAULT 1;`)
	db.Exec(`ALTER TABLE message_queue ADD COLUMN ack_received INTEGER NOT NULL DEFAULT 0;`)
	db.Exec(`ALTER TABLE message_queue ADD COLUMN ack_timeout TIMESTAMP;`)

	// Create index if it doesn't exist (idempotent) - must be after ALTER TABLE
	db.Exec(`CREATE INDEX IF NOT EXISTS idx_ack_required ON message_queue(ack_required, ack_received);`)

	return &MessageQueue{db: db}, nil
}

// Close closes the database connection
func (q *MessageQueue) Close() error {
	if q.db != nil {
		return q.db.Close()
	}
	return nil
}

// Enqueue adds a message to the queue
func (q *MessageQueue) Enqueue(entry *QueueEntry) error {
	_, err := q.db.Exec(`
		INSERT INTO message_queue (message_id, from_session, to_session, message, priority, queued_at, status)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`, entry.MessageID, entry.From, entry.To, entry.Message, entry.Priority, entry.QueuedAt, StatusQueued)

	if err != nil {
		return fmt.Errorf("failed to enqueue message: %w", err)
	}

	return nil
}

// GetPending returns all queued messages for a session, ordered by priority then time
func (q *MessageQueue) GetPending(sessionName string) ([]*QueueEntry, error) {
	// Priority order: CRITICAL > HIGH > MEDIUM > LOW
	// Then by queued_at (oldest first)
	query := `
		SELECT id, message_id, from_session, to_session, message, priority, queued_at, attempt_count, last_attempt
		FROM message_queue
		WHERE to_session = ? AND status = ?
		ORDER BY
			CASE priority
				WHEN 'CRITICAL' THEN 1
				WHEN 'HIGH' THEN 2
				WHEN 'MEDIUM' THEN 3
				WHEN 'LOW' THEN 4
				ELSE 5
			END,
			queued_at ASC
	`

	rows, err := q.db.Query(query, sessionName, StatusQueued)
	if err != nil {
		return nil, fmt.Errorf("failed to query pending messages: %w", err)
	}
	defer rows.Close()

	var entries []*QueueEntry
	for rows.Next() {
		e := &QueueEntry{}
		var lastAttempt sql.NullTime

		err := rows.Scan(
			&e.ID,
			&e.MessageID,
			&e.From,
			&e.To,
			&e.Message,
			&e.Priority,
			&e.QueuedAt,
			&e.AttemptCount,
			&lastAttempt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan row: %w", err)
		}

		if lastAttempt.Valid {
			e.LastAttempt = &lastAttempt.Time
		}

		entries = append(entries, e)
	}

	return entries, rows.Err()
}

// GetAllPending returns all queued messages across all sessions, ordered by priority then time
func (q *MessageQueue) GetAllPending() ([]*QueueEntry, error) {
	// Priority order: CRITICAL > HIGH > MEDIUM > LOW
	// Then by queued_at (oldest first)
	query := `
		SELECT id, message_id, from_session, to_session, message, priority, queued_at, attempt_count, last_attempt
		FROM message_queue
		WHERE status = ?
		ORDER BY
			CASE priority
				WHEN 'CRITICAL' THEN 1
				WHEN 'HIGH' THEN 2
				WHEN 'MEDIUM' THEN 3
				WHEN 'LOW' THEN 4
				ELSE 5
			END,
			queued_at ASC
	`

	rows, err := q.db.Query(query, StatusQueued)
	if err != nil {
		return nil, fmt.Errorf("failed to query all pending messages: %w", err)
	}
	defer rows.Close()

	var entries []*QueueEntry
	for rows.Next() {
		e := &QueueEntry{}
		var lastAttempt sql.NullTime

		err := rows.Scan(
			&e.ID,
			&e.MessageID,
			&e.From,
			&e.To,
			&e.Message,
			&e.Priority,
			&e.QueuedAt,
			&e.AttemptCount,
			&lastAttempt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan row: %w", err)
		}

		if lastAttempt.Valid {
			e.LastAttempt = &lastAttempt.Time
		}

		entries = append(entries, e)
	}

	return entries, rows.Err()
}

// MarkDelivered updates a message status to delivered
func (q *MessageQueue) MarkDelivered(messageID string) error {
	result, err := q.db.Exec(`
		UPDATE message_queue
		SET status = ?, last_attempt = ?
		WHERE message_id = ?
	`, StatusDelivered, time.Now(), messageID)

	if err != nil {
		return fmt.Errorf("failed to mark message as delivered: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rowsAffected == 0 {
		return fmt.Errorf("message not found: %s", messageID)
	}

	return nil
}

// MarkFailed increments attempt count and updates last attempt time
func (q *MessageQueue) MarkFailed(messageID string) error {
	result, err := q.db.Exec(`
		UPDATE message_queue
		SET attempt_count = attempt_count + 1, last_attempt = ?
		WHERE message_id = ?
	`, time.Now(), messageID)

	if err != nil {
		return fmt.Errorf("failed to mark message as failed: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rowsAffected == 0 {
		return fmt.Errorf("message not found: %s", messageID)
	}

	return nil
}

// IncrementAttempt increments the attempt count for a message
func (q *MessageQueue) IncrementAttempt(messageID string) error {
	result, err := q.db.Exec(`
		UPDATE message_queue
		SET attempt_count = attempt_count + 1, last_attempt = ?
		WHERE message_id = ?
	`, time.Now(), messageID)

	if err != nil {
		return fmt.Errorf("failed to increment attempt: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rowsAffected == 0 {
		return fmt.Errorf("message not found: %s", messageID)
	}

	return nil
}

// MarkPermanentlyFailed marks a message as permanently failed (after max retries)
func (q *MessageQueue) MarkPermanentlyFailed(messageID string) error {
	result, err := q.db.Exec(`
		UPDATE message_queue
		SET status = ?, last_attempt = ?
		WHERE message_id = ?
	`, StatusFailed, time.Now(), messageID)

	if err != nil {
		return fmt.Errorf("failed to mark message as permanently failed: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rowsAffected == 0 {
		return fmt.Errorf("message not found: %s", messageID)
	}

	return nil
}

// CleanupOld deletes delivered and failed messages older than retentionDays
func (q *MessageQueue) CleanupOld(retentionDays int) (int64, error) {
	cutoffTime := time.Now().AddDate(0, 0, -retentionDays)

	result, err := q.db.Exec(`
		DELETE FROM message_queue
		WHERE status IN (?, ?) AND queued_at < ?
	`, StatusDelivered, StatusFailed, cutoffTime)

	if err != nil {
		return 0, fmt.Errorf("failed to cleanup old messages: %w", err)
	}

	rowsDeleted, err := result.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("failed to get rows deleted: %w", err)
	}

	return rowsDeleted, nil
}

// GetStats returns queue statistics
func (q *MessageQueue) GetStats() (map[string]int, error) {
	stats := make(map[string]int)

	// Count by status
	rows, err := q.db.Query(`
		SELECT status, COUNT(*) as count
		FROM message_queue
		GROUP BY status
	`)
	if err != nil {
		return nil, fmt.Errorf("failed to query stats: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var status string
		var count int
		if err := rows.Scan(&status, &count); err != nil {
			return nil, err
		}
		stats[status] = count
	}

	return stats, rows.Err()
}

// MarkAcknowledged marks a message as acknowledged
func (q *MessageQueue) MarkAcknowledged(messageID string) error {
	result, err := q.db.Exec(`
		UPDATE message_queue
		SET ack_received = 1
		WHERE message_id = ?
	`, messageID)

	if err != nil {
		return fmt.Errorf("failed to mark message as acknowledged: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rowsAffected == 0 {
		return fmt.Errorf("message not found: %s", messageID)
	}

	return nil
}

// MarkTimeout marks a message as having timed out waiting for acknowledgment
func (q *MessageQueue) MarkTimeout(messageID string) error {
	result, err := q.db.Exec(`
		UPDATE message_queue
		SET ack_timeout = ?
		WHERE message_id = ?
	`, time.Now(), messageID)

	if err != nil {
		return fmt.Errorf("failed to mark timeout: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rowsAffected == 0 {
		return fmt.Errorf("message not found: %s", messageID)
	}

	return nil
}

// GetByMessageID retrieves a specific message by its ID
func (q *MessageQueue) GetByMessageID(messageID string) (*QueueEntry, error) {
	query := `
		SELECT id, message_id, from_session, to_session, message, priority,
		       queued_at, attempt_count, last_attempt, status,
		       ack_required, ack_received, ack_timeout
		FROM message_queue
		WHERE message_id = ?
	`

	var e QueueEntry
	var lastAttempt sql.NullTime
	var ackTimeout sql.NullTime
	var ackRequired, ackReceived int

	err := q.db.QueryRow(query, messageID).Scan(
		&e.ID,
		&e.MessageID,
		&e.From,
		&e.To,
		&e.Message,
		&e.Priority,
		&e.QueuedAt,
		&e.AttemptCount,
		&lastAttempt,
		&e.Status,
		&ackRequired,
		&ackReceived,
		&ackTimeout,
	)

	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("message not found: %s", messageID)
		}
		return nil, fmt.Errorf("failed to query message: %w", err)
	}

	if lastAttempt.Valid {
		e.LastAttempt = &lastAttempt.Time
	}

	if ackTimeout.Valid {
		e.AckTimeout = &ackTimeout.Time
	}

	e.AckRequired = ackRequired == 1
	e.AckReceived = ackReceived == 1

	return &e, nil
}

// GetDLQ returns all messages in the dead letter queue (permanently failed)
func (q *MessageQueue) GetDLQ() ([]*QueueEntry, error) {
	query := `
		SELECT id, message_id, from_session, to_session, message, priority,
		       queued_at, attempt_count, last_attempt, status,
		       ack_required, ack_received, ack_timeout
		FROM message_queue
		WHERE status = ?
		ORDER BY queued_at DESC
	`

	rows, err := q.db.Query(query, StatusFailed)
	if err != nil {
		return nil, fmt.Errorf("failed to query DLQ: %w", err)
	}
	defer rows.Close()

	var entries []*QueueEntry
	for rows.Next() {
		e := &QueueEntry{}
		var lastAttempt sql.NullTime
		var ackTimeout sql.NullTime
		var ackRequired, ackReceived int

		err := rows.Scan(
			&e.ID,
			&e.MessageID,
			&e.From,
			&e.To,
			&e.Message,
			&e.Priority,
			&e.QueuedAt,
			&e.AttemptCount,
			&lastAttempt,
			&e.Status,
			&ackRequired,
			&ackReceived,
			&ackTimeout,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan row: %w", err)
		}

		if lastAttempt.Valid {
			e.LastAttempt = &lastAttempt.Time
		}

		if ackTimeout.Valid {
			e.AckTimeout = &ackTimeout.Time
		}

		e.AckRequired = ackRequired == 1
		e.AckReceived = ackReceived == 1

		entries = append(entries, e)
	}

	return entries, rows.Err()
}

// RetryRecentlyFailed resets recently failed messages back to queued status.
// Called on daemon restart to give failed messages another chance.
// Only resets messages that failed within the given duration (e.g., last 24 hours).
func (q *MessageQueue) RetryRecentlyFailed(within time.Duration) (int64, error) {
	cutoff := time.Now().Add(-within)

	result, err := q.db.Exec(`
		UPDATE message_queue
		SET status = ?, attempt_count = 0, last_attempt = NULL
		WHERE status = ? AND last_attempt > ?
	`, StatusQueued, StatusFailed, cutoff)

	if err != nil {
		return 0, fmt.Errorf("failed to retry recently failed messages: %w", err)
	}

	return result.RowsAffected()
}

// GetQueueList returns all messages with optional status filter, ordered by queued_at desc
func (q *MessageQueue) GetQueueList(statusFilter string, limit int) ([]*QueueEntry, error) {
	var query string
	var args []interface{}

	if statusFilter != "" {
		query = `
			SELECT id, message_id, from_session, to_session, message, priority,
			       queued_at, attempt_count, last_attempt, status,
			       ack_required, ack_received, ack_timeout
			FROM message_queue
			WHERE status = ?
			ORDER BY queued_at DESC
			LIMIT ?
		`
		args = []interface{}{statusFilter, limit}
	} else {
		query = `
			SELECT id, message_id, from_session, to_session, message, priority,
			       queued_at, attempt_count, last_attempt, status,
			       ack_required, ack_received, ack_timeout
			FROM message_queue
			ORDER BY queued_at DESC
			LIMIT ?
		`
		args = []interface{}{limit}
	}

	rows, err := q.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query queue list: %w", err)
	}
	defer rows.Close()

	var entries []*QueueEntry
	for rows.Next() {
		e := &QueueEntry{}
		var lastAttempt sql.NullTime
		var ackTimeout sql.NullTime
		var ackRequired, ackReceived int

		err := rows.Scan(
			&e.ID,
			&e.MessageID,
			&e.From,
			&e.To,
			&e.Message,
			&e.Priority,
			&e.QueuedAt,
			&e.AttemptCount,
			&lastAttempt,
			&e.Status,
			&ackRequired,
			&ackReceived,
			&ackTimeout,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan row: %w", err)
		}

		if lastAttempt.Valid {
			e.LastAttempt = &lastAttempt.Time
		}
		if ackTimeout.Valid {
			e.AckTimeout = &ackTimeout.Time
		}
		e.AckRequired = ackRequired == 1
		e.AckReceived = ackReceived == 1

		entries = append(entries, e)
	}

	return entries, rows.Err()
}

// GetUnacknowledged returns all messages that require ack but haven't received it
func (q *MessageQueue) GetUnacknowledged() ([]*QueueEntry, error) {
	query := `
		SELECT id, message_id, from_session, to_session, message, priority,
		       queued_at, attempt_count, last_attempt, status,
		       ack_required, ack_received, ack_timeout
		FROM message_queue
		WHERE ack_required = 1 AND ack_received = 0 AND status = ?
		ORDER BY queued_at ASC
	`

	rows, err := q.db.Query(query, StatusDelivered)
	if err != nil {
		return nil, fmt.Errorf("failed to query unacknowledged: %w", err)
	}
	defer rows.Close()

	var entries []*QueueEntry
	for rows.Next() {
		e := &QueueEntry{}
		var lastAttempt sql.NullTime
		var ackTimeout sql.NullTime
		var ackRequired, ackReceived int

		err := rows.Scan(
			&e.ID,
			&e.MessageID,
			&e.From,
			&e.To,
			&e.Message,
			&e.Priority,
			&e.QueuedAt,
			&e.AttemptCount,
			&lastAttempt,
			&e.Status,
			&ackRequired,
			&ackReceived,
			&ackTimeout,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan row: %w", err)
		}

		if lastAttempt.Valid {
			e.LastAttempt = &lastAttempt.Time
		}

		if ackTimeout.Valid {
			e.AckTimeout = &ackTimeout.Time
		}

		e.AckRequired = ackRequired == 1
		e.AckReceived = ackReceived == 1

		entries = append(entries, e)
	}

	return entries, rows.Err()
}

// RenameSession updates all message queue entries that reference oldName
// in the from_session or to_session fields to use newName instead.
// Returns the total number of rows updated.
func (q *MessageQueue) RenameSession(oldName, newName string) (int, error) {
	// Update to_session
	result1, err := q.db.Exec(`
		UPDATE message_queue SET to_session = ? WHERE to_session = ?
	`, newName, oldName)
	if err != nil {
		return 0, fmt.Errorf("failed to update to_session: %w", err)
	}
	count1, _ := result1.RowsAffected()

	// Update from_session
	result2, err := q.db.Exec(`
		UPDATE message_queue SET from_session = ? WHERE from_session = ?
	`, newName, oldName)
	if err != nil {
		return int(count1), fmt.Errorf("failed to update from_session: %w", err)
	}
	count2, _ := result2.RowsAffected()

	return int(count1 + count2), nil
}
