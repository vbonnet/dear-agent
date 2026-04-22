package worktree

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/vbonnet/dear-agent/engram/hooks-bin/internal/session"
)

// GetSessionID retrieves the current Claude Code session ID.
// It attempts to extract the UUID from the Claude history file,
// falling back to auto-generated IDs if extraction fails.
//
// The session ID is used to create unique worktree names for isolation
// between parallel agent sessions.
//
// Returns:
//   - string: Session UUID or fallback ID (format: auto-<timestamp>-<random>)
//   - error: Error if history file cannot be read (fallback still provided)
//
// Example:
//
//	sessionID, err := GetSessionID()
//	if err != nil {
//	    log.Printf("Using fallback session ID: %s", sessionID)
//	}
//	fmt.Printf("Session: %s\n", sessionID)
func GetSessionID() (string, error) {
	historyPath := filepath.Join(os.Getenv("HOME"), ".claude", "history.jsonl")
	return session.ExtractUUID(historyPath)
}

// FormatWorktreeName creates a consistent worktree directory name
// from a session ID.
//
// The naming convention is "session-<uuid>" which:
//   - Makes session worktrees easily identifiable
//   - Distinguishes them from feature branch worktrees
//   - Enables automated cleanup scripts
//
// Parameters:
//   - sessionID: The session UUID or fallback ID
//
// Returns:
//   - string: Formatted worktree directory name
//
// Example:
//
//	sessionID := "abc123de-f456-7890-1234-567890abcdef"
//	name := FormatWorktreeName(sessionID)
//	// Returns: "session-abc123de-f456-7890-1234-567890abcdef"
//
//	fallbackID := "auto-1709251234-a4f2"
//	name = FormatWorktreeName(fallbackID)
//	// Returns: "session-auto-1709251234-a4f2"
func FormatWorktreeName(sessionID string) string {
	return fmt.Sprintf("session-%s", sessionID)
}
