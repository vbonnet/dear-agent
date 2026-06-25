package agysession

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// Metadata describes a saved AGY conversation discovered from the local AGY
// app-data directory.
type Metadata struct {
	ConversationID     string
	WorkspacePath      string
	ConversationDBPath string
	TranscriptPath     string
	TranscriptFullPath string
	LogPath            string
	ModTime            time.Time
}

// FindByID resolves a saved AGY conversation by its conversation UUID.
func FindByID(homeDir, conversationID string) (*Metadata, error) {
	if conversationID == "" {
		return nil, fmt.Errorf("conversation ID cannot be empty")
	}
	appDir := filepath.Join(homeDir, ".gemini", "antigravity-cli")
	conversationDBPath := filepath.Join(appDir, "conversations", conversationID+".db")
	info, err := os.Stat(conversationDBPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("no AGY saved conversation found for ID: %s", conversationID)
		}
		return nil, fmt.Errorf("stat AGY conversation DB: %w", err)
	}

	meta := &Metadata{
		ConversationID:     conversationID,
		ConversationDBPath: conversationDBPath,
		ModTime:            info.ModTime(),
	}
	populateTranscriptPaths(appDir, meta)

	if workspacePath := workspaceFromLastConversations(appDir, conversationID); workspacePath != "" {
		meta.WorkspacePath = workspacePath
		return meta, nil
	}

	workspacePath, logPath, err := workspaceFromLogs(appDir, conversationID)
	if err != nil {
		return nil, err
	}
	meta.WorkspacePath = workspacePath
	meta.LogPath = logPath
	return meta, nil
}

// FindLatestForWorkspace resolves the latest AGY conversation recorded for the
// given workspace path.
func FindLatestForWorkspace(homeDir, workspacePath string) (*Metadata, error) {
	if workspacePath == "" {
		return nil, fmt.Errorf("workspace path cannot be empty")
	}
	appDir := filepath.Join(homeDir, ".gemini", "antigravity-cli")
	lastConversations, err := loadLastConversations(appDir)
	if err != nil {
		return nil, err
	}
	conversationID := lastConversations[workspacePath]
	if conversationID != "" {
		return FindByID(homeDir, conversationID)
	}
	conversationID, _, err = latestConversationForWorkspaceFromLogs(appDir, workspacePath)
	if err != nil {
		return nil, err
	}
	return FindByID(homeDir, conversationID)
}

func populateTranscriptPaths(appDir string, meta *Metadata) {
	transcriptDir := filepath.Join(appDir, "brain", meta.ConversationID, ".system_generated", "logs")
	transcriptPath := filepath.Join(transcriptDir, "transcript.jsonl")
	if info, err := os.Stat(transcriptPath); err == nil {
		meta.TranscriptPath = transcriptPath
		if info.ModTime().After(meta.ModTime) {
			meta.ModTime = info.ModTime()
		}
	}
	transcriptFullPath := filepath.Join(transcriptDir, "transcript_full.jsonl")
	if _, err := os.Stat(transcriptFullPath); err == nil {
		meta.TranscriptFullPath = transcriptFullPath
	}
}

func loadLastConversations(appDir string) (map[string]string, error) {
	cachePath := filepath.Join(appDir, "cache", "last_conversations.json")
	data, err := os.ReadFile(cachePath)
	if err != nil {
		if os.IsNotExist(err) {
			return map[string]string{}, nil
		}
		return nil, fmt.Errorf("read AGY last_conversations cache: %w", err)
	}
	if len(strings.TrimSpace(string(data))) == 0 {
		return map[string]string{}, nil
	}
	var lastConversations map[string]string
	if err := json.Unmarshal(data, &lastConversations); err != nil {
		return nil, fmt.Errorf("parse AGY last_conversations cache: %w", err)
	}
	return lastConversations, nil
}

func workspaceFromLastConversations(appDir, conversationID string) string {
	lastConversations, err := loadLastConversations(appDir)
	if err != nil {
		return ""
	}
	for workspacePath, cachedConversationID := range lastConversations {
		if cachedConversationID == conversationID {
			return workspacePath
		}
	}
	return ""
}

func workspaceFromLogs(appDir, conversationID string) (string, string, error) {
	logDir := filepath.Join(appDir, "log")
	entries, err := os.ReadDir(logDir)
	if err != nil {
		if os.IsNotExist(err) {
			return "", "", fmt.Errorf("no AGY log directory found for conversation %s", conversationID)
		}
		return "", "", fmt.Errorf("read AGY log directory: %w", err)
	}
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Name() > entries[j].Name()
	})
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		logPath := filepath.Join(logDir, entry.Name())
		workspacePath, matched, err := scanLogForConversation(logPath, conversationID)
		if err != nil {
			return "", "", err
		}
		if matched && workspacePath != "" {
			return workspacePath, logPath, nil
		}
	}
	return "", "", fmt.Errorf("failed to determine AGY workspace for conversation %s", conversationID)
}

func latestConversationForWorkspaceFromLogs(appDir, workspacePath string) (string, string, error) {
	logDir := filepath.Join(appDir, "log")
	entries, err := os.ReadDir(logDir)
	if err != nil {
		if os.IsNotExist(err) {
			return "", "", fmt.Errorf("no AGY log directory found for workspace %s", workspacePath)
		}
		return "", "", fmt.Errorf("read AGY log directory: %w", err)
	}
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Name() > entries[j].Name()
	})
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		logPath := filepath.Join(logDir, entry.Name())
		conversationID, matched, err := scanLogForWorkspace(logPath, workspacePath)
		if err != nil {
			return "", "", err
		}
		if matched && conversationID != "" {
			return conversationID, logPath, nil
		}
	}
	return "", "", fmt.Errorf("no AGY conversation recorded for workspace: %s", workspacePath)
}

func scanLogForConversation(logPath, conversationID string) (workspacePath string, matched bool, err error) {
	file, err := os.Open(logPath)
	if err != nil {
		return "", false, fmt.Errorf("open AGY log %s: %w", logPath, err)
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		if idx := strings.Index(line, "Initializing CLI store manager for workspace "); idx >= 0 {
			workspacePath = strings.TrimSpace(line[idx+len("Initializing CLI store manager for workspace "):])
		}
		if strings.Contains(line, "Created conversation "+conversationID) ||
			strings.Contains(line, "Resuming conversation "+conversationID) ||
			strings.Contains(line, "GetConversationDetail: found conversation "+conversationID) {
			matched = true
		}
	}
	if err := scanner.Err(); err != nil {
		return "", false, fmt.Errorf("scan AGY log %s: %w", logPath, err)
	}
	return workspacePath, matched, nil
}

func scanLogForWorkspace(logPath, workspacePath string) (conversationID string, matched bool, err error) {
	file, err := os.Open(logPath)
	if err != nil {
		return "", false, fmt.Errorf("open AGY log %s: %w", logPath, err)
	}
	defer file.Close()

	currentWorkspace := ""
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		if idx := strings.Index(line, "Initializing CLI store manager for workspace "); idx >= 0 {
			currentWorkspace = strings.TrimSpace(line[idx+len("Initializing CLI store manager for workspace "):])
			continue
		}
		if currentWorkspace != workspacePath {
			continue
		}
		if id := extractConversationID(line, "Created conversation "); id != "" {
			conversationID = id
			matched = true
			continue
		}
		if id := extractConversationID(line, "Resuming conversation "); id != "" {
			conversationID = id
			matched = true
			continue
		}
		if id := extractConversationID(line, "GetConversationDetail: found conversation "); id != "" {
			conversationID = id
			matched = true
			continue
		}
	}
	if err := scanner.Err(); err != nil {
		return "", false, fmt.Errorf("scan AGY log %s: %w", logPath, err)
	}
	return conversationID, matched, nil
}

func extractConversationID(line, marker string) string {
	idx := strings.Index(line, marker)
	if idx < 0 {
		return ""
	}
	conversationID := strings.TrimSpace(line[idx+len(marker):])
	if end := strings.IndexAny(conversationID, " \t("); end >= 0 {
		conversationID = conversationID[:end]
	}
	return conversationID
}
