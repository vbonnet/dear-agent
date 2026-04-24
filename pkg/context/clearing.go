package context

import "encoding/json"

// ClearedPlaceholder is the text that replaces cleared tool results.
const ClearedPlaceholder = "[Tool result cleared — re-run if needed]"

// DefaultUnclearableTools are tool names whose results are too valuable to clear.
var DefaultUnclearableTools = map[string]bool{
	"Bash":      true,
	"FileRead":  true,
	"Read":      true,
	"WebSearch": true,
}

// ContentBlock represents a block of content within a message.
type ContentBlock struct {
	Type      string          `json:"type"`                  // "text", "tool_use", "tool_result", "image"
	Text      string          `json:"text,omitempty"`        // For text blocks
	ID        string          `json:"id,omitempty"`          // For tool_use blocks
	Name      string          `json:"name,omitempty"`        // For tool_use blocks (tool name)
	Input     json.RawMessage `json:"input,omitempty"`       // For tool_use blocks
	ToolUseID string          `json:"tool_use_id,omitempty"` // For tool_result blocks
	Content   string          `json:"content,omitempty"`     // For tool_result blocks
}

// Message represents a conversation message with role and content blocks.
type Message struct {
	Role    string         `json:"role"`
	Content []ContentBlock `json:"content"`
}

// ClearConfig controls tool result clearing behavior.
type ClearConfig struct {
	// MaxContextTokens is the context window size (e.g., 200_000).
	MaxContextTokens int

	// Threshold is the fraction of MaxContextTokens at which clearing triggers (e.g., 0.6).
	Threshold float64

	// RecentKeepCount is the number of most-recent messages whose tool results are preserved.
	RecentKeepCount int

	// UnclearableTools maps tool names that should never be cleared to true.
	// If nil, DefaultUnclearableTools is used.
	UnclearableTools map[string]bool
}

// DefaultClearConfig returns a ClearConfig with sensible defaults.
func DefaultClearConfig() ClearConfig {
	return ClearConfig{
		MaxContextTokens: 200_000,
		Threshold:        0.6,
		RecentKeepCount:  10,
		UnclearableTools: nil, // uses DefaultUnclearableTools
	}
}

// ClearOldToolResults replaces old, clearable tool results with placeholder text.
// It only acts when estimated token usage exceeds threshold * MaxContextTokens.
// Recent messages (last RecentKeepCount) are always preserved.
// Returns a new slice; input messages are not mutated.
func ClearOldToolResults(messages []Message, config ClearConfig) []Message {
	total := 0
	for _, m := range messages {
		total += estimateMessageTokens(m)
	}

	triggerAt := int(float64(config.MaxContextTokens) * config.Threshold)
	if total < triggerAt {
		return messages
	}

	unclearable := config.UnclearableTools
	if unclearable == nil {
		unclearable = DefaultUnclearableTools
	}

	boundary := len(messages) - config.RecentKeepCount
	if boundary < 0 {
		boundary = 0
	}

	result := make([]Message, len(messages))
	for i, m := range messages {
		if i >= boundary {
			result[i] = m
			continue
		}
		result[i] = clearMessageToolResults(m, unclearable)
	}

	return result
}

// clearMessageToolResults returns a copy of the message with clearable tool results replaced.
func clearMessageToolResults(m Message, unclearable map[string]bool) Message {
	if len(m.Content) == 0 {
		return m
	}

	// Build a map of tool_use IDs to tool names from this message.
	toolNames := make(map[string]string)
	for _, block := range m.Content {
		if block.Type == "tool_use" {
			toolNames[block.ID] = block.Name
		}
	}

	hasClearable := false
	for _, block := range m.Content {
		if block.Type == "tool_result" {
			if !unclearable[toolNames[block.ToolUseID]] {
				hasClearable = true
				break
			}
		}
	}
	if !hasClearable {
		return m
	}

	newContent := make([]ContentBlock, len(m.Content))
	copy(newContent, m.Content)
	for i := range newContent {
		if newContent[i].Type != "tool_result" {
			continue
		}
		if unclearable[toolNames[newContent[i].ToolUseID]] {
			continue
		}
		newContent[i].Content = ClearedPlaceholder
	}

	return Message{Role: m.Role, Content: newContent}
}

// estimateMessageTokens estimates the token count for a single message.
func estimateMessageTokens(m Message) int {
	total := 0
	for _, block := range m.Content {
		switch block.Type {
		case "text":
			total += EstimateTokens(block.Text)
		case "tool_use":
			total += EstimateTokens(string(block.Input)) + EstimateTokens(block.Name)
		case "tool_result":
			total += EstimateTokens(block.Content)
		case "image":
			total += EstimateImageTokens()
		}
	}
	return total
}
