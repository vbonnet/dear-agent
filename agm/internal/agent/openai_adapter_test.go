package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/vbonnet/dear-agent/agm/internal/agent/openai"
)

// TestOpenAIAdapterImplementsAgentInterface verifies OpenAIAdapter implements Agent interface.
func TestOpenAIAdapterImplementsAgentInterface(t *testing.T) {
	tmpDir := t.TempDir()
	adapter := createTestAdapter(t, tmpDir)

	// Verify adapter implements Agent interface
	var _ = adapter
}

// TestNewOpenAIAdapter tests OpenAI adapter creation
func TestNewOpenAIAdapter(t *testing.T) {
	tests := []struct {
		name      string
		config    *OpenAIConfig
		setupEnv  func()
		wantErr   bool
		errType   string
		checkFunc func(*testing.T, Agent)
	}{
		{
			name: "default config with env API key",
			config: &OpenAIConfig{
				Model: "gpt-4-turbo-preview",
			},
			setupEnv: func() {
				t.Setenv("OPENAI_API_KEY", "test-key-123")
			},
			wantErr: false,
			checkFunc: func(t *testing.T, a Agent) {
				if a.Name() != "openai" {
					t.Errorf("expected name 'openai', got %s", a.Name())
				}
				if a.Version() != "gpt-4-turbo-preview" {
					t.Errorf("expected version 'gpt-4-turbo-preview', got %s", a.Version())
				}
			},
		},
		{
			name: "custom model selection",
			config: &OpenAIConfig{
				APIKey: "test-key-456",
				Model:  "gpt-3.5-turbo",
			},
			wantErr: false,
			checkFunc: func(t *testing.T, a Agent) {
				if a.Version() != "gpt-3.5-turbo" {
					t.Errorf("expected version 'gpt-3.5-turbo', got %s", a.Version())
				}
			},
		},
		{
			name:    "missing API key",
			config:  &OpenAIConfig{},
			wantErr: true,
			errType: "API_KEY_MISSING",
		},
		{
			name:   "nil config uses defaults",
			config: nil,
			setupEnv: func() {
				t.Setenv("OPENAI_API_KEY", "test-key-789")
			},
			wantErr: false,
			checkFunc: func(t *testing.T, a Agent) {
				if a.Version() != "gpt-4-turbo-preview" {
					t.Errorf("expected default version 'gpt-4-turbo-preview', got %s", a.Version())
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup environment
			os.Unsetenv("OPENAI_API_KEY")
			if tt.setupEnv != nil {
				tt.setupEnv()
			}

			// Create temp sessions dir
			tmpDir := t.TempDir()
			if tt.config != nil {
				tt.config.SessionsDir = tmpDir
			}

			// Create adapter
			ctx := context.Background()
			adapter, err := NewOpenAIAdapter(ctx, tt.config)

			// Check error expectations
			if tt.wantErr {
				if err == nil {
					t.Errorf("expected error, got nil")
				} else if tt.errType != "" {
					// Verify error type if specified (use errors.As for wrapped errors)
					var clientErr *openai.ClientError
					if !errors.As(err, &clientErr) {
						t.Errorf("expected ClientError, got %T", err)
					} else if string(clientErr.Type) != tt.errType {
						t.Errorf("expected error type %s, got %s", tt.errType, clientErr.Type)
					}
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			// Run custom checks
			if tt.checkFunc != nil {
				tt.checkFunc(t, adapter)
			}
		})
	}
}

// TestCreateSession tests session creation
func TestCreateSession(t *testing.T) {
	tmpDir := t.TempDir()
	adapter := createTestAdapter(t, tmpDir)

	tests := []struct {
		name    string
		ctx     SessionContext
		wantErr bool
		check   func(*testing.T, SessionID)
	}{
		{
			name: "basic session creation",
			ctx: SessionContext{
				Name:             "test-session",
				WorkingDirectory: "/tmp",
			},
			wantErr: false,
			check: func(t *testing.T, sessionID SessionID) {
				if sessionID == "" {
					t.Error("expected non-empty session ID")
				}

				// Verify session exists
				status, err := adapter.GetSessionStatus(sessionID)
				if err != nil {
					t.Errorf("failed to get session status: %v", err)
				}
				if status != StatusActive {
					t.Errorf("expected status %s, got %s", StatusActive, status)
				}
			},
		},
		{
			name: "session with workflow",
			ctx: SessionContext{
				Name:             "workflow-session",
				WorkingDirectory: "/tmp",
				WorkflowName:     "deep-research",
			},
			wantErr: false,
			check: func(t *testing.T, sessionID SessionID) {
				// Verify workflow system message was added
				history, err := adapter.GetHistory(sessionID)
				if err != nil {
					t.Errorf("failed to get history: %v", err)
				}
				if len(history) != 1 {
					t.Errorf("expected 1 message (system), got %d", len(history))
				}
				if history[0].Role != "system" {
					t.Errorf("expected system message, got %s", history[0].Role)
				}
			},
		},
		{
			name: "session with empty name",
			ctx: SessionContext{
				WorkingDirectory: "/tmp",
			},
			wantErr: false,
			check: func(t *testing.T, sessionID SessionID) {
				if sessionID == "" {
					t.Error("expected non-empty session ID")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sessionID, err := adapter.CreateSession(tt.ctx)

			if tt.wantErr {
				if err == nil {
					t.Error("expected error, got nil")
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if tt.check != nil {
				tt.check(t, sessionID)
			}
		})
	}
}

// TestResumeSession tests session resumption
func TestResumeSession(t *testing.T) {
	tmpDir := t.TempDir()
	adapter := createTestAdapter(t, tmpDir)

	// Create a session first
	sessionID, err := adapter.CreateSession(SessionContext{
		Name:             "resume-test",
		WorkingDirectory: "/tmp",
	})
	if err != nil {
		t.Fatalf("failed to create session: %v", err)
	}

	// Test resuming existing session
	err = adapter.ResumeSession(sessionID)
	if err != nil {
		t.Errorf("failed to resume session: %v", err)
	}

	// Test resuming non-existent session
	err = adapter.ResumeSession("non-existent-id")
	if err == nil {
		t.Error("expected error resuming non-existent session, got nil")
	}
}

// TestTerminateSession tests session termination
func TestTerminateSession(t *testing.T) {
	tmpDir := t.TempDir()
	adapter := createTestAdapter(t, tmpDir)

	// Create a session
	sessionID, err := adapter.CreateSession(SessionContext{
		Name:             "terminate-test",
		WorkingDirectory: "/tmp",
	})
	if err != nil {
		t.Fatalf("failed to create session: %v", err)
	}

	// Verify session is active
	status, err := adapter.GetSessionStatus(sessionID)
	if err != nil {
		t.Fatalf("failed to get session status: %v", err)
	}
	if status != StatusActive {
		t.Errorf("expected status %s, got %s", StatusActive, status)
	}

	// Terminate session
	err = adapter.TerminateSession(sessionID)
	if err != nil {
		t.Errorf("failed to terminate session: %v", err)
	}

	// Verify session is terminated
	status, err = adapter.GetSessionStatus(sessionID)
	if err != nil {
		t.Fatalf("failed to get session status: %v", err)
	}
	if status != StatusTerminated {
		t.Errorf("expected status %s, got %s", StatusTerminated, status)
	}
}

// TestGetSessionStatus tests session status retrieval
func TestGetSessionStatus(t *testing.T) {
	tmpDir := t.TempDir()
	adapter := createTestAdapter(t, tmpDir)

	// Test non-existent session
	status, err := adapter.GetSessionStatus("non-existent")
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if status != StatusTerminated {
		t.Errorf("expected status %s for non-existent session, got %s", StatusTerminated, status)
	}

	// Create and test active session
	sessionID, err := adapter.CreateSession(SessionContext{
		Name:             "status-test",
		WorkingDirectory: "/tmp",
	})
	if err != nil {
		t.Fatalf("failed to create session: %v", err)
	}

	status, err = adapter.GetSessionStatus(sessionID)
	if err != nil {
		t.Fatalf("failed to get session status: %v", err)
	}
	if status != StatusActive {
		t.Errorf("expected status %s, got %s", StatusActive, status)
	}
}

// Mock OpenAI client for testing
type mockOpenAIClient struct {
	responses        []*openai.ChatCompletionResponse
	errors           []error
	callCount        int
	capturedMessages [][]openai.Message
	capturedModel    string
}

func (m *mockOpenAIClient) CreateChatCompletion(ctx context.Context, messages []openai.Message) (*openai.ChatCompletionResponse, error) {
	m.callCount++
	m.capturedMessages = append(m.capturedMessages, messages)

	if m.callCount > len(m.responses) {
		return nil, fmt.Errorf("mock: no more responses configured")
	}

	idx := m.callCount - 1
	if m.errors != nil && idx < len(m.errors) && m.errors[idx] != nil {
		return nil, m.errors[idx]
	}

	if idx < len(m.responses) {
		return m.responses[idx], nil
	}

	return nil, fmt.Errorf("mock: unexpected call count %d", m.callCount)
}

func (m *mockOpenAIClient) Model() string {
	if m.capturedModel != "" {
		return m.capturedModel
	}
	return "gpt-4-mock"
}

func (m *mockOpenAIClient) IsAzure() bool {
	return false
}

// TestSendMessage tests sending messages with mocked API client
func TestSendMessage(t *testing.T) {
	tests := []struct {
		name         string
		message      Message
		mockResponse *openai.ChatCompletionResponse
		mockError    error
		wantErr      bool
		checkHistory func(*testing.T, []Message)
	}{
		{
			name: "successful message send",
			message: Message{
				Role:    RoleUser,
				Content: "Hello, GPT!",
			},
			mockResponse: &openai.ChatCompletionResponse{
				Content:      "Hello! How can I help you today?",
				Model:        "gpt-4",
				FinishReason: "stop",
			},
			wantErr: false,
			checkHistory: func(t *testing.T, history []Message) {
				if len(history) != 2 {
					t.Errorf("expected 2 messages, got %d", len(history))
					return
				}

				// Check user message
				if history[0].Role != RoleUser {
					t.Errorf("expected user message, got %s", history[0].Role)
				}
				if history[0].Content != "Hello, GPT!" {
					t.Errorf("expected 'Hello, GPT!', got %s", history[0].Content)
				}

				// Check assistant response
				if history[1].Role != RoleAssistant {
					t.Errorf("expected assistant message, got %s", history[1].Role)
				}
				if history[1].Content != "Hello! How can I help you today?" {
					t.Errorf("expected 'Hello! How can I help you today?', got %s", history[1].Content)
				}
			},
		},
		{
			name: "API error handling",
			message: Message{
				Role:    RoleUser,
				Content: "This will fail",
			},
			mockError: fmt.Errorf("API error: rate limit exceeded"),
			wantErr:   true,
		},
		{
			name: "multi-turn conversation",
			message: Message{
				Role:    RoleUser,
				Content: "What's 2+2?",
			},
			mockResponse: &openai.ChatCompletionResponse{
				Content:      "2+2 equals 4.",
				Model:        "gpt-4",
				FinishReason: "stop",
			},
			wantErr: false,
			checkHistory: func(t *testing.T, history []Message) {
				if len(history) != 2 {
					t.Errorf("expected 2 messages in history, got %d", len(history))
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create fresh session manager and session for each test
			tmpDir := t.TempDir()
			sessionManager, err := openai.NewSessionManager(tmpDir)
			if err != nil {
				t.Fatalf("failed to create session manager: %v", err)
			}

			sessionID := SessionID("test-send-message-" + tt.name)
			_, err = sessionManager.CreateSession(string(sessionID), "gpt-4", tmpDir)
			if err != nil {
				t.Fatalf("failed to create session: %v", err)
			}

			// Create fresh mock client for each test
			mockClient := &mockOpenAIClient{
				responses:     []*openai.ChatCompletionResponse{tt.mockResponse},
				errors:        []error{tt.mockError},
				capturedModel: "gpt-4",
			}

			// Create adapter with mock client
			adapter := newOpenAIAdapterWithClient(mockClient, sessionManager)

			// Send message
			err = adapter.SendMessage(sessionID, tt.message)

			// Check error expectations
			if tt.wantErr {
				if err == nil {
					t.Error("expected error, got nil")
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			// Verify message was stored
			history, err := adapter.GetHistory(sessionID)
			if err != nil {
				t.Fatalf("failed to get history: %v", err)
			}

			if tt.checkHistory != nil {
				tt.checkHistory(t, history)
			}

			// Verify API was called with correct messages
			if mockClient.callCount != 1 {
				t.Errorf("expected 1 API call, got %d", mockClient.callCount)
			}
		})
	}
}

// TestSendMessage_NonExistentSession tests sending to non-existent session
func TestSendMessage_NonExistentSession(t *testing.T) {
	tmpDir := t.TempDir()
	sessionManager, err := openai.NewSessionManager(tmpDir)
	if err != nil {
		t.Fatalf("failed to create session manager: %v", err)
	}

	mockClient := &mockOpenAIClient{
		responses:     []*openai.ChatCompletionResponse{},
		capturedModel: "gpt-4",
	}

	adapter := newOpenAIAdapterWithClient(mockClient, sessionManager)

	// Try to send message to non-existent session
	err = adapter.SendMessage("non-existent-session", Message{
		Role:    RoleUser,
		Content: "Test",
	})

	if err == nil {
		t.Error("expected error for non-existent session, got nil")
	}
}

// TestSendMessage_ContextPropagation tests that full history is sent to API
func TestSendMessage_ContextPropagation(t *testing.T) {
	tmpDir := t.TempDir()
	sessionManager, err := openai.NewSessionManager(tmpDir)
	if err != nil {
		t.Fatalf("failed to create session manager: %v", err)
	}

	sessionID := SessionID("context-test")
	_, err = sessionManager.CreateSession(string(sessionID), "gpt-4", tmpDir)
	if err != nil {
		t.Fatalf("failed to create session: %v", err)
	}

	// Add some existing messages
	existingMsgs := []openai.Message{
		{Role: "system", Content: "You are a helpful assistant."},
		{Role: "user", Content: "Hello"},
		{Role: "assistant", Content: "Hi there!"},
	}

	for _, msg := range existingMsgs {
		if err := sessionManager.AddMessage(string(sessionID), msg); err != nil {
			t.Fatalf("failed to add message: %v", err)
		}
	}

	mockClient := &mockOpenAIClient{
		responses: []*openai.ChatCompletionResponse{
			{
				Content:      "Thanks for asking!",
				Model:        "gpt-4",
				FinishReason: "stop",
			},
		},
		capturedModel: "gpt-4",
	}

	adapter := newOpenAIAdapterWithClient(mockClient, sessionManager)

	// Send new message
	err = adapter.SendMessage(sessionID, Message{
		Role:    RoleUser,
		Content: "How are you?",
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify API was called with full context (existing messages + new message)
	if mockClient.callCount != 1 {
		t.Fatalf("expected 1 API call, got %d", mockClient.callCount)
	}

	capturedMessages := mockClient.capturedMessages[0]
	// Should have: system + user + assistant + new user = 4 messages
	expectedCount := 4
	if len(capturedMessages) != expectedCount {
		t.Errorf("expected %d messages passed to API, got %d", expectedCount, len(capturedMessages))
	}

	// Verify last message is the new user message
	if capturedMessages[len(capturedMessages)-1].Content != "How are you?" {
		t.Errorf("expected last message to be 'How are you?', got %s", capturedMessages[len(capturedMessages)-1].Content)
	}
}

// TestSendMessage_MultipleConsecutive tests multiple messages in sequence
func TestSendMessage_MultipleConsecutive(t *testing.T) {
	tmpDir := t.TempDir()
	sessionManager, err := openai.NewSessionManager(tmpDir)
	if err != nil {
		t.Fatalf("failed to create session manager: %v", err)
	}

	sessionID := SessionID("multi-send-test")
	_, err = sessionManager.CreateSession(string(sessionID), "gpt-4", tmpDir)
	if err != nil {
		t.Fatalf("failed to create session: %v", err)
	}

	mockClient := &mockOpenAIClient{
		responses: []*openai.ChatCompletionResponse{
			{Content: "Response 1", Model: "gpt-4", FinishReason: "stop"},
			{Content: "Response 2", Model: "gpt-4", FinishReason: "stop"},
			{Content: "Response 3", Model: "gpt-4", FinishReason: "stop"},
		},
		capturedModel: "gpt-4",
	}

	adapter := newOpenAIAdapterWithClient(mockClient, sessionManager)

	// Send three messages in sequence
	messages := []string{"Message 1", "Message 2", "Message 3"}
	for i, content := range messages {
		err := adapter.SendMessage(sessionID, Message{
			Role:    RoleUser,
			Content: content,
		})

		if err != nil {
			t.Fatalf("failed to send message %d: %v", i+1, err)
		}
	}

	// Verify all messages were stored
	history, err := adapter.GetHistory(sessionID)
	if err != nil {
		t.Fatalf("failed to get history: %v", err)
	}

	// Should have 6 messages total (3 user + 3 assistant)
	expectedCount := 6
	if len(history) != expectedCount {
		t.Errorf("expected %d messages in history, got %d", expectedCount, len(history))
	}

	// Verify alternating user/assistant pattern
	for i := 0; i < len(history); i += 2 {
		if history[i].Role != RoleUser {
			t.Errorf("message %d: expected user role, got %s", i, history[i].Role)
		}
		if i+1 < len(history) && history[i+1].Role != RoleAssistant {
			t.Errorf("message %d: expected assistant role, got %s", i+1, history[i+1].Role)
		}
	}
}

// TestGetHistory tests conversation history retrieval
func TestGetHistory(t *testing.T) {
	tmpDir := t.TempDir()
	adapter := createTestAdapter(t, tmpDir)

	// Create session
	sessionID, err := adapter.CreateSession(SessionContext{
		Name:             "history-test",
		WorkingDirectory: "/tmp",
	})
	if err != nil {
		t.Fatalf("failed to create session: %v", err)
	}

	// Get history (should be empty)
	history, err := adapter.GetHistory(sessionID)
	if err != nil {
		t.Errorf("failed to get history: %v", err)
	}
	if len(history) != 0 {
		t.Errorf("expected empty history, got %d messages", len(history))
	}

	// Test non-existent session
	_, err = adapter.GetHistory("non-existent")
	if err == nil {
		t.Error("expected error for non-existent session, got nil")
	}
}

// TestExportConversation tests conversation export
func TestExportConversation(t *testing.T) {
	tmpDir := t.TempDir()
	adapter := createTestAdapter(t, tmpDir)

	// Create session with a message
	sessionID, err := adapter.CreateSession(SessionContext{
		Name:             "export-test",
		WorkingDirectory: "/tmp",
	})
	if err != nil {
		t.Fatalf("failed to create session: %v", err)
	}

	// Manually add a message to history for testing
	openaiAdapter := adapter.(*OpenAIAdapter)
	msg := openai.Message{
		Role:      "user",
		Content:   "Hello, world!",
		Timestamp: time.Now(),
	}
	if err := openaiAdapter.sessionManager.AddMessage(string(sessionID), msg); err != nil {
		t.Fatalf("failed to add test message: %v", err)
	}

	tests := []struct {
		name    string
		format  ConversationFormat
		wantErr bool
		check   func(*testing.T, []byte)
	}{
		{
			name:    "export as JSONL",
			format:  FormatJSONL,
			wantErr: false,
			check: func(t *testing.T, data []byte) {
				if len(data) == 0 {
					t.Error("expected non-empty export data")
				}
				// Verify it's valid JSONL
				lines := splitLinesOpenAI(string(data))
				for _, line := range lines {
					if line == "" {
						continue
					}
					var msg Message
					if err := json.Unmarshal([]byte(line), &msg); err != nil {
						t.Errorf("invalid JSONL: %v", err)
					}
				}
			},
		},
		{
			name:    "export as Markdown",
			format:  FormatMarkdown,
			wantErr: false,
			check: func(t *testing.T, data []byte) {
				if len(data) == 0 {
					t.Error("expected non-empty export data")
				}
				// Verify it contains expected markdown headers
				content := string(data)
				if !contains(content, "# OpenAI Conversation") {
					t.Error("markdown export missing header")
				}
				if !contains(content, "Session ID:") {
					t.Error("markdown export missing session ID")
				}
			},
		},
		{
			name:    "export as HTML (unsupported)",
			format:  FormatHTML,
			wantErr: true,
		},
		{
			name:    "export as Native (same as JSONL)",
			format:  FormatNative,
			wantErr: false,
			check: func(t *testing.T, data []byte) {
				if len(data) == 0 {
					t.Error("expected non-empty export data")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data, err := adapter.ExportConversation(sessionID, tt.format)

			if tt.wantErr {
				if err == nil {
					t.Error("expected error, got nil")
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if tt.check != nil {
				tt.check(t, data)
			}
		})
	}
}

// TestImportConversation tests conversation import
func TestImportConversation(t *testing.T) {
	tmpDir := t.TempDir()
	adapter := createTestAdapter(t, tmpDir)

	// Create test JSONL data
	messages := []Message{
		{
			ID:        "1",
			Role:      RoleUser,
			Content:   "Hello",
			Timestamp: time.Now(),
		},
		{
			ID:        "2",
			Role:      RoleAssistant,
			Content:   "Hi there!",
			Timestamp: time.Now(),
		},
	}

	var jsonlData []byte
	for _, msg := range messages {
		data, _ := json.Marshal(msg)
		jsonlData = append(jsonlData, data...)
		jsonlData = append(jsonlData, '\n')
	}

	// Test import
	sessionID, err := adapter.ImportConversation(jsonlData, FormatJSONL)
	if err != nil {
		t.Fatalf("failed to import conversation: %v", err)
	}

	// Verify import
	history, err := adapter.GetHistory(sessionID)
	if err != nil {
		t.Errorf("failed to get history: %v", err)
	}
	if len(history) != len(messages) {
		t.Errorf("expected %d messages, got %d", len(messages), len(history))
	}

	// Test unsupported format
	_, err = adapter.ImportConversation([]byte("test"), FormatHTML)
	if err == nil {
		t.Error("expected error for unsupported format, got nil")
	}
}

// TestImportConversation_MalformedData tests import with invalid JSON
func TestImportConversation_MalformedData(t *testing.T) {
	tmpDir := t.TempDir()
	adapter := createTestAdapter(t, tmpDir)

	tests := []struct {
		name string
		data []byte
	}{
		{
			name: "invalid JSON",
			data: []byte("not valid json\n"),
		},
		{
			name: "partial JSON",
			data: []byte("{\"role\":\"user\",\"content\":\n"),
		},
		{
			name: "mixed valid and invalid",
			data: []byte("{\"role\":\"user\",\"content\":\"test\"}\ninvalid line\n"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := adapter.ImportConversation(tt.data, FormatJSONL)
			if err == nil {
				t.Error("expected error for malformed data, got nil")
			}
		})
	}
}

// TestImportConversation_EmptyData tests import with empty data
func TestImportConversation_EmptyData(t *testing.T) {
	tmpDir := t.TempDir()
	adapter := createTestAdapter(t, tmpDir)

	// Import empty data (should succeed and create empty session)
	sessionID, err := adapter.ImportConversation([]byte(""), FormatJSONL)
	if err != nil {
		t.Fatalf("failed to import empty data: %v", err)
	}

	// Verify session was created
	history, err := adapter.GetHistory(sessionID)
	if err != nil {
		t.Fatalf("failed to get history: %v", err)
	}

	if len(history) != 0 {
		t.Errorf("expected 0 messages for empty import, got %d", len(history))
	}
}

// TestCapabilities tests capability reporting
func TestCapabilities(t *testing.T) {
	tests := []struct {
		name        string
		model       string
		wantContext int
		wantVision  bool
	}{
		{
			name:        "gpt-4-turbo",
			model:       "gpt-4-turbo",
			wantContext: 128000,
			wantVision:  true,
		},
		{
			name:        "gpt-4",
			model:       "gpt-4",
			wantContext: 8192,
			wantVision:  false,
		},
		{
			name:        "gpt-3.5-turbo",
			model:       "gpt-3.5-turbo",
			wantContext: 16385,
			wantVision:  false,
		},
		{
			name:        "gpt-4-32k",
			model:       "gpt-4-32k",
			wantContext: 32768,
			wantVision:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			adapter := createTestAdapterWithModel(t, tmpDir, tt.model)

			caps := adapter.Capabilities()

			// Verify common capabilities
			if caps.SupportsSlashCommands {
				t.Error("expected SupportsSlashCommands=false for API adapter")
			}
			if !caps.SupportsHooks {
				t.Error("expected SupportsHooks=true for OpenAI adapter (synthetic hooks)")
			}
			if !caps.SupportsTools {
				t.Error("expected SupportsTools=true for OpenAI")
			}
			if !caps.SupportsStreaming {
				t.Error("expected SupportsStreaming=true for OpenAI")
			}
			if !caps.SupportsSystemPrompts {
				t.Error("expected SupportsSystemPrompts=true for OpenAI")
			}

			// Verify model-specific capabilities
			if caps.MaxContextWindow != tt.wantContext {
				t.Errorf("expected context window %d, got %d", tt.wantContext, caps.MaxContextWindow)
			}
			if caps.SupportsVision != tt.wantVision {
				t.Errorf("expected SupportsVision=%v, got %v", tt.wantVision, caps.SupportsVision)
			}
			if caps.ModelName != tt.model {
				t.Errorf("expected model %s, got %s", tt.model, caps.ModelName)
			}
		})
	}
}

// TestExportConversation_NonExistentSession tests export error handling
func TestExportConversation_NonExistentSession(t *testing.T) {
	tmpDir := t.TempDir()
	adapter := createTestAdapter(t, tmpDir)

	_, err := adapter.ExportConversation("non-existent-id", FormatJSONL)
	if err == nil {
		t.Error("expected error for non-existent session, got nil")
	}
}

// TestExportConversation_EmptyHistory tests exporting session with no messages
func TestExportConversation_EmptyHistory(t *testing.T) {
	tmpDir := t.TempDir()
	adapter := createTestAdapter(t, tmpDir)

	// Create session with no messages
	sessionID, err := adapter.CreateSession(SessionContext{
		Name:             "empty-export-test",
		WorkingDirectory: tmpDir,
	})
	if err != nil {
		t.Fatalf("failed to create session: %v", err)
	}

	tests := []struct {
		name   string
		format ConversationFormat
	}{
		{"JSONL", FormatJSONL},
		{"Markdown", FormatMarkdown},
		{"Native", FormatNative},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data, err := adapter.ExportConversation(sessionID, tt.format)
			if err != nil {
				t.Fatalf("failed to export: %v", err)
			}

			// Should return valid but minimal data
			if data == nil {
				t.Error("expected non-nil data for empty session")
			}
		})
	}
}

// TestExecuteCommand tests command execution
func TestExecuteCommand(t *testing.T) {
	tmpDir := t.TempDir()
	adapter := createTestAdapter(t, tmpDir)

	// Create a test session
	sessionID, err := adapter.CreateSession(SessionContext{
		Name:             "command-test",
		WorkingDirectory: "/tmp",
	})
	if err != nil {
		t.Fatalf("failed to create session: %v", err)
	}

	tests := []struct {
		name    string
		cmd     Command
		wantErr bool
		check   func(*testing.T)
	}{
		{
			name: "rename session",
			cmd: Command{
				Type: CommandRename,
				Params: map[string]any{
					"session_id": string(sessionID),
					"name":       "new-name",
				},
			},
			wantErr: false,
			check: func(t *testing.T) {
				// Verify title was updated
				openaiAdapter := adapter.(*OpenAIAdapter)
				info, err := openaiAdapter.sessionManager.GetSession(string(sessionID))
				if err != nil {
					t.Errorf("failed to get session: %v", err)
				}
				if info.Title != "new-name" {
					t.Errorf("expected title 'new-name', got %s", info.Title)
				}
			},
		},
		{
			name: "set directory",
			cmd: Command{
				Type: CommandSetDir,
				Params: map[string]any{
					"session_id": string(sessionID),
					"path":       "/new/path",
				},
			},
			wantErr: false,
			check: func(t *testing.T) {
				openaiAdapter := adapter.(*OpenAIAdapter)
				info, err := openaiAdapter.sessionManager.GetSession(string(sessionID))
				if err != nil {
					t.Errorf("failed to get session: %v", err)
				}
				if info.WorkingDirectory != "/new/path" {
					t.Errorf("expected working dir '/new/path', got %s", info.WorkingDirectory)
				}
			},
		},
		{
			name: "authorize directory (no-op)",
			cmd: Command{
				Type: CommandAuthorize,
				Params: map[string]any{
					"session_id": string(sessionID),
					"path":       "/some/path",
				},
			},
			wantErr: false,
		},
		{
			name: "run hook (now supported)",
			cmd: Command{
				Type: CommandRunHook,
				Params: map[string]any{
					"session_id": string(sessionID),
					"hook_name":  "SessionStart",
				},
			},
			wantErr: false,
			check: func(t *testing.T) {
				// Verify hook context file was created
				homeDir, _ := os.UserHomeDir()
				hookFile := homeDir + "/.agm/openai-hooks/" + string(sessionID) + "-SessionStart.json"
				if _, err := os.Stat(hookFile); os.IsNotExist(err) {
					t.Errorf("Hook context file was not created: %s", hookFile)
				} else {
					// Cleanup hook file
					_ = os.Remove(hookFile)
				}
			},
		},
		{
			name: "unsupported command",
			cmd: Command{
				Type: "unsupported",
				Params: map[string]any{
					"session_id": string(sessionID),
				},
			},
			wantErr: true,
		},
		{
			name: "missing session_id",
			cmd: Command{
				Type:   CommandRename,
				Params: map[string]any{},
			},
			wantErr: true,
		},
		{
			name: "non-existent session",
			cmd: Command{
				Type: CommandRename,
				Params: map[string]any{
					"session_id": "non-existent",
					"name":       "test",
				},
			},
			wantErr: true,
		},
		{
			name: "rename with missing name param",
			cmd: Command{
				Type: CommandRename,
				Params: map[string]any{
					"session_id": string(sessionID),
				},
			},
			wantErr: true,
		},
		{
			name: "setdir with missing path param",
			cmd: Command{
				Type: CommandSetDir,
				Params: map[string]any{
					"session_id": string(sessionID),
				},
			},
			wantErr: true,
		},
		{
			name: "set_system_prompt with missing prompt param",
			cmd: Command{
				Type: CommandSetSystemPrompt,
				Params: map[string]any{
					"session_id": string(sessionID),
				},
			},
			wantErr: true,
		},
		{
			name: "run_hook with missing hook_name param",
			cmd: Command{
				Type: CommandRunHook,
				Params: map[string]any{
					"session_id": string(sessionID),
				},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := adapter.ExecuteCommand(tt.cmd)

			if tt.wantErr {
				if err == nil {
					t.Error("expected error, got nil")
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if tt.check != nil {
				tt.check(t)
			}
		})
	}
}

// TestClearHistory tests clearing conversation history
func TestClearHistory(t *testing.T) {
	tmpDir := t.TempDir()
	adapter := createTestAdapter(t, tmpDir)

	// Create session and add messages
	sessionID, err := adapter.CreateSession(SessionContext{
		Name:             "clear-test",
		WorkingDirectory: "/tmp",
	})
	if err != nil {
		t.Fatalf("failed to create session: %v", err)
	}

	// Add test messages
	openaiAdapter := adapter.(*OpenAIAdapter)
	msg := openai.Message{
		Role:      "user",
		Content:   "Test message",
		Timestamp: time.Now(),
	}
	if err := openaiAdapter.sessionManager.AddMessage(string(sessionID), msg); err != nil {
		t.Fatalf("failed to add message: %v", err)
	}

	// Verify message exists
	history, err := adapter.GetHistory(sessionID)
	if err != nil {
		t.Fatalf("failed to get history: %v", err)
	}
	if len(history) != 1 {
		t.Errorf("expected 1 message before clear, got %d", len(history))
	}

	// Clear history
	err = adapter.ExecuteCommand(Command{
		Type: CommandClearHistory,
		Params: map[string]interface{}{
			"session_id": string(sessionID),
		},
	})
	if err != nil {
		t.Fatalf("failed to clear history: %v", err)
	}

	// Verify history is cleared
	history, err = adapter.GetHistory(sessionID)
	if err != nil {
		t.Fatalf("failed to get history after clear: %v", err)
	}
	if len(history) != 0 {
		t.Errorf("expected 0 messages after clear, got %d", len(history))
	}
}

// TestSetSystemPrompt tests setting system prompt
func TestSetSystemPrompt(t *testing.T) {
	tmpDir := t.TempDir()
	adapter := createTestAdapter(t, tmpDir)

	// Create session
	sessionID, err := adapter.CreateSession(SessionContext{
		Name:             "system-prompt-test",
		WorkingDirectory: "/tmp",
	})
	if err != nil {
		t.Fatalf("failed to create session: %v", err)
	}

	// Set system prompt
	systemPrompt := "You are a helpful assistant."
	err = adapter.ExecuteCommand(Command{
		Type: CommandSetSystemPrompt,
		Params: map[string]interface{}{
			"session_id": string(sessionID),
			"prompt":     systemPrompt,
		},
	})
	if err != nil {
		t.Fatalf("failed to set system prompt: %v", err)
	}

	// Verify system prompt was added
	history, err := adapter.GetHistory(sessionID)
	if err != nil {
		t.Fatalf("failed to get history: %v", err)
	}
	if len(history) != 1 {
		t.Errorf("expected 1 message (system prompt), got %d", len(history))
	}
	if history[0].Role != "system" {
		t.Errorf("expected system message, got %s", history[0].Role)
	}
	if history[0].Content != systemPrompt {
		t.Errorf("expected content %s, got %s", systemPrompt, history[0].Content)
	}
}

// TestOpenAIAdapter_RunHook tests the RunHook method.
func TestOpenAIAdapter_RunHook(t *testing.T) {
	tmpDir := t.TempDir()
	adapter := createTestAdapter(t, tmpDir).(*OpenAIAdapter)

	// Create test session
	sessionID, err := adapter.CreateSession(SessionContext{
		Name:             "hook-test",
		WorkingDirectory: tmpDir,
	})
	if err != nil {
		t.Fatalf("failed to create session: %v", err)
	}

	// Test different hook types
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
				hookDir := homeDir + "/.agm/openai-hooks"
				hookFile := hookDir + "/" + string(tt.sessionID) + "-" + tt.hookName + ".json"

				if _, err := os.Stat(hookFile); os.IsNotExist(err) {
					t.Errorf("Hook context file was not created: %s", hookFile)
				} else {
					// Verify hook context content
					data, err := os.ReadFile(hookFile)
					if err != nil {
						t.Errorf("Failed to read hook file: %v", err)
					} else {
						var hookContext map[string]interface{}
						if err := json.Unmarshal(data, &hookContext); err != nil {
							t.Errorf("Failed to parse hook context: %v", err)
						} else {
							// Verify expected fields
							if hookContext["session_id"] != string(tt.sessionID) {
								t.Errorf("Expected session_id %s, got %v", tt.sessionID, hookContext["session_id"])
							}
							if hookContext["hook_name"] != tt.hookName {
								t.Errorf("Expected hook_name %s, got %v", tt.hookName, hookContext["hook_name"])
							}
						}
					}
					// Cleanup hook file after test
					_ = os.Remove(hookFile)
				}
			}
		})
	}
}

// TestOpenAIAdapter_ExecuteCommand_RunHook tests ExecuteCommand with CommandRunHook.
func TestOpenAIAdapter_ExecuteCommand_RunHook(t *testing.T) {
	tmpDir := t.TempDir()
	adapter := createTestAdapter(t, tmpDir)

	// Create test session
	sessionID, err := adapter.CreateSession(SessionContext{
		Name:             "cmd-hook-test",
		WorkingDirectory: tmpDir,
	})
	if err != nil {
		t.Fatalf("failed to create session: %v", err)
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
	hookDir := homeDir + "/.agm/openai-hooks"
	hookFile := hookDir + "/" + string(sessionID) + "-SessionStart.json"

	if _, err := os.Stat(hookFile); os.IsNotExist(err) {
		t.Errorf("Hook context file was not created via ExecuteCommand: %s", hookFile)
	} else {
		// Cleanup
		_ = os.Remove(hookFile)
	}
}

// TestOpenAIAdapter_SessionStartHook tests automatic SessionStart hook on CreateSession.
func TestOpenAIAdapter_SessionStartHook(t *testing.T) {
	tmpDir := t.TempDir()
	adapter := createTestAdapter(t, tmpDir)

	// Clean up any existing hook files
	homeDir, _ := os.UserHomeDir()
	hookDir := homeDir + "/.agm/openai-hooks"
	_ = os.RemoveAll(hookDir)

	// Create session (should trigger SessionStart hook)
	sessionID, err := adapter.CreateSession(SessionContext{
		Name:             "auto-hook-test",
		WorkingDirectory: tmpDir,
	})
	if err != nil {
		t.Fatalf("failed to create session: %v", err)
	}

	// Wait briefly for hook to complete
	time.Sleep(100 * time.Millisecond)

	// Verify SessionStart hook file was created
	hookFile := hookDir + "/" + string(sessionID) + "-SessionStart.json"
	if _, err := os.Stat(hookFile); os.IsNotExist(err) {
		t.Errorf("SessionStart hook was not triggered automatically: %s", hookFile)
	} else {
		// Cleanup
		_ = os.Remove(hookFile)
	}
}

// TestOpenAIAdapter_SessionEndHook tests automatic SessionEnd hook on TerminateSession.
func TestOpenAIAdapter_SessionEndHook(t *testing.T) {
	tmpDir := t.TempDir()
	adapter := createTestAdapter(t, tmpDir)

	// Clean up any existing hook files
	homeDir, _ := os.UserHomeDir()
	hookDir := homeDir + "/.agm/openai-hooks"
	_ = os.RemoveAll(hookDir)

	// Create session
	sessionID, err := adapter.CreateSession(SessionContext{
		Name:             "end-hook-test",
		WorkingDirectory: tmpDir,
	})
	if err != nil {
		t.Fatalf("failed to create session: %v", err)
	}

	// Clear SessionStart hook file
	hookFileStart := hookDir + "/" + string(sessionID) + "-SessionStart.json"
	_ = os.Remove(hookFileStart)

	// Terminate session (should trigger SessionEnd hook)
	err = adapter.TerminateSession(sessionID)
	if err != nil {
		t.Fatalf("failed to terminate session: %v", err)
	}

	// Wait briefly for hook to complete
	time.Sleep(100 * time.Millisecond)

	// Verify SessionEnd hook file was created
	hookFileEnd := hookDir + "/" + string(sessionID) + "-SessionEnd.json"
	if _, err := os.Stat(hookFileEnd); os.IsNotExist(err) {
		t.Errorf("SessionEnd hook was not triggered automatically: %s", hookFileEnd)
	} else {
		// Cleanup
		_ = os.Remove(hookFileEnd)
	}
}

// TestOpenAIAdapter_HookFailureGraceful tests that hook failures don't block operations.
func TestOpenAIAdapter_HookFailureGraceful(t *testing.T) {
	tmpDir := t.TempDir()
	adapter := createTestAdapter(t, tmpDir).(*OpenAIAdapter)

	// Create session
	sessionID, err := adapter.CreateSession(SessionContext{
		Name:             "graceful-hook-test",
		WorkingDirectory: tmpDir,
	})
	if err != nil {
		t.Fatalf("failed to create session: %v", err)
	}

	// Make hook directory non-writable to simulate hook failure
	homeDir, _ := os.UserHomeDir()
	hookDir := homeDir + "/.agm/openai-hooks-readonly"
	_ = os.MkdirAll(hookDir, 0555) // Read-only directory
	defer os.RemoveAll(hookDir)

	// Temporarily override hook directory in adapter
	// Note: We can't easily override this without changing the implementation,
	// so we'll just verify that hook errors don't crash the adapter

	// Execute hook on non-existent session (should not crash)
	err = adapter.RunHook(SessionID("nonexistent"), "SessionStart")
	if err == nil {
		t.Errorf("Expected error for non-existent session, got nil")
	}

	// Verify session is still functional after hook failure
	status, err := adapter.GetSessionStatus(sessionID)
	if err != nil {
		t.Errorf("Session should still be functional after hook failure: %v", err)
	}
	if status != StatusActive {
		t.Errorf("Expected session to be active, got %v", status)
	}
}

// TestOpenAIAdapter_Capabilities_HooksSupported tests that hooks are now marked as supported.
func TestOpenAIAdapter_Capabilities_HooksSupported(t *testing.T) {
	tmpDir := t.TempDir()
	adapter := createTestAdapter(t, tmpDir)

	capabilities := adapter.Capabilities()

	if !capabilities.SupportsHooks {
		t.Error("Expected SupportsHooks to be true, got false")
	}
}

// TestName tests the Name method
func TestName(t *testing.T) {
	tmpDir := t.TempDir()
	adapter := createTestAdapter(t, tmpDir)

	name := adapter.Name()
	if name != "openai" {
		t.Errorf("expected name 'openai', got %s", name)
	}
}

// TestVersion tests the Version method with different models
func TestVersion(t *testing.T) {
	tmpDir := t.TempDir()

	tests := []struct {
		model    string
		expected string
	}{
		{"gpt-4", "gpt-4"},
		{"gpt-4-turbo", "gpt-4-turbo"},
		{"gpt-4o", "gpt-4o"},
		{"gpt-3.5-turbo", "gpt-3.5-turbo"},
	}

	for _, tt := range tests {
		t.Run(tt.model, func(t *testing.T) {
			adapter := createTestAdapterWithModel(t, tmpDir, tt.model)
			version := adapter.Version()
			if version != tt.expected {
				t.Errorf("expected version %s, got %s", tt.expected, version)
			}
		})
	}
}

// TestCapabilities_VariousModels tests capabilities for different models
func TestCapabilities_VariousModels(t *testing.T) {
	tmpDir := t.TempDir()

	tests := []struct {
		model             string
		expectedContext   int
		expectedVision    bool
		expectedStreaming bool
	}{
		{
			model:             "gpt-4",
			expectedContext:   8192,
			expectedVision:    false,
			expectedStreaming: true,
		},
		{
			model:             "gpt-4-turbo",
			expectedContext:   128000,
			expectedVision:    true,
			expectedStreaming: true,
		},
		{
			model:             "gpt-4-32k",
			expectedContext:   32768,
			expectedVision:    false,
			expectedStreaming: true,
		},
		{
			model:             "gpt-3.5-turbo",
			expectedContext:   16385,
			expectedVision:    false,
			expectedStreaming: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.model, func(t *testing.T) {
			adapter := createTestAdapterWithModel(t, tmpDir, tt.model)
			caps := adapter.Capabilities()

			if caps.MaxContextWindow != tt.expectedContext {
				t.Errorf("model %s: expected context %d, got %d", tt.model, tt.expectedContext, caps.MaxContextWindow)
			}

			if caps.SupportsVision != tt.expectedVision {
				t.Errorf("model %s: expected vision support %v, got %v", tt.model, tt.expectedVision, caps.SupportsVision)
			}

			if caps.SupportsStreaming != tt.expectedStreaming {
				t.Errorf("model %s: expected streaming support %v, got %v", tt.model, tt.expectedStreaming, caps.SupportsStreaming)
			}

			// All OpenAI models should support these
			if !caps.SupportsTools {
				t.Errorf("model %s: expected tools support", tt.model)
			}
			if !caps.SupportsSystemPrompts {
				t.Errorf("model %s: expected system prompts support", tt.model)
			}
			if caps.SupportsSlashCommands {
				t.Errorf("model %s: should not support slash commands (API-based)", tt.model)
			}
			if !caps.SupportsHooks {
				t.Errorf("model %s: should support hooks (synthetic)", tt.model)
			}
		})
	}
}

// TestResumeSession_Errors tests error cases for ResumeSession
func TestResumeSession_Errors(t *testing.T) {
	tmpDir := t.TempDir()
	adapter := createTestAdapter(t, tmpDir)

	// Try to resume non-existent session
	err := adapter.ResumeSession("definitely-does-not-exist")
	if err == nil {
		t.Error("expected error resuming non-existent session, got nil")
	}
}

// TestTerminateSession_NonExistent tests terminating non-existent session
func TestTerminateSession_NonExistent(t *testing.T) {
	tmpDir := t.TempDir()
	adapter := createTestAdapter(t, tmpDir)

	// Try to terminate non-existent session
	err := adapter.TerminateSession("does-not-exist")
	if err == nil {
		t.Error("expected error terminating non-existent session, got nil")
	}
}

// Helper functions

func createTestAdapter(t *testing.T, tmpDir string) Agent {
	return createTestAdapterWithModel(t, tmpDir, "gpt-4-turbo-preview")
}

func createTestAdapterWithModel(t *testing.T, tmpDir string, model string) Agent {
	t.Helper()

	config := &OpenAIConfig{
		APIKey:      "test-key",
		Model:       model,
		SessionsDir: tmpDir,
	}

	ctx := context.Background()
	adapter, err := NewOpenAIAdapter(ctx, config)
	if err != nil {
		t.Fatalf("failed to create adapter: %v", err)
	}

	return adapter
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > len(substr) && findSubstring(s, substr))
}

func findSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
