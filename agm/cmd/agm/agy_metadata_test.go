package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/vbonnet/dear-agent/agm/internal/manifest"
)

func TestEnrichManifestWithAgyConversation_PrefersContextProject(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)

	const conversationID = "117ff898-a964-4a9f-b460-1be4a8a49b17"
	projectPath := "/tmp/agy-project"
	writeAgyFixtureForCmdTests(t, homeDir, conversationID, projectPath, "")

	m := &manifest.Manifest{
		Context:          manifest.Context{Project: projectPath},
		WorkingDirectory: "/tmp/operator-cwd",
	}

	if err := enrichManifestWithAgyConversation(m); err != nil {
		t.Fatalf("enrichManifestWithAgyConversation returned error: %v", err)
	}
	if m.Agy == nil {
		t.Fatal("expected AGY metadata to be attached")
	}
	if m.Agy.ConversationID != conversationID {
		t.Fatalf("conversation ID = %q, want %q", m.Agy.ConversationID, conversationID)
	}
	if m.WorkingDirectory != projectPath {
		t.Fatalf("working directory = %q, want %q", m.WorkingDirectory, projectPath)
	}
	if m.Model != "" {
		t.Fatalf("discovered AGY model = %q, want unknown model left unset", m.Model)
	}
}

func TestAgyWorkspaceCandidates_DeduplicatesAndOrdersPaths(t *testing.T) {
	projectPath := "/tmp/project"
	m := &manifest.Manifest{
		Context:          manifest.Context{Project: projectPath},
		WorkingDirectory: projectPath,
		Agy:              &manifest.Agy{WorkspacePath: "/tmp/secondary"},
	}

	got := agyWorkspaceCandidates(m)
	want := []string{projectPath, "/tmp/secondary"}
	if len(got) != len(want) {
		t.Fatalf("candidate count = %d, want %d (%v)", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("candidate[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func writeAgyFixtureForCmdTests(t *testing.T, homeDir, conversationID, cachedWorkspace, loggedWorkspace string) {
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
	cacheBody := "{}"
	if cachedWorkspace != "" {
		cacheBody = `{"` + cachedWorkspace + `":"` + conversationID + `"}`
	}
	if err := os.WriteFile(filepath.Join(cacheDir, "last_conversations.json"), []byte(cacheBody), 0o644); err != nil {
		t.Fatalf("write last_conversations: %v", err)
	}

	if loggedWorkspace != "" {
		logDir := filepath.Join(appDir, "log")
		if err := os.MkdirAll(logDir, 0o755); err != nil {
			t.Fatalf("mkdir log dir: %v", err)
		}
		logBody := strings.Join([]string{
			"Initializing CLI store manager for workspace " + loggedWorkspace,
			"Created conversation " + conversationID,
		}, "\n")
		if err := os.WriteFile(filepath.Join(logDir, "cli-20260624.log"), []byte(logBody), 0o644); err != nil {
			t.Fatalf("write log: %v", err)
		}
	}
}
