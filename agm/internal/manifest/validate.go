package manifest

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"unicode/utf8"
)

var (
	// Session identifiers (tmux names, workspace IDs)
	sessionIDPattern = regexp.MustCompile(`^[a-zA-Z0-9_.-]+$`)

	// Claude UUIDs
	uuidPattern = regexp.MustCompile(
		`^[a-f0-9]{8}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{12}$`,
	)
)

// Validate checks manifest schema and required fields (v2 schema)
func (m *Manifest) Validate() error {
	// Required fields
	if m.SchemaVersion == "" {
		return errors.New("schema_version is required")
	}
	if m.SessionID == "" {
		return errors.New("session_id is required")
	}
	if m.Name == "" {
		return errors.New("name is required")
	}
	if m.Context.Project == "" {
		return errors.New("context.project is required")
	}
	if m.Tmux.SessionName == "" {
		return errors.New("tmux.session_name is required")
	}

	// UTF-8 character counting for purpose
	if utf8.RuneCountInString(m.Context.Purpose) > MaxPurposeLen {
		return fmt.Errorf("purpose exceeds %d characters (has %d)",
			MaxPurposeLen, utf8.RuneCountInString(m.Context.Purpose))
	}

	// Tags validation
	if len(m.Context.Tags) > MaxTagsCount {
		return fmt.Errorf("too many tags: %d (max %d)",
			len(m.Context.Tags), MaxTagsCount)
	}

	for i, tag := range m.Context.Tags {
		if utf8.RuneCountInString(tag) > MaxTagLen {
			return fmt.Errorf("tag[%d] exceeds %d characters (has %d)",
				i, MaxTagLen, utf8.RuneCountInString(tag))
		}
	}

	// Notes validation
	if utf8.RuneCountInString(m.Context.Notes) > MaxNotesLen {
		return fmt.Errorf("notes exceed %d characters (has %d)",
			MaxNotesLen, utf8.RuneCountInString(m.Context.Notes))
	}

	// Lifecycle validation
	if m.Lifecycle != "" && m.Lifecycle != LifecycleReaping && m.Lifecycle != LifecycleArchived {
		return fmt.Errorf("invalid lifecycle: %s (must be empty, %s, or %s)",
			m.Lifecycle, LifecycleReaping, LifecycleArchived)
	}

	return nil
}

// ValidateV1 checks manifest schema and required fields (v1 schema - for migration)
func ValidateV1(m *ManifestV1) error {
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
