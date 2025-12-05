package claude

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
)

// HistoryEntry represents a single entry from history.jsonl
type HistoryEntry struct {
	UUID      string
	WorkDir   string
	Timestamp time.Time
}

// ParseHistoryJSONL parses history.jsonl without jq dependency
func ParseHistoryJSONL(path string) ([]*HistoryEntry, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("failed to open history.jsonl: %w", err)
	}
	defer file.Close()

	var entries []*HistoryEntry
	scanner := bufio.NewScanner(file)

	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			continue
		}

		// Parse JSON line
		var data map[string]interface{}
		if err := json.Unmarshal([]byte(line), &data); err != nil {
			// Skip malformed lines
			continue
		}

		// Extract fields
		uuid, ok := data["session_id"].(string)
		if !ok {
			continue
		}

		workDir, _ := data["cwd"].(string)

		var timestamp time.Time
		if ts, ok := data["timestamp"].(string); ok {
			timestamp, _ = time.Parse(time.RFC3339, ts)
		}

		entries = append(entries, &HistoryEntry{
			UUID:      uuid,
			WorkDir:   workDir,
			Timestamp: timestamp,
		})
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("failed to scan history.jsonl: %w", err)
	}

	return entries, nil
}

// Version returns Claude version
func Version() (string, error) {
	cmd := exec.Command("claude", "--version")
	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("failed to get claude version: %w", err)
	}
	return strings.TrimSpace(string(output)), nil
}
