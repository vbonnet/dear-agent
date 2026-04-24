package conversation

import (
	"fmt"
	"strings"
)

// ValidateConversation validates a Conversation struct against JSONL schema v1.0.
// Returns aggregated errors (all issues found, not just first error).
func ValidateConversation(conv *Conversation) error {
	var errs []string

	// Validate schema version
	if conv.SchemaVersion != "1.0" {
		errs = append(errs, fmt.Sprintf("invalid schema_version: %q (expected \"1.0\")", conv.SchemaVersion))
	}

	// Validate required fields
	if conv.CreatedAt.IsZero() {
		errs = append(errs, "missing or invalid created_at timestamp")
	}

	if conv.Model == "" {
		errs = append(errs, "missing model field")
	}

	if conv.Harness == "" {
		errs = append(errs, "missing harness field")
	}

	// Validate messages
	for i, msg := range conv.Messages {
		if err := validateMessage(&msg, i); err != nil {
			errs = append(errs, err.Error())
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("validation errors:\n  - %s", strings.Join(errs, "\n  - "))
	}

	return nil
}

// validateMessage validates a single Message.
func validateMessage(msg *Message, index int) error {
	var errs []string

	// Validate timestamp
	if msg.Timestamp.IsZero() {
		errs = append(errs, fmt.Sprintf("message %d: missing or invalid timestamp", index))
	}

	// Validate role
	if msg.Role != "user" && msg.Role != "assistant" {
		errs = append(errs, fmt.Sprintf("message %d: invalid role %q (must be \"user\" or \"assistant\")", index, msg.Role))
	}

	// Validate harness
	if msg.Harness == "" {
		errs = append(errs, fmt.Sprintf("message %d: missing harness field", index))
	}

	// Validate content blocks
	if len(msg.Content) == 0 {
		errs = append(errs, fmt.Sprintf("message %d: content array is empty (must have at least one block)", index))
	}

	for j, block := range msg.Content {
		if err := validateContentBlock(block, index, j); err != nil {
			errs = append(errs, err.Error())
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("%s", strings.Join(errs, "; "))
	}

	return nil
}

// validateContentBlock validates a single ContentBlock.
func validateContentBlock(block ContentBlock, msgIndex, blockIndex int) error {
	switch b := block.(type) {
	case TextBlock:
		if b.Type != "text" {
			return fmt.Errorf("message %d, block %d: TextBlock has invalid type %q", msgIndex, blockIndex, b.Type)
		}
		if b.Text == "" {
			return fmt.Errorf("message %d, block %d: TextBlock has empty text field", msgIndex, blockIndex)
		}

	case ImageBlock:
		if b.Type != "image" {
			return fmt.Errorf("message %d, block %d: ImageBlock has invalid type %q", msgIndex, blockIndex, b.Type)
		}
		if b.Source.Type != "base64" && b.Source.Type != "url" {
			return fmt.Errorf("message %d, block %d: ImageBlock source type must be \"base64\" or \"url\"", msgIndex, blockIndex)
		}
		if b.Source.MediaType == "" {
			return fmt.Errorf("message %d, block %d: ImageBlock missing media_type", msgIndex, blockIndex)
		}
		if b.Source.Type == "base64" && b.Source.Data == "" {
			return fmt.Errorf("message %d, block %d: ImageBlock with base64 source missing data", msgIndex, blockIndex)
		}
		if b.Source.Type == "url" && b.Source.URL == "" {
			return fmt.Errorf("message %d, block %d: ImageBlock with url source missing url", msgIndex, blockIndex)
		}

	case ToolUseBlock:
		if b.Type != "tool_use" {
			return fmt.Errorf("message %d, block %d: ToolUseBlock has invalid type %q", msgIndex, blockIndex, b.Type)
		}
		if b.ID == "" {
			return fmt.Errorf("message %d, block %d: ToolUseBlock missing id", msgIndex, blockIndex)
		}
		if b.Name == "" {
			return fmt.Errorf("message %d, block %d: ToolUseBlock missing name", msgIndex, blockIndex)
		}

	case ToolResultBlock:
		if b.Type != "tool_result" {
			return fmt.Errorf("message %d, block %d: ToolResultBlock has invalid type %q", msgIndex, blockIndex, b.Type)
		}
		if b.ToolUseID == "" {
			return fmt.Errorf("message %d, block %d: ToolResultBlock missing tool_use_id", msgIndex, blockIndex)
		}

	default:
		// Unknown block type - warning logged during parsing, not validation error
	}

	return nil
}
