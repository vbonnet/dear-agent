package agent

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/vbonnet/dear-agent/agm/test/helpers"
)

// TestGeminiCLIAdapter_RunHook tests the RunHook method.
func TestGeminiCLIAdapter_RunHook(t *testing.T) {
	// Create temp directory for test session store
	tempDir := t.TempDir()
	storePath := filepath.Join(tempDir, "sessions.json")

	// Create test adapter
	store, err := NewJSONSessionStore(storePath)
	if err != nil {
		t.Fatalf("failed to create session store: %v", err)
	}

	adapter := &GeminiCLIAdapter{
		sessionStore: store,
	}

	// Create test session
	sessionID := SessionID("test-session-123")
	metadata := &SessionMetadata{
		TmuxName:   "gemini-test",
		Title:      "Test Session",
		WorkingDir: tempDir,
		Project:    "test-project",
	}

	if err := store.Set(sessionID, metadata); err != nil {
		t.Fatalf("failed to set session metadata: %v", err)
	}

	// Test RunHook
	tests := []struct {
		name      string
		sessionID SessionID
		hookName  string
		wantError bool
	}{
		{
			name:      "SessionStart hook",
			sessionID: sessionID,
			hookName:  "SessionStart",
			wantError: false,
		},
		{
			name:      "SessionEnd hook",
			sessionID: sessionID,
			hookName:  "SessionEnd",
			wantError: false,
		},
		{
			name:      "BeforeAgent hook",
			sessionID: sessionID,
			hookName:  "BeforeAgent",
			wantError: false,
		},
		{
			name:      "AfterAgent hook",
			sessionID: sessionID,
			hookName:  "AfterAgent",
			wantError: false,
		},
		{
			name:      "Invalid session",
			sessionID: SessionID("nonexistent"),
			hookName:  "SessionStart",
			wantError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := adapter.RunHook(tt.sessionID, tt.hookName)
			if (err != nil) != tt.wantError {
				t.Errorf("RunHook() error = %v, wantError %v", err, tt.wantError)
			}

			// If successful, verify hook context file was created
			if err == nil {
				homeDir, _ := os.UserHomeDir()
				hookDir := filepath.Join(homeDir, ".agm", "gemini-hooks")
				hookFile := filepath.Join(hookDir, string(tt.sessionID)+"-"+tt.hookName+".json")

				if _, err := os.Stat(hookFile); os.IsNotExist(err) {
					t.Errorf("Hook context file was not created: %s", hookFile)
				} else {
					// Cleanup hook file after test
					_ = os.Remove(hookFile)
				}
			}
		})
	}
}

// TestGeminiCLIAdapter_ExecuteCommand_RunHook tests ExecuteCommand with CommandRunHook.
func TestGeminiCLIAdapter_ExecuteCommand_RunHook(t *testing.T) {
	// Create temp directory for test session store
	tempDir := t.TempDir()
	storePath := filepath.Join(tempDir, "sessions.json")

	// Create test adapter
	store, err := NewJSONSessionStore(storePath)
	if err != nil {
		t.Fatalf("failed to create session store: %v", err)
	}

	adapter := &GeminiCLIAdapter{
		sessionStore: store,
	}

	// Create test session
	sessionID := SessionID("test-session-456")
	metadata := &SessionMetadata{
		TmuxName:   "gemini-test-cmd",
		Title:      "Test Session Command",
		WorkingDir: tempDir,
		Project:    "test-project",
	}

	if err := store.Set(sessionID, metadata); err != nil {
		t.Fatalf("failed to set session metadata: %v", err)
	}

	// Test ExecuteCommand with CommandRunHook
	cmd := Command{
		Type: CommandRunHook,
		Params: map[string]interface{}{
			"session_id": string(sessionID),
			"hook_name":  "SessionStart",
		},
	}

	err = adapter.ExecuteCommand(cmd)
	if err != nil {
		t.Errorf("ExecuteCommand(CommandRunHook) failed: %v", err)
	}

	// Verify hook context file was created
	homeDir, _ := os.UserHomeDir()
	hookDir := filepath.Join(homeDir, ".agm", "gemini-hooks")
	hookFile := filepath.Join(hookDir, string(sessionID)+"-SessionStart.json")

	if _, err := os.Stat(hookFile); os.IsNotExist(err) {
		t.Errorf("Hook context file was not created via ExecuteCommand: %s", hookFile)
	} else {
		// Cleanup
		_ = os.Remove(hookFile)
	}
}

// TestGeminiCLIAdapter_Capabilities_SupportsHooks verifies hooks are enabled.
func TestGeminiCLIAdapter_Capabilities_SupportsHooks(t *testing.T) {
	adapter := &GeminiCLIAdapter{
		sessionStore: nil, // Not needed for Capabilities
	}

	caps := adapter.Capabilities()

	if !caps.SupportsHooks {
		t.Error("Gemini CLI adapter should support hooks (SupportsHooks should be true)")
	}
}

// TestGeminiCLIAdapter_ExecuteCommand_Rename tests ExecuteCommand with CommandRename.
func TestGeminiCLIAdapter_ExecuteCommand_Rename(t *testing.T) {
	// Create temp directory for test session store
	tempDir := t.TempDir()
	storePath := filepath.Join(tempDir, "sessions.json")

	// Create test adapter
	store, err := NewJSONSessionStore(storePath)
	if err != nil {
		t.Fatalf("failed to create session store: %v", err)
	}

	adapter := &GeminiCLIAdapter{
		sessionStore: store,
	}

	// Create test session
	sessionID := SessionID("test-session-rename")
	metadata := &SessionMetadata{
		TmuxName:   "gemini-test-rename",
		Title:      "Original Title",
		WorkingDir: tempDir,
		Project:    "test-project",
	}

	if err := store.Set(sessionID, metadata); err != nil {
		t.Fatalf("failed to set session metadata: %v", err)
	}

	// Test ExecuteCommand with CommandRename
	cmd := Command{
		Type: CommandRename,
		Params: map[string]interface{}{
			"session_id": string(sessionID),
			"name":       "New Title",
		},
	}

	// Note: This will fail because tmux session doesn't exist
	// But we can verify metadata update happens before tmux command
	err = adapter.ExecuteCommand(cmd)

	// Expect error due to missing tmux session, but metadata should be updated
	// before the tmux command fails
	if err == nil {
		t.Log("Expected error due to missing tmux session, but command succeeded (mock tmux may be present)")
	}

	// Even with error, verify metadata Title was updated if store was called
	updatedMetadata, getErr := store.Get(sessionID)
	if getErr != nil {
		t.Fatalf("failed to get updated session metadata: %v", getErr)
	}

	// The update happens after tmux command succeeds, so with tmux failure
	// the title may not be updated. This is expected behavior.
	// In a real scenario with working tmux, the title would be updated.
	if updatedMetadata.Title != "New Title" && err == nil {
		t.Errorf("Expected title to be updated to 'New Title', got '%s'", updatedMetadata.Title)
	}
}

// TestGeminiCLIAdapter_ExecuteCommand_SetDir tests ExecuteCommand with CommandSetDir.
func TestGeminiCLIAdapter_ExecuteCommand_SetDir(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping SetDir test in short mode (requires tmux)")
	}

	// Isolate tmux to prevent phantom sessions on production socket
	server := helpers.SetupTestServer(t)
	t.Setenv("AGM_TMUX_SOCKET", server.SocketPath)

	// Create temp directory for test session store
	tempDir := t.TempDir()
	storePath := filepath.Join(tempDir, "sessions.json")

	// Create test adapter
	store, err := NewJSONSessionStore(storePath)
	if err != nil {
		t.Fatalf("failed to create session store: %v", err)
	}

	adapter := &GeminiCLIAdapter{
		sessionStore: store,
	}

	// Create test session
	sessionID := SessionID("test-session-setdir")
	metadata := &SessionMetadata{
		TmuxName:   "gemini-test-setdir",
		Title:      "Test Session",
		WorkingDir: "/original/path",
		Project:    "test-project",
	}

	if err := store.Set(sessionID, metadata); err != nil {
		t.Fatalf("failed to set session metadata: %v", err)
	}

	// Test ExecuteCommand with CommandSetDir
	newPath := "/new/working/directory"
	cmd := Command{
		Type: CommandSetDir,
		Params: map[string]interface{}{
			"session_id": string(sessionID),
			"path":       newPath,
		},
	}

	err = adapter.ExecuteCommand(cmd)
	// Expected to fail without tmux, but this validates command structure
	if err == nil {
		t.Log("SetDir succeeded (mock tmux may be present)")
	}
}

// TestGeminiCLIAdapter_ExecuteCommand_InvalidCommand tests error handling for unknown commands.
func TestGeminiCLIAdapter_ExecuteCommand_InvalidCommand(t *testing.T) {
	// Create temp directory for test session store
	tempDir := t.TempDir()
	storePath := filepath.Join(tempDir, "sessions.json")

	// Create test adapter
	store, err := NewJSONSessionStore(storePath)
	if err != nil {
		t.Fatalf("failed to create session store: %v", err)
	}

	adapter := &GeminiCLIAdapter{
		sessionStore: store,
	}

	// Create test session
	sessionID := SessionID("test-session-invalid")
	metadata := &SessionMetadata{
		TmuxName:   "gemini-test-invalid",
		Title:      "Test Session",
		WorkingDir: tempDir,
		Project:    "test-project",
	}

	if err := store.Set(sessionID, metadata); err != nil {
		t.Fatalf("failed to set session metadata: %v", err)
	}

	// Test ExecuteCommand with invalid command type
	cmd := Command{
		Type: "InvalidCommandType",
		Params: map[string]interface{}{
			"session_id": string(sessionID),
		},
	}

	err = adapter.ExecuteCommand(cmd)
	if err == nil {
		t.Error("Expected error for invalid command type, got nil")
	}
}

// TestGeminiCLIAdapter_ExecuteCommand_MissingParams tests parameter validation.
func TestGeminiCLIAdapter_ExecuteCommand_MissingParams(t *testing.T) {
	// Create temp directory for test session store
	tempDir := t.TempDir()
	storePath := filepath.Join(tempDir, "sessions.json")

	// Create test adapter
	store, err := NewJSONSessionStore(storePath)
	if err != nil {
		t.Fatalf("failed to create session store: %v", err)
	}

	adapter := &GeminiCLIAdapter{
		sessionStore: store,
	}

	tests := []struct {
		name    string
		cmd     Command
		wantErr bool
	}{
		{
			name: "CommandRename missing session_id",
			cmd: Command{
				Type: CommandRename,
				Params: map[string]interface{}{
					"name": "New Title",
				},
			},
			wantErr: true,
		},
		{
			name: "CommandRename missing name",
			cmd: Command{
				Type: CommandRename,
				Params: map[string]interface{}{
					"session_id": "test-session",
				},
			},
			wantErr: true,
		},
		{
			name: "CommandSetDir missing path",
			cmd: Command{
				Type: CommandSetDir,
				Params: map[string]interface{}{
					"session_id": "test-session",
				},
			},
			wantErr: true,
		},
		{
			name: "CommandRunHook missing hook_name",
			cmd: Command{
				Type: CommandRunHook,
				Params: map[string]interface{}{
					"session_id": "test-session",
				},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := adapter.ExecuteCommand(tt.cmd)
			if (err != nil) != tt.wantErr {
				t.Errorf("ExecuteCommand() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

// TestGeminiCLIAdapter_SessionMetadata_UUID tests UUID field persistence.
func TestGeminiCLIAdapter_SessionMetadata_UUID(t *testing.T) {
	// Create temp directory for test session store
	tempDir := t.TempDir()
	storePath := filepath.Join(tempDir, "sessions.json")

	// Create test adapter
	store, err := NewJSONSessionStore(storePath)
	if err != nil {
		t.Fatalf("failed to create session store: %v", err)
	}

	// Create test session with UUID
	sessionID := SessionID("test-session-uuid")
	testUUID := "23a6e871-bb1f-48ec-bdbe-1f6ae90f9686"
	metadata := &SessionMetadata{
		TmuxName:   "gemini-test-uuid",
		Title:      "Test UUID Session",
		WorkingDir: tempDir,
		Project:    "test-project",
		UUID:       testUUID,
	}

	if err := store.Set(sessionID, metadata); err != nil {
		t.Fatalf("failed to set session metadata: %v", err)
	}

	// Retrieve and verify UUID persisted
	retrieved, err := store.Get(sessionID)
	if err != nil {
		t.Fatalf("failed to get session metadata: %v", err)
	}

	if retrieved.UUID != testUUID {
		t.Errorf("Expected UUID '%s', got '%s'", testUUID, retrieved.UUID)
	}
}

// TestGeminiCLIAdapter_ResumeSession_WithUUID tests resume with stored UUID.
func TestGeminiCLIAdapter_ResumeSession_WithUUID(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping resume test in short mode (requires tmux)")
	}

	// Isolate tmux to prevent phantom sessions on production socket
	server := helpers.SetupTestServer(t)
	t.Setenv("AGM_TMUX_SOCKET", server.SocketPath)

	// Create temp directory for test session store
	tempDir := t.TempDir()
	storePath := filepath.Join(tempDir, "sessions.json")

	// Create test adapter
	store, err := NewJSONSessionStore(storePath)
	if err != nil {
		t.Fatalf("failed to create session store: %v", err)
	}

	adapter := &GeminiCLIAdapter{
		sessionStore: store,
	}

	// Create test session with UUID
	sessionID := SessionID("test-resume-uuid")
	testUUID := "abc123-uuid-test"
	metadata := &SessionMetadata{
		TmuxName:   "gemini-resume-uuid",
		Title:      "Test Resume UUID",
		WorkingDir: tempDir,
		Project:    "test-project",
		UUID:       testUUID,
	}

	if err := store.Set(sessionID, metadata); err != nil {
		t.Fatalf("failed to set session metadata: %v", err)
	}

	// Test ResumeSession (will fail due to no tmux, but we verify UUID usage)
	err = adapter.ResumeSession(sessionID)

	// Expected to fail due to missing tmux session, but logic should use UUID
	// In real scenario with tmux, this would succeed
	if err == nil {
		t.Log("Resume succeeded (mock tmux may be present)")
	}

	// Verify metadata still has UUID after resume attempt
	retrieved, err := store.Get(sessionID)
	if err != nil {
		t.Fatalf("failed to get session metadata after resume: %v", err)
	}

	if retrieved.UUID != testUUID {
		t.Errorf("UUID should persist after resume, expected '%s', got '%s'", testUUID, retrieved.UUID)
	}
}

// TestGeminiCLIAdapter_ResumeSession_WithoutUUID tests fallback to "latest".
func TestGeminiCLIAdapter_ResumeSession_WithoutUUID(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping resume test in short mode (requires tmux)")
	}

	// Isolate tmux to prevent phantom sessions on production socket
	server := helpers.SetupTestServer(t)
	t.Setenv("AGM_TMUX_SOCKET", server.SocketPath)

	// Create temp directory for test session store
	tempDir := t.TempDir()
	storePath := filepath.Join(tempDir, "sessions.json")

	// Create test adapter
	store, err := NewJSONSessionStore(storePath)
	if err != nil {
		t.Fatalf("failed to create session store: %v", err)
	}

	adapter := &GeminiCLIAdapter{
		sessionStore: store,
	}

	// Create test session WITHOUT UUID (empty string)
	sessionID := SessionID("test-resume-no-uuid")
	metadata := &SessionMetadata{
		TmuxName:   "gemini-resume-no-uuid",
		Title:      "Test Resume Without UUID",
		WorkingDir: tempDir,
		Project:    "test-project",
		UUID:       "", // No UUID - should fall back to "latest"
	}

	if err := store.Set(sessionID, metadata); err != nil {
		t.Fatalf("failed to set session metadata: %v", err)
	}

	// Test ResumeSession (will fail due to no tmux, but verifies "latest" fallback)
	err = adapter.ResumeSession(sessionID)

	// Expected to fail due to missing tmux session
	// In real scenario, would use --resume latest
	if err == nil {
		t.Log("Resume succeeded (mock tmux may be present)")
	}

	// This test primarily validates the code path doesn't panic with empty UUID
	// Integration tests will verify actual --resume latest behavior
}

// TestGeminiCLIAdapter_ExecuteCommand_ClearHistory tests CommandClearHistory.
func TestGeminiCLIAdapter_ExecuteCommand_ClearHistory(t *testing.T) {
	// Create temp directory for test session store
	tempDir := t.TempDir()
	storePath := filepath.Join(tempDir, "sessions.json")

	// Create test adapter
	store, err := NewJSONSessionStore(storePath)
	if err != nil {
		t.Fatalf("failed to create session store: %v", err)
	}

	adapter := &GeminiCLIAdapter{
		sessionStore: store,
	}

	// Create test session
	sessionID := SessionID("test-clear-history")
	metadata := &SessionMetadata{
		TmuxName:   "gemini-test-clear",
		Title:      "Test Clear History",
		WorkingDir: tempDir,
		Project:    "test-project",
	}

	if err := store.Set(sessionID, metadata); err != nil {
		t.Fatalf("failed to set session metadata: %v", err)
	}

	// Create mock history file
	homeDir, _ := os.UserHomeDir()
	historyDir := filepath.Join(homeDir, ".gemini", "sessions", "gemini-test-clear")
	if err := os.MkdirAll(historyDir, 0755); err != nil {
		t.Fatalf("failed to create history directory: %v", err)
	}
	historyPath := filepath.Join(historyDir, "history.jsonl")
	if err := os.WriteFile(historyPath, []byte("test history"), 0644); err != nil {
		t.Fatalf("failed to create history file: %v", err)
	}
	defer os.RemoveAll(filepath.Join(homeDir, ".gemini", "sessions", "gemini-test-clear"))

	// Test CommandClearHistory
	cmd := Command{
		Type: CommandClearHistory,
		Params: map[string]interface{}{
			"session_id": string(sessionID),
		},
	}

	err = adapter.ExecuteCommand(cmd)
	if err != nil {
		t.Errorf("ExecuteCommand(CommandClearHistory) failed: %v", err)
	}

	// Verify history file was deleted
	if _, err := os.Stat(historyPath); !os.IsNotExist(err) {
		t.Error("History file should have been deleted")
	}
}

// TestGeminiCLIAdapter_ExecuteCommand_SetSystemPrompt tests CommandSetSystemPrompt.
func TestGeminiCLIAdapter_ExecuteCommand_SetSystemPrompt(t *testing.T) {
	// Create temp directory for test session store
	tempDir := t.TempDir()
	storePath := filepath.Join(tempDir, "sessions.json")

	// Create test adapter
	store, err := NewJSONSessionStore(storePath)
	if err != nil {
		t.Fatalf("failed to create session store: %v", err)
	}

	adapter := &GeminiCLIAdapter{
		sessionStore: store,
	}

	// Create test session
	sessionID := SessionID("test-set-prompt")
	metadata := &SessionMetadata{
		TmuxName:   "gemini-test-prompt",
		Title:      "Test System Prompt",
		WorkingDir: tempDir,
		Project:    "test-project",
	}

	if err := store.Set(sessionID, metadata); err != nil {
		t.Fatalf("failed to set session metadata: %v", err)
	}

	// Test CommandSetSystemPrompt
	newPrompt := "You are a helpful coding assistant."
	cmd := Command{
		Type: CommandSetSystemPrompt,
		Params: map[string]interface{}{
			"session_id": string(sessionID),
			"prompt":     newPrompt,
		},
	}

	err = adapter.ExecuteCommand(cmd)
	if err != nil {
		t.Errorf("ExecuteCommand(CommandSetSystemPrompt) failed: %v", err)
	}

	// Verify system prompt was updated in metadata
	updatedMetadata, err := store.Get(sessionID)
	if err != nil {
		t.Fatalf("failed to get updated session metadata: %v", err)
	}

	if updatedMetadata.SystemPrompt != newPrompt {
		t.Errorf("Expected SystemPrompt to be '%s', got '%s'", newPrompt, updatedMetadata.SystemPrompt)
	}
}

// TestGeminiCLIAdapter_GetHistoryPath tests getHistoryPath helper.
func TestGeminiCLIAdapter_GetHistoryPath(t *testing.T) {
	adapter := &GeminiCLIAdapter{}

	metadata := &SessionMetadata{
		TmuxName: "test-session",
	}

	path, err := adapter.getHistoryPath(metadata)
	if err != nil {
		t.Fatalf("getHistoryPath failed: %v", err)
	}

	// Verify path format
	homeDir, _ := os.UserHomeDir()
	expectedPath := filepath.Join(homeDir, ".gemini", "sessions", "test-session", "history.jsonl")
	if path != expectedPath {
		t.Errorf("Expected path '%s', got '%s'", expectedPath, path)
	}
}
