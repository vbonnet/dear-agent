package main

import (
	"testing"
	"time"

	"github.com/vbonnet/dear-agent/agm/internal/agent"
)

// TestSendViaAgent tests sending messages via the Agent interface (for API-based sessions)
func TestSendViaAgent(t *testing.T) {
	// Create mock OpenAI adapter
	mockAdapter := &mockAgentAdapter{
		sentMessages: make([]agent.Message, 0),
	}

	tests := []struct {
		name          string
		sessionID     string
		senderName    string
		messageID     string
		message       string
		wantErr       bool
		checkMessages func(*testing.T, []agent.Message)
	}{
		{
			name:       "send message successfully",
			sessionID:  "test-session-1",
			senderName: "astrocyte",
			messageID:  "1234567890-astrocyte-001",
			message:    "Test message content",
			wantErr:    false,
			checkMessages: func(t *testing.T, messages []agent.Message) {
				if len(messages) != 1 {
					t.Errorf("expected 1 message sent, got %d", len(messages))
					return
				}

				msg := messages[0]
				if msg.Role != agent.RoleUser {
					t.Errorf("expected role %s, got %s", agent.RoleUser, msg.Role)
				}
				if msg.Content != "Test message content" {
					t.Errorf("expected content 'Test message content', got %s", msg.Content)
				}
				if msg.ID != "1234567890-astrocyte-001" {
					t.Errorf("expected ID '1234567890-astrocyte-001', got %s", msg.ID)
				}
			},
		},
		{
			name:       "send message with metadata",
			sessionID:  "test-session-2",
			senderName: "manual-user",
			messageID:  "1234567890-manual-002",
			message:    "[From: manual-user | ID: 1234567890-manual-002]\nHello",
			wantErr:    false,
			checkMessages: func(t *testing.T, messages []agent.Message) {
				if len(messages) != 1 {
					t.Errorf("expected 1 message sent, got %d", len(messages))
					return
				}

				msg := messages[0]
				if msg.Content != "[From: manual-user | ID: 1234567890-manual-002]\nHello" {
					t.Errorf("expected formatted message, got %s", msg.Content)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Reset mock adapter
			mockAdapter.sentMessages = make([]agent.Message, 0)

			// Create message
			msg := agent.Message{
				ID:        tt.messageID,
				Role:      agent.RoleUser,
				Content:   tt.message,
				Timestamp: time.Now(),
			}

			// Send via adapter
			err := mockAdapter.SendMessage(agent.SessionID(tt.sessionID), msg)

			// Check error
			if (err != nil) != tt.wantErr {
				t.Errorf("SendMessage() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			// Check messages
			if tt.checkMessages != nil && !tt.wantErr {
				tt.checkMessages(t, mockAdapter.sentMessages)
			}
		})
	}
}

// TestDetectAgentType tests detection of agent type from manifest
func TestDetectAgentType(t *testing.T) {
	tests := []struct {
		name      string
		agentType string
		wantTmux  bool
		wantAPI   bool
	}{
		{
			name:      "claude is tmux-based",
			agentType: "claude",
			wantTmux:  true,
			wantAPI:   false,
		},
		{
			name:      "gemini is tmux-based",
			agentType: "gemini",
			wantTmux:  true,
			wantAPI:   false,
		},
		{
			name:      "openai is API-based",
			agentType: "openai",
			wantTmux:  false,
			wantAPI:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			isTmux := isTmuxBasedAgent(tt.agentType)
			isAPI := !isTmux

			if isTmux != tt.wantTmux {
				t.Errorf("isTmuxBasedAgent(%s) = %v, want %v", tt.agentType, isTmux, tt.wantTmux)
			}
			if isAPI != tt.wantAPI {
				t.Errorf("isAPIBasedAgent(%s) = %v, want %v", tt.agentType, isAPI, tt.wantAPI)
			}
		})
	}
}

// mockAgentAdapter is a mock implementation of the Agent interface for testing
type mockAgentAdapter struct {
	sentMessages []agent.Message
	sendError    error
}

func (m *mockAgentAdapter) Name() string {
	return "mock"
}

func (m *mockAgentAdapter) Version() string {
	return "1.0.0"
}

func (m *mockAgentAdapter) CreateSession(ctx agent.SessionContext) (agent.SessionID, error) {
	return agent.SessionID("mock-session"), nil
}

func (m *mockAgentAdapter) ResumeSession(sessionID agent.SessionID) error {
	return nil
}

func (m *mockAgentAdapter) TerminateSession(sessionID agent.SessionID) error {
	return nil
}

func (m *mockAgentAdapter) GetSessionStatus(sessionID agent.SessionID) (agent.Status, error) {
	return agent.StatusActive, nil
}

func (m *mockAgentAdapter) SendMessage(sessionID agent.SessionID, message agent.Message) error {
	if m.sendError != nil {
		return m.sendError
	}

	m.sentMessages = append(m.sentMessages, message)
	return nil
}

func (m *mockAgentAdapter) GetHistory(sessionID agent.SessionID) ([]agent.Message, error) {
	return m.sentMessages, nil
}

func (m *mockAgentAdapter) ExportConversation(sessionID agent.SessionID, format agent.ConversationFormat) ([]byte, error) {
	return []byte(""), nil
}

func (m *mockAgentAdapter) ImportConversation(data []byte, format agent.ConversationFormat) (agent.SessionID, error) {
	return agent.SessionID("imported"), nil
}

func (m *mockAgentAdapter) Capabilities() agent.Capabilities {
	return agent.Capabilities{
		SupportsSlashCommands: false,
		SupportsHooks:         true,
		SupportsTools:         false,
		SupportsVision:        false,
		SupportsMultimodal:    false,
		SupportsStreaming:     false,
		SupportsSystemPrompts: true,
		MaxContextWindow:      8192,
		ModelName:             "mock-1.0",
	}
}

func (m *mockAgentAdapter) ExecuteCommand(cmd agent.Command) error {
	return nil
}

// Helper function to determine if an agent is tmux-based
// This will be implemented in send.go
func isTmuxBasedAgent(agentType string) bool {
	switch agentType {
	case "claude", "gemini":
		return true
	case "openai", "gpt":
		return false
	default:
		// Unknown agents default to tmux-based for backward compatibility
		return true
	}
}
