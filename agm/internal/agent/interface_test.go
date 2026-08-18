package agent

import (
	"encoding/json"
	"testing"
	"time"
)

type mockHarness struct {
	name         string
	version      string
	capabilities Capabilities
}

func (m *mockHarness) Name() string {
	return m.name
}

func (m *mockHarness) Version() string {
	return m.version
}

func (m *mockHarness) Capabilities() Capabilities {
	return m.capabilities
}

func TestHarnessInterfaceIsMetadataOnly(t *testing.T) {
	var _ Harness = (*mockHarness)(nil)

	harness := &mockHarness{
		name:    "mock",
		version: "1.0",
		capabilities: Capabilities{
			SupportsTools:    true,
			MaxContextWindow: 100000,
			ModelName:        "mock-model",
		},
	}
	if harness.Name() != "mock" || harness.Version() != "1.0" {
		t.Fatalf("unexpected harness identity: %s %s", harness.Name(), harness.Version())
	}
	if got := harness.Capabilities().ModelName; got != "mock-model" {
		t.Fatalf("Capabilities().ModelName = %q, want mock-model", got)
	}
}

// TestCapabilities_Struct tests the Capabilities struct.
func TestCapabilities_Struct(t *testing.T) {
	tests := []struct {
		name string
		caps Capabilities
	}{
		{
			name: "claude capabilities",
			caps: Capabilities{
				SupportsSlashCommands: true,
				SupportsTools:         true,
				SupportsVision:        true,
				MaxContextWindow:      200000,
				ModelName:             "claude-sonnet-4.5",
			},
		},
		{
			name: "gemini capabilities",
			caps: Capabilities{
				SupportsSlashCommands: false,
				SupportsTools:         true,
				SupportsVision:        true,
				MaxContextWindow:      1000000,
				ModelName:             "gemini-2.0-flash",
			},
		},
		{
			name: "gpt capabilities",
			caps: Capabilities{
				SupportsSlashCommands: false,
				SupportsTools:         true,
				SupportsVision:        true,
				MaxContextWindow:      128000,
				ModelName:             "gpt-4-turbo",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.caps.ModelName == "" {
				t.Error("ModelName should not be empty")
			}
			if tt.caps.MaxContextWindow <= 0 {
				t.Error("MaxContextWindow should be positive")
			}
		})
	}
}

// TestCommandType_Validation tests CommandType constants.
func TestCommandType_Validation(t *testing.T) {
	tests := []struct {
		name     string
		cmdType  CommandType
		expected string
	}{
		{"rename", CommandRename, "rename_session"},
		{"setdir", CommandSetDir, "set_directory"},
		{"runhook", CommandRunHook, "run_hook"},
		{"authorize", CommandAuthorize, "authorize_directory"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if string(tt.cmdType) != tt.expected {
				t.Errorf("got %v, want %v", tt.cmdType, tt.expected)
			}
		})
	}
}

// TestMessage_Serialization tests Message JSON round-trip.
func TestMessage_Serialization(t *testing.T) {
	msg := Message{
		ID:        "test-id",
		Role:      RoleUser,
		Content:   "Hello, world!",
		Timestamp: time.Now().UTC(),
		Metadata: map[string]interface{}{
			"tokens": 10,
			"model":  "test-model",
		},
	}

	// Serialize
	data, err := json.Marshal(msg)
	if err != nil {
		t.Fatalf("failed to marshal: %v", err)
	}

	// Deserialize
	var decoded Message
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	// Verify
	if decoded.ID != msg.ID {
		t.Errorf("ID mismatch: got %v, want %v", decoded.ID, msg.ID)
	}
	if decoded.Role != msg.Role {
		t.Errorf("Role mismatch: got %v, want %v", decoded.Role, msg.Role)
	}
	if decoded.Content != msg.Content {
		t.Errorf("Content mismatch: got %v, want %v", decoded.Content, msg.Content)
	}
}

// TestSessionContext_Validation tests SessionContext struct.
func TestSessionContext_Validation(t *testing.T) {
	tests := []struct {
		name    string
		ctx     SessionContext
		wantErr bool
	}{
		{
			name: "valid context",
			ctx: SessionContext{
				Name:             "test-session",
				WorkingDirectory: "~/project",
				Project:          "ai-tools",
			},
			wantErr: false,
		},
		{
			name: "minimal context",
			ctx: SessionContext{
				Name:             "test",
				WorkingDirectory: "/tmp",
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.ctx.Name == "" {
				t.Error("Name should not be empty")
			}
			if tt.ctx.WorkingDirectory == "" {
				t.Error("WorkingDirectory should not be empty")
			}
		})
	}
}

// TestRole_Constants tests Role enum constants.
func TestRole_Constants(t *testing.T) {
	if RoleUser != "user" {
		t.Errorf("RoleUser should be 'user', got %v", RoleUser)
	}
	if RoleAssistant != "assistant" {
		t.Errorf("RoleAssistant should be 'assistant', got %v", RoleAssistant)
	}
}

// TestStatus_Constants tests Status enum constants.
func TestStatus_Constants(t *testing.T) {
	if StatusActive != "active" {
		t.Errorf("StatusActive should be 'active', got %v", StatusActive)
	}
	if StatusSuspended != "suspended" {
		t.Errorf("StatusSuspended should be 'suspended', got %v", StatusSuspended)
	}
	if StatusTerminated != "terminated" {
		t.Errorf("StatusTerminated should be 'terminated', got %v", StatusTerminated)
	}
}

// TestConversationFormat_Constants tests ConversationFormat enum constants.
func TestConversationFormat_Constants(t *testing.T) {
	if FormatJSONL != "jsonl" {
		t.Errorf("FormatJSONL should be 'jsonl', got %v", FormatJSONL)
	}
	if FormatHTML != "html" {
		t.Errorf("FormatHTML should be 'html', got %v", FormatHTML)
	}
	if FormatMarkdown != "markdown" {
		t.Errorf("FormatMarkdown should be 'markdown', got %v", FormatMarkdown)
	}
}
