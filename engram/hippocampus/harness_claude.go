package hippocampus

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// ClaudeCodeAdapter discovers and reads Claude Code session transcripts.
// Sessions are stored as JSONL files under ~/.claude/projects/<key>/<session-id>/
type ClaudeCodeAdapter struct {
	claudeDir string // default: ~/.claude
}

// NewClaudeCodeAdapter creates a Claude Code harness adapter.
// If claudeDir is empty, defaults to ~/.claude.
func NewClaudeCodeAdapter(claudeDir string) *ClaudeCodeAdapter {
	if claudeDir == "" {
		home, _ := os.UserHomeDir()
		claudeDir = filepath.Join(home, ".claude")
	}
	return &ClaudeCodeAdapter{claudeDir: claudeDir}
}

func (c *ClaudeCodeAdapter) Name() string {
	return "claude-code"
}

// GetMemoryDir returns the auto-memory directory for a project.
// Claude Code derives the project key from the absolute path by replacing
// / with - and prepending -.
func (c *ClaudeCodeAdapter) GetMemoryDir(projectPath string) (string, error) {
	absPath, err := filepath.Abs(projectPath)
	if err != nil {
		return "", fmt.Errorf("resolve project path: %w", err)
	}

	// Convert path to Claude Code project key: $HOME/src -> -home-user-src
	key := strings.ReplaceAll(absPath, string(filepath.Separator), "-")

	memDir := filepath.Join(c.claudeDir, "projects", key, "memory")

	// Check if directory exists
	if _, err := os.Stat(memDir); err != nil {
		return "", fmt.Errorf("memory dir not found: %w", err)
	}

	return memDir, nil
}

// DiscoverSessions finds Claude Code sessions since a given time.
// Scans ~/.claude/projects/<key>/<session-id>/ directories.
func (c *ClaudeCodeAdapter) DiscoverSessions(ctx context.Context, projectPath string, since time.Time) ([]SessionInfo, error) {
	absPath, err := filepath.Abs(projectPath)
	if err != nil {
		return nil, fmt.Errorf("resolve project path: %w", err)
	}

	key := strings.ReplaceAll(absPath, string(filepath.Separator), "-")
	projectDir := filepath.Join(c.claudeDir, "projects", key)

	entries, err := os.ReadDir(projectDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil // no sessions yet
		}
		return nil, fmt.Errorf("read project dir: %w", err)
	}

	var sessions []SessionInfo

	for _, entry := range entries {
		if err := ctx.Err(); err != nil {
			return nil, err
		}

		if !entry.IsDir() {
			continue
		}

		// Skip special directories
		name := entry.Name()
		if name == "memory" || name == "plans" || name == "subagents" {
			continue
		}

		info, err := entry.Info()
		if err != nil {
			continue
		}

		// Filter by modification time
		if info.ModTime().Before(since) {
			continue
		}

		// Look for JSONL files in this session directory
		sessionDir := filepath.Join(projectDir, name)
		jsonlFiles, err := findJSONLFiles(sessionDir)
		if err != nil || len(jsonlFiles) == 0 {
			continue
		}

		sessions = append(sessions, SessionInfo{
			ID:        name,
			StartTime: info.ModTime(), // approximate
			Project:   projectPath,
			FilePath:  jsonlFiles[0], // primary transcript
		})
	}

	return sessions, nil
}

// ReadTranscript reads user and assistant text from a Claude Code JSONL file.
// Extracts only text content blocks, skipping tool use/results and progress events.
func (c *ClaudeCodeAdapter) ReadTranscript(ctx context.Context, session SessionInfo) (string, error) {
	file, err := os.Open(session.FilePath)
	if err != nil {
		return "", fmt.Errorf("open transcript: %w", err)
	}
	defer file.Close()

	var texts []string
	scanner := bufio.NewScanner(file)

	// Increase buffer size for long JSONL lines
	buf := make([]byte, 0, 64*1024)
	scanner.Buffer(buf, 1024*1024) // 1MB max line

	for scanner.Scan() {
		if err := ctx.Err(); err != nil {
			return "", err
		}

		line := scanner.Bytes()
		text := extractTextFromJSONL(line)
		if text != "" {
			texts = append(texts, text)
		}
	}

	if err := scanner.Err(); err != nil {
		return "", fmt.Errorf("scan transcript: %w", err)
	}

	return strings.Join(texts, "\n"), nil
}

// extractTextFromJSONL extracts text content from a Claude Code JSONL line.
// Only extracts from user and assistant messages with text content blocks.
func extractTextFromJSONL(line []byte) string {
	var entry struct {
		Type    string `json:"type"`
		Message *struct {
			Role    string          `json:"role"`
			Content json.RawMessage `json:"content"`
		} `json:"message"`
	}

	if err := json.Unmarshal(line, &entry); err != nil {
		return ""
	}

	// Only process user and assistant messages
	if entry.Type != "user" && entry.Type != "assistant" {
		return ""
	}

	if entry.Message == nil {
		return ""
	}

	role := entry.Message.Role
	if role != "user" && role != "assistant" {
		return ""
	}

	// Parse content blocks
	var blocks []json.RawMessage
	if err := json.Unmarshal(entry.Message.Content, &blocks); err != nil {
		// Content might be a string directly
		var text string
		if err := json.Unmarshal(entry.Message.Content, &text); err == nil && text != "" {
			return role + ": " + text
		}
		return ""
	}

	var texts []string
	for _, block := range blocks {
		var b struct {
			Type string `json:"type"`
			Text string `json:"text"`
		}
		if err := json.Unmarshal(block, &b); err != nil {
			continue
		}
		if b.Type == "text" && b.Text != "" {
			texts = append(texts, b.Text)
		}
	}

	if len(texts) == 0 {
		return ""
	}

	return role + ": " + strings.Join(texts, " ")
}

// findJSONLFiles recursively finds .jsonl files in a directory.
func findJSONLFiles(dir string) ([]string, error) {
	var files []string

	err := filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil // skip errors
		}
		if !d.IsDir() && strings.HasSuffix(d.Name(), ".jsonl") {
			files = append(files, path)
		}
		return nil
	})

	return files, err
}
