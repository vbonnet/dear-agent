package context

import (
	"context"
	"fmt"
	"strings"

	"github.com/vbonnet/dear-agent/pkg/llm/provider"
)

// CompactSummarySystemPrompt instructs the summarizer model to produce a
// compact summary without invoking any tools (NO_TOOLS_PREAMBLE pattern).
const CompactSummarySystemPrompt = `You are a conversation summarizer.
You must NOT invoke any tools. Your only job is to produce a concise
summary of the conversation so far, preserving:
- Key decisions made
- Current task state and progress
- Important file paths and line numbers mentioned
- Any errors or blockers encountered
- The user's most recent request

Output format: a single <compact_summary> block.`

// CompactionConfig controls conversation compaction behavior.
type CompactionConfig struct {
	// Model is the model identifier for summarization (e.g., "claude-haiku-4-5-20251001").
	Model string

	// MaxTokens is the maximum number of tokens for the summary response.
	MaxTokens int

	// PreserveRecent is the number of most-recent messages to keep verbatim.
	PreserveRecent int

	// WindowSize is the total context window size (e.g., 200_000).
	WindowSize int

	// Threshold is the fraction of WindowSize at which compaction triggers (e.g., 0.8).
	Threshold float64
}

// DefaultCompactionConfig returns a CompactionConfig with sensible defaults.
func DefaultCompactionConfig() CompactionConfig {
	return CompactionConfig{
		Model:          "claude-haiku-4-5-20251001",
		MaxTokens:      4096,
		PreserveRecent: 3,
		WindowSize:     200_000,
		Threshold:      0.8,
	}
}

// ConversationCompactResult holds the output of a conversation compaction.
type ConversationCompactResult struct {
	// Messages is the compacted message list (summary + recent messages).
	Messages []Message

	// Summary is the generated compact summary text.
	Summary string

	// OriginalTokens is the estimated token count before compaction.
	OriginalTokens int

	// CompactedTokens is the estimated token count after compaction.
	CompactedTokens int

	// Skipped is true when compaction was not needed (below threshold).
	Skipped bool
}

// CompactConversation summarizes the conversation history using an LLM,
// replacing all messages except the last PreserveRecent with a compact summary.
//
// The summarization model is called with NO_TOOLS_PREAMBLE to prevent tool
// invocation. The output is wrapped in a <compact_summary> block and inserted
// as a synthetic assistant message at the start of the compacted conversation.
//
// If estimated token usage is below threshold, returns the original messages
// unchanged (fail-open).
func CompactConversation(ctx context.Context, messages []Message, llm provider.Provider, cfg CompactionConfig) (*ConversationCompactResult, error) {
	if len(messages) == 0 {
		return &ConversationCompactResult{Skipped: true}, nil
	}

	// Estimate current token usage.
	originalTokens := 0
	for _, m := range messages {
		originalTokens += estimateMessageTokens(m)
	}

	// Check threshold.
	triggerAt := int(float64(cfg.WindowSize) * cfg.Threshold)
	if originalTokens < triggerAt {
		return &ConversationCompactResult{
			Messages:       messages,
			OriginalTokens: originalTokens,
			Skipped:        true,
		}, nil
	}

	// Build the conversation text for the summarizer.
	conversationText := renderMessagesForSummary(messages)

	// Call the LLM with NO_TOOLS_PREAMBLE.
	resp, err := llm.Generate(ctx, &provider.GenerateRequest{
		SystemPrompt: CompactSummarySystemPrompt,
		Prompt:       conversationText,
		Model:        cfg.Model,
		MaxTokens:    cfg.MaxTokens,
		Temperature:  0.0, // Deterministic summarization.
	})
	if err != nil {
		// Fail-open: return original messages on LLM error.
		return &ConversationCompactResult{
			Messages:       messages,
			OriginalTokens: originalTokens,
			Skipped:        true,
		}, fmt.Errorf("compaction LLM call failed (returning original): %w", err)
	}

	summary := resp.Text

	// Build compacted message list: summary + last N messages.
	preserveCount := cfg.PreserveRecent
	if preserveCount > len(messages) {
		preserveCount = len(messages)
	}

	compacted := make([]Message, 0, 1+preserveCount)
	compacted = append(compacted, Message{
		Role: "assistant",
		Content: []ContentBlock{{
			Type: "text",
			Text: "<compact_summary>\n" + summary + "\n</compact_summary>",
		}},
	})
	compacted = append(compacted, messages[len(messages)-preserveCount:]...)

	compactedTokens := 0
	for _, m := range compacted {
		compactedTokens += estimateMessageTokens(m)
	}

	return &ConversationCompactResult{
		Messages:        compacted,
		Summary:         summary,
		OriginalTokens:  originalTokens,
		CompactedTokens: compactedTokens,
	}, nil
}

// renderMessagesForSummary converts messages into a readable text format
// suitable for the summarizer LLM.
func renderMessagesForSummary(messages []Message) string {
	var sb strings.Builder
	for _, m := range messages {
		fmt.Fprintf(&sb, "[%s]\n", m.Role)
		for _, block := range m.Content {
			switch block.Type {
			case "text":
				sb.WriteString(block.Text)
				sb.WriteString("\n")
			case "tool_use":
				fmt.Fprintf(&sb, "<tool_use name=%q id=%q />\n", block.Name, block.ID)
			case "tool_result":
				// Truncate very long tool results for the summarizer.
				content := block.Content
				if len(content) > 2000 {
					content = content[:2000] + "\n... [truncated]"
				}
				fmt.Fprintf(&sb, "<tool_result id=%q>\n%s\n</tool_result>\n", block.ToolUseID, content)
			}
		}
		sb.WriteString("\n")
	}
	return sb.String()
}
