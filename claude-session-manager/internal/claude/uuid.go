package claude

import (
	"fmt"
	"os"
	"path/filepath"
	"time"
)

const (
	// MaxEntryAge is the maximum age for a history entry to be considered "recent"
	// when capturing the latest UUID. Entries older than this are rejected.
	MaxEntryAge = 5 * time.Second

	// DefaultMaxRetries is the default number of retry attempts for UUID detection
	DefaultMaxRetries = 10

	// DefaultBaseDelay is the base delay for exponential backoff (100ms)
	DefaultBaseDelay = 100 * time.Millisecond
)

// CaptureLatestUUID attempts to capture the most recently created Claude session UUID
// by reading the history.jsonl file and finding the latest entry within timeout window
//
// Deprecated: Use CaptureLatestUUIDWithRetry for more reliable UUID detection.
// This function remains for backward compatibility but will be removed in a future version.
func CaptureLatestUUID(timeout time.Duration) (string, error) {
	historyPath := filepath.Join(os.Getenv("HOME"), ".claude/history.jsonl")

	// Wait for Claude to write to history
	time.Sleep(timeout)

	// Parse history
	entries, _, err := ParseHistory(historyPath)
	if err != nil {
		return "", err
	}

	if len(entries) == 0 {
		return "", fmt.Errorf("no entries found in history")
	}

	// Get most recent entry (last in file)
	latest := entries[len(entries)-1]

	// Verify timestamp is recent (within MaxEntryAge)
	entryTime := time.Unix(0, int64(latest.Timestamp)*int64(time.Millisecond))
	if time.Since(entryTime) > MaxEntryAge {
		return "", fmt.Errorf("latest entry is too old (%v ago), expected within %v", time.Since(entryTime), MaxEntryAge)
	}

	return latest.SessionID, nil
}

// CaptureLatestUUIDWithRetry attempts to capture the most recently created Claude session UUID
// with exponential backoff retry logic to handle race conditions where Claude hasn't written
// the UUID to history.jsonl yet.
//
// The function retries up to maxRetries times, with exponential backoff between attempts:
// 100ms, 200ms, 400ms, 800ms, 1600ms, 3200ms, etc.
//
// Each attempt parses history.jsonl and validates that the latest entry was created within
// MaxEntryAge (5 seconds). This ensures we don't capture UUIDs from previous sessions.
//
// Parameters:
//   - maxRetries: Maximum number of retry attempts (recommended: 10)
//   - baseDelay: Base delay between retries (recommended: 100ms)
//
// Returns:
//   - string: UUID of the most recent Claude session
//   - error: Error if UUID not found after all retries
//
// Example:
//   uuid, err := CaptureLatestUUIDWithRetry(claude.DefaultMaxRetries, claude.DefaultBaseDelay)
//   if err != nil {
//       // Handle error (UUID not found after retries)
//   }
func CaptureLatestUUIDWithRetry(maxRetries int, baseDelay time.Duration) (string, error) {
	historyPath := filepath.Join(os.Getenv("HOME"), ".claude/history.jsonl")
	startTime := time.Now()

	for attempt := 0; attempt < maxRetries; attempt++ {
		// Parse history
		entries, _, err := ParseHistory(historyPath)

		// If parse succeeds and we have entries, try to get latest UUID
		if err == nil && len(entries) > 0 {
			latest := entries[len(entries)-1]

			// Verify timestamp is recent (within MaxEntryAge)
			entryTime := time.Unix(0, int64(latest.Timestamp)*int64(time.Millisecond))
			age := time.Since(entryTime)

			if age <= MaxEntryAge {
				// Found a fresh UUID, return it
				return latest.SessionID, nil
			}

			// Entry exists but is too old, continue retrying
			// (UUID from previous session, not current one)
		}

		// If this isn't the last attempt, sleep with exponential backoff
		if attempt < maxRetries-1 {
			delay := baseDelay * time.Duration(1<<attempt) // 2^attempt exponential backoff
			time.Sleep(delay)
		}
	}

	// All retries exhausted
	totalWait := time.Since(startTime)
	return "", fmt.Errorf("UUID not found after %d retries (%v total wait time)\n"+
		"  • Claude may have crashed before writing UUID\n"+
		"  • Run 'csm doctor' to check session health\n"+
		"  • Check tmux session is still running: tmux list-sessions",
		maxRetries, totalWait.Round(time.Millisecond))
}
