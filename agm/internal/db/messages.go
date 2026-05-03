package db

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/vbonnet/dear-agent/agm/internal/conversation"
)

// MessageOptions provides filtering options for retrieving messages
type MessageOptions struct {
	Role    string    // Filter by role ("user", "assistant", or empty for all)
	Harness string    // Filter by harness ("claude-code", "gemini-cli", "codex-cli", or empty for all)
	After   time.Time // Only return messages after this timestamp
	Before  time.Time // Only return messages before this timestamp
	Limit   int       // Maximum number of messages to return (0 = no limit)
	Offset  int       // Number of messages to skip
}

// CreateMessage inserts a new message into the database
func (db *DB) CreateMessage(sessionID string, msg *conversation.Message) error {
	if sessionID == "" {
		return fmt.Errorf("session_id cannot be empty")
	}
	if msg == nil {
		return fmt.Errorf("message cannot be nil")
	}
	if msg.Role == "" {
		return fmt.Errorf("message role cannot be empty")
	}
	if msg.Harness == "" {
		return fmt.Errorf("message harness cannot be empty")
	}

	// Marshal content blocks to JSON
	contentJSON, err := json.Marshal(msg.Content)
	if err != nil {
		return fmt.Errorf("failed to marshal message content: %w", err)
	}

	// Extract token usage
	var inputTokens, outputTokens int
	if msg.Usage != nil {
		inputTokens = msg.Usage.InputTokens
		outputTokens = msg.Usage.OutputTokens
	}

	query := `
		INSERT INTO messages (
			session_id, timestamp, role, harness, content,
			input_tokens, output_tokens
		) VALUES (?, ?, ?, ?, ?, ?, ?)
	`

	_, err = db.conn.Exec(query, //nolint:noctx // TODO(context): plumb ctx through this layer
		sessionID,
		msg.Timestamp,
		msg.Role,
		msg.Harness,
		contentJSON,
		inputTokens,
		outputTokens,
	)

	if err != nil {
		return fmt.Errorf("failed to insert message: %w", err)
	}

	return nil
}

// GetMessages retrieves messages for a session with optional filtering
func (db *DB) GetMessages(sessionID string, opts *MessageOptions) ([]*conversation.Message, error) {
	if sessionID == "" {
		return nil, fmt.Errorf("session_id cannot be empty")
	}

	query := `
		SELECT message_id, timestamp, role, harness, content,
			input_tokens, output_tokens
		FROM messages
		WHERE session_id = ?
	`

	args := []interface{}{sessionID}

	// Apply filters
	if opts != nil {
		if opts.Role != "" {
			query += " AND role = ?"
			args = append(args, opts.Role)
		}

		if opts.Harness != "" {
			query += " AND harness = ?"
			args = append(args, opts.Harness)
		}

		if !opts.After.IsZero() {
			query += " AND timestamp > ?"
			args = append(args, opts.After)
		}

		if !opts.Before.IsZero() {
			query += " AND timestamp < ?"
			args = append(args, opts.Before)
		}
	}

	// Order by timestamp ascending (chronological order)
	query += " ORDER BY timestamp ASC"

	// Apply limit and offset
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

	rows, err := db.conn.Query(query, args...) //nolint:noctx // TODO(context): plumb ctx through this layer
	if err != nil {
		return nil, fmt.Errorf("failed to query messages: %w", err)
	}
	defer rows.Close()

	var messages []*conversation.Message
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

// DeleteMessages deletes all messages for a session
func (db *DB) DeleteMessages(sessionID string) error {
	if sessionID == "" {
		return fmt.Errorf("session_id cannot be empty")
	}

	query := `DELETE FROM messages WHERE session_id = ?`

	_, err := db.conn.Exec(query, sessionID) //nolint:noctx // TODO(context): plumb ctx through this layer
	if err != nil {
		return fmt.Errorf("failed to delete messages: %w", err)
	}

	// Note: We don't check rowsAffected because it's valid to delete zero messages
	// (e.g., if the session has no messages yet)

	return nil
}

// scanMessage scans a row into a Message struct
func scanMessage(row scanner) (*conversation.Message, error) {
	var msg conversation.Message
	var messageID int64
	var contentJSON []byte
	var inputTokens, outputTokens int

	err := row.Scan(
		&messageID,
		&msg.Timestamp,
		&msg.Role,
		&msg.Harness,
		&contentJSON,
		&inputTokens,
		&outputTokens,
	)

	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("message not found")
		}
		return nil, fmt.Errorf("failed to scan message: %w", err)
	}

	// Unmarshal content blocks
	if len(contentJSON) > 0 {
		// Use the Message's custom UnmarshalJSON which handles ContentBlock interface
		tempMsg := &conversation.Message{}
		msgBytes, err := json.Marshal(map[string]interface{}{
			"timestamp": msg.Timestamp,
			"role":      msg.Role,
			"harness":   msg.Harness,
			"content":   json.RawMessage(contentJSON),
		})
		if err != nil {
			return nil, fmt.Errorf("failed to prepare message for unmarshaling: %w", err)
		}

		if err := json.Unmarshal(msgBytes, tempMsg); err != nil {
			return nil, fmt.Errorf("failed to unmarshal message content: %w", err)
		}
		msg.Content = tempMsg.Content
	}

	// Reconstruct token usage if present
	if inputTokens > 0 || outputTokens > 0 {
		msg.Usage = &conversation.TokenUsage{
			InputTokens:  inputTokens,
			OutputTokens: outputTokens,
		}
	}

	return &msg, nil
}
