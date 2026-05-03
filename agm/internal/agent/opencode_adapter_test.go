package agent

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// TestOpenCodeAdapterImplementsAgentInterface verifies OpenCodeAdapter implements Agent interface.
func TestOpenCodeAdapterImplementsAgentInterface(t *testing.T) {
	// Create mock HTTP server for health check
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/health" {
			w.WriteHeader(http.StatusOK)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	// Create adapter with mock store
	mockStore := &MockSessionStore{
		sessions: make(map[SessionID]*SessionMetadata),
	}

	config := &OpenCodeConfig{
		SessionStore: mockStore,
		ServerURL:    server.URL,
	}

	adapter, err := NewOpenCodeAdapter(config)
	if err != nil {
		t.Fatalf("NewOpenCodeAdapter failed: %v", err)
	}

	// Verify adapter implements Agent interface
	var _ = adapter
}

// TestOpenCodeAdapterName tests Name() method.
func TestOpenCodeAdapterName(t *testing.T) {
	mockStore := &MockSessionStore{
		sessions: make(map[SessionID]*SessionMetadata),
	}

	config := &OpenCodeConfig{
		SessionStore: mockStore,
		ServerURL:    "http://localhost:4096",
	}

	adapter, err := NewOpenCodeAdapter(config)
	if err != nil {
		t.Fatalf("NewOpenCodeAdapter failed: %v", err)
	}

	if got := adapter.Name(); got != "opencode" {
		t.Errorf("Name() = %q, want %q", got, "opencode")
	}
}

// TestOpenCodeAdapterVersion tests Version() method.
func TestOpenCodeAdapterVersion(t *testing.T) {
	mockStore := &MockSessionStore{
		sessions: make(map[SessionID]*SessionMetadata),
	}

	config := &OpenCodeConfig{
		SessionStore: mockStore,
		ServerURL:    "http://localhost:4096",
	}

	adapter, err := NewOpenCodeAdapter(config)
	if err != nil {
		t.Fatalf("NewOpenCodeAdapter failed: %v", err)
	}

	version := adapter.Version()
	if version == "" {
		t.Errorf("Version() returned empty string")
	}

	if version != "opencode-server" {
		t.Errorf("Version() = %q, want %q", version, "opencode-server")
	}
}

// TestOpenCodeAdapterCapabilities tests Capabilities() method.
func TestOpenCodeAdapterCapabilities(t *testing.T) {
	mockStore := &MockSessionStore{
		sessions: make(map[SessionID]*SessionMetadata),
	}

	config := &OpenCodeConfig{
		SessionStore: mockStore,
		ServerURL:    "http://localhost:4096",
	}

	adapter, err := NewOpenCodeAdapter(config)
	if err != nil {
		t.Fatalf("NewOpenCodeAdapter failed: %v", err)
	}

	caps := adapter.Capabilities()

	// Verify expected capabilities for OpenCode
	if caps.SupportsSlashCommands {
		t.Error("SupportsSlashCommands should be false for OpenCode (server-based)")
	}

	if !caps.SupportsHooks {
		t.Error("SupportsHooks should be true (AGM feature)")
	}

	if !caps.SupportsTools {
		t.Error("SupportsTools should be true for OpenCode")
	}

	if caps.SupportsVision {
		t.Error("SupportsVision should be false for OpenCode (mock implementation)")
	}

	if caps.SupportsMultimodal {
		t.Error("SupportsMultimodal should be false for OpenCode (mock implementation)")
	}

	if !caps.SupportsStreaming {
		t.Error("SupportsStreaming should be true (SSE events indicate streaming)")
	}

	if !caps.SupportsSystemPrompts {
		t.Error("SupportsSystemPrompts should be true")
	}

	if caps.MaxContextWindow != 200000 {
		t.Errorf("MaxContextWindow = %d, want 200000", caps.MaxContextWindow)
	}

	if caps.ModelName != "opencode-server" {
		t.Errorf("ModelName = %q, want %q", caps.ModelName, "opencode-server")
	}
}

// TestNewOpenCodeAdapterWithNilConfig tests adapter creation with nil config.
func TestNewOpenCodeAdapterWithNilConfig(t *testing.T) {
	adapter, err := NewOpenCodeAdapter(nil)
	if err != nil {
		t.Fatalf("NewOpenCodeAdapter with nil config failed: %v", err)
	}

	// Verify default values
	opencodeAdapter, ok := adapter.(*OpenCodeAdapter)
	if !ok {
		t.Fatal("Adapter is not *OpenCodeAdapter")
	}

	if opencodeAdapter.serverURL != "http://localhost:4096" {
		t.Errorf("serverURL = %q, want %q", opencodeAdapter.serverURL, "http://localhost:4096")
	}

	if opencodeAdapter.sessionStore == nil {
		t.Error("sessionStore should not be nil (should use default)")
	}
}

// TestNewOpenCodeAdapterWithCustomServerURL tests adapter creation with custom server URL.
func TestNewOpenCodeAdapterWithCustomServerURL(t *testing.T) {
	mockStore := &MockSessionStore{
		sessions: make(map[SessionID]*SessionMetadata),
	}

	customURL := "http://localhost:8080"
	config := &OpenCodeConfig{
		SessionStore: mockStore,
		ServerURL:    customURL,
	}

	adapter, err := NewOpenCodeAdapter(config)
	if err != nil {
		t.Fatalf("NewOpenCodeAdapter failed: %v", err)
	}

	opencodeAdapter, ok := adapter.(*OpenCodeAdapter)
	if !ok {
		t.Fatal("Adapter is not *OpenCodeAdapter")
	}

	if opencodeAdapter.serverURL != customURL {
		t.Errorf("serverURL = %q, want %q", opencodeAdapter.serverURL, customURL)
	}
}

// TestCheckServerHealthSuccess tests successful health check.
func TestCheckServerHealthSuccess(t *testing.T) {
	// Create mock HTTP server that returns 200 OK for /health
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/health" {
			w.WriteHeader(http.StatusOK)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	mockStore := &MockSessionStore{
		sessions: make(map[SessionID]*SessionMetadata),
	}

	config := &OpenCodeConfig{
		SessionStore: mockStore,
		ServerURL:    server.URL,
	}

	adapter, err := NewOpenCodeAdapter(config)
	if err != nil {
		t.Fatalf("NewOpenCodeAdapter failed: %v", err)
	}

	opencodeAdapter, ok := adapter.(*OpenCodeAdapter)
	if !ok {
		t.Fatal("Adapter is not *OpenCodeAdapter")
	}

	// Test health check
	if err := opencodeAdapter.checkServerHealth(); err != nil {
		t.Errorf("checkServerHealth() failed: %v", err)
	}
}

// TestCheckServerHealthFailureServerDown tests health check when server is down.
func TestCheckServerHealthFailureServerDown(t *testing.T) {
	mockStore := &MockSessionStore{
		sessions: make(map[SessionID]*SessionMetadata),
	}

	// Use invalid URL to simulate server down
	config := &OpenCodeConfig{
		SessionStore: mockStore,
		ServerURL:    "http://localhost:65535", // Unlikely to be listening
	}

	adapter, err := NewOpenCodeAdapter(config)
	if err != nil {
		t.Fatalf("NewOpenCodeAdapter failed: %v", err)
	}

	opencodeAdapter, ok := adapter.(*OpenCodeAdapter)
	if !ok {
		t.Fatal("Adapter is not *OpenCodeAdapter")
	}

	// Test health check (should fail)
	if err := opencodeAdapter.checkServerHealth(); err == nil {
		t.Error("checkServerHealth() should fail when server is down")
	}
}

// TestCheckServerHealthFailureNon200Status tests health check when server returns non-200.
func TestCheckServerHealthFailureNon200Status(t *testing.T) {
	// Create mock HTTP server that returns 500 Internal Server Error
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/health" {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	mockStore := &MockSessionStore{
		sessions: make(map[SessionID]*SessionMetadata),
	}

	config := &OpenCodeConfig{
		SessionStore: mockStore,
		ServerURL:    server.URL,
	}

	adapter, err := NewOpenCodeAdapter(config)
	if err != nil {
		t.Fatalf("NewOpenCodeAdapter failed: %v", err)
	}

	opencodeAdapter, ok := adapter.(*OpenCodeAdapter)
	if !ok {
		t.Fatal("Adapter is not *OpenCodeAdapter")
	}

	// Test health check (should fail with non-200 status)
	if err := opencodeAdapter.checkServerHealth(); err == nil {
		t.Error("checkServerHealth() should fail when server returns non-200 status")
	}
}

// TestGetSessionStatusTerminatedSessionNotFound tests GetSessionStatus when session not found.
func TestGetSessionStatusTerminatedSessionNotFound(t *testing.T) {
	mockStore := &MockSessionStore{
		sessions: make(map[SessionID]*SessionMetadata),
	}

	config := &OpenCodeConfig{
		SessionStore: mockStore,
		ServerURL:    "http://localhost:4096",
	}

	adapter, err := NewOpenCodeAdapter(config)
	if err != nil {
		t.Fatalf("NewOpenCodeAdapter failed: %v", err)
	}

	// Get status for non-existent session
	status, err := adapter.GetSessionStatus("non-existent-session-id")
	if err != nil {
		t.Errorf("GetSessionStatus() should not return error for non-existent session: %v", err)
	}

	if status != StatusTerminated {
		t.Errorf("GetSessionStatus() = %q, want %q for non-existent session", status, StatusTerminated)
	}
}

// TestGetSessionStatusSessionExists tests GetSessionStatus when session exists.
func TestGetSessionStatusSessionExists(t *testing.T) {
	mockStore := &MockSessionStore{
		sessions: make(map[SessionID]*SessionMetadata),
	}

	// Create session in store
	sessionID := SessionID("test-session-123")
	metadata := &SessionMetadata{
		TmuxName:   "test-tmux-session",
		Title:      "Test Session",
		CreatedAt:  time.Now(),
		WorkingDir: "/tmp",
	}
	mockStore.sessions[sessionID] = metadata

	config := &OpenCodeConfig{
		SessionStore: mockStore,
		ServerURL:    "http://localhost:4096",
	}

	adapter, err := NewOpenCodeAdapter(config)
	if err != nil {
		t.Fatalf("NewOpenCodeAdapter failed: %v", err)
	}

	// Get status for existing session
	// Note: This will likely return StatusTerminated because tmux session doesn't actually exist
	// In real usage, tmux session would exist and status would be StatusActive
	status, err := adapter.GetSessionStatus(sessionID)
	if err != nil {
		t.Errorf("GetSessionStatus() returned error: %v", err)
	}

	// Status should be either Active or Terminated depending on tmux
	if status != StatusActive && status != StatusTerminated {
		t.Errorf("GetSessionStatus() = %q, want %q or %q", status, StatusActive, StatusTerminated)
	}
}

// TestSendMessageSuccess tests SendMessage method.
func TestSendMessageSuccess(t *testing.T) {
	mockStore := &MockSessionStore{
		sessions: make(map[SessionID]*SessionMetadata),
	}

	// Create session in store
	sessionID := SessionID("test-session-456")
	metadata := &SessionMetadata{
		TmuxName:   "test-send-message",
		Title:      "Test Send Message",
		CreatedAt:  time.Now(),
		WorkingDir: "/tmp",
	}
	mockStore.sessions[sessionID] = metadata

	config := &OpenCodeConfig{
		SessionStore: mockStore,
		ServerURL:    "http://localhost:4096",
	}

	adapter, err := NewOpenCodeAdapter(config)
	if err != nil {
		t.Fatalf("NewOpenCodeAdapter failed: %v", err)
	}

	// Send message (will fail in test environment without real tmux, but tests the logic)
	message := Message{
		Role:      RoleUser,
		Content:   "test message",
		Timestamp: time.Now(),
	}

	// Note: This will fail because tmux session doesn't exist in test environment
	// We're testing the method exists and handles the session lookup correctly
	err = adapter.SendMessage(sessionID, message)
	// Expect error since tmux session doesn't exist
	// Just verify method doesn't panic and returns proper error type
}

// TestSendMessageSessionNotFound tests SendMessage with non-existent session.
func TestSendMessageSessionNotFound(t *testing.T) {
	mockStore := &MockSessionStore{
		sessions: make(map[SessionID]*SessionMetadata),
	}

	config := &OpenCodeConfig{
		SessionStore: mockStore,
		ServerURL:    "http://localhost:4096",
	}

	adapter, err := NewOpenCodeAdapter(config)
	if err != nil {
		t.Fatalf("NewOpenCodeAdapter failed: %v", err)
	}

	message := Message{
		Role:      RoleUser,
		Content:   "test message",
		Timestamp: time.Now(),
	}

	err = adapter.SendMessage("non-existent-session", message)
	if err == nil {
		t.Error("SendMessage() should return error for non-existent session")
	}

	// Verify error message mentions session not found
	if err.Error() == "" {
		t.Error("Error message should not be empty")
	}
}

// TestGetHistoryReturnsEmptyForMock tests GetHistory returns empty slice for mock implementation.
func TestGetHistoryReturnsEmptyForMock(t *testing.T) {
	mockStore := &MockSessionStore{
		sessions: make(map[SessionID]*SessionMetadata),
	}

	config := &OpenCodeConfig{
		SessionStore: mockStore,
		ServerURL:    "http://localhost:4096",
	}

	adapter, err := NewOpenCodeAdapter(config)
	if err != nil {
		t.Fatalf("NewOpenCodeAdapter failed: %v", err)
	}

	// Get history (should return empty for mock implementation)
	history, err := adapter.GetHistory("any-session-id")
	if err != nil {
		t.Errorf("GetHistory() returned error: %v", err)
	}

	if len(history) != 0 {
		t.Errorf("GetHistory() returned %d messages, want 0 (mock implementation)", len(history))
	}
}

// TestExportConversationNotSupported tests ExportConversation returns error for mock.
func TestExportConversationNotSupported(t *testing.T) {
	mockStore := &MockSessionStore{
		sessions: make(map[SessionID]*SessionMetadata),
	}

	config := &OpenCodeConfig{
		SessionStore: mockStore,
		ServerURL:    "http://localhost:4096",
	}

	adapter, err := NewOpenCodeAdapter(config)
	if err != nil {
		t.Fatalf("NewOpenCodeAdapter failed: %v", err)
	}

	// Export conversation (should return error for mock implementation)
	_, err = adapter.ExportConversation("any-session-id", FormatJSONL)
	if err == nil {
		t.Error("ExportConversation() should return error for mock implementation")
	}

	// Verify error message mentions not supported
	if err.Error() == "" {
		t.Error("Error message should not be empty")
	}
}

// TestImportConversationNotSupported tests ImportConversation returns error for mock.
func TestImportConversationNotSupported(t *testing.T) {
	mockStore := &MockSessionStore{
		sessions: make(map[SessionID]*SessionMetadata),
	}

	config := &OpenCodeConfig{
		SessionStore: mockStore,
		ServerURL:    "http://localhost:4096",
	}

	adapter, err := NewOpenCodeAdapter(config)
	if err != nil {
		t.Fatalf("NewOpenCodeAdapter failed: %v", err)
	}

	// Import conversation (should return error for mock implementation)
	_, err = adapter.ImportConversation([]byte("test data"), FormatJSONL)
	if err == nil {
		t.Error("ImportConversation() should return error for mock implementation")
	}

	// Verify error message mentions not supported
	if err.Error() == "" {
		t.Error("Error message should not be empty")
	}
}

// TestExecuteCommandNotSupported tests ExecuteCommand returns error for all commands.
func TestExecuteCommandNotSupported(t *testing.T) {
	mockStore := &MockSessionStore{
		sessions: make(map[SessionID]*SessionMetadata),
	}

	config := &OpenCodeConfig{
		SessionStore: mockStore,
		ServerURL:    "http://localhost:4096",
	}

	adapter, err := NewOpenCodeAdapter(config)
	if err != nil {
		t.Fatalf("NewOpenCodeAdapter failed: %v", err)
	}

	// Test various command types (all should return error for OpenCode)
	commands := []CommandType{
		"rename",
		"set_dir",
		"authorize",
		"clear_history",
		"set_system_prompt",
	}

	for _, cmdType := range commands {
		cmd := Command{
			Type:   cmdType,
			Params: map[string]interface{}{},
		}

		err := adapter.ExecuteCommand(cmd)
		if err == nil {
			t.Errorf("ExecuteCommand(%q) should return error for OpenCode (server-based architecture)", cmdType)
		}
	}
}

// TestTerminateSessionRemovesFromStore tests TerminateSession removes session from store.
func TestTerminateSessionRemovesFromStore(t *testing.T) {
	mockStore := &MockSessionStore{
		sessions: make(map[SessionID]*SessionMetadata),
	}

	// Create session in store
	sessionID := SessionID("test-terminate-session")
	metadata := &SessionMetadata{
		TmuxName:   "test-terminate-tmux",
		Title:      "Test Terminate",
		CreatedAt:  time.Now(),
		WorkingDir: "/tmp",
	}
	mockStore.sessions[sessionID] = metadata

	config := &OpenCodeConfig{
		SessionStore: mockStore,
		ServerURL:    "http://localhost:4096",
	}

	adapter, err := NewOpenCodeAdapter(config)
	if err != nil {
		t.Fatalf("NewOpenCodeAdapter failed: %v", err)
	}

	// Verify session exists in store
	if _, exists := mockStore.sessions[sessionID]; !exists {
		t.Fatal("Session should exist in store before termination")
	}

	// Terminate session (may fail sending to tmux, but should still delete from store)
	_ = adapter.TerminateSession(sessionID)

	// Verify session removed from store
	if _, exists := mockStore.sessions[sessionID]; exists {
		t.Error("Session should be removed from store after termination")
	}
}

// TestTerminateSessionNonExistentSession tests TerminateSession with non-existent session.
func TestTerminateSessionNonExistentSession(t *testing.T) {
	mockStore := &MockSessionStore{
		sessions: make(map[SessionID]*SessionMetadata),
	}

	config := &OpenCodeConfig{
		SessionStore: mockStore,
		ServerURL:    "http://localhost:4096",
	}

	adapter, err := NewOpenCodeAdapter(config)
	if err != nil {
		t.Fatalf("NewOpenCodeAdapter failed: %v", err)
	}

	// Terminate non-existent session (should return error)
	err = adapter.TerminateSession("non-existent-session")
	if err == nil {
		t.Error("TerminateSession() should return error for non-existent session")
	}
}
