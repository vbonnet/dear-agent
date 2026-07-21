// Package agysession resolves saved AGY conversation metadata from the local
// Antigravity CLI app-data directory.
package agysession

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/vbonnet/dear-agent/agm/internal/lock"
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

const workspaceCreateLockRetryDelay = 25 * time.Millisecond

type workspaceFileLock interface {
	TryLock() error
	Unlock() error
}

var newWorkspaceFileLock = func(path string) (workspaceFileLock, error) {
	return lock.New(path)
}

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
	if err := ValidateConversationID(conversationID); err != nil {
		return nil, err
	}
	appDir := filepath.Join(homeDir, ".gemini", "antigravity-cli")
	conversationDBPath := filepath.Join(appDir, "conversations", conversationID+".db")
	info, err := os.Stat(conversationDBPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("%w for ID: %s", ErrConversationNotFound, conversationID)
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

// ValidateConversationID rejects values that are unsafe as both an AGY CLI
// argument and a provider-owned path component. Current AGY IDs are UUIDs, but
// the bounded identifier grammar preserves compatibility with future formats.
func ValidateConversationID(conversationID string) error {
	if len(conversationID) == 0 || len(conversationID) > 128 {
		return fmt.Errorf("invalid AGY native conversation ID: expected 1-128 safe identifier characters")
	}
	for i := 0; i < len(conversationID); i++ {
		c := conversationID[i]
		if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || (i > 0 && (c == '-' || c == '_')) {
			continue
		}
		return fmt.Errorf("invalid AGY native conversation ID: expected an alphanumeric identifier containing only letters, digits, hyphens, or underscores")
	}
	return nil
}

// CanonicalWorkspacePath returns one physical path spelling for an existing
// AGY workspace. AGY's latest-conversation mapping is workspace-global, so
// lock, launch, and identity lookup callers must not treat a symlink alias as a
// different workspace. Removed historical workspaces retain their cleaned
// absolute spelling so saved-session metadata remains searchable.
func CanonicalWorkspacePath(workDir string) (string, error) {
	if strings.TrimSpace(workDir) == "" {
		return "", fmt.Errorf("AGY workspace path cannot be empty")
	}
	absoluteWorkDir, err := filepath.Abs(workDir)
	if err != nil {
		return "", fmt.Errorf("resolve absolute AGY workspace path: %w", err)
	}
	resolvedWorkDir, err := filepath.EvalSymlinks(absoluteWorkDir)
	if err != nil {
		if os.IsNotExist(err) {
			return filepath.Clean(absoluteWorkDir), nil
		}
		return "", fmt.Errorf("resolve AGY workspace symlinks: %w", err)
	}
	return filepath.Clean(resolvedWorkDir), nil
}

// AcquireWorkspaceCreateLock serializes AGY conversation creation for one
// canonical workspace. AGY exposes only a workspace-global latest-conversation
// mapping, so every AGM launch surface must hold this lock until its native ID
// has been discovered and persisted.
func AcquireWorkspaceCreateLock(ctx context.Context, workDir string) (func() error, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	canonicalWorkDir, err := CanonicalWorkspacePath(workDir)
	if err != nil {
		return nil, fmt.Errorf("resolve AGY workspace lock path: %w", err)
	}
	stateDir := os.Getenv("AGM_STATE_DIR")
	if stateDir == "" {
		stateDir = fmt.Sprintf("/tmp/agm-%d", os.Getuid())
	}
	digest := sha256.Sum256([]byte(canonicalWorkDir))
	fileLock, err := newWorkspaceFileLock(filepath.Join(stateDir, fmt.Sprintf("agy-create-%x.lock", digest[:16])))
	if err != nil {
		return nil, fmt.Errorf("create AGY workspace lock: %w", err)
	}
	for {
		if err := ctx.Err(); err != nil {
			_ = fileLock.Unlock()
			return nil, fmt.Errorf("acquire AGY workspace lock: %w", err)
		}
		lockErr := fileLock.TryLock()
		if lockErr == nil {
			return fileLock.Unlock, nil
		}
		var contention *lock.LockError
		if !errors.As(lockErr, &contention) {
			_ = fileLock.Unlock()
			return nil, fmt.Errorf("acquire AGY workspace lock: %w", lockErr)
		}
		timer := time.NewTimer(workspaceCreateLockRetryDelay)
		select {
		case <-ctx.Done():
			timer.Stop()
			_ = fileLock.Unlock()
			return nil, fmt.Errorf("acquire AGY workspace lock: %w", ctx.Err())
		case <-timer.C:
		}
	}
}

// FindLatestForWorkspace resolves the latest AGY conversation recorded for the
// given workspace path.
func FindLatestForWorkspace(homeDir, workspacePath string) (*Metadata, error) {
	if workspacePath == "" {
		return nil, fmt.Errorf("workspace path cannot be empty")
	}
	canonicalWorkspacePath, err := CanonicalWorkspacePath(workspacePath)
	if err != nil {
		return nil, fmt.Errorf("resolve AGY workspace for saved-session lookup: %w", err)
	}
	workspacePath = canonicalWorkspacePath
	appDir := filepath.Join(homeDir, ".gemini", "antigravity-cli")
	lastConversations, err := loadLastConversations(appDir)
	if err != nil {
		return nil, err
	}
	metadata, err := cachedConversationForWorkspace(homeDir, lastConversations, workspacePath)
	if err != nil {
		return nil, err
	}
	if metadata != nil {
		return metadata, nil
	}
	return latestUsableConversationForWorkspaceFromLogs(homeDir, appDir, workspacePath)
}

func cachedConversationForWorkspace(homeDir string, lastConversations map[string]string, workspacePath string) (*Metadata, error) {
	keys := make([]string, 0, len(lastConversations))
	for providerWorkspacePath := range lastConversations {
		keys = append(keys, providerWorkspacePath)
	}
	sort.Strings(keys)
	seenConversationIDs := make(map[string]struct{}, len(keys))
	var selected *Metadata
	for _, providerWorkspacePath := range keys {
		canonicalProviderPath, err := CanonicalWorkspacePath(providerWorkspacePath)
		if err != nil {
			return nil, fmt.Errorf("resolve AGY cached workspace path %q: %w", providerWorkspacePath, err)
		}
		if canonicalProviderPath != workspacePath {
			continue
		}
		conversationID := lastConversations[providerWorkspacePath]
		if conversationID == "" {
			continue
		}
		if _, seen := seenConversationIDs[conversationID]; seen {
			continue
		}
		seenConversationIDs[conversationID] = struct{}{}
		metadata, findErr := FindByID(homeDir, conversationID)
		if errors.Is(findErr, ErrConversationNotFound) {
			continue
		}
		if findErr != nil {
			return nil, findErr
		}
		if selected == nil || metadata.ModTime.After(selected.ModTime) {
			captured := *metadata
			captured.WorkspacePath = workspacePath
			selected = &captured
		}
	}
	return selected, nil
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
	keys := make([]string, 0, len(lastConversations))
	for providerWorkspacePath := range lastConversations {
		keys = append(keys, providerWorkspacePath)
	}
	sort.Strings(keys)
	for _, providerWorkspacePath := range keys {
		if lastConversations[providerWorkspacePath] != conversationID {
			continue
		}
		canonicalProviderPath, canonicalErr := CanonicalWorkspacePath(providerWorkspacePath)
		if canonicalErr == nil {
			return canonicalProviderPath
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

func latestUsableConversationForWorkspaceFromLogs(homeDir, appDir, workspacePath string) (*Metadata, error) {
	candidates, err := agyLogPaths(appDir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("%w: no AGY log directory found for workspace %s", ErrConversationNotFound, workspacePath)
		}
		return nil, err
	}
	var selected *Metadata
	_, _, err = latestUsableConversationForWorkspaceFromLogCandidates(candidates, workspacePath, func(conversationID string) (bool, error) {
		metadata, findErr := FindByID(homeDir, conversationID)
		if findErr != nil {
			if errors.Is(findErr, ErrConversationNotFound) {
				return false, nil
			}
			return false, findErr
		}
		selected = metadata
		return true, nil
	})
	if err != nil {
		return nil, err
	}
	if selected == nil {
		return nil, fmt.Errorf("resolve AGY workspace conversation: provider returned empty metadata")
	}
	selected.WorkspacePath = workspacePath
	return selected, nil
}

func latestConversationForWorkspaceFromLogCandidates(candidates agyLogCandidates, workspacePath string) (string, string, error) {
	return latestUsableConversationForWorkspaceFromLogCandidates(candidates, workspacePath, func(string) (bool, error) {
		return true, nil
	})
}

func latestUsableConversationForWorkspaceFromLogCandidates(
	candidates agyLogCandidates,
	workspacePath string,
	usable func(string) (bool, error),
) (string, string, error) {
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
			isUsable, usableErr := usable(conversationID)
			if usableErr != nil {
				return "", "", usableErr
			}
			if isUsable {
				return conversationID, logPath, nil
			}
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
		detectedWorkspacePath, ok, workspaceErr := workspaceFromLogLine(line)
		if workspaceErr != nil {
			return "", false, false, fmt.Errorf("scan AGY log %s workspace marker: %w", logPath, workspaceErr)
		}
		if ok {
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
		detectedWorkspacePath, ok, workspaceErr := workspaceFromLogLine(line)
		if workspaceErr != nil {
			return "", false, false, fmt.Errorf("scan AGY log %s workspace marker: %w", logPath, workspaceErr)
		}
		if ok {
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

func workspaceFromLogLine(line string) (string, bool, error) {
	_, workspacePath, found := strings.Cut(line, workspaceMarker)
	if !found {
		return "", false, nil
	}
	canonicalWorkspacePath, err := CanonicalWorkspacePath(strings.TrimSpace(workspacePath))
	if err != nil {
		return "", true, err
	}
	return canonicalWorkspacePath, true, nil
}
