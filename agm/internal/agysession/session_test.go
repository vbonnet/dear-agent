package agysession

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestFindByID_UsesLastConversationsCache(t *testing.T) {
	homeDir := t.TempDir()
	appDir := filepath.Join(homeDir, ".gemini", "antigravity-cli")
	conversationID := "117ff898-a964-4a9f-b460-1be4a8a49b17"
	workspace := "/tmp/agy-probe"

	writeAgyFixture(t, appDir, conversationID, workspace, "")

	meta, err := FindByID(homeDir, conversationID)
	if err != nil {
		t.Fatalf("FindByID returned error: %v", err)
	}
	if meta.WorkspacePath != workspace {
		t.Fatalf("workspace path = %q, want %q", meta.WorkspacePath, workspace)
	}
	if meta.ConversationDBPath == "" || !strings.HasSuffix(meta.ConversationDBPath, conversationID+".db") {
		t.Fatalf("unexpected conversation DB path: %q", meta.ConversationDBPath)
	}
	if meta.TranscriptPath == "" || !strings.HasSuffix(meta.TranscriptPath, "transcript.jsonl") {
		t.Fatalf("unexpected transcript path: %q", meta.TranscriptPath)
	}
}

func TestFindByID_RejectsUnsafeConversationIDBeforePathLookup(t *testing.T) {
	_, err := FindByID(t.TempDir(), "../../escape; touch /tmp/no")
	if err == nil || !strings.Contains(err.Error(), "invalid AGY native conversation ID") {
		t.Fatalf("FindByID error = %v, want unsafe native ID rejection", err)
	}
}

func TestAcquireWorkspaceCreateLockSerializesAndHonorsCancellation(t *testing.T) {
	t.Setenv("AGM_STATE_DIR", t.TempDir())
	preCanceled, cancelBeforeAcquire := context.WithCancel(t.Context())
	cancelBeforeAcquire()
	if _, err := AcquireWorkspaceCreateLock(preCanceled, t.TempDir()); !errors.Is(err, context.Canceled) {
		t.Fatalf("pre-canceled workspace lock error = %v, want context cancellation", err)
	}
	workDir := t.TempDir()
	aliasWorkDir := filepath.Join(t.TempDir(), "workspace-alias")
	if err := os.Symlink(workDir, aliasWorkDir); err != nil {
		t.Fatalf("create workspace symlink: %v", err)
	}
	release, err := AcquireWorkspaceCreateLock(t.Context(), workDir)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := release(); err != nil {
			t.Errorf("release first workspace lock: %v", err)
		}
	}()

	ctx, cancel := context.WithCancel(t.Context())
	result := make(chan error, 1)
	go func() {
		_, lockErr := AcquireWorkspaceCreateLock(ctx, aliasWorkDir)
		result <- lockErr
	}()
	select {
	case err := <-result:
		t.Fatalf("contending workspace lock returned before cancellation: %v", err)
	case <-time.After(75 * time.Millisecond):
	}
	cancel()
	select {
	case err := <-result:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("contending workspace lock error = %v, want context cancellation", err)
		}
	case <-time.After(time.Second):
		t.Fatal("contending workspace lock ignored caller cancellation")
	}
}

func TestFindLatestForWorkspaceCanonicalizesProviderCacheSymlinkAlias(t *testing.T) {
	homeDir := t.TempDir()
	appDir := filepath.Join(homeDir, ".gemini", "antigravity-cli")
	conversationID := "117ff898-a964-4a9f-b460-1be4a8a49b17"
	workspace, err := CanonicalWorkspacePath(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	aliasWorkDir := filepath.Join(t.TempDir(), "workspace-alias")
	if err := os.Symlink(workspace, aliasWorkDir); err != nil {
		t.Fatalf("create workspace symlink: %v", err)
	}
	writeAgyFixture(t, appDir, conversationID, aliasWorkDir, "")

	meta, err := FindLatestForWorkspace(homeDir, workspace)
	if err != nil {
		t.Fatalf("FindLatestForWorkspace with symlinked provider cache key: %v", err)
	}
	if meta.ConversationID != conversationID || meta.WorkspacePath != workspace {
		t.Fatalf("symlink lookup metadata = ID %q workspace %q, want %q/%q", meta.ConversationID, meta.WorkspacePath, conversationID, workspace)
	}
}

func TestFindLatestForWorkspaceCanonicalizesProviderLogSymlinkAlias(t *testing.T) {
	homeDir := t.TempDir()
	appDir := filepath.Join(homeDir, ".gemini", "antigravity-cli")
	conversationID := "117ff898-a964-4a9f-b460-1be4a8a49b17"
	workspace, err := CanonicalWorkspacePath(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	aliasWorkDir := filepath.Join(t.TempDir(), "workspace-alias")
	if err := os.Symlink(workspace, aliasWorkDir); err != nil {
		t.Fatalf("create workspace symlink: %v", err)
	}
	writeAgyFixture(t, appDir, conversationID, "", aliasWorkDir)

	meta, err := FindLatestForWorkspace(homeDir, workspace)
	if err != nil {
		t.Fatalf("FindLatestForWorkspace with symlinked provider log marker: %v", err)
	}
	if meta.ConversationID != conversationID || meta.WorkspacePath != workspace {
		t.Fatalf("symlink log metadata = ID %q workspace %q, want %q/%q", meta.ConversationID, meta.WorkspacePath, conversationID, workspace)
	}
}

type permanentWorkspaceFileLock struct {
	attempts int
}

func (fileLock *permanentWorkspaceFileLock) TryLock() error {
	fileLock.attempts++
	return syscall.EIO
}

func (*permanentWorkspaceFileLock) Unlock() error { return nil }

func TestAcquireWorkspaceCreateLockStopsOnPermanentFlockError(t *testing.T) {
	original := newWorkspaceFileLock
	t.Cleanup(func() { newWorkspaceFileLock = original })
	fileLock := &permanentWorkspaceFileLock{}
	newWorkspaceFileLock = func(string) (workspaceFileLock, error) { return fileLock, nil }

	_, err := AcquireWorkspaceCreateLock(t.Context(), t.TempDir())
	if !errors.Is(err, syscall.EIO) {
		t.Fatalf("AcquireWorkspaceCreateLock error = %v, want permanent EIO", err)
	}
	if fileLock.attempts != 1 {
		t.Fatalf("permanent flock attempts = %d, want exactly one", fileLock.attempts)
	}
}

func TestFindByID_CacheHitDoesNotReadInvalidLogDirectory(t *testing.T) {
	homeDir := t.TempDir()
	appDir := filepath.Join(homeDir, ".gemini", "antigravity-cli")
	conversationID := "117ff898-a964-4a9f-b460-1be4a8a49b17"
	workspace := "/tmp/agy-cache-fast-path"
	writeAgyFixture(t, appDir, conversationID, workspace, "")
	if err := os.WriteFile(filepath.Join(appDir, "log"), []byte("not a directory"), 0o644); err != nil {
		t.Fatalf("write invalid log directory fixture: %v", err)
	}

	meta, err := FindByID(homeDir, conversationID)
	if err != nil {
		t.Fatalf("cache hit unexpectedly read AGY logs: %v", err)
	}
	if meta.WorkspacePath != workspace || meta.LogPath != "" {
		t.Fatalf("cache result = workspace %q, log %q", meta.WorkspacePath, meta.LogPath)
	}
}

func TestFindByID_FallsBackToLogs(t *testing.T) {
	homeDir := t.TempDir()
	appDir := filepath.Join(homeDir, ".gemini", "antigravity-cli")
	conversationID := "117ff898-a964-4a9f-b460-1be4a8a49b17"
	workspace := "/tmp/agy-from-log"

	writeAgyFixture(t, appDir, conversationID, "", workspace)

	meta, err := FindByID(homeDir, conversationID)
	if err != nil {
		t.Fatalf("FindByID returned error: %v", err)
	}
	if meta.WorkspacePath != workspace {
		t.Fatalf("workspace path = %q, want %q", meta.WorkspacePath, workspace)
	}
	if meta.LogPath == "" || !strings.HasSuffix(meta.LogPath, ".log") {
		t.Fatalf("expected log path to be recorded, got %q", meta.LogPath)
	}
}

func TestFindLatestForWorkspace(t *testing.T) {
	homeDir := t.TempDir()
	appDir := filepath.Join(homeDir, ".gemini", "antigravity-cli")
	conversationID := "117ff898-a964-4a9f-b460-1be4a8a49b17"
	workspace := "/tmp/agy-probe"

	writeAgyFixture(t, appDir, conversationID, workspace, "")

	meta, err := FindLatestForWorkspace(homeDir, workspace)
	if err != nil {
		t.Fatalf("FindLatestForWorkspace returned error: %v", err)
	}
	if meta.ConversationID != conversationID {
		t.Fatalf("conversation ID = %q, want %q", meta.ConversationID, conversationID)
	}
	if meta.WorkspacePath != workspace {
		t.Fatalf("workspace path = %q, want %q", meta.WorkspacePath, workspace)
	}
}

func TestFindLatestForWorkspace_FallsBackToLogs(t *testing.T) {
	homeDir := t.TempDir()
	appDir := filepath.Join(homeDir, ".gemini", "antigravity-cli")
	conversationID := "117ff898-a964-4a9f-b460-1be4a8a49b17"
	workspace := "/tmp/agy-from-log"

	writeAgyFixture(t, appDir, conversationID, "", workspace)

	meta, err := FindLatestForWorkspace(homeDir, workspace)
	if err != nil {
		t.Fatalf("FindLatestForWorkspace returned error: %v", err)
	}
	if meta.ConversationID != conversationID {
		t.Fatalf("conversation ID = %q, want %q", meta.ConversationID, conversationID)
	}
	if meta.WorkspacePath != workspace {
		t.Fatalf("workspace path = %q, want %q", meta.WorkspacePath, workspace)
	}
}

func TestFindLatestForWorkspace_FallsBackFromDanglingCacheToLogs(t *testing.T) {
	homeDir := t.TempDir()
	appDir := filepath.Join(homeDir, ".gemini", "antigravity-cli")
	workspace := "/tmp/agy-dangling-cache"
	danglingID := "dangling-conversation"
	currentID := "current-conversation"

	writeAgyFixture(t, appDir, danglingID, workspace, "")
	if err := os.Remove(filepath.Join(appDir, "conversations", danglingID+".db")); err != nil {
		t.Fatalf("remove stale conversation DB: %v", err)
	}
	writeAgyFixture(t, appDir, currentID, "", workspace)
	cachePath := filepath.Join(appDir, "cache", "last_conversations.json")
	if err := os.WriteFile(cachePath, []byte(`{"`+workspace+`":"`+danglingID+`"}`), 0o644); err != nil {
		t.Fatalf("restore dangling cache entry: %v", err)
	}

	meta, err := FindLatestForWorkspace(homeDir, workspace)
	if err != nil {
		t.Fatalf("FindLatestForWorkspace: %v", err)
	}
	if meta.ConversationID != currentID {
		t.Fatalf("conversation ID = %q, want log-backed %q", meta.ConversationID, currentID)
	}
}

func TestFindLatestForWorkspace_SkipsDeletedConversationsWhileScanningLogs(t *testing.T) {
	homeDir := t.TempDir()
	appDir := filepath.Join(homeDir, ".gemini", "antigravity-cli")
	workspace := "/tmp/agy-deleted-newest-log"
	danglingID := "deleted-newest-conversation"
	currentID := "usable-older-conversation"

	writeAgyFixture(t, appDir, danglingID, workspace, "")
	if err := os.Remove(filepath.Join(appDir, "conversations", danglingID+".db")); err != nil {
		t.Fatalf("remove stale conversation DB: %v", err)
	}
	writeAgyFixture(t, appDir, currentID, "", "")
	cachePath := filepath.Join(appDir, "cache", "last_conversations.json")
	if err := os.WriteFile(cachePath, []byte(`{"`+workspace+`":"`+danglingID+`"}`), 0o644); err != nil {
		t.Fatalf("restore dangling cache entry: %v", err)
	}
	olderLog := writeAgyLog(t, appDir, "older-usable.log", []string{
		workspaceMarker + workspace,
		"Created conversation " + currentID,
	})
	newerLog := writeAgyLog(t, appDir, "newer-deleted.log", []string{
		workspaceMarker + workspace,
		"Created conversation " + danglingID,
	})
	base := time.Date(2026, time.July, 21, 12, 0, 0, 0, time.UTC)
	if err := os.Chtimes(olderLog, base, base); err != nil {
		t.Fatalf("set older log time: %v", err)
	}
	if err := os.Chtimes(newerLog, base.Add(time.Minute), base.Add(time.Minute)); err != nil {
		t.Fatalf("set newer log time: %v", err)
	}

	meta, err := FindLatestForWorkspace(homeDir, workspace)
	if err != nil {
		t.Fatalf("FindLatestForWorkspace: %v", err)
	}
	if meta.ConversationID != currentID {
		t.Fatalf("conversation ID = %q, want older usable %q", meta.ConversationID, currentID)
	}
}

func TestFindLatestForWorkspace_ClassifiesDanglingCacheWithoutLogsAsAbsent(t *testing.T) {
	homeDir := t.TempDir()
	appDir := filepath.Join(homeDir, ".gemini", "antigravity-cli")
	workspace := "/tmp/agy-dangling-cache-only"
	conversationID := "dangling-conversation"
	writeAgyFixture(t, appDir, conversationID, workspace, "")
	if err := os.Remove(filepath.Join(appDir, "conversations", conversationID+".db")); err != nil {
		t.Fatalf("remove stale conversation DB: %v", err)
	}

	_, err := FindLatestForWorkspace(homeDir, workspace)
	if !errors.Is(err, ErrConversationNotFound) {
		t.Fatalf("FindLatestForWorkspace error = %v, want ErrConversationNotFound", err)
	}
}

func TestFindLatestForWorkspace_StripsGetConversationDetailSuffix(t *testing.T) {
	homeDir := t.TempDir()
	appDir := filepath.Join(homeDir, ".gemini", "antigravity-cli")
	conversationID := "117ff898-a964-4a9f-b460-1be4a8a49b17"
	workspace := "/tmp/agy-detail-suffix"

	writeAgyFixtureWithLogLines(t, appDir, conversationID, "", []string{
		"Initializing CLI store manager for workspace " + workspace,
		"GetConversationDetail: found conversation " + conversationID + " (active=true)",
	})

	meta, err := FindLatestForWorkspace(homeDir, workspace)
	if err != nil {
		t.Fatalf("FindLatestForWorkspace returned error: %v", err)
	}
	if meta.ConversationID != conversationID {
		t.Fatalf("conversation ID = %q, want %q", meta.ConversationID, conversationID)
	}
}

func TestFindLatestForWorkspaceDistinguishesNoConversation(t *testing.T) {
	homeDir := t.TempDir()
	_, err := FindLatestForWorkspace(homeDir, "/tmp/agy-never-opened")
	if !errors.Is(err, ErrConversationNotFound) {
		t.Fatalf("FindLatestForWorkspace error = %v, want ErrConversationNotFound", err)
	}
}

func TestWorkspaceFromLogsPrefersNewestModificationTime(t *testing.T) {
	appDir := t.TempDir()
	conversationID := "117ff898-a964-4a9f-b460-1be4a8a49b17"
	oldPath := writeAgyLog(t, appDir, "zzzz-name-but-old.log", []string{
		workspaceMarker + "/tmp/old-workspace",
		"Created conversation " + conversationID,
	})
	newPath := writeAgyLog(t, appDir, "aaaa-name-but-new.log", []string{
		workspaceMarker + "/tmp/new-workspace",
		"Resuming conversation " + conversationID,
	})
	base := time.Date(2026, time.July, 20, 12, 0, 0, 0, time.UTC)
	if err := os.Chtimes(oldPath, base, base); err != nil {
		t.Fatalf("set old log time: %v", err)
	}
	if err := os.Chtimes(newPath, base.Add(time.Minute), base.Add(time.Minute)); err != nil {
		t.Fatalf("set new log time: %v", err)
	}

	workspace, logPath, err := workspaceFromLogs(appDir, conversationID)
	if err != nil {
		t.Fatalf("workspaceFromLogs: %v", err)
	}
	if workspace != "/tmp/new-workspace" || logPath != newPath {
		t.Fatalf("newest result = workspace %q, log %q", workspace, logPath)
	}
}

func TestWorkspaceFromLogsReportsCandidateBudgetExhaustion(t *testing.T) {
	appDir := t.TempDir()
	targetID := "117ff898-a964-4a9f-b460-1be4a8a49b17"
	base := time.Date(2026, time.July, 20, 12, 0, 0, 0, time.UTC)
	for i := range maxAgyLogFiles + 1 {
		conversationID := fmt.Sprintf("fixture-%03d", i)
		if i == 0 {
			conversationID = targetID
		}
		path := writeAgyLog(t, appDir, fmt.Sprintf("cli-%03d.log", i), []string{
			workspaceMarker + fmt.Sprintf("/tmp/workspace-%03d", i),
			"Created conversation " + conversationID,
		})
		modTime := base.Add(time.Duration(i) * time.Minute)
		if err := os.Chtimes(path, modTime, modTime); err != nil {
			t.Fatalf("set log %d time: %v", i, err)
		}
	}

	_, _, err := workspaceFromLogs(appDir, targetID)
	if !errors.Is(err, ErrLogDiscoveryBudgetExhausted) {
		t.Fatalf("error = %v, want ErrLogDiscoveryBudgetExhausted", err)
	}
	if !strings.Contains(err.Error(), "omitted 1 older logs") {
		t.Fatalf("budget error lacks omitted-file evidence: %v", err)
	}
}

func TestWorkspaceFromLogsReportsDirectoryEntryBudgetExhaustion(t *testing.T) {
	appDir := t.TempDir()
	logDir := filepath.Join(appDir, "log")
	if err := os.MkdirAll(logDir, 0o755); err != nil {
		t.Fatalf("mkdir log dir: %v", err)
	}
	for i := range maxAgyLogDirEntries + 1 {
		path := filepath.Join(logDir, fmt.Sprintf("entry-%03d.log", i))
		if err := os.WriteFile(path, nil, 0o644); err != nil {
			t.Fatalf("write log directory entry %d: %v", i, err)
		}
	}

	candidates, err := agyLogPaths(appDir)
	if err != nil {
		t.Fatalf("agyLogPaths should preserve its bounded candidates: %v", err)
	}
	if candidates.unprocessedEntries != 1 || len(candidates.paths) != maxAgyLogFiles {
		t.Fatalf("directory budget result = %+v", candidates)
	}

	_, _, err = workspaceFromLogCandidates(candidates, "missing-conversation")
	if !errors.Is(err, ErrLogDiscoveryBudgetExhausted) {
		t.Fatalf("error = %v, want ErrLogDiscoveryBudgetExhausted", err)
	}
	want := "left at least 1 directory entries unprocessed"
	if !strings.Contains(err.Error(), want) {
		t.Fatalf("budget error lacks directory-entry evidence: %v", err)
	}
}

func TestWorkspaceFromLogCandidatesReturnsKnownMatchWithDirectoryExhaustion(t *testing.T) {
	appDir := t.TempDir()
	conversationID := "117ff898-a964-4a9f-b460-1be4a8a49b17"
	logPath := writeAgyLog(t, appDir, "bounded-match.log", []string{
		workspaceMarker + "/tmp/bounded-match",
		"Created conversation " + conversationID,
	})
	candidates := agyLogCandidates{paths: []string{logPath}, unprocessedEntries: 1}

	workspace, matchedPath, err := workspaceFromLogCandidates(candidates, conversationID)
	if err != nil {
		t.Fatalf("conclusive known-ID match should survive directory exhaustion: %v", err)
	}
	if workspace != "/tmp/bounded-match" || matchedPath != logPath {
		t.Fatalf("known-ID result = workspace %q log %q", workspace, matchedPath)
	}
}

func TestLatestConversationForWorkspaceRejectsDirectoryEntryExhaustion(t *testing.T) {
	appDir := t.TempDir()
	workspace := "/tmp/inconclusive-latest"
	logPath := writeAgyLog(t, appDir, "bounded-match.log", []string{
		workspaceMarker + workspace,
		"Created conversation bounded-conversation",
	})
	candidates := agyLogCandidates{paths: []string{logPath}, unprocessedEntries: 1}

	conversationID, _, err := latestConversationForWorkspaceFromLogCandidates(candidates, workspace)
	if !errors.Is(err, ErrLogDiscoveryBudgetExhausted) {
		t.Fatalf("error = %v, want directory budget exhaustion", err)
	}
	if conversationID != "" {
		t.Fatalf("latest conversation = %q, want no inconclusive bounded match", conversationID)
	}
}

func TestCollectAgyLogCandidatesSkipsRemovedEntry(t *testing.T) {
	logDir := t.TempDir()
	removedPath := filepath.Join(logDir, "rotated.log")
	if err := os.WriteFile(removedPath, []byte("fixture\n"), 0o644); err != nil {
		t.Fatalf("write rotating log: %v", err)
	}
	keptPath := filepath.Join(logDir, "current.log")
	if err := os.WriteFile(keptPath, []byte("current\n"), 0o644); err != nil {
		t.Fatalf("write current log: %v", err)
	}
	entries, err := os.ReadDir(logDir)
	if err != nil {
		t.Fatalf("capture log directory snapshot: %v", err)
	}
	if err := os.Remove(removedPath); err != nil {
		t.Fatalf("remove rotating log: %v", err)
	}

	candidates, err := collectAgyLogCandidates(logDir, entries)
	if err != nil {
		t.Fatalf("removed snapshot entry should be skipped: %v", err)
	}
	if len(candidates) != 1 || candidates[0].path != keptPath {
		t.Fatalf("candidates = %+v, want only retained log %q", candidates, keptPath)
	}
}

func TestWorkspaceFromLogCandidatesSkipsRemovedFileBeforeScan(t *testing.T) {
	appDir := t.TempDir()
	conversationID := "117ff898-a964-4a9f-b460-1be4a8a49b17"
	validPath := writeAgyLog(t, appDir, "retained.log", []string{
		workspaceMarker + "/tmp/retained-workspace",
		"Created conversation " + conversationID,
	})
	candidates := agyLogCandidates{paths: []string{
		filepath.Join(appDir, "log", "rotated-away.log"),
		validPath,
	}}

	workspace, logPath, err := workspaceFromLogCandidates(candidates, conversationID)
	if err != nil {
		t.Fatalf("removed candidate should not hide retained known-ID match: %v", err)
	}
	if workspace != "/tmp/retained-workspace" || logPath != validPath {
		t.Fatalf("known-ID result = workspace %q log %q", workspace, logPath)
	}
}

func TestLatestConversationForWorkspaceFromLogCandidatesSkipsRemovedFileBeforeScan(t *testing.T) {
	appDir := t.TempDir()
	workspace := "/tmp/retained-workspace"
	validPath := writeAgyLog(t, appDir, "retained.log", []string{
		workspaceMarker + workspace,
		"Created conversation retained-conversation",
	})
	candidates := agyLogCandidates{paths: []string{
		filepath.Join(appDir, "log", "rotated-away.log"),
		validPath,
	}}

	conversationID, logPath, err := latestConversationForWorkspaceFromLogCandidates(candidates, workspace)
	if err != nil {
		t.Fatalf("removed candidate should not hide retained latest-workspace match: %v", err)
	}
	if conversationID != "retained-conversation" || logPath != validPath {
		t.Fatalf("latest-workspace result = conversation %q log %q", conversationID, logPath)
	}
}

func TestWorkspaceFromLogsReportsPerFileByteBudgetExhaustion(t *testing.T) {
	appDir := t.TempDir()
	conversationID := "117ff898-a964-4a9f-b460-1be4a8a49b17"
	logDir := filepath.Join(appDir, "log")
	if err := os.MkdirAll(logDir, 0o755); err != nil {
		t.Fatalf("mkdir log dir: %v", err)
	}
	padding := strings.Repeat("padding\n", maxAgyLogScanBytes/len("padding\n")+1)
	content := padding + workspaceMarker + "/tmp/beyond-budget\nCreated conversation " + conversationID + "\n"
	if err := os.WriteFile(filepath.Join(logDir, "oversized.log"), []byte(content), 0o644); err != nil {
		t.Fatalf("write oversized log: %v", err)
	}

	_, _, err := workspaceFromLogs(appDir, conversationID)
	if !errors.Is(err, ErrLogDiscoveryBudgetExhausted) {
		t.Fatalf("error = %v, want ErrLogDiscoveryBudgetExhausted", err)
	}
	if !strings.Contains(err.Error(), "truncated 1 oversized logs") {
		t.Fatalf("budget error lacks truncated-file evidence: %v", err)
	}
}

func TestWorkspaceFromLogsRejectsOversizedLine(t *testing.T) {
	appDir := t.TempDir()
	conversationID := "117ff898-a964-4a9f-b460-1be4a8a49b17"
	writeAgyLog(t, appDir, "oversized-line.log", []string{
		strings.Repeat("x", maxLogLineSize+1),
		workspaceMarker + "/tmp/after-oversized-line",
		"Created conversation " + conversationID,
	})

	_, _, err := workspaceFromLogs(appDir, conversationID)
	if err == nil || !strings.Contains(err.Error(), "token too long") {
		t.Fatalf("error = %v, want explicit oversized-line scan error", err)
	}
	if errors.Is(err, ErrLogDiscoveryBudgetExhausted) {
		t.Fatalf("oversized-line parse failure was misclassified as budget exhaustion: %v", err)
	}
}

func TestWorkspaceFromLogsReturnsMatchInsideTruncatedFile(t *testing.T) {
	appDir := t.TempDir()
	conversationID := "117ff898-a964-4a9f-b460-1be4a8a49b17"
	logDir := filepath.Join(appDir, "log")
	if err := os.MkdirAll(logDir, 0o755); err != nil {
		t.Fatalf("mkdir log dir: %v", err)
	}
	content := workspaceMarker + "/tmp/inside-budget\nCreated conversation " + conversationID + "\n" +
		strings.Repeat("padding\n", maxAgyLogScanBytes/len("padding\n")+1)
	if err := os.WriteFile(filepath.Join(logDir, "match-first.log"), []byte(content), 0o644); err != nil {
		t.Fatalf("write truncated match log: %v", err)
	}

	workspace, _, err := workspaceFromLogs(appDir, conversationID)
	if err != nil {
		t.Fatalf("match inside byte budget should succeed: %v", err)
	}
	if workspace != "/tmp/inside-budget" {
		t.Fatalf("workspace = %q", workspace)
	}
}

func TestLatestConversationForWorkspaceRejectsTruncatedPrefixMatch(t *testing.T) {
	appDir := t.TempDir()
	workspace := "/tmp/latest-workspace"
	logDir := filepath.Join(appDir, "log")
	if err := os.MkdirAll(logDir, 0o755); err != nil {
		t.Fatalf("mkdir log dir: %v", err)
	}
	content := workspaceMarker + workspace + "\nCreated conversation older-prefix\n" +
		strings.Repeat("padding\n", maxAgyLogScanBytes/len("padding\n")+1) +
		workspaceMarker + workspace + "\nCreated conversation newer-tail\n"
	if err := os.WriteFile(filepath.Join(logDir, "truncated-match.log"), []byte(content), 0o644); err != nil {
		t.Fatalf("write truncated match log: %v", err)
	}

	conversationID, _, err := latestConversationForWorkspaceFromLogs(appDir, workspace)
	if !errors.Is(err, ErrLogDiscoveryBudgetExhausted) {
		t.Fatalf("error = %v, want ErrLogDiscoveryBudgetExhausted", err)
	}
	if conversationID != "" {
		t.Fatalf("conversation ID = %q, want no inconclusive prefix match", conversationID)
	}
}

func TestLatestConversationForWorkspaceRejectsOlderMatchAfterTruncatedNewerLog(t *testing.T) {
	appDir := t.TempDir()
	workspace := "/tmp/latest-workspace"
	newerPath := writeAgyLog(t, appDir, "newer-truncated.log", []string{
		strings.Repeat("padding\n", maxAgyLogScanBytes/len("padding\n")+1),
	})
	olderPath := writeAgyLog(t, appDir, "older-match.log", []string{
		workspaceMarker + workspace,
		"Created conversation older-file",
	})
	base := time.Date(2026, time.July, 20, 12, 0, 0, 0, time.UTC)
	if err := os.Chtimes(olderPath, base, base); err != nil {
		t.Fatalf("set older log time: %v", err)
	}
	if err := os.Chtimes(newerPath, base.Add(time.Minute), base.Add(time.Minute)); err != nil {
		t.Fatalf("set newer log time: %v", err)
	}

	conversationID, _, err := latestConversationForWorkspaceFromLogs(appDir, workspace)
	if !errors.Is(err, ErrLogDiscoveryBudgetExhausted) {
		t.Fatalf("error = %v, want ErrLogDiscoveryBudgetExhausted", err)
	}
	if conversationID != "" {
		t.Fatalf("conversation ID = %q, want no older match after an incomplete newer log", conversationID)
	}
}

func TestLogHasUnreadTailDetectsGrowthAfterBoundedScan(t *testing.T) {
	logPath := filepath.Join(t.TempDir(), "growing.log")
	if err := os.WriteFile(logPath, []byte("initial line\n"), 0o644); err != nil {
		t.Fatalf("write initial log: %v", err)
	}
	reader, err := os.Open(logPath)
	if err != nil {
		t.Fatalf("open log: %v", err)
	}
	t.Cleanup(func() { _ = reader.Close() })
	scanner := bufio.NewScanner(io.LimitReader(reader, maxAgyLogScanBytes))
	for scanner.Scan() {
	}
	if err := scanner.Err(); err != nil {
		t.Fatalf("scan initial log: %v", err)
	}
	writer, err := os.OpenFile(logPath, os.O_APPEND|os.O_WRONLY, 0)
	if err != nil {
		t.Fatalf("open growing log: %v", err)
	}
	if _, err := writer.WriteString("appended marker\n"); err != nil {
		_ = writer.Close()
		t.Fatalf("append log: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close growing log: %v", err)
	}

	truncated, err := logHasUnreadTail(reader)
	if err != nil {
		t.Fatalf("logHasUnreadTail: %v", err)
	}
	if !truncated {
		t.Fatal("log growth after bounded scan was treated as complete")
	}
}

func writeAgyFixture(t *testing.T, appDir, conversationID, cachedWorkspace, loggedWorkspace string) {
	t.Helper()

	var logLines []string
	if loggedWorkspace != "" {
		logLines = []string{
			"Initializing CLI store manager for workspace " + loggedWorkspace,
			"Created conversation " + conversationID,
		}
	}
	writeAgyFixtureWithLogLines(t, appDir, conversationID, cachedWorkspace, logLines)
}

func writeAgyFixtureWithLogLines(t *testing.T, appDir, conversationID, cachedWorkspace string, logLines []string) {
	t.Helper()

	dbPath := filepath.Join(appDir, "conversations", conversationID+".db")
	if err := os.MkdirAll(filepath.Dir(dbPath), 0o755); err != nil {
		t.Fatalf("mkdir conversations: %v", err)
	}
	if err := os.WriteFile(dbPath, []byte("sqlite"), 0o644); err != nil {
		t.Fatalf("write conversation DB: %v", err)
	}

	transcriptDir := filepath.Join(appDir, "brain", conversationID, ".system_generated", "logs")
	if err := os.MkdirAll(transcriptDir, 0o755); err != nil {
		t.Fatalf("mkdir transcript dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(transcriptDir, "transcript.jsonl"), []byte("{}\n"), 0o644); err != nil {
		t.Fatalf("write transcript: %v", err)
	}
	if err := os.WriteFile(filepath.Join(transcriptDir, "transcript_full.jsonl"), []byte("{}\n"), 0o644); err != nil {
		t.Fatalf("write transcript_full: %v", err)
	}

	cacheDir := filepath.Join(appDir, "cache")
	if err := os.MkdirAll(cacheDir, 0o755); err != nil {
		t.Fatalf("mkdir cache dir: %v", err)
	}
	cacheBody := "{}"
	if cachedWorkspace != "" {
		cacheBody = `{"` + cachedWorkspace + `":"` + conversationID + `"}`
	}
	if err := os.WriteFile(filepath.Join(cacheDir, "last_conversations.json"), []byte(cacheBody), 0o644); err != nil {
		t.Fatalf("write last_conversations: %v", err)
	}

	if len(logLines) > 0 {
		logDir := filepath.Join(appDir, "log")
		if err := os.MkdirAll(logDir, 0o755); err != nil {
			t.Fatalf("mkdir log dir: %v", err)
		}
		logBody := strings.Join(logLines, "\n")
		if err := os.WriteFile(filepath.Join(logDir, "cli-20260624.log"), []byte(logBody), 0o644); err != nil {
			t.Fatalf("write log: %v", err)
		}
	}
}

func writeAgyLog(t *testing.T, appDir, name string, lines []string) string {
	t.Helper()
	logDir := filepath.Join(appDir, "log")
	if err := os.MkdirAll(logDir, 0o755); err != nil {
		t.Fatalf("mkdir log dir: %v", err)
	}
	path := filepath.Join(logDir, name)
	if err := os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0o644); err != nil {
		t.Fatalf("write log %s: %v", name, err)
	}
	return path
}
