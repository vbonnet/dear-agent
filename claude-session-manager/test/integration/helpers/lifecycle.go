//go:build integration

package helpers

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
)

// ListFilter defines filters for listing sessions
type ListFilter struct {
	Archived bool
	All      bool
	Agent    string
}

// Session represents a AGM session (minimal struct for testing)
type Session struct {
	ID       string
	Agent    string
	Status   string
	Archived bool
}

// ArchiveTestSession archives a test session using agm archive command
// Note: Session should be inactive (tmux killed) before calling this
func ArchiveTestSession(sessionsDir, sessionID string, reason string) error {
	args := []string{"session", "archive", sessionID, "--sessions-dir", sessionsDir, "--force"}
	// --force skips confirmation prompt (test env has no TTY)
	// Note: --reason flag is a Phase 2 feature, not available in Phase 1

	cmd := exec.Command("agm", args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to archive session %s: %w (output: %s)", sessionID, err, string(output))
	}
	return nil
}

// CreateArchivedSession creates a pre-archived session fixture for testing
// Uses in-place archiving (lifecycle field) instead of moving to archive directory
func CreateArchivedSession(env *TestEnv, sessionID, agent string) error {
	// Default to claude for backward compatibility
	if agent == "" {
		agent = "claude"
	}

	// Create session directory in normal location (not in archive subdirectory)
	sessionDir := filepath.Join(env.SessionsDir, sessionID)
	if err := os.MkdirAll(sessionDir, 0700); err != nil {
		return fmt.Errorf("failed to create session directory: %w", err)
	}

	// Create project directory (required for valid session)
	projectDir := filepath.Join(sessionDir, "project")
	if err := os.MkdirAll(projectDir, 0700); err != nil {
		return fmt.Errorf("failed to create project directory: %w", err)
	}

	// Create a v2 manifest with lifecycle: archived
	manifestPath := filepath.Join(sessionDir, "manifest.yaml")
	manifest := fmt.Sprintf(`schema_version: "2.0"
session_id: %s
name: %s
created_at: "2026-01-20T19:00:00Z"
updated_at: "2026-01-20T19:00:00Z"
lifecycle: "archived"
context:
  project: "%s"
  purpose: ""
  tags: []
  notes: ""
tmux:
  session_name: "%s"
agent: "%s"
claude:
  uuid: ""
`, sessionID, sessionID, projectDir, sessionID, agent)

	if err := os.WriteFile(manifestPath, []byte(manifest), 0600); err != nil {
		return fmt.Errorf("failed to write manifest: %w", err)
	}

	return nil
}

// ResumeTestSession resumes a test session using agm resume command
func ResumeTestSession(sessionsDir, sessionID string) error {
	cmd := exec.Command("agm", "session", "resume", sessionID, "--sessions-dir", sessionsDir)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to resume session %s: %w (output: %s)", sessionID, err, string(output))
	}
	return nil
}

// manifestJSON represents the JSON structure returned by agm list --json
type manifestJSON struct {
	SessionID string `json:"SessionID"`
	Name      string `json:"Name"`
	Agent     string `json:"Agent"`
	Lifecycle string `json:"Lifecycle"`
}

// ListTestSessions lists sessions using agm list command with JSON output
func ListTestSessions(sessionsDir string, filter ListFilter) ([]Session, error) {
	args := []string{"session", "list", "--sessions-dir", sessionsDir, "--json"}
	if filter.Archived {
		args = append(args, "--archived")
	}
	if filter.All {
		args = append(args, "--all")
	}
	if filter.Agent != "" {
		args = append(args, "--agent", filter.Agent)
	}

	cmd := exec.Command("agm", args...)
	output, err := cmd.Output()
	if err != nil {
		// If no sessions, agm list may return exit code 0 with empty output
		if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() != 0 {
			return nil, fmt.Errorf("failed to list sessions: %w", err)
		}
	}

	// Parse JSON output - extract JSON array from output
	// agm may append version info after JSON, so find the array boundaries
	var manifests []manifestJSON
	outputStr := string(output)

	// Find first [ and last ]
	startIdx := -1
	endIdx := -1
	for i, ch := range outputStr {
		if ch == '[' && startIdx == -1 {
			startIdx = i
		}
		if ch == ']' {
			endIdx = i + 1
		}
	}

	// If no JSON array found, check if it's the "no sessions" case
	if startIdx == -1 || endIdx == -1 {
		// agm outputs "No sessions found" when there are no sessions
		// This is a valid response, not an error
		return []Session{}, nil
	}

	jsonBytes := []byte(outputStr[startIdx:endIdx])
	if err := json.Unmarshal(jsonBytes, &manifests); err != nil {
		return nil, fmt.Errorf("failed to parse JSON output: %w", err)
	}

	// Convert to Session structs
	var sessions []Session
	for _, m := range manifests {
		status := "active"
		if m.Lifecycle == "archived" {
			status = "archived"
		} else if m.Lifecycle != "" {
			status = m.Lifecycle
		}

		session := Session{
			ID:       m.Name, // Use Name field as ID (tmux session name)
			Agent:    m.Agent,
			Status:   status,
			Archived: m.Lifecycle == "archived",
		}
		sessions = append(sessions, session)
	}

	return sessions, nil
}

// CleanupArchivedSession removes an archived session fixture
func CleanupArchivedSession(env *TestEnv, sessionID string) error {
	// Archived sessions are now in-place, not in separate archive directory
	sessionDir := filepath.Join(env.SessionsDir, sessionID)
	if err := os.RemoveAll(sessionDir); err != nil {
		return fmt.Errorf("failed to cleanup archived session: %w", err)
	}
	return nil
}

// CreateSessionManifest creates a manifest file for a test session
// This registers the session with AGM so commands like resume/archive can find it
func CreateSessionManifest(sessionsDir, sessionName, agent string) error {
	// Validate session name
	if sessionName == "" {
		return fmt.Errorf("session name cannot be empty")
	}

	// Reject path traversal characters
	if strings.Contains(sessionName, "/") || strings.Contains(sessionName, "\\") {
		return fmt.Errorf("session name cannot contain path separators: %s", sessionName)
	}

	// Create session directory
	sessionDir := filepath.Join(sessionsDir, sessionName)

	// Check if session already exists (for duplicate detection)
	manifestPath := filepath.Join(sessionDir, "manifest.yaml")
	if _, err := os.Stat(manifestPath); err == nil {
		return fmt.Errorf("session already exists: %s", sessionName)
	}

	if err := os.MkdirAll(sessionDir, 0755); err != nil {
		return fmt.Errorf("failed to create session directory: %w", err)
	}

	// Create a test project directory (AGM health check requires it to exist)
	projectDir := filepath.Join(sessionsDir, sessionName, "project")
	if err := os.MkdirAll(projectDir, 0755); err != nil {
		return fmt.Errorf("failed to create project directory: %w", err)
	}

	// Create manifest content
	now := time.Now().UTC().Format(time.RFC3339)
	sessionID := uuid.New().String()

	manifest := fmt.Sprintf(`schema_version: "2.0"
session_id: "%s"
name: "%s"
created_at: "%s"
updated_at: "%s"
lifecycle: ""
context:
  project: "%s"
  purpose: ""
  tags: []
  notes: ""
tmux:
  session_name: "%s"
agent: "%s"
claude:
  uuid: ""
`, sessionID, sessionName, now, now, projectDir, sessionName, agent)

	// Write manifest file (path already declared for duplicate check above)
	if err := os.WriteFile(manifestPath, []byte(manifest), 0644); err != nil {
		return fmt.Errorf("failed to write manifest: %w", err)
	}

	return nil
}
