package history

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

const (
	// scannerBufSize is 4MB — corrupted files with null bytes can produce very long "lines"
	scannerBufSize = 4 * 1024 * 1024
)

// Entry represents a single line from ~/.claude/history.jsonl (old format)
type Entry struct {
	UUID      string    `json:"uuid"`
	Directory string    `json:"directory"`
	Timestamp time.Time `json:"timestamp"`
	Name      string    `json:"name,omitempty"`
}

// ConversationEntry represents a user prompt from history.jsonl (new format)
type ConversationEntry struct {
	Display        string                 `json:"display"`
	PastedContents map[string]interface{} `json:"pastedContents"`
	Timestamp      int64                  `json:"timestamp"` // Unix timestamp in milliseconds
	Project        string                 `json:"project"`
	SessionID      string                 `json:"sessionId,omitempty"`
}

// SessionHistory groups conversation entries by session ID
type SessionHistory struct {
	SessionID string
	Entries   []*ConversationEntry
	Project   string // Most common project path for this session
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
	scanner.Buffer(make([]byte, 0, 64*1024), scannerBufSize)

	lineNum := 0
	for scanner.Scan() {
		lineNum++
		line := scanner.Bytes()

		// Strip null bytes from corrupted lines
		if bytes.ContainsRune(line, 0) {
			line = bytes.ReplaceAll(line, []byte{0}, nil)
		}

		// Skip empty lines
		if len(bytes.TrimSpace(line)) == 0 {
			continue
		}

		// Try parsing as new format first (ConversationEntry with int64 timestamp)
		var convEntry ConversationEntry
		if err := json.Unmarshal(line, &convEntry); err == nil {
			// Successfully parsed new format, but skip since we're looking for old format
			continue
		}

		// Try parsing as old format (Entry with time.Time timestamp)
		var entry Entry
		if err := json.Unmarshal(line, &entry); err != nil {
			// Both formats failed - skip this line silently
			continue
		}

		entries = append(entries, &entry)
	}

	// On scanner error, return partial results instead of failing
	_ = scanner.Err()

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

// ReadConversations reads conversation entries from history.jsonl and groups by session ID
func (p *Parser) ReadConversations(limit int) ([]*SessionHistory, error) {
	file, err := os.Open(p.historyPath)
	if err != nil {
		if os.IsNotExist(err) {
			return []*SessionHistory{}, nil
		}
		return nil, fmt.Errorf("failed to open history file: %w", err)
	}
	defer file.Close()

	var allEntries []*ConversationEntry
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 0, 64*1024), scannerBufSize)
	lineNum := 0

	for scanner.Scan() {
		lineNum++
		line := scanner.Bytes()

		// Strip null bytes from corrupted lines
		if bytes.ContainsRune(line, 0) {
			line = bytes.ReplaceAll(line, []byte{0}, nil)
		}

		if len(bytes.TrimSpace(line)) == 0 {
			continue
		}

		var entry ConversationEntry
		if err := json.Unmarshal(line, &entry); err != nil {
			// Failed to parse as new format - skip silently
			continue
		}

		// Only include entries with sessionId for grouping
		if entry.SessionID != "" {
			allEntries = append(allEntries, &entry)
		}
	}

	// On scanner error, return partial results instead of failing
	_ = scanner.Err()

	// Sort by timestamp descending (most recent first)
	for i := 0; i < len(allEntries); i++ {
		for j := i + 1; j < len(allEntries); j++ {
			if allEntries[j].Timestamp > allEntries[i].Timestamp {
				allEntries[i], allEntries[j] = allEntries[j], allEntries[i]
			}
		}
	}

	// Limit to N most recent entries
	if limit > 0 && limit < len(allEntries) {
		allEntries = allEntries[:limit]
	}

	// Group by sessionId
	sessionMap := make(map[string]*SessionHistory)
	projectCounts := make(map[string]map[string]int) // sessionID -> project -> count

	for _, entry := range allEntries {
		if _, exists := sessionMap[entry.SessionID]; !exists {
			sessionMap[entry.SessionID] = &SessionHistory{
				SessionID: entry.SessionID,
				Entries:   []*ConversationEntry{},
			}
			projectCounts[entry.SessionID] = make(map[string]int)
		}

		sessionMap[entry.SessionID].Entries = append(sessionMap[entry.SessionID].Entries, entry)

		// Track project frequency
		if entry.Project != "" {
			projectCounts[entry.SessionID][entry.Project]++
		}
	}

	// Determine most common project for each session
	for sessionID, session := range sessionMap {
		maxCount := 0
		mostCommonProject := ""
		for project, count := range projectCounts[sessionID] {
			if count > maxCount {
				maxCount = count
				mostCommonProject = project
			}
		}
		session.Project = mostCommonProject
	}

	// Convert map to slice
	var sessions []*SessionHistory
	for _, session := range sessionMap {
		sessions = append(sessions, session)
	}

	// Sort sessions by most recent conversation timestamp
	for i := 0; i < len(sessions); i++ {
		for j := i + 1; j < len(sessions); j++ {
			if len(sessions[j].Entries) > 0 && len(sessions[i].Entries) > 0 {
				if sessions[j].Entries[0].Timestamp > sessions[i].Entries[0].Timestamp {
					sessions[i], sessions[j] = sessions[j], sessions[i]
				}
			}
		}
	}

	return sessions, nil
}

// GetConversationSummary returns a text summary of conversations for a session (for LLM)
func GetConversationSummary(session *SessionHistory, maxEntries int) string {
	if session == nil || len(session.Entries) == 0 {
		return ""
	}

	entries := session.Entries
	if maxEntries > 0 && len(entries) > maxEntries {
		entries = entries[:maxEntries]
	}

	var summaries []string
	for _, entry := range entries {
		if entry.Display != "" {
			// Truncate long displays
			display := entry.Display
			if len(display) > 100 {
				display = display[:97] + "..."
			}
			summaries = append(summaries, display)
		}
	}

	if len(summaries) == 0 {
		return ""
	}

	return fmt.Sprintf("Conversation snippets: %s", summaries[0])
}
