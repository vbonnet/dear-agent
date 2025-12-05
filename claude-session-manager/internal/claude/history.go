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

const (
	// MaxHistoryEntries is the maximum number of entries to parse from history.jsonl
	// to prevent unbounded memory allocation. Set to 1 million entries (~100MB at 100 bytes/entry).
	MaxHistoryEntries = 1_000_000
)

// RawEntry represents a single entry from history.jsonl
type RawEntry struct {
	SessionID string  `json:"sessionId"` // May be empty (skipped entries, null in history file)
	Project   string  `json:"project"`   // Directory path
	Timestamp float64 `json:"timestamp"` // Unix milliseconds
}

// ParseStats contains statistics about the parsing operation
type ParseStats struct {
	TotalLines    int // Total lines read (including empty)
	ValidEntries  int // Entries successfully parsed with sessionId
	SkippedEmpty  int // Lines skipped (empty or no sessionId)
	SkippedErrors int // Lines skipped due to parse errors
}

// ParseHistory parses history.jsonl and returns valid entries with statistics
func ParseHistory(path string) ([]RawEntry, *ParseStats, error) {
	file, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil, fmt.Errorf("history.jsonl not found at %s. Have you used Claude CLI before?", path)
		}
		if os.IsPermission(err) {
			return nil, nil, fmt.Errorf("cannot read history.jsonl: permission denied")
		}
		return nil, nil, fmt.Errorf("failed to open history: %w", err)
	}
	defer file.Close()

	stats := &ParseStats{}
	var entries []RawEntry
	scanner := bufio.NewScanner(file)

	for scanner.Scan() {
		stats.TotalLines++
		line := scanner.Text()

		// Skip empty lines
		if line == "" {
			stats.SkippedEmpty++
			continue
		}

		var entry RawEntry
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			// Log warning to stderr, skip line, continue
			log.Printf("Warning: Skipped malformed line %d: %v\n", stats.TotalLines, err)
			stats.SkippedErrors++
			continue
		}

		// Skip entries without sessionId
		if entry.SessionID == "" {
			stats.SkippedEmpty++
			continue
		}

		// Enforce maximum entries limit
		if len(entries) >= MaxHistoryEntries {
			return nil, nil, fmt.Errorf("exceeded maximum history entries (%d). File may be corrupted or too large", MaxHistoryEntries)
		}

		entries = append(entries, entry)
		stats.ValidEntries++
	}

	if err := scanner.Err(); err != nil {
		return nil, nil, fmt.Errorf("error reading history: %w", err)
	}

	return entries, stats, nil
}
