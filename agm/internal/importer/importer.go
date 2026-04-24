// Package importer provides importer functionality.
package importer

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/google/uuid"
	"github.com/vbonnet/dear-agent/agm/internal/dolt"
	"github.com/vbonnet/dear-agent/agm/internal/history"
	"github.com/vbonnet/dear-agent/agm/internal/manifest"
	"github.com/vbonnet/dear-agent/agm/internal/tmux"
)

// SessionMetadata holds extracted metadata from history.jsonl
type SessionMetadata struct {
	UUID         string
	ProjectPath  string
	LastModified time.Time
}

// ImportOrphanedSession imports an orphaned conversation by creating an AGM manifest
//
// Parameters:
//   - conversationUUID: Claude conversation UUID to import
//   - sessionName: Name for the AGM session (sanitized for tmux)
//   - workspace: Workspace name (e.g., "oss", "acme")
//   - adapter: Dolt database adapter
//   - sessionsDir: Directory where manifests are stored (for YAML backward compat)
//
// Returns:
//   - sessionID: Generated AGM session ID
//   - error: Error if import fails
func ImportOrphanedSession(conversationUUID, sessionName, workspace string, adapter *dolt.Adapter, sessionsDir string) (string, error) {
	// 1. Validate inputs
	if conversationUUID == "" {
		return "", fmt.Errorf("conversation UUID cannot be empty")
	}
	if sessionName == "" {
		return "", fmt.Errorf("session name cannot be empty")
	}
	if adapter == nil {
		return "", fmt.Errorf("Dolt adapter not available")
	}

	// 2. Check for duplicate (UUID already has manifest)
	if err := ValidateNotDuplicate(conversationUUID, adapter); err != nil {
		return "", err
	}

	// 3. Infer project path from conversation file location
	projectPath, err := InferProjectPath(conversationUUID)
	if err != nil {
		return "", fmt.Errorf("failed to infer project path: %w", err)
	}

	// 4. Extract metadata from history.jsonl
	metadata, err := ExtractMetadataFromHistory(conversationUUID)
	if err != nil {
		// Non-fatal: we can still import without history metadata
		metadata = &SessionMetadata{
			UUID:         conversationUUID,
			ProjectPath:  projectPath,
			LastModified: time.Now(),
		}
	} else {
		// Override project path if history has more accurate info
		if metadata.ProjectPath != "" {
			projectPath = metadata.ProjectPath
		}
	}

	// 5. Generate new session ID
	sessionID := uuid.New().String()

	// 6. Sanitize tmux session name
	tmuxName := tmux.SanitizeSessionName(sessionName)

	// 7. Create manifest
	m := &manifest.Manifest{
		SchemaVersion: manifest.SchemaVersion,
		SessionID:     sessionID,
		Name:          sessionName,
		CreatedAt:     metadata.LastModified,
		UpdatedAt:     time.Now(),
		Lifecycle:     "", // Active by default
		Workspace:     workspace,
		Context: manifest.Context{
			Project: projectPath,
		},
		Claude: manifest.Claude{
			UUID: conversationUUID,
		},
		Tmux: manifest.Tmux{
			SessionName: tmuxName,
		},
	}

	// Write to Dolt database
	if err := adapter.CreateSession(m); err != nil {
		return "", fmt.Errorf("failed to create session in Dolt: %w", err)
	}

	return sessionID, nil
}

// InferProjectPath finds the project directory for a conversation UUID
// by scanning ~/.claude/projects/*/<uuid>.jsonl
func InferProjectPath(conversationUUID string) (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("failed to get home directory: %w", err)
	}

	projectsDir := filepath.Join(home, ".claude", "projects")
	entries, err := os.ReadDir(projectsDir)
	if err != nil {
		return "", fmt.Errorf("failed to read projects directory: %w", err)
	}

	// Search for <uuid>.jsonl in each project directory
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		conversationFile := filepath.Join(projectsDir, entry.Name(), conversationUUID+".jsonl")
		if _, err := os.Stat(conversationFile); err == nil {
			// Found it! Return the project directory path
			return filepath.Join(projectsDir, entry.Name()), nil
		}
	}

	return "", fmt.Errorf("no conversation file found for UUID: %s", conversationUUID)
}

// ExtractMetadataFromHistory extracts metadata from history.jsonl for a UUID
func ExtractMetadataFromHistory(conversationUUID string) (*SessionMetadata, error) {
	parser := history.NewParser("")
	sessions, err := parser.ReadConversations(10000) // Read last 10k entries
	if err != nil {
		return nil, fmt.Errorf("failed to read history: %w", err)
	}

	// Find the session matching our UUID
	for _, session := range sessions {
		if session.SessionID == conversationUUID {
			// Get last modified time from most recent entry
			var lastModified time.Time
			if len(session.Entries) > 0 {
				// Timestamp is in milliseconds
				lastModified = time.Unix(0, session.Entries[0].Timestamp*int64(time.Millisecond))
			} else {
				lastModified = time.Now()
			}

			return &SessionMetadata{
				UUID:         conversationUUID,
				ProjectPath:  session.Project,
				LastModified: lastModified,
			}, nil
		}
	}

	return nil, fmt.Errorf("no history entries found for UUID: %s", conversationUUID)
}

// ValidateNotDuplicate checks if a UUID already has a manifest
func ValidateNotDuplicate(conversationUUID string, adapter *dolt.Adapter) error {
	// List all sessions from Dolt
	if adapter == nil {
		return fmt.Errorf("Dolt adapter not available")
	}

	manifests, err := adapter.ListSessions(&dolt.SessionFilter{})
	if err != nil {
		return fmt.Errorf("failed to list sessions from Dolt: %w", err)
	}

	// Check if any manifest has this UUID
	for _, m := range manifests {
		if m.Claude.UUID == conversationUUID {
			return fmt.Errorf("conversation UUID %s already has manifest (session: %s)", conversationUUID, m.Name)
		}
	}

	return nil
}

// ValidateNotDuplicateWithAdapter checks if a UUID already has a session in Dolt
// Test helper function for Phase 5 migration
func ValidateNotDuplicateWithAdapter(conversationUUID string, adapter dolt.Storage) error {
	// List all sessions from database
	manifests, err := adapter.ListSessions(&dolt.SessionFilter{})
	if err != nil {
		return fmt.Errorf("failed to list sessions: %w", err)
	}

	// Check if any session has this UUID
	for _, m := range manifests {
		if m.Claude.UUID == conversationUUID {
			return fmt.Errorf("conversation UUID %s already has manifest (session: %s)", conversationUUID, m.Name)
		}
	}

	return nil
}

// ImportOrphanedSessionWithAdapter imports an orphaned session using Dolt adapter
// Test helper function for Phase 5 migration
func ImportOrphanedSessionWithAdapter(conversationUUID, sessionName, workspace string, adapter *dolt.Adapter) (string, error) {
	// 1. Validate inputs
	if conversationUUID == "" {
		return "", fmt.Errorf("conversation UUID cannot be empty")
	}
	if sessionName == "" {
		return "", fmt.Errorf("session name cannot be empty")
	}

	// 2. Check for duplicate (UUID already in database)
	if err := ValidateNotDuplicateWithAdapter(conversationUUID, adapter); err != nil {
		return "", err
	}

	// 3. Infer project path from conversation file location
	projectPath, err := InferProjectPath(conversationUUID)
	if err != nil {
		return "", fmt.Errorf("failed to infer project path: %w", err)
	}

	// 4. Extract metadata from history.jsonl
	metadata, err := ExtractMetadataFromHistory(conversationUUID)
	if err != nil {
		// Non-fatal: we can still import without history metadata
		metadata = &SessionMetadata{
			UUID:         conversationUUID,
			ProjectPath:  projectPath,
			LastModified: time.Now(),
		}
	} else {
		// Override project path if history has more accurate info
		if metadata.ProjectPath != "" {
			projectPath = metadata.ProjectPath
		}
	}

	// 5. Generate new session ID
	sessionID := uuid.New().String()

	// 6. Sanitize tmux session name
	tmuxName := tmux.SanitizeSessionName(sessionName)

	// 7. Create manifest
	m := &manifest.Manifest{
		SchemaVersion: manifest.SchemaVersion,
		SessionID:     sessionID,
		Name:          sessionName,
		CreatedAt:     metadata.LastModified,
		UpdatedAt:     time.Now(),
		Lifecycle:     "", // Active by default
		Workspace:     workspace,
		Context: manifest.Context{
			Project: projectPath,
		},
		Claude: manifest.Claude{
			UUID: conversationUUID,
		},
		Tmux: manifest.Tmux{
			SessionName: tmuxName,
		},
	}

	// 8. Save to database
	if err := adapter.CreateSession(m); err != nil {
		return "", fmt.Errorf("failed to save session to database: %w", err)
	}

	return sessionID, nil
}
