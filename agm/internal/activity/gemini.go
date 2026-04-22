package activity

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/vbonnet/dear-agent/agm/internal/agent"
)

// GeminiActivityTracker implements ActivityTracker for Gemini sessions.
// It reads from per-session history files at ~/.agm/gemini/<sessionID>/history.jsonl
// which is simpler than Claude (no sessionID filtering, smaller files).
type GeminiActivityTracker struct {
	baseDir string
}

// NewGeminiActivityTracker creates a tracker with default base directory.
// Default path: ~/.agm/gemini/
func NewGeminiActivityTracker() *GeminiActivityTracker {
	home, _ := os.UserHomeDir()
	return &GeminiActivityTracker{
		baseDir: filepath.Join(home, ".agm", "gemini"),
	}
}

// NewGeminiActivityTrackerWithPath creates a tracker with custom base directory.
// This is useful for testing with custom history directories.
func NewGeminiActivityTrackerWithPath(baseDir string) *GeminiActivityTracker {
	return &GeminiActivityTracker{
		baseDir: baseDir,
	}
}

// GetLastActivity reads per-session Gemini history file and returns the most recent timestamp.
//
// Implementation:
// 1. Build path: ~/.agm/gemini/<sessionID>/history.jsonl
// 2. Read file line-by-line (JSONL format)
// 3. Parse each line as agent.Message
// 4. Track maximum timestamp
// 5. Return max timestamp or error
func (t *GeminiActivityTracker) GetLastActivity(sessionID string) (time.Time, error) {
	// Build path to per-session history file
	historyPath := filepath.Join(t.baseDir, sessionID, "history.jsonl")

	// Open file
	file, err := os.Open(historyPath)
	if err != nil {
		// Check if file doesn't exist
		if os.IsNotExist(err) {
			return time.Time{}, fmt.Errorf("%w: %s", ErrHistoryNotFound, historyPath)
		}
		// Check if permission denied
		if os.IsPermission(err) {
			return time.Time{}, fmt.Errorf("%w: %s", ErrPermissionDenied, historyPath)
		}
		// Other errors
		return time.Time{}, fmt.Errorf("failed to open history file: %w", err)
	}
	defer file.Close()

	// Read file line-by-line
	scanner := bufio.NewScanner(file)
	var maxTimestamp time.Time
	lineNum := 0
	hasMessages := false

	for scanner.Scan() {
		lineNum++
		line := scanner.Text()

		// Skip empty lines
		if line == "" {
			continue
		}

		// Parse line as agent.Message
		var msg agent.Message
		if err := json.Unmarshal([]byte(line), &msg); err != nil {
			// JSON parsing failed - corrupted history
			return time.Time{}, fmt.Errorf("%w: line %d: %v", ErrHistoryCorrupted, lineNum, err)
		}

		// Track maximum timestamp
		if msg.Timestamp.After(maxTimestamp) {
			maxTimestamp = msg.Timestamp
		}
		hasMessages = true
	}

	// Check for scanner errors
	if err := scanner.Err(); err != nil {
		return time.Time{}, fmt.Errorf("error reading history file: %w", err)
	}

	// Check if file was empty
	if !hasMessages {
		return time.Time{}, fmt.Errorf("%w: %s", ErrEmptyHistory, historyPath)
	}

	// Return max timestamp in UTC
	return maxTimestamp.UTC(), nil
}

// GetLastActivityBatch efficiently retrieves last activity for multiple Gemini sessions.
// Reads each session's history file once and returns a map of results.
//
// For Gemini, this is still per-session file reads (unlike Claude's single file),
// but it's still more efficient than individual calls if we need to batch other operations.
func (t *GeminiActivityTracker) GetLastActivityBatch(sessionIDs []string) map[string]time.Time {
	result := make(map[string]time.Time)

	for _, sessionID := range sessionIDs {
		timestamp, err := t.GetLastActivity(sessionID)
		if err != nil {
			// Skip sessions with errors (missing files, etc.)
			continue
		}
		result[sessionID] = timestamp
	}

	return result
}
