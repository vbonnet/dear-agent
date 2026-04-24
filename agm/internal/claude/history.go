// Package claude provides functionality for parsing and managing Claude CLI
// session history. It reads from ~/.claude/history.jsonl and provides
// deduplication, filtering, and UUID capture capabilities.
package claude

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"os"

	"github.com/vbonnet/dear-agent/agm/internal/logging"
)

const (
	// scannerBufSize is 4MB — corrupted files with null bytes can produce very long "lines"
	scannerBufSize = 4 * 1024 * 1024
)

var logger = logging.DefaultLogger()

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
	scanner.Buffer(make([]byte, 0, 64*1024), scannerBufSize)

	for scanner.Scan() {
		stats.TotalLines++
		line := scanner.Bytes()

		// Strip null bytes from corrupted lines
		if bytes.ContainsRune(line, 0) {
			line = bytes.ReplaceAll(line, []byte{0}, nil)
		}

		// Skip empty lines
		if len(bytes.TrimSpace(line)) == 0 {
			stats.SkippedEmpty++
			continue
		}

		var entry RawEntry
		if err := json.Unmarshal(line, &entry); err != nil {
			// Log warning to stderr, skip line, continue
			logger.Warn("Skipped malformed line in history", "line", stats.TotalLines, "error", err)
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
			logger.Warn("Exceeded maximum history entries, returning partial results", "max", MaxHistoryEntries)
			break
		}

		entries = append(entries, entry)
		stats.ValidEntries++
	}

	if err := scanner.Err(); err != nil {
		// Return partial results instead of failing — corrupted files often have
		// valid lines before the corruption point
		logger.Warn("Scanner error reading history, returning partial results", "error", err, "validEntries", stats.ValidEntries)
		return entries, stats, nil
	}

	return entries, stats, nil
}
