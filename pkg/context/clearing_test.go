package context

import (
	"strings"
	"testing"
)

func makeTextMessage(role, text string) Message {
	return Message{
		Role: role,
		Content: []ContentBlock{
			{Type: "text", Text: text},
		},
	}
}

func makeToolResultMessage(toolUseID, toolName, content string) Message {
	return Message{
		Role: "assistant",
		Content: []ContentBlock{
			{Type: "tool_use", ID: toolUseID, Name: toolName},
			{Type: "tool_result", ToolUseID: toolUseID, Content: content},
		},
	}
}

func TestClearOldToolResults_UnderThreshold(t *testing.T) {
	msgs := []Message{
		makeTextMessage("user", "hello"),
		makeToolResultMessage("t1", "Grep", "some grep output"),
	}
	config := ClearConfig{
		MaxContextTokens: 200_000,
		Threshold:        0.6,
		RecentKeepCount:  5,
	}
	result := ClearOldToolResults(msgs, config)
	tr := findToolResult(result[1])
	if tr != nil && tr.Content == ClearedPlaceholder {
		t.Error("tool result was cleared despite being under threshold")
	}
}

func TestClearOldToolResults_ClearsOldGrepResult(t *testing.T) {
	bigContent := strings.Repeat("x", 600_000) // ~200K tokens, exceeds 60% threshold
	msgs := []Message{
		makeToolResultMessage("t1", "Grep", bigContent),
		makeTextMessage("user", "do something"),
		makeTextMessage("assistant", "ok"),
	}
	config := ClearConfig{
		MaxContextTokens: 200_000,
		Threshold:        0.6,
		RecentKeepCount:  2,
	}
	result := ClearOldToolResults(msgs, config)

	tr := findToolResult(result[0])
	if tr == nil {
		t.Fatal("expected tool result block in message 0")
	}
	if tr.Content != ClearedPlaceholder {
		t.Errorf("old Grep result not cleared, got length %d", len(tr.Content))
	}
}

func TestClearOldToolResults_PreservesUnclearableTools(t *testing.T) {
	bigContent := strings.Repeat("x", 600_000)
	msgs := []Message{
		makeToolResultMessage("t1", "Bash", bigContent),
		makeToolResultMessage("t2", "Read", "file contents"),
		makeToolResultMessage("t3", "WebSearch", "search results"),
		makeTextMessage("user", "continue"),
	}
	config := ClearConfig{
		MaxContextTokens: 200_000,
		Threshold:        0.6,
		RecentKeepCount:  1,
	}
	result := ClearOldToolResults(msgs, config)

	for i, name := range []string{"Bash", "Read", "WebSearch"} {
		tr := findToolResult(result[i])
		if tr != nil && tr.Content == ClearedPlaceholder {
			t.Errorf("%s result was cleared but should be unclearable", name)
		}
	}
}

func TestClearOldToolResults_PreservesRecentMessages(t *testing.T) {
	bigContent := strings.Repeat("x", 600_000)
	msgs := []Message{
		makeToolResultMessage("t1", "Glob", bigContent), // old — should clear
		makeToolResultMessage("t2", "Glob", "recent"),   // recent — should keep
		makeTextMessage("user", "last"),                 // recent — should keep
	}
	config := ClearConfig{
		MaxContextTokens: 200_000,
		Threshold:        0.6,
		RecentKeepCount:  2,
	}
	result := ClearOldToolResults(msgs, config)

	tr0 := findToolResult(result[0])
	if tr0 == nil || tr0.Content != ClearedPlaceholder {
		t.Error("old Glob result should be cleared")
	}

	tr1 := findToolResult(result[1])
	if tr1 != nil && tr1.Content == ClearedPlaceholder {
		t.Error("recent Glob result should not be cleared")
	}
}

func TestClearOldToolResults_PlaceholderText(t *testing.T) {
	if ClearedPlaceholder != "[Tool result cleared — re-run if needed]" {
		t.Errorf("unexpected placeholder text: %q", ClearedPlaceholder)
	}
}

func TestClearOldToolResults_ClearsEditWriteGlob(t *testing.T) {
	bigContent := strings.Repeat("x", 600_000)
	clearableTools := []string{"Glob", "Grep", "Edit", "Write"}
	msgs := []Message{
		makeTextMessage("assistant", bigContent), // bulk to exceed threshold
	}
	for i, name := range clearableTools {
		msgs = append(msgs, makeToolResultMessage("t"+string(rune('a'+i)), name, "output"))
	}
	msgs = append(msgs, makeTextMessage("user", "done"))

	config := ClearConfig{
		MaxContextTokens: 200_000,
		Threshold:        0.6,
		RecentKeepCount:  1,
	}
	result := ClearOldToolResults(msgs, config)

	for i, name := range clearableTools {
		tr := findToolResult(result[i+1])
		if tr == nil {
			t.Errorf("no tool result found for %s", name)
			continue
		}
		if tr.Content != ClearedPlaceholder {
			t.Errorf("%s result was not cleared", name)
		}
	}
}

func findToolResult(m Message) *ContentBlock {
	for i := range m.Content {
		if m.Content[i].Type == "tool_result" {
			return &m.Content[i]
		}
	}
	return nil
}
