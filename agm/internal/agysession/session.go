// Package agysession resolves saved AGY conversation metadata from the local
// Antigravity CLI app-data directory.
package agysession

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const (
	workspaceMarker     = "Initializing CLI store manager for workspace "
	maxLogLineSize      = 1024 * 1024
	maxAgyLogDirEntries = 256
	maxAgyLogFiles      = 64
	maxAgyLogScanBytes  = 2 * 1024 * 1024
)

// ErrLogDiscoveryBudgetExhausted means AGY metadata was not found inside the
// deterministic newest-first log budget. Callers can distinguish this from a
// complete bounded search that simply found no matching metadata.
var ErrLogDiscoveryBudgetExhausted = errors.New("AGY log discovery budget exhausted")

// ErrConversationNotFound means a complete provider metadata search found no
// saved AGY conversation for the requested workspace. It is distinct from an
// unreadable, corrupt, or incompletely searched metadata store.
var ErrConversationNotFound = errors.New("AGY conversation not found")

type agyLogCandidates struct {
	paths              []string
	omitted            int
	unprocessedEntries int
}

type agyLogCandidate struct {
	path    string
	name    string
	modTime time.Time
}

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
	candidates, err := agyLogPaths(appDir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", "", fmt.Errorf("no AGY log directory found for conversation %s", conversationID)
		}
		return "", "", err
	}
	return workspaceFromLogCandidates(candidates, conversationID)
}

func workspaceFromLogCandidates(candidates agyLogCandidates, conversationID string) (string, string, error) {
	truncatedFiles := 0
	for _, logPath := range candidates.paths {
		workspacePath, matched, truncated, err := scanLogForConversation(logPath, conversationID)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				continue
			}
			return "", "", err
		}
		if truncated {
			truncatedFiles++
		}
		if matched && workspacePath != "" {
			return workspacePath, logPath, nil
		}
	}
	if candidates.unprocessedEntries > 0 || candidates.omitted > 0 || truncatedFiles > 0 {
		return "", "", logDiscoveryBudgetError(
			"conversation "+conversationID, len(candidates.paths), candidates.unprocessedEntries, candidates.omitted, truncatedFiles)
	}
	return "", "", fmt.Errorf("failed to determine AGY workspace for conversation %s", conversationID)
}

func latestConversationForWorkspaceFromLogs(appDir, workspacePath string) (string, string, error) {
	candidates, err := agyLogPaths(appDir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", "", fmt.Errorf("%w: no AGY log directory found for workspace %s", ErrConversationNotFound, workspacePath)
		}
		return "", "", err
	}
	return latestConversationForWorkspaceFromLogCandidates(candidates, workspacePath)
}

func latestConversationForWorkspaceFromLogCandidates(candidates agyLogCandidates, workspacePath string) (string, string, error) {
	if candidates.unprocessedEntries > 0 {
		// Directory order does not establish recency, so an unprocessed entry
		// could be newer than every bounded candidate. Unlike known-ID lookup,
		// latest-workspace discovery cannot accept any match conclusively.
		return "", "", logDiscoveryBudgetError(
			"workspace "+workspacePath, 0, candidates.unprocessedEntries, candidates.omitted, 0)
	}
	for index, logPath := range candidates.paths {
		conversationID, matched, truncated, err := scanLogForWorkspace(logPath, workspacePath)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				continue
			}
			return "", "", err
		}
		if truncated {
			// A prefix match is not conclusive for latest-session lookup: the
			// unscanned tail may contain a newer marker for this workspace. A
			// truncated newer candidate also makes any older-file match unsafe.
			return "", "", logDiscoveryBudgetError("workspace "+workspacePath, index+1, 0, candidates.omitted, 1)
		}
		if matched && conversationID != "" {
			return conversationID, logPath, nil
		}
	}
	if candidates.omitted > 0 {
		return "", "", logDiscoveryBudgetError("workspace "+workspacePath, len(candidates.paths), 0, candidates.omitted, 0)
	}
	return "", "", fmt.Errorf("%w: no AGY conversation recorded for workspace: %s", ErrConversationNotFound, workspacePath)
}

func agyLogPaths(appDir string) (agyLogCandidates, error) {
	logDir := filepath.Join(appDir, "log")
	dir, err := os.Open(logDir)
	if err != nil {
		return agyLogCandidates{}, fmt.Errorf("open AGY log directory: %w", err)
	}
	defer dir.Close()
	entries, err := dir.ReadDir(maxAgyLogDirEntries + 1)
	if err != nil && !errors.Is(err, io.EOF) {
		return agyLogCandidates{}, fmt.Errorf("read AGY log directory: %w", err)
	}
	unprocessedEntries := 0
	if len(entries) > maxAgyLogDirEntries {
		unprocessedEntries = len(entries) - maxAgyLogDirEntries
		entries = entries[:maxAgyLogDirEntries]
	}
	logs, err := collectAgyLogCandidates(logDir, entries)
	if err != nil {
		return agyLogCandidates{}, err
	}
	sort.Slice(logs, func(i, j int) bool {
		if logs[i].modTime.Equal(logs[j].modTime) {
			return logs[i].name > logs[j].name
		}
		return logs[i].modTime.After(logs[j].modTime)
	})
	omitted := 0
	if len(logs) > maxAgyLogFiles {
		omitted = len(logs) - maxAgyLogFiles
		logs = logs[:maxAgyLogFiles]
	}
	paths := make([]string, len(logs))
	for i, log := range logs {
		paths[i] = log.path
	}
	return agyLogCandidates{
		paths: paths, omitted: omitted, unprocessedEntries: unprocessedEntries,
	}, nil
}

func collectAgyLogCandidates(logDir string, entries []os.DirEntry) ([]agyLogCandidate, error) {
	logs := make([]agyLogCandidate, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		info, infoErr := entry.Info()
		if infoErr != nil {
			// Log rotation may invalidate the bounded directory snapshot before
			// metadata collection. Only a disappeared entry is safe to omit;
			// permission and other errors could hide a newer candidate.
			if errors.Is(infoErr, os.ErrNotExist) {
				continue
			}
			return nil, fmt.Errorf("stat AGY log %s: %w", entry.Name(), infoErr)
		}
		if !info.Mode().IsRegular() {
			continue
		}
		logs = append(logs, agyLogCandidate{
			path: filepath.Join(logDir, entry.Name()), name: entry.Name(), modTime: info.ModTime(),
		})
	}
	return logs, nil
}

func scanLogForConversation(logPath, conversationID string) (workspacePath string, matched, truncated bool, err error) {
	file, err := os.Open(logPath)
	if err != nil {
		return "", false, false, fmt.Errorf("open AGY log %s: %w", logPath, err)
	}
	defer file.Close()
	scanner := bufio.NewScanner(io.LimitReader(file, maxAgyLogScanBytes))
	scanner.Buffer(make([]byte, 0, 64*1024), maxLogLineSize)
	currentWorkspace := ""
	for scanner.Scan() {
		line := scanner.Text()
		if detectedWorkspacePath, ok := workspaceFromLogLine(line); ok {
			currentWorkspace = detectedWorkspacePath
		}
		if strings.Contains(line, "Created conversation "+conversationID) ||
			strings.Contains(line, "Resuming conversation "+conversationID) ||
			strings.Contains(line, "GetConversationDetail: found conversation "+conversationID) {
			matched = true
			workspacePath = currentWorkspace
		}
	}
	if err := scanner.Err(); err != nil {
		return "", false, truncated, fmt.Errorf("scan AGY log %s: %w", logPath, err)
	}
	truncated, err = logHasUnreadTail(file)
	if err != nil {
		return "", false, false, fmt.Errorf("probe AGY log %s for unread tail: %w", logPath, err)
	}
	return workspacePath, matched, truncated, nil
}

func scanLogForWorkspace(logPath, workspacePath string) (conversationID string, matched, truncated bool, err error) {
	file, err := os.Open(logPath)
	if err != nil {
		return "", false, false, fmt.Errorf("open AGY log %s: %w", logPath, err)
	}
	defer file.Close()
	currentWorkspace := ""
	scanner := bufio.NewScanner(io.LimitReader(file, maxAgyLogScanBytes))
	scanner.Buffer(make([]byte, 0, 64*1024), maxLogLineSize)
	for scanner.Scan() {
		line := scanner.Text()
		if detectedWorkspacePath, ok := workspaceFromLogLine(line); ok {
			currentWorkspace = detectedWorkspacePath
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
		return "", false, truncated, fmt.Errorf("scan AGY log %s: %w", logPath, err)
	}
	truncated, err = logHasUnreadTail(file)
	if err != nil {
		return "", false, false, fmt.Errorf("probe AGY log %s for unread tail: %w", logPath, err)
	}
	return conversationID, matched, truncated, nil
}

func logHasUnreadTail(file *os.File) (bool, error) {
	var probe [1]byte
	n, err := file.Read(probe[:])
	if err == nil {
		return n > 0, nil
	}
	if errors.Is(err, io.EOF) {
		return false, nil
	}
	return false, err
}

func logDiscoveryBudgetError(target string, scanned, unprocessedEntries, omitted, truncated int) error {
	return fmt.Errorf("%w for %s: scanned %d selected logs (max %d), at most %d bytes each; left at least %d directory entries unprocessed, omitted %d older logs from the selected set, and truncated %d oversized logs",
		ErrLogDiscoveryBudgetExhausted, target, scanned, maxAgyLogFiles, maxAgyLogScanBytes, unprocessedEntries, omitted, truncated)
}

func extractConversationID(line, marker string) string {
	_, conversationID, found := strings.Cut(line, marker)
	if !found {
		return ""
	}
	conversationID = strings.TrimSpace(conversationID)
	for _, sep := range []string{" ", "\t", "("} {
		prefix, _, found := strings.Cut(conversationID, sep)
		if found {
			conversationID = prefix
		}
	}
	return conversationID
}

func workspaceFromLogLine(line string) (string, bool) {
	_, workspacePath, found := strings.Cut(line, workspaceMarker)
	if !found {
		return "", false
	}
	return strings.TrimSpace(workspacePath), true
}
