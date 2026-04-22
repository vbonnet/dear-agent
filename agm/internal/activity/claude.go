// Package activity provides activity functionality.
package activity

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/vbonnet/dear-agent/agm/internal/history"
)

// ClaudeActivityTracker implements ActivityTracker for Claude sessions.
// It reads from the centralized ~/.claude/history.jsonl file and filters
// by sessionID to find the most recent message timestamp.
type ClaudeActivityTracker struct {
	historyPath string
}

// NewClaudeActivityTracker creates a tracker with default history path.
// Default path: ~/.claude/history.jsonl
func NewClaudeActivityTracker() *ClaudeActivityTracker {
	home, _ := os.UserHomeDir()
	return &ClaudeActivityTracker{
		historyPath: filepath.Join(home, ".claude", "history.jsonl"),
	}
}

// NewClaudeActivityTrackerWithPath creates a tracker with custom history path.
// This is useful for testing with custom history files.
func NewClaudeActivityTrackerWithPath(path string) *ClaudeActivityTracker {
	return &ClaudeActivityTracker{
		historyPath: path,
	}
}

// GetLastActivity reads centralized Claude history, filters by sessionID,
// and returns the most recent timestamp.
//
// Implementation:
// 1. Create history parser for ~/.claude/history.jsonl
// 2. Read all conversation entries
// 3. Filter entries by sessionID
// 4. Find maximum timestamp among filtered entries
// 5. Convert int64 milliseconds to time.Time
// 6. Return timestamp or error
func (t *ClaudeActivityTracker) GetLastActivity(sessionID string) (time.Time, error) {
	// Create parser for Claude history file
	parser := history.NewParser(t.historyPath)

	// Read all conversation entries (limit=0 means no limit)
	sessions, err := parser.ReadConversations(0)
	if err != nil {
		// Check if file doesn't exist
		if os.IsNotExist(err) {
			return time.Time{}, fmt.Errorf("%w: %s", ErrHistoryNotFound, t.historyPath)
		}
		// Check if permission denied
		if os.IsPermission(err) {
			return time.Time{}, fmt.Errorf("%w: %s", ErrPermissionDenied, t.historyPath)
		}
		// Other errors (likely corrupted JSON)
		return time.Time{}, fmt.Errorf("%w: %v", ErrHistoryCorrupted, err)
	}

	// Find the session with matching sessionID
	var targetSession *history.SessionHistory
	for _, session := range sessions {
		if session.SessionID == sessionID {
			targetSession = session
			break
		}
	}

	// If session not found in history
	if targetSession == nil {
		return time.Time{}, fmt.Errorf("%w: session %s not found in history", ErrHistoryNotFound, sessionID)
	}

	// Check if session has any entries
	if len(targetSession.Entries) == 0 {
		return time.Time{}, fmt.Errorf("%w: session %s", ErrEmptyHistory, sessionID)
	}

	// Find maximum timestamp among session entries
	var maxTimestamp int64 = 0
	for _, entry := range targetSession.Entries {
		if entry.Timestamp > maxTimestamp {
			maxTimestamp = entry.Timestamp
		}
	}

	// Convert int64 milliseconds to time.Time
	// Timestamp is Unix milliseconds, so divide by 1000 for seconds
	// and use remainder for nanoseconds
	seconds := maxTimestamp / 1000
	nanos := (maxTimestamp % 1000) * 1_000_000
	timestamp := time.Unix(seconds, nanos).UTC()

	return timestamp, nil
}

// GetLastActivityBatch efficiently retrieves last activity for multiple sessions.
// Reads history file once and builds a lookup map for all requested sessions.
//
// This is the preferred method when checking activity for multiple sessions
// as it avoids reading the history file multiple times.
func (t *ClaudeActivityTracker) GetLastActivityBatch(sessionIDs []string) map[string]time.Time {
	result := make(map[string]time.Time)

	// Create parser for Claude history file
	parser := history.NewParser(t.historyPath)

	// Read all conversation entries (limit=0 means no limit)
	sessions, err := parser.ReadConversations(0)
	if err != nil {
		// On any error, return empty map (no activity data available)
		return result
	}

	// Build set of requested session IDs for fast lookup
	requestedSessions := make(map[string]bool, len(sessionIDs))
	for _, id := range sessionIDs {
		requestedSessions[id] = true
	}

	// Process each session from history
	for _, session := range sessions {
		// Skip if not in requested list
		if !requestedSessions[session.SessionID] {
			continue
		}

		// Skip if session has no entries
		if len(session.Entries) == 0 {
			continue
		}

		// Find maximum timestamp among session entries
		var maxTimestamp int64 = 0
		for _, entry := range session.Entries {
			if entry.Timestamp > maxTimestamp {
				maxTimestamp = entry.Timestamp
			}
		}

		// Convert int64 milliseconds to time.Time
		seconds := maxTimestamp / 1000
		nanos := (maxTimestamp % 1000) * 1_000_000
		timestamp := time.Unix(seconds, nanos).UTC()

		result[session.SessionID] = timestamp
	}

	return result
}
