package manifest

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

var (
	// Session identifiers (tmux names, workspace IDs)
	sessionIDPattern = regexp.MustCompile(`^[a-zA-Z0-9_.-]+$`)

	// Claude UUIDs
	uuidPattern = regexp.MustCompile(
		`^[a-f0-9]{8}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{12}$`,
	)
)

// Validate checks manifest schema and required fields
func Validate(m *Manifest) error {
	// Session ID
	if m.SessionID == "" {
		return fmt.Errorf("session_id is required")
	}
	if !sessionIDPattern.MatchString(m.SessionID) {
		return fmt.Errorf("invalid session_id: %s", m.SessionID)
	}
	if len(m.SessionID) > 100 {
		return fmt.Errorf("session_id too long (max 100 chars): %s", m.SessionID)
	}

	// Status
	if m.Status == "" {
		return fmt.Errorf("status is required")
	}
	validStatuses := map[string]bool{
		StatusActive:     true,
		StatusDiscovered: true,
		StatusStale:      true,
		StatusArchived:   true,
	}
	if !validStatuses[m.Status] {
		return fmt.Errorf("invalid status: %s", m.Status)
	}

	// Timestamps
	if m.CreatedAt.IsZero() {
		return fmt.Errorf("created_at is required")
	}
	if m.LastActivity.IsZero() {
		return fmt.Errorf("last_activity is required")
	}

	// Worktree
	if m.Worktree.Path == "" {
		return fmt.Errorf("worktree.path is required")
	}
	// Branch is optional (may be empty for non-git worktrees)

	// Claude
	if m.Claude.SessionID == "" {
		return fmt.Errorf("claude.session_id is required")
	}
	if !uuidPattern.MatchString(m.Claude.SessionID) {
		return fmt.Errorf("invalid claude.session_id (must be UUID): %s", m.Claude.SessionID)
	}

	// Tmux
	if m.Tmux.SessionName == "" {
		return fmt.Errorf("tmux.session_name is required")
	}
	if !sessionIDPattern.MatchString(m.Tmux.SessionName) {
		return fmt.Errorf("invalid tmux.session_name: %s", m.Tmux.SessionName)
	}

	return nil
}

// ValidatePath checks if path is safe (no traversal, within home directory)
func ValidatePath(path string) error {
	// Canonicalize path
	absPath, err := filepath.Abs(path)
	if err != nil {
		return fmt.Errorf("failed to resolve path: %w", err)
	}

	// Check prefix (must be in home directory)
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("failed to get home directory: %w", err)
	}

	if !strings.HasPrefix(absPath, homeDir) {
		return fmt.Errorf("path outside home directory: %s", path)
	}

	return nil
}

// ValidateSessionID checks if session ID is valid
func ValidateSessionID(id string) error {
	if !sessionIDPattern.MatchString(id) {
		return fmt.Errorf("invalid session ID: %s", id)
	}
	if len(id) > 100 {
		return fmt.Errorf("session ID too long (max 100 chars)")
	}
	return nil
}

// ValidateUUID checks if UUID is valid
func ValidateUUID(uuid string) error {
	if !uuidPattern.MatchString(uuid) {
		return fmt.Errorf("invalid UUID: %s", uuid)
	}
	return nil
}
