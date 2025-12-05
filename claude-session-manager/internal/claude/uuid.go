package claude

import (
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// CaptureLatestUUID attempts to capture the most recently created Claude session UUID
// by reading the history.jsonl file and finding the latest entry within timeout window
func CaptureLatestUUID(timeout time.Duration) (string, error) {
	historyPath := filepath.Join(os.Getenv("HOME"), ".claude/history.jsonl")

	// Wait for Claude to write to history
	time.Sleep(timeout)

	// Parse history
	entries, err := ParseHistory(historyPath)
	if err != nil {
		return "", err
	}

	if len(entries) == 0 {
		return "", fmt.Errorf("no entries found in history")
	}

	// Get most recent entry (last in file)
	latest := entries[len(entries)-1]

	// Verify timestamp is recent (within last 5 seconds)
	entryTime := time.Unix(0, int64(latest.Timestamp)*int64(time.Millisecond))
	if time.Since(entryTime) > 5*time.Second {
		return "", fmt.Errorf("latest entry is too old (%v ago), expected within 5s", time.Since(entryTime))
	}

	return latest.SessionID, nil
}
