package conversation

import (
	"encoding/json"
	"fmt"
	"time"
)

// Conversation represents a complete conversation with metadata and messages.
// JSONL format: first line is Conversation (header), subsequent lines are Messages.
type Conversation struct {
	SchemaVersion string      `json:"schema_version"`           // Must be "1.0"
	CreatedAt     time.Time   `json:"created_at"`               // ISO8601 timestamp
	Model         string      `json:"model"`                    // e.g., "claude-sonnet-4-5"
	Harness       string      `json:"harness"`                  // Primary harness: "claude-code", "gemini-cli", "codex-cli", "opencode-cli"
	TotalMessages int         `json:"total_messages,omitempty"` // Count of messages
	TotalTokens   *TokenUsage `json:"total_tokens,omitempty"`   // Aggregate token usage
	Messages      []Message   `json:"-"`                        // Not serialized in header line
}

// Message represents a single conversation turn.
type Message struct {
	Timestamp time.Time      `json:"timestamp"`       // ISO8601 timestamp
	Role      string         `json:"role"`            // "user" or "assistant"
	Harness   string         `json:"harness"`         // "claude-code", "gemini-cli", "codex-cli", "opencode-cli"
	Content   []ContentBlock `json:"content"`         // Array of content blocks
	Usage     *TokenUsage    `json:"usage,omitempty"` // Token usage for this message
}

// TokenUsage tracks token consumption.
type TokenUsage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
}

// ContentBlock is the interface for all content types.
type ContentBlock interface {
	BlockType() string
}

// TextBlock represents text content.
type TextBlock struct {
	Type string `json:"type"` // "text"
	Text string `json:"text"`
}

func (t TextBlock) BlockType() string { return "text" }

// ImageBlock represents image content.
type ImageBlock struct {
	Type   string `json:"type"` // "image"
	Source struct {
		Type      string `json:"type"`           // "base64" or "url"
		MediaType string `json:"media_type"`     // MIME type (e.g., "image/png")
		Data      string `json:"data,omitempty"` // Base64-encoded data
		URL       string `json:"url,omitempty"`  // Image URL
	} `json:"source"`
}

func (i ImageBlock) BlockType() string { return "image" }

// ToolUseBlock represents a tool invocation.
type ToolUseBlock struct {
	Type  string          `json:"type"`  // "tool_use"
	ID    string          `json:"id"`    // Unique tool use ID
	Name  string          `json:"name"`  // Tool name
	Input json.RawMessage `json:"input"` // Arbitrary JSON input
}

func (t ToolUseBlock) BlockType() string { return "tool_use" }

// ToolResultBlock represents tool execution result.
type ToolResultBlock struct {
	Type      string `json:"type"`        // "tool_result"
	ToolUseID string `json:"tool_use_id"` // References ToolUseBlock.ID
	Content   string `json:"content"`     // Result string
}

func (t ToolResultBlock) BlockType() string { return "tool_result" }

// UnmarshalJSON implements custom JSON deserialization for Message.
// Handles ContentBlock interface by parsing content as json.RawMessage first,
// then discriminating by type field.
func (m *Message) UnmarshalJSON(data []byte) error {
	type Alias Message // Prevent recursion
	aux := &struct {
		Content []json.RawMessage `json:"content"`
		*Alias
	}{
		Alias: (*Alias)(m),
	}

	if err := json.Unmarshal(data, &aux); err != nil {
		return err
	}

	// Parse content blocks
	blocks, err := parseContentBlocks(aux.Content)
	if err != nil {
		return err
	}
	m.Content = blocks

	return nil
}

// parseContentBlocks converts []json.RawMessage to []ContentBlock by
// inspecting the "type" field and unmarshaling to the appropriate struct.
func parseContentBlocks(raw []json.RawMessage) ([]ContentBlock, error) {
	var blocks []ContentBlock

	for i, r := range raw {
		var base struct{ Type string }
		if err := json.Unmarshal(r, &base); err != nil {
			return nil, fmt.Errorf("content block %d: invalid JSON: %w", i, err)
		}

		switch base.Type {
		case "text":
			var tb TextBlock
			if err := json.Unmarshal(r, &tb); err != nil {
				return nil, fmt.Errorf("content block %d (text): %w", i, err)
			}
			blocks = append(blocks, tb)

		case "image":
			var ib ImageBlock
			if err := json.Unmarshal(r, &ib); err != nil {
				return nil, fmt.Errorf("content block %d (image): %w", i, err)
			}
			blocks = append(blocks, ib)

		case "tool_use":
			var tub ToolUseBlock
			if err := json.Unmarshal(r, &tub); err != nil {
				return nil, fmt.Errorf("content block %d (tool_use): %w", i, err)
			}
			blocks = append(blocks, tub)

		case "tool_result":
			var trb ToolResultBlock
			if err := json.Unmarshal(r, &trb); err != nil {
				return nil, fmt.Errorf("content block %d (tool_result): %w", i, err)
			}
			blocks = append(blocks, trb)

		default:
			// Unknown block type - log warning but continue (graceful degradation)
			fmt.Fprintf(nil, "Warning: unknown content block type: %s\n", base.Type)
		}
	}

	return blocks, nil
}
