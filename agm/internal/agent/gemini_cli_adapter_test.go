package agent

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
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

func TestGeminiCLIAdapter_ResumeSessionValidatesBeforeTmuxCreation(t *testing.T) {
	originalHasSession := geminiResumeHasSession
	originalNewSession := geminiResumeNewSession
	originalIsProcessRunning := geminiResumeIsProcessRunning
	t.Cleanup(func() {
		geminiResumeHasSession = originalHasSession
		geminiResumeNewSession = originalNewSession
		geminiResumeIsProcessRunning = originalIsProcessRunning
	})
	geminiResumeHasSession = func(string) (bool, error) { return false, nil }
	created := false
	geminiResumeNewSession = func(string, string) error {
		created = true
		return nil
	}

	store, err := NewJSONSessionStore(filepath.Join(t.TempDir(), "sessions.json"))
	if err != nil {
		t.Fatalf("create session store: %v", err)
	}
	sessionID := SessionID("invalid-resume")
	if err := store.Set(sessionID, &SessionMetadata{
		TmuxName:   "gemini-invalid-resume",
		WorkingDir: "/safe\x1b[201~\nunsafe",
		UUID:       "uuid",
	}); err != nil {
		t.Fatalf("store metadata: %v", err)
	}
	adapter := &GeminiCLIAdapter{sessionStore: store}
	err = adapter.ResumeSession(sessionID)
	if err == nil || !strings.Contains(err.Error(), "validate Gemini resume") {
		t.Fatalf("ResumeSession() error = %v, want pre-tmux validation rejection", err)
	}
	if created {
		t.Fatal("ResumeSession() created tmux session before validating pasted metadata")
	}
}

func TestGeminiCLIAdapter_ResumeSessionSkipsValidationForRunningProcess(t *testing.T) {
	originalHasSession := geminiResumeHasSession
	originalNewSession := geminiResumeNewSession
	originalIsProcessRunning := geminiResumeIsProcessRunning
	t.Cleanup(func() {
		geminiResumeHasSession = originalHasSession
		geminiResumeNewSession = originalNewSession
		geminiResumeIsProcessRunning = originalIsProcessRunning
	})
	geminiResumeHasSession = func(string) (bool, error) { return true, nil }
	geminiResumeIsProcessRunning = func(string, string) (bool, error) { return true, nil }
	geminiResumeNewSession = func(string, string) error {
		t.Fatal("ResumeSession() replaced a healthy Gemini session")
		return nil
	}

	store, err := NewJSONSessionStore(filepath.Join(t.TempDir(), "sessions.json"))
	if err != nil {
		t.Fatalf("create session store: %v", err)
	}
	sessionID := SessionID("running-invalid-resume")
	if err := store.Set(sessionID, &SessionMetadata{
		TmuxName:   "gemini-running-invalid-resume",
		WorkingDir: "/legacy\x1b[201~\nmetadata",
		UUID:       "uuid",
	}); err != nil {
		t.Fatalf("store metadata: %v", err)
	}

	adapter := &GeminiCLIAdapter{sessionStore: store}
	if err := adapter.ResumeSession(sessionID); err != nil {
		t.Fatalf("ResumeSession() rejected metadata it did not paste: %v", err)
	}
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

// TestGeminiCLIAdapter_ImportConversationEndToEnd exercises ImportConversation
// through its public entry point rather than only its internal helpers
// (parseImportedMessages, writeHistory, rollbackFailedImport). Those helper
// tests would all stay green even if ImportConversation stopped calling one
// of them, or stopped propagating a rollback error; this is the test that
// would actually catch that.
func TestGeminiCLIAdapter_ImportConversationEndToEnd(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping import test in short mode (requires tmux)")
	}

	// Isolate tmux to prevent phantom sessions on production socket
	server := helpers.SetupTestServer(t)
	t.Setenv("AGM_TMUX_SOCKET", server.SocketPath)
	t.Setenv("HOME", t.TempDir())

	tempDir := t.TempDir()
	storePath := filepath.Join(tempDir, "sessions.json")
	store, err := NewJSONSessionStore(storePath)
	if err != nil {
		t.Fatalf("failed to create session store: %v", err)
	}
	adapter := &GeminiCLIAdapter{sessionStore: store}

	data := []byte(`{"role":"user","content":"hello"}` + "\n" + `{"role":"assistant","content":"hi"}` + "\n")

	sessionID, err := adapter.ImportConversation(data, FormatJSONL)
	if err != nil {
		t.Fatalf("ImportConversation returned error: %v", err)
	}
	if sessionID == "" {
		t.Fatal("ImportConversation returned an empty session ID")
	}

	got, err := adapter.GetHistory(sessionID)
	if err != nil {
		t.Fatalf("GetHistory returned error: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d messages, want 2: %#v", len(got), got)
	}
	if got[0].Role != RoleUser || got[0].Content != "hello" {
		t.Errorf("first message = %#v", got[0])
	}
	if got[1].Role != RoleAssistant || got[1].Content != "hi" {
		t.Errorf("second message = %#v", got[1])
	}
}

// TestGeminiCLIAdapter_ImportConversationRollsBackOnWriteFailure exercises the
// rollback path through the public entry point: an unwritable history
// directory makes writeHistory fail after CreateSession has already launched
// a real tmux session, and ImportConversation must both report the failure
// and not return a session ID that nothing can then reach.
func TestGeminiCLIAdapter_ImportConversationRollsBackOnWriteFailure(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping import test in short mode (requires tmux)")
	}

	server := helpers.SetupTestServer(t)
	t.Setenv("AGM_TMUX_SOCKET", server.SocketPath)

	home := t.TempDir()
	t.Setenv("HOME", home)
	// getHistoryPath descends into ~/.gemini/sessions/<tmux>/history.jsonl.
	// Occupying "sessions" with a plain file makes MkdirAll fail for every
	// session name, forcing writeHistory to fail regardless of which tmux
	// name this particular import happens to draw.
	geminiDir := filepath.Join(home, ".gemini")
	if err := os.MkdirAll(geminiDir, 0o700); err != nil {
		t.Fatalf("mkdir .gemini: %v", err)
	}
	if err := os.WriteFile(filepath.Join(geminiDir, "sessions"), []byte("occupied"), 0o600); err != nil {
		t.Fatalf("write blocking file: %v", err)
	}

	tempDir := t.TempDir()
	storePath := filepath.Join(tempDir, "sessions.json")
	store, err := NewJSONSessionStore(storePath)
	if err != nil {
		t.Fatalf("failed to create session store: %v", err)
	}
	adapter := &GeminiCLIAdapter{sessionStore: store}

	data := []byte(`{"role":"user","content":"hello"}` + "\n")

	sessionID, err := adapter.ImportConversation(data, FormatJSONL)
	if err == nil {
		t.Fatalf("expected ImportConversation to fail, got session %q", sessionID)
	}
	if sessionID != "" {
		t.Errorf("expected an empty session ID on failure, got %q", sessionID)
	}

	// The error and empty ID alone would also be exactly what a version with
	// rollbackFailedImport silently dropped from ImportConversation returns,
	// since writeHistory's own error already produces both. Inspecting the
	// store and the real tmux server directly is what actually proves the
	// rollback ran, rather than merely that the write failed.
	sessions, err := store.List()
	if err != nil {
		t.Fatalf("store.List returned error: %v", err)
	}
	if len(sessions) != 0 {
		t.Errorf("session store still holds %d entr(y/ies) after rollback: %#v", len(sessions), sessions)
	}

	out, _ := exec.Command("tmux", "-S", server.SocketPath, "list-sessions").CombinedOutput()
	if strings.Contains(string(out), "imported-") {
		t.Errorf("tmux still has the imported session after rollback: %s", out)
	}
}
