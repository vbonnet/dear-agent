// Package session provides session UUID extraction utilities.
package session

import (
	"bufio"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"time"
)

var uuidRegex = regexp.MustCompile(`^[a-f0-9]{8}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{12}$`)

type historyEntry struct {
	SessionID string `json:"sessionId"`
}

// ExtractUUID reads the last line of history.jsonl and extracts session UUID
// Returns fallback UUID if extraction fails
func ExtractUUID(historyPath string) (string, error) {
	file, err := os.Open(historyPath)
	if err != nil {
		return generateFallbackUUID("history_missing"), fmt.Errorf("history file not found: %w", err)
	}
	defer file.Close()

	// Read last line efficiently using scanner
	var lastLine string
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		lastLine = scanner.Text()
	}

	if err := scanner.Err(); err != nil {
		return generateFallbackUUID("scan_error"), fmt.Errorf("error reading history: %w", err)
	}

	if lastLine == "" {
		return generateFallbackUUID("empty_file"), fmt.Errorf("history file empty")
	}

	// Parse JSON
	var entry historyEntry
	if err := json.Unmarshal([]byte(lastLine), &entry); err != nil {
		return generateFallbackUUID("invalid_json"), fmt.Errorf("JSON parse failed: %w", err)
	}

	// Validate UUID format
	if !uuidRegex.MatchString(entry.SessionID) {
		return generateFallbackUUID("invalid_format"), fmt.Errorf("invalid UUID format: %s", entry.SessionID)
	}

	return entry.SessionID, nil
}

// generateFallbackUUID creates auto-<timestamp>-<random> format
func generateFallbackUUID(_ string) string {
	timestamp := time.Now().Unix()

	// Generate 4-char random hex suffix
	b := make([]byte, 2)
	if _, err := rand.Read(b); err != nil {
		// Fallback to timestamp-based suffix
		return fmt.Sprintf("auto-%d-%04x", timestamp, timestamp&0xFFFF)
	}

	return fmt.Sprintf("auto-%d-%04x", timestamp, b)
}
