package history

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// Entry represents a single line from ~/.claude/history.jsonl
type Entry struct {
	UUID      string    `json:"uuid"`
	Directory string    `json:"directory"`
	Timestamp time.Time `json:"timestamp"`
	Name      string    `json:"name,omitempty"`
}

// Parser reads and parses Claude history file
type Parser struct {
	historyPath string
}

// NewParser creates a parser for the given history file
// If path is empty, uses default ~/.claude/history.jsonl
func NewParser(path string) *Parser {
	if path == "" {
		home, _ := os.UserHomeDir()
		path = filepath.Join(home, ".claude", "history.jsonl")
	}
	return &Parser{historyPath: path}
}

// ReadAll reads all history entries
func (p *Parser) ReadAll() ([]*Entry, error) {
	file, err := os.Open(p.historyPath)
	if err != nil {
		if os.IsNotExist(err) {
			// History file doesn't exist yet - return empty list
			return []*Entry{}, nil
		}
		return nil, fmt.Errorf("failed to open history file: %w", err)
	}
	defer file.Close()

	var entries []*Entry
	scanner := bufio.NewScanner(file)

	lineNum := 0
	for scanner.Scan() {
		lineNum++
		line := scanner.Text()

		// Skip empty lines
		if line == "" {
			continue
		}

		var entry Entry
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			// Log warning but continue parsing
			fmt.Fprintf(os.Stderr, "Warning: failed to parse history line %d: %v\n", lineNum, err)
			continue
		}

		entries = append(entries, &entry)
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("error reading history file: %w", err)
	}

	return entries, nil
}

// FindByDirectory finds the most recent UUID for a given directory
func (p *Parser) FindByDirectory(directory string) (*Entry, error) {
	entries, err := p.ReadAll()
	if err != nil {
		return nil, err
	}

	// Normalize the directory path for comparison
	absDir, err := filepath.Abs(directory)
	if err != nil {
		absDir = directory
	}

	// Find most recent entry matching directory
	var latest *Entry
	for _, entry := range entries {
		entryDir, err := filepath.Abs(entry.Directory)
		if err != nil {
			entryDir = entry.Directory
		}

		if entryDir == absDir {
			if latest == nil || entry.Timestamp.After(latest.Timestamp) {
				latest = entry
			}
		}
	}

	if latest == nil {
		return nil, fmt.Errorf("no history entry found for directory: %s", directory)
	}

	return latest, nil
}

// FindByUUID finds all entries with a given UUID
func (p *Parser) FindByUUID(uuid string) ([]*Entry, error) {
	entries, err := p.ReadAll()
	if err != nil {
		return nil, err
	}

	var matches []*Entry
	for _, entry := range entries {
		if entry.UUID == uuid {
			matches = append(matches, entry)
		}
	}

	return matches, nil
}

// GetRecentEntries returns the N most recent history entries
func (p *Parser) GetRecentEntries(limit int) ([]*Entry, error) {
	entries, err := p.ReadAll()
	if err != nil {
		return nil, err
	}

	// Sort by timestamp descending (most recent first)
	// Simple bubble sort since we expect small N
	for i := 0; i < len(entries); i++ {
		for j := i + 1; j < len(entries); j++ {
			if entries[j].Timestamp.After(entries[i].Timestamp) {
				entries[i], entries[j] = entries[j], entries[i]
			}
		}
	}

	// Return top N
	if limit > 0 && limit < len(entries) {
		return entries[:limit], nil
	}

	return entries, nil
}
