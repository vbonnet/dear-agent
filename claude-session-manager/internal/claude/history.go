// Package claude provides functionality for parsing and managing Claude CLI
// session history. It reads from ~/.claude/history.jsonl and provides
// deduplication, filtering, and UUID capture capabilities.
package claude

import (
	"bufio"
	"encoding/json"
	"fmt"
	"log"
	"os"
)

// RawEntry represents a single entry from history.jsonl
type RawEntry struct {
	SessionID string  `json:"sessionId"` // May be empty
	Project   string  `json:"project"`   // Directory path
	Timestamp float64 `json:"timestamp"` // Unix milliseconds
}

// ParseHistory parses history.jsonl and returns valid entries
func ParseHistory(path string) ([]RawEntry, error) {
	file, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("history.jsonl not found at %s. Have you used Claude CLI before?", path)
		}
		if os.IsPermission(err) {
			return nil, fmt.Errorf("cannot read history.jsonl: permission denied")
		}
		return nil, fmt.Errorf("failed to open history: %w", err)
	}
	defer file.Close()

	var entries []RawEntry
	scanner := bufio.NewScanner(file)
	lineNum := 0

	for scanner.Scan() {
		lineNum++
		line := scanner.Text()

		// Skip empty lines
		if line == "" {
			continue
		}

		var entry RawEntry
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			// Log warning to stderr, skip line, continue
			log.Printf("Warning: Skipped malformed line %d: %v\n", lineNum, err)
			continue
		}

		// Skip entries without sessionId
		if entry.SessionID == "" {
			continue
		}

		entries = append(entries, entry)
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("error reading history: %w", err)
	}

	return entries, nil
}
