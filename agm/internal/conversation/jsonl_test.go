package conversation

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestParseJSONL_Valid(t *testing.T) {
	// Create temp file with valid JSONL
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "test.jsonl")

	content := `{"schema_version":"1.0","created_at":"2026-01-19T18:00:00Z","model":"claude-sonnet-4-5","harness":"claude-code","total_messages":2}
{"timestamp":"2026-01-19T18:00:01Z","role":"user","harness":"claude-code","content":[{"type":"text","text":"Hello"}]}
{"timestamp":"2026-01-19T18:00:02Z","role":"assistant","harness":"claude-code","content":[{"type":"text","text":"Hi!"}]}
`
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}

	// Parse
	conv, err := ParseJSONL(path)
	if err != nil {
		t.Fatalf("ParseJSONL failed: %v", err)
	}

	// Validate header
	if conv.SchemaVersion != "1.0" {
		t.Errorf("expected schema_version 1.0, got %s", conv.SchemaVersion)
	}
	if conv.Model != "claude-sonnet-4-5" {
		t.Errorf("expected model claude-sonnet-4-5, got %s", conv.Model)
	}
	if conv.Harness != "claude-code" {
		t.Errorf("expected harness claude-code, got %s", conv.Harness)
	}

	// Validate messages
	if len(conv.Messages) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(conv.Messages))
	}

	msg1 := conv.Messages[0]
	if msg1.Role != "user" {
		t.Errorf("message 0: expected role user, got %s", msg1.Role)
	}
	if len(msg1.Content) != 1 {
		t.Fatalf("message 0: expected 1 content block, got %d", len(msg1.Content))
	}
	if tb, ok := msg1.Content[0].(TextBlock); ok {
		if tb.Text != "Hello" {
			t.Errorf("message 0: expected text 'Hello', got '%s'", tb.Text)
		}
	} else {
		t.Errorf("message 0: expected TextBlock, got %T", msg1.Content[0])
	}
}

func TestParseJSONL_EmptyFile(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "empty.jsonl")

	if err := os.WriteFile(path, []byte{}, 0600); err != nil {
		t.Fatal(err)
	}

	_, err := ParseJSONL(path)
	if err == nil {
		t.Fatal("expected error for empty file, got nil")
	}
	if err.Error() != "empty file" {
		t.Errorf("expected 'empty file' error, got: %v", err)
	}
}

func TestParseJSONL_InvalidHeader(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "invalid.jsonl")

	content := `not valid json
`
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}

	_, err := ParseJSONL(path)
	if err == nil {
		t.Fatal("expected error for invalid header, got nil")
	}
}

func TestParseJSONL_UnsupportedSchema(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "unsupported.jsonl")

	content := `{"schema_version":"2.0","created_at":"2026-01-19T18:00:00Z","model":"claude","agent":"claude"}
`
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}

	_, err := ParseJSONL(path)
	if err == nil {
		t.Fatal("expected error for unsupported schema, got nil")
	}
}

func TestWriteJSONL_Valid(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "output.jsonl")

	// Create conversation
	conv := &Conversation{
		SchemaVersion: "1.0",
		CreatedAt:     time.Date(2026, 1, 19, 18, 0, 0, 0, time.UTC),
		Model:         "claude-sonnet-4-5",
		Harness:       "claude-code",
		Messages: []Message{
			{
				Timestamp: time.Date(2026, 1, 19, 18, 0, 1, 0, time.UTC),
				Role:      "user",
				Harness:   "claude-code",
				Content: []ContentBlock{
					TextBlock{Type: "text", Text: "Hello"},
				},
			},
		},
	}

	// Write
	if err := WriteJSONL(path, conv); err != nil {
		t.Fatalf("WriteJSONL failed: %v", err)
	}

	// Verify file exists
	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Fatal("output file not created")
	}

	// Verify can parse
	parsed, err := ParseJSONL(path)
	if err != nil {
		t.Fatalf("failed to parse written file: %v", err)
	}

	if parsed.SchemaVersion != conv.SchemaVersion {
		t.Errorf("schema version mismatch: expected %s, got %s", conv.SchemaVersion, parsed.SchemaVersion)
	}
	if len(parsed.Messages) != len(conv.Messages) {
		t.Errorf("message count mismatch: expected %d, got %d", len(conv.Messages), len(parsed.Messages))
	}
}

func TestWriteJSONL_AtomicWrite(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "atomic.jsonl")

	conv := &Conversation{
		SchemaVersion: "1.0",
		CreatedAt:     time.Now(),
		Model:         "test",
		Harness:       "test",
		Messages:      []Message{},
	}

	// Write
	if err := WriteJSONL(path, conv); err != nil {
		t.Fatalf("WriteJSONL failed: %v", err)
	}

	// Verify temp file is cleaned up
	tmpPath := path + ".tmp"
	if _, err := os.Stat(tmpPath); !os.IsNotExist(err) {
		t.Error("temp file not cleaned up after write")
	}

	// Verify output file exists
	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Fatal("output file not created")
	}
}

func TestContentBlockParsing(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "blocks.jsonl")

	// JSONL with multiple content block types
	content := `{"schema_version":"1.0","created_at":"2026-01-19T18:00:00Z","model":"claude","agent":"claude"}
{"timestamp":"2026-01-19T18:00:01Z","role":"user","agent":"claude","content":[{"type":"text","text":"Use tool"},{"type":"tool_use","id":"tool_1","name":"calc","input":{"expr":"2+2"}}]}
{"timestamp":"2026-01-19T18:00:02Z","role":"user","agent":"claude","content":[{"type":"tool_result","tool_use_id":"tool_1","content":"4"}]}
`
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}

	conv, err := ParseJSONL(path)
	if err != nil {
		t.Fatalf("ParseJSONL failed: %v", err)
	}

	// Verify message 1 has text + tool_use blocks
	if len(conv.Messages[0].Content) != 2 {
		t.Fatalf("expected 2 content blocks, got %d", len(conv.Messages[0].Content))
	}

	if _, ok := conv.Messages[0].Content[0].(TextBlock); !ok {
		t.Errorf("block 0: expected TextBlock, got %T", conv.Messages[0].Content[0])
	}

	if tub, ok := conv.Messages[0].Content[1].(ToolUseBlock); ok {
		if tub.ID != "tool_1" {
			t.Errorf("tool_use: expected id 'tool_1', got '%s'", tub.ID)
		}
		if tub.Name != "calc" {
			t.Errorf("tool_use: expected name 'calc', got '%s'", tub.Name)
		}
	} else {
		t.Errorf("block 1: expected ToolUseBlock, got %T", conv.Messages[0].Content[1])
	}

	// Verify message 2 has tool_result block
	if len(conv.Messages[1].Content) != 1 {
		t.Fatalf("expected 1 content block, got %d", len(conv.Messages[1].Content))
	}

	if trb, ok := conv.Messages[1].Content[0].(ToolResultBlock); ok {
		if trb.ToolUseID != "tool_1" {
			t.Errorf("tool_result: expected tool_use_id 'tool_1', got '%s'", trb.ToolUseID)
		}
		if trb.Content != "4" {
			t.Errorf("tool_result: expected content '4', got '%s'", trb.Content)
		}
	} else {
		t.Errorf("expected ToolResultBlock, got %T", conv.Messages[1].Content[0])
	}
}
