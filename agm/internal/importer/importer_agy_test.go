package importer

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/vbonnet/dear-agent/agm/internal/agysession"
	"github.com/vbonnet/dear-agent/agm/internal/dolt"
)

func TestBuildAgyImportedManifestLeavesUnknownModelUnset(t *testing.T) {
	now := time.Date(2026, time.July, 20, 12, 30, 0, 0, time.UTC)
	createdAt := now.Add(-time.Hour)
	meta := &agysession.Metadata{
		ConversationID:     "117ff898-a964-4a9f-b460-1be4a8a49b17",
		WorkspacePath:      "/tmp/agy workspace",
		ConversationDBPath: "/tmp/agy.db",
		TranscriptPath:     "/tmp/transcript.jsonl",
		ModTime:            createdAt,
	}
	m := buildAgyImportedManifest(meta, "AGY imported session", "oss", "agm-session-id", now)

	if m.SessionID != "agm-session-id" || m.Name != "AGY imported session" || m.Workspace != "oss" {
		t.Fatalf("manifest identity = %+v", m)
	}
	if m.Harness != "agy" || m.Model != "" {
		t.Fatalf("manifest harness/model = %q/%q", m.Harness, m.Model)
	}
	if m.WorkingDirectory != meta.WorkspacePath || m.Context.Project != meta.WorkspacePath {
		t.Fatalf("manifest workspace paths = %q/%q", m.WorkingDirectory, m.Context.Project)
	}
	if m.Agy == nil || m.Agy.ConversationID != meta.ConversationID || m.Agy.ConversationDB != meta.ConversationDBPath || m.Agy.TranscriptPath != meta.TranscriptPath {
		t.Fatalf("manifest AGY metadata = %+v", m.Agy)
	}
	if !m.CreatedAt.Equal(createdAt) || !m.UpdatedAt.Equal(now) {
		t.Fatalf("manifest timestamps = %s/%s", m.CreatedAt, m.UpdatedAt)
	}
	if m.Tmux.SessionName != "AGY-imported-session" {
		t.Fatalf("tmux session name = %q", m.Tmux.SessionName)
	}
}

func TestImportOrphanedSessionWithOptions_Agy(t *testing.T) {
	adapter := dolt.GetTestAdapter(t)
	if adapter == nil {
		t.Skip("Dolt not available for testing")
	}
	defer adapter.Close()

	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)

	conversationID := "117ff898-a964-4a9f-b460-1be4a8a49b17"
	workspacePath := filepath.Join(homeDir, "worktrees", "agy-probe")
	createAgyImportFixture(t, homeDir, conversationID, workspacePath)

	sessionID, err := ImportOrphanedSessionWithOptions(
		conversationID,
		"agy-import",
		"oss",
		adapter,
		"",
		ImportOptions{Harness: "agy"},
	)
	if err != nil {
		t.Fatalf("ImportOrphanedSessionWithOptions returned error: %v", err)
	}

	m, err := adapter.GetSession(sessionID)
	if err != nil {
		t.Fatalf("GetSession returned error: %v", err)
	}
	if m.Harness != "agy" {
		t.Fatalf("harness = %q, want agy", m.Harness)
	}
	if m.Model != "" {
		t.Fatalf("model = %q, want unknown model left unset", m.Model)
	}
	if m.WorkingDirectory != workspacePath {
		t.Fatalf("working directory = %q, want %q", m.WorkingDirectory, workspacePath)
	}
	if m.Context.Project != workspacePath {
		t.Fatalf("context.project = %q, want %q", m.Context.Project, workspacePath)
	}
	if m.Agy == nil {
		t.Fatal("expected AGY metadata to be persisted")
	}
	if m.Agy.ConversationID != conversationID {
		t.Fatalf("AGY conversation ID = %q, want %q", m.Agy.ConversationID, conversationID)
	}
	if m.Agy.WorkspacePath != workspacePath {
		t.Fatalf("AGY workspace path = %q, want %q", m.Agy.WorkspacePath, workspacePath)
	}
}

func createAgyImportFixture(t *testing.T, homeDir, conversationID, workspacePath string) {
	t.Helper()

	appDir := filepath.Join(homeDir, ".gemini", "antigravity-cli")
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

	cacheDir := filepath.Join(appDir, "cache")
	if err := os.MkdirAll(cacheDir, 0o755); err != nil {
		t.Fatalf("mkdir cache dir: %v", err)
	}
	cacheBody := `{"` + workspacePath + `":"` + conversationID + `"}`
	if err := os.WriteFile(filepath.Join(cacheDir, "last_conversations.json"), []byte(cacheBody), 0o644); err != nil {
		t.Fatalf("write last_conversations: %v", err)
	}
}
