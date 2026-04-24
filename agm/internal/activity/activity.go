// Package activity provides tracking of last activity timestamps for agent sessions.
// It supports both Claude and Gemini session history formats.
package activity

import (
	"errors"
	"time"
)

// ActivityTracker provides last activity timestamps for agent sessions.
// Implementations read session history files and return the timestamp of
// the most recent message.
type ActivityTracker interface {
	// GetLastActivity returns the timestamp of the most recent message
	// in the session's history. Returns error if history unavailable.
	//
	// Parameters:
	//   sessionID: Session UUID (must match session directory name)
	//
	// Returns:
	//   time.Time: Timestamp of last message in session (UTC timezone)
	//   error: Non-nil if history file missing, corrupted, or permission denied
	GetLastActivity(sessionID string) (time.Time, error)

	// GetLastActivityBatch returns the last activity timestamps for multiple sessions.
	// This is much more efficient than calling GetLastActivity repeatedly as it reads
	// the history file only once.
	//
	// Parameters:
	//   sessionIDs: List of session UUIDs to look up
	//
	// Returns:
	//   map[string]time.Time: Map of sessionID -> last activity timestamp
	//   Only sessions found in history are included in the map
	GetLastActivityBatch(sessionIDs []string) map[string]time.Time
}

// Custom error types for activity tracking failures.
var (
	// ErrHistoryNotFound indicates the history file doesn't exist for this session.
	ErrHistoryNotFound = errors.New("history file not found")

	// ErrHistoryCorrupted indicates JSON parsing failed (malformed data).
	ErrHistoryCorrupted = errors.New("history file corrupted")

	// ErrPermissionDenied indicates cannot read history file (filesystem permissions).
	ErrPermissionDenied = errors.New("permission denied reading history")

	// ErrEmptyHistory indicates history file exists but contains no messages.
	ErrEmptyHistory = errors.New("history file contains no messages")
)
