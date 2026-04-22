package conversation

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestRoundTrip(t *testing.T) {
	tmpDir := t.TempDir()
	path1 := filepath.Join(tmpDir, "test1.jsonl")
	path2 := filepath.Join(tmpDir, "test2.jsonl")

	// Create original conversation
	original := &Conversation{
		SchemaVersion: "1.0",
		CreatedAt:     time.Date(2026, 1, 19, 18, 0, 0, 0, time.UTC),
		Model:         "claude-sonnet-4-5",
		Harness:       "claude-code",
		TotalTokens: &TokenUsage{
			InputTokens:  100,
			OutputTokens: 50,
		},
		Messages: []Message{
			{
				Timestamp: time.Date(2026, 1, 19, 18, 0, 1, 0, time.UTC),
				Role:      "user",
				Harness:   "claude-code",
				Content: []ContentBlock{
					TextBlock{Type: "text", Text: "Hello, world"},
				},
			},
			{
				Timestamp: time.Date(2026, 1, 19, 18, 0, 2, 0, time.UTC),
				Role:      "assistant",
				Harness:   "claude-code",
				Content: []ContentBlock{
					TextBlock{Type: "text", Text: "Hi there!"},
				},
				Usage: &TokenUsage{
					InputTokens:  5,
					OutputTokens: 2,
				},
			},
			{
				Timestamp: time.Date(2026, 1, 19, 18, 0, 10, 0, time.UTC),
				Role:      "user",
				Harness:   "gemini-cli",
				Content: []ContentBlock{
					TextBlock{Type: "text", Text: "Switching to Gemini"},
				},
			},
			{
				Timestamp: time.Date(2026, 1, 19, 18, 0, 15, 0, time.UTC),
				Role:      "user",
				Harness:   "claude-code",
				Content: []ContentBlock{
					TextBlock{Type: "text", Text: "Use calculator"},
					ToolUseBlock{
						Type:  "tool_use",
						ID:    "tool_123",
						Name:  "calculator",
						Input: []byte(`{"expression":"2+2"}`),
					},
				},
			},
			{
				Timestamp: time.Date(2026, 1, 19, 18, 0, 16, 0, time.UTC),
				Role:      "user",
				Harness:   "claude-code",
				Content: []ContentBlock{
					ToolResultBlock{
						Type:      "tool_result",
						ToolUseID: "tool_123",
						Content:   "4",
					},
				},
			},
		},
	}

	// Write original
	if err := WriteJSONL(path1, original); err != nil {
		t.Fatalf("WriteJSONL (first) failed: %v", err)
	}

	// Parse from file1
	parsed, err := ParseJSONL(path1)
	if err != nil {
		t.Fatalf("ParseJSONL failed: %v", err)
	}

	// Write parsed to file2
	if err := WriteJSONL(path2, parsed); err != nil {
		t.Fatalf("WriteJSONL (second) failed: %v", err)
	}

	// Compare file1 and file2 byte-for-byte
	data1, err := os.ReadFile(path1)
	if err != nil {
		t.Fatalf("read file1: %v", err)
	}

	data2, err := os.ReadFile(path2)
	if err != nil {
		t.Fatalf("read file2: %v", err)
	}

	if !bytes.Equal(data1, data2) {
		t.Errorf("Round-trip failed: files not identical\nFile1:\n%s\nFile2:\n%s", string(data1), string(data2))
	}
}

func TestRoundTrip_AllContentBlockTypes(t *testing.T) {
	tmpDir := t.TempDir()
	path1 := filepath.Join(tmpDir, "blocks1.jsonl")
	path2 := filepath.Join(tmpDir, "blocks2.jsonl")

	// Create conversation with all content block types
	conv := &Conversation{
		SchemaVersion: "1.0",
		CreatedAt:     time.Date(2026, 1, 19, 18, 0, 0, 0, time.UTC),
		Model:         "claude",
		Harness:       "claude-code",
		Messages: []Message{
			{
				Timestamp: time.Date(2026, 1, 19, 18, 0, 1, 0, time.UTC),
				Role:      "user",
				Harness:   "claude-code",
				Content: []ContentBlock{
					TextBlock{Type: "text", Text: "Text block"},
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
							Data:      "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mNk+M9QDwADhgGAWjR9awAAAABJRU5ErkJggg==",
						},
					},
					ToolUseBlock{
						Type:  "tool_use",
						ID:    "tool_1",
						Name:  "test_tool",
						Input: []byte(`{"key":"value"}`),
					},
				},
			},
			{
				Timestamp: time.Date(2026, 1, 19, 18, 0, 2, 0, time.UTC),
				Role:      "user",
				Harness:   "claude-code",
				Content: []ContentBlock{
					ToolResultBlock{
						Type:      "tool_result",
						ToolUseID: "tool_1",
						Content:   "result",
					},
				},
			},
		},
	}

	// Write + Parse + Write
	if err := WriteJSONL(path1, conv); err != nil {
		t.Fatalf("WriteJSONL (first) failed: %v", err)
	}

	parsed, err := ParseJSONL(path1)
	if err != nil {
		t.Fatalf("ParseJSONL failed: %v", err)
	}

	if err := WriteJSONL(path2, parsed); err != nil {
		t.Fatalf("WriteJSONL (second) failed: %v", err)
	}

	// Compare files
	data1, _ := os.ReadFile(path1)
	data2, _ := os.ReadFile(path2)

	if !bytes.Equal(data1, data2) {
		t.Error("Round-trip failed for all content block types")
	}
}
