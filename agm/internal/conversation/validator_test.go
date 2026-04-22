package conversation

import (
	"strings"
	"testing"
	"time"
)

func TestValidateConversation_Valid(t *testing.T) {
	conv := &Conversation{
		SchemaVersion: "1.0",
		CreatedAt:     time.Now(),
		Model:         "claude",
		Harness:       "claude-code",
		Messages: []Message{
			{
				Timestamp: time.Now(),
				Role:      "user",
				Harness:   "claude-code",
				Content: []ContentBlock{
					TextBlock{Type: "text", Text: "Hello"},
				},
			},
		},
	}

	if err := ValidateConversation(conv); err != nil {
		t.Errorf("ValidateConversation failed for valid conversation: %v", err)
	}
}

func TestValidateConversation_InvalidSchemaVersion(t *testing.T) {
	conv := &Conversation{
		SchemaVersion: "2.0",
		CreatedAt:     time.Now(),
		Model:         "claude",
		Harness:       "claude-code",
		Messages:      []Message{},
	}

	err := ValidateConversation(conv)
	if err == nil {
		t.Fatal("expected validation error for invalid schema version")
	}
	if !strings.Contains(err.Error(), "schema_version") {
		t.Errorf("expected schema_version error, got: %v", err)
	}
}

func TestValidateConversation_MissingFields(t *testing.T) {
	conv := &Conversation{
		SchemaVersion: "1.0",
		// Missing created_at, model, agent
		Messages: []Message{},
	}

	err := ValidateConversation(conv)
	if err == nil {
		t.Fatal("expected validation error for missing fields")
	}
	errMsg := err.Error()
	if !strings.Contains(errMsg, "created_at") {
		t.Error("expected created_at validation error")
	}
	if !strings.Contains(errMsg, "model") {
		t.Error("expected model validation error")
	}
	if !strings.Contains(errMsg, "harness") {
		t.Error("expected harness validation error")
	}
}

func TestValidateConversation_InvalidMessageRole(t *testing.T) {
	conv := &Conversation{
		SchemaVersion: "1.0",
		CreatedAt:     time.Now(),
		Model:         "claude",
		Harness:       "claude-code",
		Messages: []Message{
			{
				Timestamp: time.Now(),
				Role:      "system", // Invalid role
				Harness:   "claude-code",
				Content: []ContentBlock{
					TextBlock{Type: "text", Text: "Hello"},
				},
			},
		},
	}

	err := ValidateConversation(conv)
	if err == nil {
		t.Fatal("expected validation error for invalid role")
	}
	if !strings.Contains(err.Error(), "role") {
		t.Errorf("expected role validation error, got: %v", err)
	}
}

func TestValidateConversation_EmptyContent(t *testing.T) {
	conv := &Conversation{
		SchemaVersion: "1.0",
		CreatedAt:     time.Now(),
		Model:         "claude",
		Harness:       "claude-code",
		Messages: []Message{
			{
				Timestamp: time.Now(),
				Role:      "user",
				Harness:   "claude-code",
				Content:   []ContentBlock{}, // Empty content
			},
		},
	}

	err := ValidateConversation(conv)
	if err == nil {
		t.Fatal("expected validation error for empty content")
	}
	if !strings.Contains(err.Error(), "content array is empty") {
		t.Errorf("expected content validation error, got: %v", err)
	}
}

func TestValidateConversation_InvalidTextBlock(t *testing.T) {
	conv := &Conversation{
		SchemaVersion: "1.0",
		CreatedAt:     time.Now(),
		Model:         "claude",
		Harness:       "claude-code",
		Messages: []Message{
			{
				Timestamp: time.Now(),
				Role:      "user",
				Harness:   "claude-code",
				Content: []ContentBlock{
					TextBlock{Type: "text", Text: ""}, // Empty text
				},
			},
		},
	}

	err := ValidateConversation(conv)
	if err == nil {
		t.Fatal("expected validation error for empty text block")
	}
}

func TestValidateConversation_InvalidToolUseBlock(t *testing.T) {
	conv := &Conversation{
		SchemaVersion: "1.0",
		CreatedAt:     time.Now(),
		Model:         "claude",
		Harness:       "claude-code",
		Messages: []Message{
			{
				Timestamp: time.Now(),
				Role:      "user",
				Harness:   "claude-code",
				Content: []ContentBlock{
					ToolUseBlock{Type: "tool_use", ID: "", Name: "tool"}, // Missing ID
				},
			},
		},
	}

	err := ValidateConversation(conv)
	if err == nil {
		t.Fatal("expected validation error for tool use block missing ID")
	}
}

func TestValidateConversation_AllContentBlockTypes(t *testing.T) {
	conv := &Conversation{
		SchemaVersion: "1.0",
		CreatedAt:     time.Now(),
		Model:         "claude",
		Harness:       "claude-code",
		Messages: []Message{
			{
				Timestamp: time.Now(),
				Role:      "user",
				Harness:   "claude-code",
				Content: []ContentBlock{
					TextBlock{Type: "text", Text: "Text"},
					ImageBlock{
						Type: "image",
						Source: struct {
							Type      string `json:"type"`
							MediaType string `json:"media_type"`
							Data      string `json:"data,omitempty"`
							URL       string `json:"url,omitempty"`
						}{
							Type:      "base64",
							MediaType: "image/png",
							Data:      "data",
						},
					},
					ToolUseBlock{Type: "tool_use", ID: "1", Name: "tool", Input: []byte("{}")},
					ToolResultBlock{Type: "tool_result", ToolUseID: "1", Content: "result"},
				},
			},
		},
	}

	if err := ValidateConversation(conv); err != nil {
		t.Errorf("ValidateConversation failed for all block types: %v", err)
	}
}
