package dolt

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// Message represents a conversation message in Dolt storage
type Message struct {
	ID             string         `json:"id"`
	SessionID      string         `json:"session_id"`
	Role           string         `json:"role"`      // user, assistant, system
	Content        string         `json:"content"`   // JSON-encoded content blocks
	Timestamp      int64          `json:"timestamp"` // Unix timestamp in milliseconds
	SequenceNumber int            `json:"sequence_number"`
	Harness        string         `json:"harness,omitempty"`
	InputTokens    int            `json:"input_tokens,omitempty"`
	OutputTokens   int            `json:"output_tokens,omitempty"`
	Metadata       map[string]any `json:"metadata,omitempty"`
}

// CreateMessage inserts a new message into the database
func (a *Adapter) CreateMessage(msg *Message) error {
	if msg == nil {
		return fmt.Errorf("message cannot be nil")
	}
	if msg.SessionID == "" {
		return fmt.Errorf("session_id cannot be empty")
	}
	if msg.Content == "" {
		return fmt.Errorf("content cannot be empty")
	}

	// Generate ID if not set
	if msg.ID == "" {
		msg.ID = uuid.New().String()
	}

	// Set timestamp if not set
	if msg.Timestamp == 0 {
		msg.Timestamp = time.Now().UnixMilli()
	}

	// Marshal metadata to JSON
	var metadataJSON []byte
	var err error
	if len(msg.Metadata) > 0 {
		metadataJSON, err = json.Marshal(msg.Metadata)
		if err != nil {
			return fmt.Errorf("failed to marshal metadata: %w", err)
		}
	}

	query := `
		INSERT INTO agm_messages (
			id, session_id, role, content, timestamp, sequence_number,
			harness, input_tokens, output_tokens, metadata
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`

	_, err = a.conn.Exec(query,
		msg.ID,
		msg.SessionID,
		msg.Role,
		msg.Content,
		msg.Timestamp,
		msg.SequenceNumber,
		msg.Harness,
		msg.InputTokens,
		msg.OutputTokens,
		metadataJSON,
	)

	if err != nil {
		return fmt.Errorf("failed to insert message: %w", err)
	}

	return nil
}

// CreateMessages inserts multiple messages in a batch
func (a *Adapter) CreateMessages(messages []*Message) error {
	if len(messages) == 0 {
		return nil
	}

	// Start transaction for batch insert
	tx, err := a.conn.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	stmt, err := tx.Prepare(`
		INSERT INTO agm_messages (
			id, session_id, role, content, timestamp, sequence_number,
			harness, input_tokens, output_tokens, metadata
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`)
	if err != nil {
		return fmt.Errorf("failed to prepare statement: %w", err)
	}
	defer stmt.Close()

	for _, msg := range messages {
		if msg == nil {
			continue
		}

		// Generate ID if not set
		if msg.ID == "" {
			msg.ID = uuid.New().String()
		}

		// Set timestamp if not set
		if msg.Timestamp == 0 {
			msg.Timestamp = time.Now().UnixMilli()
		}

		// Marshal metadata to JSON
		var metadataJSON []byte
		if len(msg.Metadata) > 0 {
			metadataJSON, err = json.Marshal(msg.Metadata)
			if err != nil {
				return fmt.Errorf("failed to marshal metadata: %w", err)
			}
		}

		_, err = stmt.Exec(
			msg.ID,
			msg.SessionID,
			msg.Role,
			msg.Content,
			msg.Timestamp,
			msg.SequenceNumber,
			msg.Harness,
			msg.InputTokens,
			msg.OutputTokens,
			metadataJSON,
		)

		if err != nil {
			return fmt.Errorf("failed to insert message %s: %w", msg.ID, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	return nil
}

// GetMessage retrieves a message by ID
func (a *Adapter) GetMessage(messageID string) (*Message, error) {
	if messageID == "" {
		return nil, fmt.Errorf("message_id cannot be empty")
	}

	query := `
		SELECT id, session_id, role, content, timestamp, sequence_number,
			harness, input_tokens, output_tokens, metadata
		FROM agm_messages
		WHERE id = ?
	`

	row := a.conn.QueryRow(query, messageID)
	return scanMessage(row)
}

// GetSessionMessages retrieves all messages for a session
func (a *Adapter) GetSessionMessages(sessionID string) ([]*Message, error) {
	if sessionID == "" {
		return nil, fmt.Errorf("session_id cannot be empty")
	}

	query := `
		SELECT id, session_id, role, content, timestamp, sequence_number,
			harness, input_tokens, output_tokens, metadata
		FROM agm_messages
		WHERE session_id = ?
		ORDER BY sequence_number ASC
	`

	rows, err := a.conn.Query(query, sessionID)
	if err != nil {
		return nil, fmt.Errorf("failed to query messages: %w", err)
	}
	defer rows.Close()

	var messages []*Message
	for rows.Next() {
		msg, err := scanMessage(rows)
		if err != nil {
			return nil, err
		}
		messages = append(messages, msg)
	}

	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating rows: %w", err)
	}

	return messages, nil
}

// UpdateMessage updates an existing message
func (a *Adapter) UpdateMessage(msg *Message) error {
	if msg == nil {
		return fmt.Errorf("message cannot be nil")
	}
	if msg.ID == "" {
		return fmt.Errorf("message_id cannot be empty")
	}

	// Marshal metadata to JSON
	var metadataJSON []byte
	var err error
	if len(msg.Metadata) > 0 {
		metadataJSON, err = json.Marshal(msg.Metadata)
		if err != nil {
			return fmt.Errorf("failed to marshal metadata: %w", err)
		}
	}

	query := `
		UPDATE agm_messages
		SET role = ?, content = ?, harness = ?,
			input_tokens = ?, output_tokens = ?, metadata = ?
		WHERE id = ?
	`

	result, err := a.conn.Exec(query,
		msg.Role,
		msg.Content,
		msg.Harness,
		msg.InputTokens,
		msg.OutputTokens,
		metadataJSON,
		msg.ID,
	)

	if err != nil {
		return fmt.Errorf("failed to update message: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rowsAffected == 0 {
		return fmt.Errorf("message not found: %s", msg.ID)
	}

	return nil
}

// DeleteMessage deletes a message from the database
func (a *Adapter) DeleteMessage(messageID string) error {
	if messageID == "" {
		return fmt.Errorf("message_id cannot be empty")
	}

	query := `DELETE FROM agm_messages WHERE id = ?`

	result, err := a.conn.Exec(query, messageID)
	if err != nil {
		return fmt.Errorf("failed to delete message: %w", err)
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

// DeleteSessionMessages deletes all messages for a session
func (a *Adapter) DeleteSessionMessages(sessionID string) error {
	if sessionID == "" {
		return fmt.Errorf("session_id cannot be empty")
	}

	query := `DELETE FROM agm_messages WHERE session_id = ?`

	_, err := a.conn.Exec(query, sessionID)
	if err != nil {
		return fmt.Errorf("failed to delete session messages: %w", err)
	}

	return nil
}

// scanMessage scans a database row into a Message struct
func scanMessage(row scanner) (*Message, error) {
	var msg Message
	var metadataJSON []byte

	err := row.Scan(
		&msg.ID,
		&msg.SessionID,
		&msg.Role,
		&msg.Content,
		&msg.Timestamp,
		&msg.SequenceNumber,
		&msg.Harness,
		&msg.InputTokens,
		&msg.OutputTokens,
		&metadataJSON,
	)

	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("message not found")
		}
		return nil, fmt.Errorf("failed to scan message: %w", err)
	}

	// Unmarshal metadata
	if len(metadataJSON) > 0 {
		if err := json.Unmarshal(metadataJSON, &msg.Metadata); err != nil {
			return nil, fmt.Errorf("failed to unmarshal metadata: %w", err)
		}
	}

	return &msg, nil
}
