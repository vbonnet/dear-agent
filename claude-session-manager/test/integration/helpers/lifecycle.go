package helpers

import (
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

// Session represents a CSM session (minimal struct for testing)
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
// This creates the archived directory structure manually
func CreateArchivedSession(env *TestEnv, sessionID, agent string) error {
	// Default to claude for backward compatibility
	if agent == "" {
		agent = "claude"
	}

	// Create archived session directory
	archiveDir := filepath.Join(env.SessionsDir, "archive", sessionID)
	if err := os.MkdirAll(archiveDir, 0700); err != nil {
		return fmt.Errorf("failed to create archived session directory: %w", err)
	}

	// Create a basic archived manifest
	manifestPath := filepath.Join(archiveDir, "manifest.yaml")
	manifest := fmt.Sprintf(`session_id: %s
agent: %s
status: archived
archived_at: "2026-01-20T19:00:00Z"
`, sessionID, agent)

	if err := os.WriteFile(manifestPath, []byte(manifest), 0600); err != nil {
		return fmt.Errorf("failed to write archived manifest: %w", err)
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

// ListTestSessions lists sessions using agm list command
func ListTestSessions(sessionsDir string, filter ListFilter) ([]Session, error) {
	args := []string{"session", "list", "--sessions-dir", sessionsDir}
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

	// Parse output - simple line-based parsing
	// Expected format: one session per line with ID, agent, status
	// Example: "session-123  claude  active"
	var sessions []Session
	for _, line := range strings.Split(string(output), "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}

		// Parse line (split by whitespace)
		parts := strings.Fields(trimmed)
		if len(parts) < 3 {
			continue // Skip malformed lines
		}

		session := Session{
			ID:       parts[0],
			Agent:    parts[1],
			Status:   parts[2],
			Archived: parts[2] == "archived",
		}
		sessions = append(sessions, session)
	}

	return sessions, nil
}

// CleanupArchivedSession removes an archived session fixture
func CleanupArchivedSession(env *TestEnv, sessionID string) error {
	archiveDir := filepath.Join(env.SessionsDir, "archive", sessionID)
	if err := os.RemoveAll(archiveDir); err != nil {
		return fmt.Errorf("failed to cleanup archived session: %w", err)
	}
	return nil
}

// CreateSessionManifest creates a manifest file for a test session
// This registers the session with CSM so commands like resume/archive can find it
func CreateSessionManifest(sessionsDir, sessionName, agent string) error {
	// Create session directory
	sessionDir := filepath.Join(sessionsDir, sessionName)
	if err := os.MkdirAll(sessionDir, 0755); err != nil {
		return fmt.Errorf("failed to create session directory: %w", err)
	}

	// Create a test project directory (CSM health check requires it to exist)
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

	// Write manifest file
	manifestPath := filepath.Join(sessionDir, "manifest.yaml")
	if err := os.WriteFile(manifestPath, []byte(manifest), 0644); err != nil {
		return fmt.Errorf("failed to write manifest: %w", err)
	}

	return nil
}
