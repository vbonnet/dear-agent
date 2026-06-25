package importer

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/vbonnet/dear-agent/agm/internal/dolt"
)

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
	if m.Model != "3.5-flash" {
		t.Fatalf("model = %q, want 3.5-flash", m.Model)
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
