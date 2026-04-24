// Package heartbeat provides state tracking for tool use sequences.
// It enables heartbeat output during silent batch operations and long-running commands.
package heartbeat

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

// fileOps are tool names that count as file operations for batch tracking.
var fileOps = map[string]bool{
	"Edit":         true,
	"Write":        true,
	"Read":         true,
	"NotebookEdit": true,
}

// BatchState tracks consecutive file operations.
type BatchState struct {
	Count          int       `json:"count"`
	ToolType       string    `json:"tool_type"`
	StartedAt      time.Time `json:"started_at"`
	LastFile       string    `json:"last_file"`
	FilesProcessed []string  `json:"files_processed"`
}

// OperationState tracks a long-running operation's start time.
type OperationState struct {
	ToolName       string    `json:"tool_name"`
	StartedAt      time.Time `json:"started_at"`
	CommandPreview string    `json:"command_preview,omitempty"`
}

// TaskTrackingState tracks task tool usage for heartbeat display.
type TaskTrackingState struct {
	TotalCreated    int       `json:"total_created"`
	LastTaskSubject string    `json:"last_task_subject,omitempty"`
	LastUpdatedAt   time.Time `json:"last_updated_at,omitempty"`
}

// State holds both batch and operation tracking state.
type State struct {
	Batch     *BatchState        `json:"batch,omitempty"`
	Operation *OperationState    `json:"operation,omitempty"`
	Tasks     *TaskTrackingState `json:"tasks,omitempty"`
}

// DefaultStatePath returns the default heartbeat state file path.
func DefaultStatePath() string {
	home := os.Getenv("HOME")
	if home == "" {
		home = "/tmp"
	}
	return filepath.Join(home, ".claude", "heartbeat-state.json")
}

// LoadState reads heartbeat state from disk.
// Returns empty state if file doesn't exist or is unreadable.
func LoadState(path string) State {
	data, err := os.ReadFile(path)
	if err != nil {
		return State{}
	}
	var state State
	if err := json.Unmarshal(data, &state); err != nil {
		return State{}
	}
	return state
}

// SaveState writes heartbeat state to disk.
// Creates parent directories if needed.
func SaveState(path string, state State) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0750); err != nil {
		return err
	}
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0600)
}

// RecordToolUse updates batch tracking based on the tool used.
// File operations (Edit, Write, Read, NotebookEdit) increment the batch counter.
// Non-file operations reset it.
func (s *State) RecordToolUse(toolName string, params map[string]interface{}) {
	if fileOps[toolName] {
		if s.Batch == nil {
			s.Batch = &BatchState{
				StartedAt: time.Now(),
			}
		}
		s.Batch.Count++
		s.Batch.ToolType = toolName

		// Extract file path from parameters
		filePath := extractFilePath(toolName, params)
		s.Batch.LastFile = filePath

		if filePath != "" {
			s.Batch.FilesProcessed = append(s.Batch.FilesProcessed, filePath)
			// Cap at 20
			if len(s.Batch.FilesProcessed) > 20 {
				s.Batch.FilesProcessed = s.Batch.FilesProcessed[len(s.Batch.FilesProcessed)-20:]
			}
		}
	} else {
		s.ResetBatch()
	}
}

// IsInBatch returns true if 3 or more consecutive file operations have occurred.
func (s *State) IsInBatch() bool {
	return s.Batch != nil && s.Batch.Count >= 3
}

// ResetBatch clears batch tracking state.
func (s *State) ResetBatch() {
	s.Batch = nil
}

// RecordOperationStart records the start of a potentially long-running operation.
func (s *State) RecordOperationStart(toolName, commandPreview string) {
	if len(commandPreview) > 80 {
		commandPreview = commandPreview[:80]
	}
	s.Operation = &OperationState{
		ToolName:       toolName,
		StartedAt:      time.Now(),
		CommandPreview: commandPreview,
	}
}

// GetOperationDuration returns the duration since the operation started.
// Returns 0 if no operation is being tracked.
func (s *State) GetOperationDuration() time.Duration {
	if s.Operation == nil {
		return 0
	}
	return time.Since(s.Operation.StartedAt)
}

// extractFilePath pulls the file path from tool parameters.
func extractFilePath(toolName string, params map[string]interface{}) string {
	switch toolName {
	case "Edit":
		if fp, ok := params["file_path"].(string); ok {
			return fp
		}
	case "Write":
		if fp, ok := params["file_path"].(string); ok {
			return fp
		}
	case "Read":
		if fp, ok := params["file_path"].(string); ok {
			return fp
		}
	case "NotebookEdit":
		if fp, ok := params["file_path"].(string); ok {
			return fp
		}
		if fp, ok := params["notebook_path"].(string); ok {
			return fp
		}
	}
	return ""
}
