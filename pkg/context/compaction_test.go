package context

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/vbonnet/dear-agent/pkg/llm/provider"
)

// mockProvider implements provider.Provider for testing compaction.
type mockProvider struct {
	response string
	err      error
	called   bool
	lastReq  *provider.GenerateRequest
}

func (m *mockProvider) Name() string { return "mock" }

func (m *mockProvider) Generate(_ context.Context, req *provider.GenerateRequest) (*provider.GenerateResponse, error) {
	m.called = true
	m.lastReq = req
	if m.err != nil {
		return nil, m.err
	}
	return &provider.GenerateResponse{
		Text:  m.response,
		Model: req.Model,
	}, nil
}

func (m *mockProvider) Capabilities() provider.Capabilities {
	return provider.Capabilities{}
}

func TestCompactConversation_BelowThreshold(t *testing.T) {
	msgs := []Message{
		makeTextMessage("user", "hello"),
		makeTextMessage("assistant", "hi"),
	}
	mock := &mockProvider{response: "summary"}
	cfg := DefaultCompactionConfig()

	result, err := CompactConversation(context.Background(), msgs, mock, cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Skipped {
		t.Error("expected compaction to be skipped below threshold")
	}
	if mock.called {
		t.Error("LLM should not be called when below threshold")
	}
	if len(result.Messages) != 2 {
		t.Errorf("expected original 2 messages, got %d", len(result.Messages))
	}
}

func TestCompactConversation_AboveThreshold(t *testing.T) {
	bigContent := strings.Repeat("x", 700_000) // ~233K tokens, well above 80% of 200K
	msgs := []Message{
		makeTextMessage("user", bigContent),
		makeTextMessage("assistant", "I see a lot of text"),
		makeTextMessage("user", "summarize it"),
		makeTextMessage("assistant", "done"),
		makeTextMessage("user", "thanks"),
	}
	mock := &mockProvider{response: "This was a conversation about a large text block."}
	cfg := DefaultCompactionConfig()
	cfg.PreserveRecent = 3

	result, err := CompactConversation(context.Background(), msgs, mock, cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Skipped {
		t.Error("expected compaction to trigger above threshold")
	}
	if !mock.called {
		t.Error("LLM should be called for compaction")
	}

	// Should have 1 summary + 3 recent = 4 messages.
	if len(result.Messages) != 4 {
		t.Fatalf("expected 4 messages, got %d", len(result.Messages))
	}

	// First message should be the compact summary.
	firstContent := result.Messages[0].Content[0].Text
	if !strings.Contains(firstContent, "<compact_summary>") {
		t.Error("first message should contain <compact_summary> tag")
	}
	if !strings.Contains(firstContent, "large text block") {
		t.Error("summary content not found in first message")
	}

	// Last 3 messages should be preserved verbatim (indices 2,3,4 of original).
	if result.Messages[1].Content[0].Text != "summarize it" {
		t.Errorf("recent message 1 not preserved, got %q", result.Messages[1].Content[0].Text)
	}
	if result.Messages[3].Content[0].Text != "thanks" {
		t.Error("recent message 3 not preserved")
	}

	// Token reduction should be significant.
	if result.CompactedTokens >= result.OriginalTokens {
		t.Errorf("compacted tokens (%d) should be less than original (%d)",
			result.CompactedTokens, result.OriginalTokens)
	}
}

func TestCompactConversation_LLMError_FailOpen(t *testing.T) {
	bigContent := strings.Repeat("x", 700_000)
	msgs := []Message{
		makeTextMessage("user", bigContent),
		makeTextMessage("assistant", "ok"),
	}
	mock := &mockProvider{err: fmt.Errorf("API rate limit exceeded")}
	cfg := DefaultCompactionConfig()

	result, err := CompactConversation(context.Background(), msgs, mock, cfg)
	if err == nil {
		t.Error("expected error to be returned")
	}
	if !strings.Contains(err.Error(), "rate limit") {
		t.Errorf("error should contain underlying cause: %v", err)
	}
	// Fail-open: original messages returned.
	if result.Skipped != true {
		t.Error("should be marked as skipped on LLM error")
	}
	if len(result.Messages) != 2 {
		t.Errorf("expected original 2 messages on error, got %d", len(result.Messages))
	}
}

func TestCompactConversation_EmptyMessages(t *testing.T) {
	mock := &mockProvider{response: "summary"}
	cfg := DefaultCompactionConfig()

	result, err := CompactConversation(context.Background(), nil, mock, cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Skipped {
		t.Error("empty messages should skip compaction")
	}
}

func TestCompactConversation_PreserveRecentClamped(t *testing.T) {
	bigContent := strings.Repeat("x", 700_000)
	msgs := []Message{
		makeTextMessage("user", bigContent),
		makeTextMessage("assistant", "only two messages"),
	}
	mock := &mockProvider{response: "summary of big content"}
	cfg := DefaultCompactionConfig()
	cfg.PreserveRecent = 10 // More than available messages.

	result, err := CompactConversation(context.Background(), msgs, mock, cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// 1 summary + 2 preserved (clamped from 10 to 2) = 3.
	if len(result.Messages) != 3 {
		t.Errorf("expected 3 messages, got %d", len(result.Messages))
	}
}

func TestCompactConversation_SystemPromptUsed(t *testing.T) {
	bigContent := strings.Repeat("x", 700_000)
	msgs := []Message{
		makeTextMessage("user", bigContent),
	}
	mock := &mockProvider{response: "summary"}
	cfg := DefaultCompactionConfig()
	cfg.PreserveRecent = 0

	_, err := CompactConversation(context.Background(), msgs, mock, cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if mock.lastReq.SystemPrompt != CompactSummarySystemPrompt {
		t.Error("should use CompactSummarySystemPrompt as system prompt")
	}
	if mock.lastReq.Temperature != 0.0 {
		t.Error("should use temperature 0 for deterministic summarization")
	}
}

func TestRenderMessagesForSummary(t *testing.T) {
	msgs := []Message{
		makeTextMessage("user", "find the bug"),
		makeToolResultMessage("t1", "Grep", "match at line 42"),
		makeTextMessage("assistant", "found it"),
	}
	rendered := renderMessagesForSummary(msgs)

	if !strings.Contains(rendered, "[user]") {
		t.Error("should contain role markers")
	}
	if !strings.Contains(rendered, "find the bug") {
		t.Error("should contain user text")
	}
	if !strings.Contains(rendered, "match at line 42") {
		t.Error("should contain tool result")
	}
}

func TestRenderMessagesForSummary_TruncatesLongToolResults(t *testing.T) {
	longResult := strings.Repeat("y", 5000)
	msgs := []Message{
		makeToolResultMessage("t1", "Grep", longResult),
	}
	rendered := renderMessagesForSummary(msgs)

	if !strings.Contains(rendered, "[truncated]") {
		t.Error("should truncate tool results longer than 2000 chars")
	}
}
