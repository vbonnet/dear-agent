package openai

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestWithSessionLockContextCancelsContendedWait(t *testing.T) {
	sm, err := NewSessionManager(t.TempDir())
	if err != nil {
		t.Fatalf("NewSessionManager() error: %v", err)
	}
	entered := make(chan struct{})
	release := make(chan struct{})
	firstDone := make(chan error, 1)
	go func() {
		firstDone <- sm.WithSessionLock("context-lock", func() error {
			close(entered)
			<-release
			return nil
		})
	}()
	select {
	case <-entered:
	case <-time.After(2 * time.Second):
		t.Fatal("first session lock was not acquired")
	}

	ctx, cancel := context.WithTimeout(t.Context(), 40*time.Millisecond)
	defer cancel()
	callbackCalled := false
	err = sm.WithSessionLockContext(ctx, "context-lock", func() error {
		callbackCalled = true
		return nil
	})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("contended context lock error = %v, want deadline exceeded", err)
	}
	if callbackCalled {
		t.Fatal("contended callback ran after its context expired")
	}
	close(release)
	if err := <-firstDone; err != nil {
		t.Fatalf("release first session lock: %v", err)
	}
}

func TestMetadataUpdatesSerializeAndPreserveIndependentFields(t *testing.T) {
	sessionsDir := t.TempDir()
	first, err := NewSessionManager(sessionsDir)
	if err != nil {
		t.Fatalf("create first manager: %v", err)
	}
	const sessionID = "serialized-metadata"
	if _, err := first.CreateSession(sessionID, "gpt-4", "/original"); err != nil {
		t.Fatalf("create session: %v", err)
	}
	second, err := NewSessionManager(sessionsDir)
	if err != nil {
		t.Fatalf("create second manager: %v", err)
	}

	lockEntered := make(chan struct{})
	releaseLock := make(chan struct{})
	lockDone := make(chan error, 1)
	go func() {
		lockDone <- first.WithSessionLock(sessionID, func() error {
			close(lockEntered)
			<-releaseLock
			return nil
		})
	}()
	select {
	case <-lockEntered:
	case <-time.After(2 * time.Second):
		t.Fatal("metadata transaction lock was not acquired")
	}

	updateDone := make(chan error, 1)
	go func() { updateDone <- second.UpdateTitle(sessionID, "updated-title") }()
	select {
	case err := <-updateDone:
		close(releaseLock)
		t.Fatalf("metadata update bypassed the session lock: %v", err)
	case <-time.After(75 * time.Millisecond):
	}
	close(releaseLock)
	if err := <-lockDone; err != nil {
		t.Fatalf("release metadata transaction lock: %v", err)
	}
	if err := <-updateDone; err != nil {
		t.Fatalf("update title after lock release: %v", err)
	}

	if err := first.UpdateWorkingDirectory(sessionID, "/updated"); err != nil {
		t.Fatalf("update directory through stale manager: %v", err)
	}
	wantRuntime := SessionRuntimeConfig{Temperature: 0.7, MaxTokens: 707}
	if err := second.UpdateRuntimeConfig(sessionID, wantRuntime); err != nil {
		t.Fatalf("update runtime config through stale manager: %v", err)
	}
	reader, err := NewSessionManager(sessionsDir)
	if err != nil {
		t.Fatalf("create metadata reader: %v", err)
	}
	info, err := reader.GetSession(sessionID)
	if err != nil {
		t.Fatalf("read serialized metadata: %v", err)
	}
	if info.Title != "updated-title" || info.WorkingDirectory != "/updated" || info.RuntimeConfig == nil || *info.RuntimeConfig != wantRuntime {
		t.Fatalf("independent metadata updates clobbered fields: %#v", info)
	}
}

func TestNewSessionManager(t *testing.T) {
	tempDir := t.TempDir()

	sm, err := NewSessionManager(tempDir)
	if err != nil {
		t.Fatalf("NewSessionManager failed: %v", err)
	}

	if sm == nil {
		t.Fatal("Expected non-nil session manager")
		return
	}

	if sm.baseDir != tempDir {
		t.Errorf("Expected baseDir %s, got %s", tempDir, sm.baseDir)
	}
}

func TestCreateSession(t *testing.T) {
	tempDir := t.TempDir()
	sm, err := NewSessionManager(tempDir)
	if err != nil {
		t.Fatalf("NewSessionManager failed: %v", err)
	}

	sessionID := "test-session-1"
	model := "gpt-4"
	workingDir := "/test/path"

	info, err := sm.CreateSession(sessionID, model, workingDir)
	if err != nil {
		t.Fatalf("CreateSession failed: %v", err)
	}

	if info.ID != sessionID {
		t.Errorf("Expected ID %s, got %s", sessionID, info.ID)
	}

	if info.Model != model {
		t.Errorf("Expected Model %s, got %s", model, info.Model)
	}

	if info.WorkingDirectory != workingDir {
		t.Errorf("Expected WorkingDirectory %s, got %s", workingDir, info.WorkingDirectory)
	}

	if info.CreatedAt.IsZero() {
		t.Error("Expected non-zero CreatedAt")
	}

	// Verify session directory was created
	sessionDir := filepath.Join(tempDir, sessionID)
	if _, err := os.Stat(sessionDir); os.IsNotExist(err) {
		t.Errorf("Expected session directory to exist: %s", sessionDir)
	}

	// Verify metadata file was created
	metadataPath := filepath.Join(sessionDir, "metadata.json")
	if _, err := os.Stat(metadataPath); os.IsNotExist(err) {
		t.Errorf("Expected metadata file to exist: %s", metadataPath)
	}
}

func TestCreateSessionDuplicate(t *testing.T) {
	tempDir := t.TempDir()
	sm, err := NewSessionManager(tempDir)
	if err != nil {
		t.Fatalf("NewSessionManager failed: %v", err)
	}

	sessionID := "duplicate-session"

	_, err = sm.CreateSession(sessionID, "gpt-4", "/test")
	if err != nil {
		t.Fatalf("First CreateSession failed: %v", err)
	}

	_, err = sm.CreateSession(sessionID, "gpt-4", "/test")
	if err == nil {
		t.Error("Expected error when creating duplicate session")
	}
}

func TestGetSession(t *testing.T) {
	tempDir := t.TempDir()
	sm, err := NewSessionManager(tempDir)
	if err != nil {
		t.Fatalf("NewSessionManager failed: %v", err)
	}

	sessionID := "test-session-2"

	// Create session
	created, err := sm.CreateSession(sessionID, "gpt-3.5-turbo", "/work")
	if err != nil {
		t.Fatalf("CreateSession failed: %v", err)
	}

	// Get session
	retrieved, err := sm.GetSession(sessionID)
	if err != nil {
		t.Fatalf("GetSession failed: %v", err)
	}

	if retrieved.ID != created.ID {
		t.Errorf("Expected ID %s, got %s", created.ID, retrieved.ID)
	}

	if retrieved.Model != created.Model {
		t.Errorf("Expected Model %s, got %s", created.Model, retrieved.Model)
	}
}

func TestGetSessionNotFound(t *testing.T) {
	tempDir := t.TempDir()
	sm, err := NewSessionManager(tempDir)
	if err != nil {
		t.Fatalf("NewSessionManager failed: %v", err)
	}

	_, err = sm.GetSession("non-existent")
	if err == nil {
		t.Error("Expected error when getting non-existent session")
	}
}

func TestListSessions(t *testing.T) {
	tempDir := t.TempDir()
	sm, err := NewSessionManager(tempDir)
	if err != nil {
		t.Fatalf("NewSessionManager failed: %v", err)
	}

	// Create multiple sessions
	sessions := []string{"session-1", "session-2", "session-3"}
	for _, id := range sessions {
		if _, err := sm.CreateSession(id, "gpt-4", "/work"); err != nil {
			t.Fatalf("CreateSession failed: %v", err)
		}
	}

	// List sessions
	list := sm.ListSessions()

	if len(list) != len(sessions) {
		t.Errorf("Expected %d sessions, got %d", len(sessions), len(list))
	}

	// Verify all sessions are in the list
	sessionMap := make(map[string]bool)
	for _, id := range list {
		sessionMap[id] = true
	}

	for _, id := range sessions {
		if !sessionMap[id] {
			t.Errorf("Session %s not found in list", id)
		}
	}
}

func TestAddMessage(t *testing.T) {
	tempDir := t.TempDir()
	sm, err := NewSessionManager(tempDir)
	if err != nil {
		t.Fatalf("NewSessionManager failed: %v", err)
	}

	sessionID := "message-test-session"

	_, err = sm.CreateSession(sessionID, "gpt-4", "/work")
	if err != nil {
		t.Fatalf("CreateSession failed: %v", err)
	}

	// Add message
	msg := Message{
		Role:    "user",
		Content: "Hello, world!",
	}

	err = sm.AddMessage(sessionID, msg)
	if err != nil {
		t.Fatalf("AddMessage failed: %v", err)
	}

	// Verify message was added
	messages, err := sm.GetMessages(sessionID)
	if err != nil {
		t.Fatalf("GetMessages failed: %v", err)
	}

	if len(messages) != 1 {
		t.Fatalf("Expected 1 message, got %d", len(messages))
	}

	if messages[0].Role != msg.Role {
		t.Errorf("Expected Role %s, got %s", msg.Role, messages[0].Role)
	}

	if messages[0].Content != msg.Content {
		t.Errorf("Expected Content %s, got %s", msg.Content, messages[0].Content)
	}

	if messages[0].Timestamp.IsZero() {
		t.Error("Expected non-zero Timestamp")
	}
}

func TestAddMultipleMessages(t *testing.T) {
	tempDir := t.TempDir()
	sm, err := NewSessionManager(tempDir)
	if err != nil {
		t.Fatalf("NewSessionManager failed: %v", err)
	}

	sessionID := "multi-message-session"

	_, err = sm.CreateSession(sessionID, "gpt-4", "/work")
	if err != nil {
		t.Fatalf("CreateSession failed: %v", err)
	}

	// Add multiple messages
	messages := []Message{
		{Role: "user", Content: "First message"},
		{Role: "assistant", Content: "Second message"},
		{Role: "user", Content: "Third message"},
	}

	for _, msg := range messages {
		if err := sm.AddMessage(sessionID, msg); err != nil {
			t.Fatalf("AddMessage failed: %v", err)
		}
	}

	// Retrieve messages
	retrieved, err := sm.GetMessages(sessionID)
	if err != nil {
		t.Fatalf("GetMessages failed: %v", err)
	}

	if len(retrieved) != len(messages) {
		t.Fatalf("Expected %d messages, got %d", len(messages), len(retrieved))
	}

	for i, msg := range retrieved {
		if msg.Content != messages[i].Content {
			t.Errorf("Message %d: expected Content %s, got %s", i, messages[i].Content, msg.Content)
		}
	}
}

func TestUpdateTitle(t *testing.T) {
	tempDir := t.TempDir()
	sm, err := NewSessionManager(tempDir)
	if err != nil {
		t.Fatalf("NewSessionManager failed: %v", err)
	}

	sessionID := "title-test-session"

	_, err = sm.CreateSession(sessionID, "gpt-4", "/work")
	if err != nil {
		t.Fatalf("CreateSession failed: %v", err)
	}

	newTitle := "My Custom Title"

	err = sm.UpdateTitle(sessionID, newTitle)
	if err != nil {
		t.Fatalf("UpdateTitle failed: %v", err)
	}

	// Verify title was updated
	info, err := sm.GetSession(sessionID)
	if err != nil {
		t.Fatalf("GetSession failed: %v", err)
	}

	if info.Title != newTitle {
		t.Errorf("Expected Title %s, got %s", newTitle, info.Title)
	}
}

func TestUpdateWorkingDirectory(t *testing.T) {
	tempDir := t.TempDir()
	sm, err := NewSessionManager(tempDir)
	if err != nil {
		t.Fatalf("NewSessionManager failed: %v", err)
	}

	sessionID := "workdir-test-session"

	_, err = sm.CreateSession(sessionID, "gpt-4", "/old/path")
	if err != nil {
		t.Fatalf("CreateSession failed: %v", err)
	}

	newWorkDir := "/new/path"

	err = sm.UpdateWorkingDirectory(sessionID, newWorkDir)
	if err != nil {
		t.Fatalf("UpdateWorkingDirectory failed: %v", err)
	}

	// Verify working directory was updated
	info, err := sm.GetSession(sessionID)
	if err != nil {
		t.Fatalf("GetSession failed: %v", err)
	}

	if info.WorkingDirectory != newWorkDir {
		t.Errorf("Expected WorkingDirectory %s, got %s", newWorkDir, info.WorkingDirectory)
	}
}

func TestDeleteSession(t *testing.T) {
	tempDir := t.TempDir()
	sm, err := NewSessionManager(tempDir)
	if err != nil {
		t.Fatalf("NewSessionManager failed: %v", err)
	}

	sessionID := "delete-test-session"

	_, err = sm.CreateSession(sessionID, "gpt-4", "/work")
	if err != nil {
		t.Fatalf("CreateSession failed: %v", err)
	}

	// Delete session
	err = sm.DeleteSession(sessionID)
	if err != nil {
		t.Fatalf("DeleteSession failed: %v", err)
	}

	// Verify session was deleted
	_, err = sm.GetSession(sessionID)
	if err == nil {
		t.Error("Expected error when getting deleted session")
	}

	// Verify session directory was removed
	sessionDir := filepath.Join(tempDir, sessionID)
	if _, err := os.Stat(sessionDir); !os.IsNotExist(err) {
		t.Error("Expected session directory to be deleted")
	}
}

func TestPersistence(t *testing.T) {
	tempDir := t.TempDir()

	// Create first session manager and add session
	sm1, err := NewSessionManager(tempDir)
	if err != nil {
		t.Fatalf("NewSessionManager failed: %v", err)
	}

	sessionID := "persistence-test"
	model := "gpt-4"

	created, err := sm1.CreateSession(sessionID, model, "/work")
	if err != nil {
		t.Fatalf("CreateSession failed: %v", err)
	}

	// Add some messages
	msg1 := Message{Role: "user", Content: "Test message 1"}
	msg2 := Message{Role: "assistant", Content: "Test message 2"}

	if err := sm1.AddMessage(sessionID, msg1); err != nil {
		t.Fatalf("AddMessage failed: %v", err)
	}
	if err := sm1.AddMessage(sessionID, msg2); err != nil {
		t.Fatalf("AddMessage failed: %v", err)
	}

	// Create second session manager (simulating restart)
	sm2, err := NewSessionManager(tempDir)
	if err != nil {
		t.Fatalf("NewSessionManager (2nd) failed: %v", err)
	}

	// Verify session was loaded
	loaded, err := sm2.GetSession(sessionID)
	if err != nil {
		t.Fatalf("GetSession failed after reload: %v", err)
	}

	if loaded.ID != created.ID {
		t.Errorf("Expected ID %s, got %s", created.ID, loaded.ID)
	}

	if loaded.Model != created.Model {
		t.Errorf("Expected Model %s, got %s", created.Model, loaded.Model)
	}

	if loaded.MessageCount != 2 {
		t.Errorf("Expected MessageCount 2, got %d", loaded.MessageCount)
	}

	// Verify messages were persisted
	messages, err := sm2.GetMessages(sessionID)
	if err != nil {
		t.Fatalf("GetMessages failed after reload: %v", err)
	}

	if len(messages) != 2 {
		t.Fatalf("Expected 2 messages, got %d", len(messages))
	}

	if messages[0].Content != msg1.Content {
		t.Errorf("Message 0: expected Content %s, got %s", msg1.Content, messages[0].Content)
	}

	if messages[1].Content != msg2.Content {
		t.Errorf("Message 1: expected Content %s, got %s", msg2.Content, messages[1].Content)
	}
}

func TestMessageTimestampAutoSet(t *testing.T) {
	tempDir := t.TempDir()
	sm, err := NewSessionManager(tempDir)
	if err != nil {
		t.Fatalf("NewSessionManager failed: %v", err)
	}

	sessionID := "timestamp-test"

	_, err = sm.CreateSession(sessionID, "gpt-4", "/work")
	if err != nil {
		t.Fatalf("CreateSession failed: %v", err)
	}

	// Add message without timestamp
	msg := Message{
		Role:    "user",
		Content: "Test message",
	}

	before := time.Now()
	err = sm.AddMessage(sessionID, msg)
	after := time.Now()

	if err != nil {
		t.Fatalf("AddMessage failed: %v", err)
	}

	// Retrieve and verify timestamp was set
	messages, err := sm.GetMessages(sessionID)
	if err != nil {
		t.Fatalf("GetMessages failed: %v", err)
	}

	if len(messages) != 1 {
		t.Fatalf("Expected 1 message, got %d", len(messages))
	}

	timestamp := messages[0].Timestamp

	if timestamp.Before(before) || timestamp.After(after) {
		t.Errorf("Expected timestamp between %v and %v, got %v", before, after, timestamp)
	}
}

func TestCreateSession_WithModelSelection(t *testing.T) {
	tempDir := t.TempDir()
	sm, err := NewSessionManager(tempDir)
	if err != nil {
		t.Fatalf("NewSessionManager failed: %v", err)
	}

	tests := []struct {
		name      string
		sessionID string
		model     string
		workDir   string
		shouldErr bool
	}{
		{
			name:      "gpt-4",
			sessionID: "session-gpt4",
			model:     "gpt-4",
			workDir:   "/work",
			shouldErr: false,
		},
		{
			name:      "gpt-4-turbo",
			sessionID: "session-gpt4-turbo",
			model:     "gpt-4-turbo",
			workDir:   "/work",
			shouldErr: false,
		},
		{
			name:      "gpt-4o",
			sessionID: "session-gpt4o",
			model:     "gpt-4o",
			workDir:   "/work",
			shouldErr: false,
		},
		{
			name:      "o3 reasoning model",
			sessionID: "session-o3",
			model:     "o3",
			workDir:   "/work",
			shouldErr: false,
		},
		{
			name:      "invalid model",
			sessionID: "session-invalid",
			model:     "gpt-5-ultra",
			workDir:   "/work",
			shouldErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			info, err := sm.CreateSession(tt.sessionID, tt.model, tt.workDir)

			if tt.shouldErr {
				if err == nil {
					t.Errorf("expected error for model %q, got nil", tt.model)
				}
			} else {
				if err != nil {
					t.Fatalf("CreateSession failed: %v", err)
				}

				if info.Model != tt.model {
					t.Errorf("expected model %q, got %q", tt.model, info.Model)
				}
			}
		})
	}
}

func TestCreateSession_ModelFromEnvVar(t *testing.T) {
	tempDir := t.TempDir()
	sm, err := NewSessionManager(tempDir)
	if err != nil {
		t.Fatalf("NewSessionManager failed: %v", err)
	}

	// Set environment variable
	testModel := "gpt-4o-mini"
	t.Setenv("OPENAI_MODEL", testModel)
	defer os.Unsetenv("OPENAI_MODEL")

	sessionID := "env-test-session"

	// Create session with empty model - should use env var
	info, err := sm.CreateSession(sessionID, "", "/work")
	if err != nil {
		t.Fatalf("CreateSession failed: %v", err)
	}

	if info.Model != testModel {
		t.Errorf("expected model %q from env var, got %q", testModel, info.Model)
	}
}

func TestCreateSession_DefaultModel(t *testing.T) {
	tempDir := t.TempDir()
	sm, err := NewSessionManager(tempDir)
	if err != nil {
		t.Fatalf("NewSessionManager failed: %v", err)
	}

	// Ensure OPENAI_MODEL is not set
	t.Setenv("OPENAI_MODEL", "") // restored on test cleanup
	os.Unsetenv("OPENAI_MODEL")

	sessionID := "default-model-session"

	// Create session with empty model - should use default
	info, err := sm.CreateSession(sessionID, "", "/work")
	if err != nil {
		t.Fatalf("CreateSession failed: %v", err)
	}

	expectedModel := "gpt-4-turbo-preview"
	if info.Model != expectedModel {
		t.Errorf("expected default model %q, got %q", expectedModel, info.Model)
	}
}

func TestCreateSession_ModelPersistence(t *testing.T) {
	tempDir := t.TempDir()

	// Create first session manager
	sm1, err := NewSessionManager(tempDir)
	if err != nil {
		t.Fatalf("NewSessionManager failed: %v", err)
	}

	sessionID := "model-persist-session"
	testModel := "gpt-4o"

	created, err := sm1.CreateSession(sessionID, testModel, "/work")
	if err != nil {
		t.Fatalf("CreateSession failed: %v", err)
	}

	if created.Model != testModel {
		t.Errorf("expected model %q, got %q", testModel, created.Model)
	}

	// Create second session manager (simulating restart)
	sm2, err := NewSessionManager(tempDir)
	if err != nil {
		t.Fatalf("NewSessionManager (2nd) failed: %v", err)
	}

	// Verify model was persisted
	loaded, err := sm2.GetSession(sessionID)
	if err != nil {
		t.Fatalf("GetSession failed after reload: %v", err)
	}

	if loaded.Model != testModel {
		t.Errorf("expected persisted model %q, got %q", testModel, loaded.Model)
	}
}

func TestUpdateRuntimeConfigPersistence(t *testing.T) {
	tempDir := t.TempDir()
	sm, err := NewSessionManager(tempDir)
	if err != nil {
		t.Fatalf("NewSessionManager failed: %v", err)
	}
	if _, err := sm.CreateSession("runtime-config-session", "gpt-4o", "/work"); err != nil {
		t.Fatalf("CreateSession failed: %v", err)
	}
	want := SessionRuntimeConfig{
		Temperature:     1.15,
		MaxTokens:       2048,
		BaseURL:         "https://azure.example.test",
		IsAzure:         true,
		AzureAPIVersion: "2025-01-01-preview",
	}
	if err := sm.UpdateRuntimeConfig("runtime-config-session", want); err != nil {
		t.Fatalf("UpdateRuntimeConfig failed: %v", err)
	}

	reloaded, err := NewSessionManager(tempDir)
	if err != nil {
		t.Fatalf("reload SessionManager failed: %v", err)
	}
	info, err := reloaded.GetSession("runtime-config-session")
	if err != nil {
		t.Fatalf("GetSession after reload failed: %v", err)
	}
	if info.RuntimeConfig == nil || *info.RuntimeConfig != want {
		t.Fatalf("persisted runtime config = %#v, want %#v", info.RuntimeConfig, want)
	}
}
