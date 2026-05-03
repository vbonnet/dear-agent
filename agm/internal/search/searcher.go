// Package search provides search functionality.
package search

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/vbonnet/dear-agent/agm/internal/conversation"
	"github.com/vbonnet/dear-agent/agm/internal/dolt"
	"github.com/vbonnet/dear-agent/agm/internal/history"
)

// SearchResult represents a match in a conversation
type SearchResult struct {
	SessionUUID    string
	SessionName    string
	Workspace      string
	MatchCount     int
	ContextSnippet string
	ProjectPath    string
}

// SearchOptions configures the search behavior
type SearchOptions struct {
	Query         string
	UseRegex      bool
	CaseSensitive bool
	Workspace     string
}

// Searcher performs content-based searches across conversation files
type Searcher struct {
	adapter *dolt.Adapter
}

// NewSearcher creates a new searcher with the given Dolt adapter
func NewSearcher(adapter *dolt.Adapter) *Searcher {
	return &Searcher{adapter: adapter}
}

// Search performs a content-based search across all sessions
func (s *Searcher) Search(opts SearchOptions) ([]*SearchResult, error) {
	// Step 1: Load all sessions from Dolt to get session-to-UUID mapping
	if s.adapter == nil {
		return nil, fmt.Errorf("dolt adapter not available")
	}

	manifests, err := s.adapter.ListSessions(&dolt.SessionFilter{})
	if err != nil {
		return nil, fmt.Errorf("failed to list sessions from Dolt: %w", err)
	}

	// Create UUID to session name mapping
	uuidToName := make(map[string]string)
	uuidToWorkspace := make(map[string]string)
	for _, m := range manifests {
		if m.Claude.UUID != "" {
			uuidToName[m.Claude.UUID] = m.Name
			uuidToWorkspace[m.Claude.UUID] = m.Workspace
		}
	}

	// Step 2: Get conversation files from history
	parser := history.NewParser("")
	sessions, err := parser.ReadConversations(10000) // Read last 10k entries
	if err != nil {
		return nil, fmt.Errorf("failed to read conversations: %w", err)
	}

	// Step 3: Search each session's conversation file
	var results []*SearchResult
	for _, session := range sessions {
		// Apply workspace filter
		workspace := uuidToWorkspace[session.SessionID]
		if opts.Workspace != "" && workspace != opts.Workspace {
			continue
		}

		// Find conversation file
		conversationPath := findConversationFile(session.SessionID)
		if conversationPath == "" {
			continue
		}

		// Search conversation content
		matchCount, snippet, err := s.searchConversationFile(conversationPath, opts)
		if err != nil {
			// Log error but continue with other files
			fmt.Fprintf(os.Stderr, "Warning: failed to search %s: %v\n", conversationPath, err)
			continue
		}

		if matchCount > 0 {
			sessionName := uuidToName[session.SessionID]
			if sessionName == "" {
				sessionName = "(no manifest)"
			}

			results = append(results, &SearchResult{
				SessionUUID:    session.SessionID,
				SessionName:    sessionName,
				Workspace:      workspace,
				MatchCount:     matchCount,
				ContextSnippet: snippet,
				ProjectPath:    session.Project,
			})
		}
	}

	return results, nil
}

// SearchWithAdapter performs a content-based search using a Dolt adapter
// Test helper function for Phase 5 migration
func (s *Searcher) SearchWithAdapter(opts SearchOptions, adapter *dolt.Adapter) ([]*SearchResult, error) {
	// Step 1: Load all sessions from database to get session-to-UUID mapping
	manifests, err := adapter.ListSessions(&dolt.SessionFilter{})
	if err != nil {
		return nil, fmt.Errorf("failed to list sessions: %w", err)
	}

	// Create UUID to session name mapping
	uuidToName := make(map[string]string)
	uuidToWorkspace := make(map[string]string)
	for _, m := range manifests {
		if m.Claude.UUID != "" {
			uuidToName[m.Claude.UUID] = m.Name
			uuidToWorkspace[m.Claude.UUID] = m.Workspace
		}
	}

	// Step 2: Get conversation files from history
	parser := history.NewParser("")
	sessions, err := parser.ReadConversations(10000) // Read last 10k entries
	if err != nil {
		return nil, fmt.Errorf("failed to read conversations: %w", err)
	}

	// Step 3: Search each session's conversation file
	var results []*SearchResult
	for _, session := range sessions {
		// Apply workspace filter
		workspace := uuidToWorkspace[session.SessionID]
		if opts.Workspace != "" && workspace != opts.Workspace {
			continue
		}

		// Find conversation file
		conversationPath := findConversationFile(session.SessionID)
		if conversationPath == "" {
			continue
		}

		// Search conversation content
		matchCount, snippet, err := s.searchConversationFile(conversationPath, opts)
		if err != nil {
			// Log error but continue with other files
			fmt.Fprintf(os.Stderr, "Warning: failed to search %s: %v\n", conversationPath, err)
			continue
		}

		if matchCount > 0 {
			sessionName := uuidToName[session.SessionID]
			if sessionName == "" {
				sessionName = "(no manifest)"
			}

			results = append(results, &SearchResult{
				SessionUUID:    session.SessionID,
				SessionName:    sessionName,
				Workspace:      workspace,
				MatchCount:     matchCount,
				ContextSnippet: snippet,
				ProjectPath:    session.Project,
			})
		}
	}

	return results, nil
}

// searchConversationFile searches a single conversation JSONL file
func (s *Searcher) searchConversationFile(path string, opts SearchOptions) (int, string, error) {
	file, err := os.Open(path)
	if err != nil {
		return 0, "", fmt.Errorf("failed to open file: %w", err)
	}
	defer file.Close()

	var counter func(string) int
	if opts.UseRegex {
		// Compile regex pattern
		flags := ""
		if !opts.CaseSensitive {
			flags = "(?i)"
		}
		re, err := regexp.Compile(flags + opts.Query)
		if err != nil {
			return 0, "", fmt.Errorf("invalid regex pattern: %w", err)
		}
		counter = func(text string) int {
			matches := re.FindAllString(text, -1)
			return len(matches)
		}
	} else {
		// Simple substring match - count all occurrences
		if opts.CaseSensitive {
			counter = func(text string) int {
				return strings.Count(text, opts.Query)
			}
		} else {
			queryLower := strings.ToLower(opts.Query)
			counter = func(text string) int {
				return strings.Count(strings.ToLower(text), queryLower)
			}
		}
	}

	matchCount := 0
	var firstMatch string
	scanner := bufio.NewScanner(file)
	lineNum := 0

	for scanner.Scan() {
		lineNum++
		line := scanner.Text()

		// Skip empty lines
		if line == "" {
			continue
		}

		// Skip header line (first line)
		if lineNum == 1 {
			continue
		}

		// Try to parse as message
		var msg conversation.Message
		if err := json.Unmarshal([]byte(line), &msg); err != nil {
			// Skip malformed lines
			continue
		}

		// Search content blocks
		for _, block := range msg.Content {
			if textBlock, ok := block.(conversation.TextBlock); ok {
				count := counter(textBlock.Text)
				if count > 0 {
					matchCount += count
					// Capture first match for snippet
					if firstMatch == "" {
						firstMatch = truncateText(textBlock.Text, 100)
					}
				}
			}
		}
	}

	if err := scanner.Err(); err != nil {
		return 0, "", fmt.Errorf("scan error: %w", err)
	}

	return matchCount, firstMatch, nil
}

// findConversationFile searches for conversation .jsonl file in ~/.claude/projects/
func findConversationFile(uuid string) string {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return ""
	}

	projectsDir := filepath.Join(homeDir, ".claude", "projects")
	entries, err := os.ReadDir(projectsDir)
	if err != nil {
		return ""
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		jsonlPath := filepath.Join(projectsDir, entry.Name(), uuid+".jsonl")
		if _, err := os.Stat(jsonlPath); err == nil {
			return jsonlPath
		}
	}

	return ""
}

// truncateText truncates text to maxLen characters, adding ellipsis if needed
func truncateText(text string, maxLen int) string {
	// Remove newlines and excessive whitespace
	text = strings.Join(strings.Fields(text), " ")

	if len(text) <= maxLen {
		return text
	}

	return text[:maxLen-3] + "..."
}
