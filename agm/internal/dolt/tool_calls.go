package dolt

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// ToolCall represents a tool usage record in Dolt storage
type ToolCall struct {
	ID              string         `json:"id"`
	MessageID       string         `json:"message_id"`
	SessionID       string         `json:"session_id"`
	ToolName        string         `json:"tool_name"`
	Arguments       map[string]any `json:"arguments,omitempty"`
	Result          map[string]any `json:"result,omitempty"`
	Error           string         `json:"error,omitempty"`
	Timestamp       int64          `json:"timestamp"` // Unix timestamp in milliseconds
	ExecutionTimeMs int            `json:"execution_time_ms,omitempty"`
}

// CreateToolCall inserts a new tool call record into the database
func (a *Adapter) CreateToolCall(call *ToolCall) error {
	if call == nil {
		return fmt.Errorf("tool call cannot be nil")
	}
	if call.MessageID == "" {
		return fmt.Errorf("message_id cannot be empty")
	}
	if call.SessionID == "" {
		return fmt.Errorf("session_id cannot be empty")
	}
	if call.ToolName == "" {
		return fmt.Errorf("tool_name cannot be empty")
	}

	// Generate ID if not set
	if call.ID == "" {
		call.ID = uuid.New().String()
	}

	// Set timestamp if not set
	if call.Timestamp == 0 {
		call.Timestamp = time.Now().UnixMilli()
	}

	// Marshal arguments to JSON
	var argumentsJSON []byte
	var err error
	if len(call.Arguments) > 0 {
		argumentsJSON, err = json.Marshal(call.Arguments)
		if err != nil {
			return fmt.Errorf("failed to marshal arguments: %w", err)
		}
	}

	// Marshal result to JSON
	var resultJSON []byte
	if len(call.Result) > 0 {
		resultJSON, err = json.Marshal(call.Result)
		if err != nil {
			return fmt.Errorf("failed to marshal result: %w", err)
		}
	}

	query := `
		INSERT INTO agm_tool_calls (
			id, message_id, session_id, tool_name, arguments,
			result, error, timestamp, execution_time_ms
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
	`

	_, err = a.conn.Exec(query, //nolint:noctx // TODO(context): plumb ctx through this layer
		call.ID,
		call.MessageID,
		call.SessionID,
		call.ToolName,
		argumentsJSON,
		resultJSON,
		call.Error,
		call.Timestamp,
		call.ExecutionTimeMs,
	)

	if err != nil {
		return fmt.Errorf("failed to insert tool call: %w", err)
	}

	return nil
}

// CreateToolCalls inserts multiple tool calls in a batch
func (a *Adapter) CreateToolCalls(calls []*ToolCall) error {
	if len(calls) == 0 {
		return nil
	}

	// Start transaction for batch insert
	tx, err := a.conn.Begin() //nolint:noctx // TODO(context): plumb ctx through this layer
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	//nolint:noctx // TODO(context): plumb ctx through this layer
	stmt, err := tx.Prepare(`
		INSERT INTO agm_tool_calls (
			id, message_id, session_id, tool_name, arguments,
			result, error, timestamp, execution_time_ms
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
	`)
	if err != nil {
		return fmt.Errorf("failed to prepare statement: %w", err)
	}
	defer stmt.Close()

	for _, call := range calls {
		if call == nil {
			continue
		}

		// Generate ID if not set
		if call.ID == "" {
			call.ID = uuid.New().String()
		}

		// Set timestamp if not set
		if call.Timestamp == 0 {
			call.Timestamp = time.Now().UnixMilli()
		}

		// Marshal arguments to JSON
		var argumentsJSON []byte
		if len(call.Arguments) > 0 {
			argumentsJSON, err = json.Marshal(call.Arguments)
			if err != nil {
				return fmt.Errorf("failed to marshal arguments: %w", err)
			}
		}

		// Marshal result to JSON
		var resultJSON []byte
		if len(call.Result) > 0 {
			resultJSON, err = json.Marshal(call.Result)
			if err != nil {
				return fmt.Errorf("failed to marshal result: %w", err)
			}
		}

		_, err = stmt.Exec( //nolint:noctx // TODO(context): plumb ctx through this layer
			call.ID,
			call.MessageID,
			call.SessionID,
			call.ToolName,
			argumentsJSON,
			resultJSON,
			call.Error,
			call.Timestamp,
			call.ExecutionTimeMs,
		)

		if err != nil {
			return fmt.Errorf("failed to insert tool call %s: %w", call.ID, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	return nil
}

// GetToolCall retrieves a tool call by ID
func (a *Adapter) GetToolCall(toolCallID string) (*ToolCall, error) {
	if toolCallID == "" {
		return nil, fmt.Errorf("tool_call_id cannot be empty")
	}

	query := `
		SELECT id, message_id, session_id, tool_name, arguments,
			result, error, timestamp, execution_time_ms
		FROM agm_tool_calls
		WHERE id = ?
	`

	row := a.conn.QueryRow(query, toolCallID) //nolint:noctx // TODO(context): plumb ctx through this layer
	return scanToolCall(row)
}

// GetMessageToolCalls retrieves all tool calls for a message
func (a *Adapter) GetMessageToolCalls(messageID string) ([]*ToolCall, error) {
	if messageID == "" {
		return nil, fmt.Errorf("message_id cannot be empty")
	}

	query := `
		SELECT id, message_id, session_id, tool_name, arguments,
			result, error, timestamp, execution_time_ms
		FROM agm_tool_calls
		WHERE message_id = ?
		ORDER BY timestamp ASC
	`

	rows, err := a.conn.Query(query, messageID) //nolint:noctx // TODO(context): plumb ctx through this layer
	if err != nil {
		return nil, fmt.Errorf("failed to query tool calls: %w", err)
	}
	defer rows.Close()

	var calls []*ToolCall
	for rows.Next() {
		call, err := scanToolCall(rows)
		if err != nil {
			return nil, err
		}
		calls = append(calls, call)
	}

	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating rows: %w", err)
	}

	return calls, nil
}

// GetSessionToolCalls retrieves all tool calls for a session
func (a *Adapter) GetSessionToolCalls(sessionID string) ([]*ToolCall, error) {
	if sessionID == "" {
		return nil, fmt.Errorf("session_id cannot be empty")
	}

	query := `
		SELECT id, message_id, session_id, tool_name, arguments,
			result, error, timestamp, execution_time_ms
		FROM agm_tool_calls
		WHERE session_id = ?
		ORDER BY timestamp ASC
	`

	rows, err := a.conn.Query(query, sessionID) //nolint:noctx // TODO(context): plumb ctx through this layer
	if err != nil {
		return nil, fmt.Errorf("failed to query tool calls: %w", err)
	}
	defer rows.Close()

	var calls []*ToolCall
	for rows.Next() {
		call, err := scanToolCall(rows)
		if err != nil {
			return nil, err
		}
		calls = append(calls, call)
	}

	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating rows: %w", err)
	}

	return calls, nil
}

// GetToolCallStats returns statistics for tool usage in a session
func (a *Adapter) GetToolCallStats(sessionID string) (map[string]any, error) {
	if sessionID == "" {
		return nil, fmt.Errorf("session_id cannot be empty")
	}

	query := `
		SELECT
			tool_name,
			COUNT(*) as call_count,
			AVG(execution_time_ms) as avg_execution_time,
			SUM(CASE WHEN error IS NOT NULL AND error != '' THEN 1 ELSE 0 END) as error_count
		FROM agm_tool_calls
		WHERE session_id = ?
		GROUP BY tool_name
		ORDER BY call_count DESC
	`

	rows, err := a.conn.Query(query, sessionID) //nolint:noctx // TODO(context): plumb ctx through this layer
	if err != nil {
		return nil, fmt.Errorf("failed to query tool call stats: %w", err)
	}
	defer rows.Close()

	stats := make(map[string]any)
	toolStats := make([]map[string]any, 0)

	for rows.Next() {
		var toolName string
		var callCount int
		var avgExecutionTime sql.NullFloat64
		var errorCount int

		if err := rows.Scan(&toolName, &callCount, &avgExecutionTime, &errorCount); err != nil {
			return nil, fmt.Errorf("failed to scan tool stats: %w", err)
		}

		toolStat := map[string]any{
			"tool_name":   toolName,
			"call_count":  callCount,
			"error_count": errorCount,
		}

		if avgExecutionTime.Valid {
			toolStat["avg_execution_time_ms"] = avgExecutionTime.Float64
		}

		toolStats = append(toolStats, toolStat)
	}

	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating rows: %w", err)
	}

	stats["tools"] = toolStats
	stats["total_calls"] = len(toolStats)

	return stats, nil
}

// scanToolCall scans a database row into a ToolCall struct
func scanToolCall(row scanner) (*ToolCall, error) {
	var call ToolCall
	var argumentsJSON []byte
	var resultJSON []byte
	var errorStr sql.NullString
	var executionTimeMs sql.NullInt64

	err := row.Scan(
		&call.ID,
		&call.MessageID,
		&call.SessionID,
		&call.ToolName,
		&argumentsJSON,
		&resultJSON,
		&errorStr,
		&call.Timestamp,
		&executionTimeMs,
	)

	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("tool call not found")
		}
		return nil, fmt.Errorf("failed to scan tool call: %w", err)
	}

	// Unmarshal arguments
	if len(argumentsJSON) > 0 {
		if err := json.Unmarshal(argumentsJSON, &call.Arguments); err != nil {
			return nil, fmt.Errorf("failed to unmarshal arguments: %w", err)
		}
	}

	// Unmarshal result
	if len(resultJSON) > 0 {
		if err := json.Unmarshal(resultJSON, &call.Result); err != nil {
			return nil, fmt.Errorf("failed to unmarshal result: %w", err)
		}
	}

	// Set error if present
	if errorStr.Valid {
		call.Error = errorStr.String
	}

	// Set execution time if present
	if executionTimeMs.Valid {
		call.ExecutionTimeMs = int(executionTimeMs.Int64)
	}

	return &call, nil
}
