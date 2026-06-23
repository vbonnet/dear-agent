package agent

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/vbonnet/dear-agent/agm/test/helpers"
)

func TestAgyAdapter_Capabilities(t *testing.T) {
	adapter := &AgyAdapter{sessionStore: nil}
	caps := adapter.Capabilities()

	if !caps.SupportsHooks {
		t.Error("agy adapter should support hooks")
	}
	if !caps.SupportsTools {
		t.Error("agy adapter should support tools")
	}
	if caps.MaxContextWindow != 1048576 {
		t.Errorf("MaxContextWindow = %d, want 1048576", caps.MaxContextWindow)
	}
	if caps.ModelName != "gemini-3.5-flash" {
		t.Errorf("ModelName = %q, want %q", caps.ModelName, "gemini-3.5-flash")
	}
}

func TestAgyAdapter_Name(t *testing.T) {
	adapter := &AgyAdapter{sessionStore: nil}
	if got := adapter.Name(); got != "agy" {
		t.Errorf("Name() = %q, want %q", got, "agy")
	}
}

func TestAgyAdapter_SessionMetadata_UUID(t *testing.T) {
	tempDir := t.TempDir()
	store, err := NewJSONSessionStore(filepath.Join(tempDir, "sessions.json"))
	if err != nil {
		t.Fatalf("failed to create session store: %v", err)
	}

	sessionID := SessionID("test-agy-uuid")
	testUUID := "agy-conv-abc123"
	metadata := &SessionMetadata{
		TmuxName:   "agy-test-uuid",
		Title:      "Test UUID Session",
		WorkingDir: tempDir,
		Project:    "test-project",
		UUID:       testUUID,
	}

	if err := store.Set(sessionID, metadata); err != nil {
		t.Fatalf("failed to set session metadata: %v", err)
	}

	retrieved, err := store.Get(sessionID)
	if err != nil {
		t.Fatalf("failed to get session metadata: %v", err)
	}
	if retrieved.UUID != testUUID {
		t.Errorf("UUID = %q, want %q", retrieved.UUID, testUUID)
	}
}

func TestAgyAdapter_ExecuteCommand_Rename(t *testing.T) {
	tempDir := t.TempDir()
	store, err := NewJSONSessionStore(filepath.Join(tempDir, "sessions.json"))
	if err != nil {
		t.Fatalf("failed to create session store: %v", err)
	}
	adapter := &AgyAdapter{sessionStore: store}

	sessionID := SessionID("test-agy-rename")
	if err := store.Set(sessionID, &SessionMetadata{
		TmuxName:   "agy-test-rename",
		Title:      "Original Title",
		WorkingDir: tempDir,
	}); err != nil {
		t.Fatalf("failed to set session metadata: %v", err)
	}

	cmd := Command{
		Type: CommandRename,
		Params: map[string]any{
			"session_id": string(sessionID),
			"name":       "New Title",
		},
	}
	if err := adapter.ExecuteCommand(cmd); err != nil {
		t.Fatalf("ExecuteCommand(CommandRename) failed: %v", err)
	}

	updated, err := store.Get(sessionID)
	if err != nil {
		t.Fatalf("failed to get updated metadata: %v", err)
	}
	if updated.Title != "New Title" {
		t.Errorf("Title = %q, want %q", updated.Title, "New Title")
	}
}

func TestAgyAdapter_ExecuteCommand_SetSystemPrompt(t *testing.T) {
	tempDir := t.TempDir()
	store, err := NewJSONSessionStore(filepath.Join(tempDir, "sessions.json"))
	if err != nil {
		t.Fatalf("failed to create session store: %v", err)
	}
	adapter := &AgyAdapter{sessionStore: store}

	sessionID := SessionID("test-agy-prompt")
	if err := store.Set(sessionID, &SessionMetadata{
		TmuxName:   "agy-test-prompt",
		Title:      "Test",
		WorkingDir: tempDir,
	}); err != nil {
		t.Fatalf("failed to set session metadata: %v", err)
	}

	newPrompt := "You are a Go coding assistant."
	cmd := Command{
		Type: CommandSetSystemPrompt,
		Params: map[string]any{
			"session_id": string(sessionID),
			"prompt":     newPrompt,
		},
	}
	if err := adapter.ExecuteCommand(cmd); err != nil {
		t.Fatalf("ExecuteCommand(CommandSetSystemPrompt) failed: %v", err)
	}

	updated, err := store.Get(sessionID)
	if err != nil {
		t.Fatalf("failed to get updated metadata: %v", err)
	}
	if updated.SystemPrompt != newPrompt {
		t.Errorf("SystemPrompt = %q, want %q", updated.SystemPrompt, newPrompt)
	}
}

func TestAgyAdapter_ExecuteCommand_ClearHistory(t *testing.T) {
	tempDir := t.TempDir()
	store, err := NewJSONSessionStore(filepath.Join(tempDir, "sessions.json"))
	if err != nil {
		t.Fatalf("failed to create session store: %v", err)
	}
	adapter := &AgyAdapter{sessionStore: store}

	sessionID := SessionID("test-agy-clear")
	if err := store.Set(sessionID, &SessionMetadata{
		TmuxName:   "agy-test-clear",
		Title:      "Test Clear",
		WorkingDir: tempDir,
	}); err != nil {
		t.Fatalf("failed to set session metadata: %v", err)
	}

	// Create a mock history file
	homeDir, _ := os.UserHomeDir()
	historyDir := filepath.Join(homeDir, ".agy", "sessions", "agy-test-clear")
	if err := os.MkdirAll(historyDir, 0o755); err != nil {
		t.Fatalf("failed to create history dir: %v", err)
	}
	historyPath := filepath.Join(historyDir, "history.jsonl")
	if err := os.WriteFile(historyPath, []byte("test history\n"), 0o644); err != nil {
		t.Fatalf("failed to write history file: %v", err)
	}
	defer os.RemoveAll(filepath.Join(homeDir, ".agy", "sessions", "agy-test-clear"))

	cmd := Command{
		Type: CommandClearHistory,
		Params: map[string]any{
			"session_id": string(sessionID),
		},
	}
	if err := adapter.ExecuteCommand(cmd); err != nil {
		t.Fatalf("ExecuteCommand(CommandClearHistory) failed: %v", err)
	}
	if _, err := os.Stat(historyPath); !os.IsNotExist(err) {
		t.Error("history file should have been removed")
	}
}

func TestAgyAdapter_ExecuteCommand_InvalidCommand(t *testing.T) {
	tempDir := t.TempDir()
	store, err := NewJSONSessionStore(filepath.Join(tempDir, "sessions.json"))
	if err != nil {
		t.Fatalf("failed to create session store: %v", err)
	}
	adapter := &AgyAdapter{sessionStore: store}

	sessionID := SessionID("test-agy-invalid")
	if err := store.Set(sessionID, &SessionMetadata{
		TmuxName:   "agy-test-invalid",
		Title:      "Test",
		WorkingDir: tempDir,
	}); err != nil {
		t.Fatalf("failed to set session metadata: %v", err)
	}

	cmd := Command{
		Type: "UnknownCommand",
		Params: map[string]any{
			"session_id": string(sessionID),
		},
	}
	if err := adapter.ExecuteCommand(cmd); err == nil {
		t.Error("expected error for unknown command type, got nil")
	}
}

func TestAgyAdapter_ExecuteCommand_MissingParams(t *testing.T) {
	tempDir := t.TempDir()
	store, err := NewJSONSessionStore(filepath.Join(tempDir, "sessions.json"))
	if err != nil {
		t.Fatalf("failed to create session store: %v", err)
	}
	adapter := &AgyAdapter{sessionStore: store}

	tests := []struct {
		name string
		cmd  Command
	}{
		{
			name: "Rename missing session_id",
			cmd: Command{
				Type:   CommandRename,
				Params: map[string]any{"name": "x"},
			},
		},
		{
			name: "Rename missing name",
			cmd: Command{
				Type:   CommandRename,
				Params: map[string]any{"session_id": "s"},
			},
		},
		{
			name: "SetDir missing path",
			cmd: Command{
				Type:   CommandSetDir,
				Params: map[string]any{"session_id": "s"},
			},
		},
		{
			name: "RunHook missing hook_name",
			cmd: Command{
				Type:   CommandRunHook,
				Params: map[string]any{"session_id": "s"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := adapter.ExecuteCommand(tt.cmd); err == nil {
				t.Errorf("expected error for %s, got nil", tt.name)
			}
		})
	}
}

func TestAgyAdapter_ExecuteCommand_SetDir(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping SetDir test in short mode (requires tmux)")
	}

	server := helpers.SetupTestServer(t)
	t.Setenv("AGM_TMUX_SOCKET", server.SocketPath)

	tempDir := t.TempDir()
	store, err := NewJSONSessionStore(filepath.Join(tempDir, "sessions.json"))
	if err != nil {
		t.Fatalf("failed to create session store: %v", err)
	}
	adapter := &AgyAdapter{sessionStore: store}

	sessionID := SessionID("test-agy-setdir")
	if err := store.Set(sessionID, &SessionMetadata{
		TmuxName:   "agy-test-setdir",
		Title:      "Test",
		WorkingDir: "/original",
	}); err != nil {
		t.Fatalf("failed to set session metadata: %v", err)
	}

	cmd := Command{
		Type: CommandSetDir,
		Params: map[string]any{
			"session_id": string(sessionID),
			"path":       "/new/path",
		},
	}
	// May fail without a real tmux pane; validates command dispatch structure.
	_ = adapter.ExecuteCommand(cmd)
}

func TestAgyAdapter_ResumeSession_WithUUID(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping resume test in short mode (requires tmux)")
	}

	server := helpers.SetupTestServer(t)
	t.Setenv("AGM_TMUX_SOCKET", server.SocketPath)

	tempDir := t.TempDir()
	store, err := NewJSONSessionStore(filepath.Join(tempDir, "sessions.json"))
	if err != nil {
		t.Fatalf("failed to create session store: %v", err)
	}
	adapter := &AgyAdapter{sessionStore: store}

	sessionID := SessionID("test-agy-resume-uuid")
	testUUID := "agy-conv-xyz789"
	if err := store.Set(sessionID, &SessionMetadata{
		TmuxName:   "agy-resume-uuid",
		Title:      "Resume UUID Test",
		WorkingDir: tempDir,
		UUID:       testUUID,
	}); err != nil {
		t.Fatalf("failed to set session metadata: %v", err)
	}

	_ = adapter.ResumeSession(sessionID) // may fail without real tmux

	retrieved, err := store.Get(sessionID)
	if err != nil {
		t.Fatalf("failed to get session metadata after resume: %v", err)
	}
	if retrieved.UUID != testUUID {
		t.Errorf("UUID should persist after resume, got %q", retrieved.UUID)
	}
}

func TestAgyAdapter_ResumeSession_WithoutUUID(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping resume test in short mode (requires tmux)")
	}

	server := helpers.SetupTestServer(t)
	t.Setenv("AGM_TMUX_SOCKET", server.SocketPath)

	tempDir := t.TempDir()
	store, err := NewJSONSessionStore(filepath.Join(tempDir, "sessions.json"))
	if err != nil {
		t.Fatalf("failed to create session store: %v", err)
	}
	adapter := &AgyAdapter{sessionStore: store}

	sessionID := SessionID("test-agy-resume-no-uuid")
	if err := store.Set(sessionID, &SessionMetadata{
		TmuxName:   "agy-resume-no-uuid",
		Title:      "Resume No UUID",
		WorkingDir: tempDir,
		UUID:       "",
	}); err != nil {
		t.Fatalf("failed to set session metadata: %v", err)
	}

	// Should fall back to --continue; must not panic.
	_ = adapter.ResumeSession(sessionID)
}
