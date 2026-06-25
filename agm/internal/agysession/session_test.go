package agysession

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
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
