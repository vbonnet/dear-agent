package activities

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// CheckpointStateInput contains parameters for checkpointing session state
type CheckpointStateInput struct {
	SessionID       string                 // Session identifier
	SessionName     string                 // Human-readable session name
	WorkflowID      string                 // Temporal workflow ID
	WorkflowRunID   string                 // Temporal workflow run ID
	State           map[string]interface{} // Workflow state to checkpoint
	Metadata        map[string]string      // Additional metadata
	CheckpointType  string                 // Type: "periodic", "manual", "before_escalation"
}

// CheckpointStateOutput contains the result of checkpointing
type CheckpointStateOutput struct {
	SessionID       string    // Session that was checkpointed
	CheckpointPath  string    // Path to the checkpoint file
	CheckpointedAt  time.Time // When the checkpoint was created
	StateSize       int       // Size of the state in bytes
	Success         bool      // Whether checkpoint succeeded
}

// SessionCheckpoint represents the structure of a saved checkpoint
type SessionCheckpoint struct {
	Version         string                 `json:"version"`          // Checkpoint format version
	SessionID       string                 `json:"session_id"`       // Session identifier
	SessionName     string                 `json:"session_name"`     // Session name
	WorkflowID      string                 `json:"workflow_id"`      // Temporal workflow ID
	WorkflowRunID   string                 `json:"workflow_run_id"`  // Temporal workflow run ID
	State           map[string]interface{} `json:"state"`            // Workflow state
	Metadata        map[string]string      `json:"metadata"`         // Additional metadata
	CheckpointType  string                 `json:"checkpoint_type"`  // Type of checkpoint
	CreatedAt       time.Time              `json:"created_at"`       // Creation timestamp
	LastUpdated     time.Time              `json:"last_updated"`     // Last update timestamp
}

// CheckpointStateActivity saves session state to persistent storage
// Phase 1: Saves to JSON file in session directory
// Phase 2: Will migrate to SQLite database
func CheckpointStateActivity(ctx context.Context, input CheckpointStateInput) (*CheckpointStateOutput, error) {
	// Validate input
	if input.SessionID == "" {
		return nil, fmt.Errorf("session ID cannot be empty")
	}
	if input.WorkflowID == "" {
		return nil, fmt.Errorf("workflow ID cannot be empty")
	}

	// Set defaults
	if input.CheckpointType == "" {
		input.CheckpointType = "periodic"
	}
	if input.State == nil {
		input.State = make(map[string]interface{})
	}
	if input.Metadata == nil {
		input.Metadata = make(map[string]string)
	}

	// Ensure session directory exists
	sessionDir, err := EnsureSessionDir(input.SessionID)
	if err != nil {
		return nil, fmt.Errorf("failed to ensure session directory: %w", err)
	}

	// Create checkpoint structure
	checkpoint := SessionCheckpoint{
		Version:        "1.0",
		SessionID:      input.SessionID,
		SessionName:    input.SessionName,
		WorkflowID:     input.WorkflowID,
		WorkflowRunID:  input.WorkflowRunID,
		State:          input.State,
		Metadata:       input.Metadata,
		CheckpointType: input.CheckpointType,
		CreatedAt:      time.Now(),
		LastUpdated:    time.Now(),
	}

	// Check if checkpoint file exists (for updating LastUpdated)
	checkpointPath := filepath.Join(sessionDir, "checkpoint.json")
	if existingData, err := os.ReadFile(checkpointPath); err == nil {
		var existing SessionCheckpoint
		if err := json.Unmarshal(existingData, &existing); err == nil {
			checkpoint.CreatedAt = existing.CreatedAt // Preserve original creation time
		}
	}

	// Marshal checkpoint to JSON
	checkpointData, err := json.MarshalIndent(checkpoint, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("failed to marshal checkpoint: %w", err)
	}

	// Write checkpoint atomically (write to temp file, then rename)
	tempPath := checkpointPath + ".tmp"
	if err := os.WriteFile(tempPath, checkpointData, 0644); err != nil {
		return nil, fmt.Errorf("failed to write checkpoint temp file: %w", err)
	}

	if err := os.Rename(tempPath, checkpointPath); err != nil {
		os.Remove(tempPath) // Clean up temp file
		return nil, fmt.Errorf("failed to rename checkpoint file: %w", err)
	}

	// Create output
	output := &CheckpointStateOutput{
		SessionID:      input.SessionID,
		CheckpointPath: checkpointPath,
		CheckpointedAt: checkpoint.LastUpdated,
		StateSize:      len(checkpointData),
		Success:        true,
	}

	return output, nil
}

// LoadCheckpointActivity loads the latest checkpoint for a session
// This is used for session recovery and state restoration
func LoadCheckpointActivity(ctx context.Context, sessionID string) (*SessionCheckpoint, error) {
	if sessionID == "" {
		return nil, fmt.Errorf("session ID cannot be empty")
	}

	// Get session directory
	sessionDir, err := GetSessionDataDir(sessionID)
	if err != nil {
		return nil, fmt.Errorf("failed to get session directory: %w", err)
	}

	// Read checkpoint file
	checkpointPath := filepath.Join(sessionDir, "checkpoint.json")
	checkpointData, err := os.ReadFile(checkpointPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("no checkpoint found for session %s", sessionID)
		}
		return nil, fmt.Errorf("failed to read checkpoint: %w", err)
	}

	// Unmarshal checkpoint
	var checkpoint SessionCheckpoint
	if err := json.Unmarshal(checkpointData, &checkpoint); err != nil {
		return nil, fmt.Errorf("failed to unmarshal checkpoint: %w", err)
	}

	return &checkpoint, nil
}

// ListCheckpointsActivity lists all available checkpoints (for future use with multiple checkpoints)
func ListCheckpointsActivity(ctx context.Context, sessionID string) ([]SessionCheckpoint, error) {
	if sessionID == "" {
		return nil, fmt.Errorf("session ID cannot be empty")
	}

	// For Phase 1, we only have one checkpoint per session
	checkpoint, err := LoadCheckpointActivity(ctx, sessionID)
	if err != nil {
		// No checkpoint exists
		return []SessionCheckpoint{}, nil
	}

	return []SessionCheckpoint{*checkpoint}, nil
}

// DeleteCheckpointActivity removes checkpoint data for a session
// Used during session cleanup
func DeleteCheckpointActivity(ctx context.Context, sessionID string) error {
	if sessionID == "" {
		return fmt.Errorf("session ID cannot be empty")
	}

	sessionDir, err := GetSessionDataDir(sessionID)
	if err != nil {
		return fmt.Errorf("failed to get session directory: %w", err)
	}

	checkpointPath := filepath.Join(sessionDir, "checkpoint.json")
	if err := os.Remove(checkpointPath); err != nil {
		if os.IsNotExist(err) {
			// Already deleted, not an error
			return nil
		}
		return fmt.Errorf("failed to delete checkpoint: %w", err)
	}

	return nil
}

// SaveWorkflowState is a helper to save arbitrary workflow state
func SaveWorkflowState(sessionID string, key string, value interface{}) error {
	// Load existing checkpoint
	checkpoint, err := LoadCheckpointActivity(context.Background(), sessionID)
	if err != nil {
		// Create new checkpoint if none exists
		checkpoint = &SessionCheckpoint{
			Version:   "1.0",
			SessionID: sessionID,
			State:     make(map[string]interface{}),
			Metadata:  make(map[string]string),
			CreatedAt: time.Now(),
		}
	}

	// Update state
	checkpoint.State[key] = value
	checkpoint.LastUpdated = time.Now()

	// Save checkpoint
	input := CheckpointStateInput{
		SessionID:      sessionID,
		SessionName:    checkpoint.SessionName,
		WorkflowID:     checkpoint.WorkflowID,
		WorkflowRunID:  checkpoint.WorkflowRunID,
		State:          checkpoint.State,
		Metadata:       checkpoint.Metadata,
		CheckpointType: "state_update",
	}

	_, err = CheckpointStateActivity(context.Background(), input)
	return err
}

// GetWorkflowState is a helper to retrieve specific workflow state
func GetWorkflowState(sessionID string, key string) (interface{}, error) {
	checkpoint, err := LoadCheckpointActivity(context.Background(), sessionID)
	if err != nil {
		return nil, err
	}

	value, exists := checkpoint.State[key]
	if !exists {
		return nil, fmt.Errorf("state key '%s' not found", key)
	}

	return value, nil
}
