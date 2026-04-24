package conversation

import (
	"os"
	"path/filepath"
	"testing"
)

func TestConvertHTMLToJSONL_ValidHTML(t *testing.T) {
	tmpDir := t.TempDir()
	htmlPath := filepath.Join(tmpDir, "test.html")
	jsonlPath := filepath.Join(tmpDir, "test.jsonl")

	// Create simple HTML
	html := `<html>
<body>
	<div class="user-message">Hello</div>
	<div class="assistant-message">Hi there!</div>
</body>
</html>`

	if err := os.WriteFile(htmlPath, []byte(html), 0600); err != nil {
		t.Fatal(err)
	}

	// Convert
	if err := ConvertHTMLToJSONL(htmlPath, jsonlPath); err != nil {
		t.Fatalf("ConvertHTMLToJSONL failed: %v", err)
	}

	// Verify output exists
	if _, err := os.Stat(jsonlPath); os.IsNotExist(err) {
		t.Fatal("JSONL file not created")
	}

	// Parse and verify
	conv, err := ParseJSONL(jsonlPath)
	if err != nil {
		t.Fatalf("failed to parse converted JSONL: %v", err)
	}

	if conv.SchemaVersion != "1.0" {
		t.Errorf("expected schema_version 1.0, got %s", conv.SchemaVersion)
	}

	if len(conv.Messages) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(conv.Messages))
	}

	// Verify first message
	msg1 := conv.Messages[0]
	if msg1.Role != "user" {
		t.Errorf("expected role 'user', got '%s'", msg1.Role)
	}
	if msg1.Harness != "claude-code" {
		t.Errorf("expected harness 'claude-code', got '%s'", msg1.Harness)
	}
	if len(msg1.Content) != 1 {
		t.Fatalf("expected 1 content block, got %d", len(msg1.Content))
	}
	if tb, ok := msg1.Content[0].(TextBlock); ok {
		if tb.Text != "Hello" {
			t.Errorf("expected text 'Hello', got '%s'", tb.Text)
		}
	} else {
		t.Errorf("expected TextBlock, got %T", msg1.Content[0])
	}

	// Verify second message
	msg2 := conv.Messages[1]
	if msg2.Role != "assistant" {
		t.Errorf("expected role 'assistant', got '%s'", msg2.Role)
	}
	if tb, ok := msg2.Content[0].(TextBlock); ok {
		if tb.Text != "Hi there!" {
			t.Errorf("expected text 'Hi there!', got '%s'", tb.Text)
		}
	} else {
		t.Errorf("expected TextBlock, got %T", msg2.Content[0])
	}
}

func TestConvertHTMLToJSONL_EmptyHTML(t *testing.T) {
	tmpDir := t.TempDir()
	htmlPath := filepath.Join(tmpDir, "empty.html")
	jsonlPath := filepath.Join(tmpDir, "empty.jsonl")

	html := `<html><body></body></html>`
	if err := os.WriteFile(htmlPath, []byte(html), 0600); err != nil {
		t.Fatal(err)
	}

	// Convert empty HTML
	if err := ConvertHTMLToJSONL(htmlPath, jsonlPath); err != nil {
		t.Fatalf("ConvertHTMLToJSONL failed: %v", err)
	}

	// Parse
	conv, err := ParseJSONL(jsonlPath)
	if err != nil {
		t.Fatalf("failed to parse JSONL: %v", err)
	}

	if len(conv.Messages) != 0 {
		t.Errorf("expected 0 messages for empty HTML, got %d", len(conv.Messages))
	}
}

func TestConvertHTMLToJSONL_InvalidHTML(t *testing.T) {
	tmpDir := t.TempDir()
	htmlPath := filepath.Join(tmpDir, "invalid.html")
	jsonlPath := filepath.Join(tmpDir, "invalid.jsonl")

	// Create invalid HTML file (not HTML at all)
	if err := os.WriteFile(htmlPath, []byte("not html"), 0600); err != nil {
		t.Fatal(err)
	}

	// Convert should succeed (parses as minimal HTML)
	if err := ConvertHTMLToJSONL(htmlPath, jsonlPath); err != nil {
		t.Fatalf("ConvertHTMLToJSONL failed: %v", err)
	}

	// Output should exist
	if _, err := os.Stat(jsonlPath); os.IsNotExist(err) {
		t.Fatal("output file not created")
	}

	// Should create valid JSONL with header (even if no messages extracted)
	conv, err := ParseJSONL(jsonlPath)
	if err != nil {
		t.Fatalf("failed to parse output JSONL: %v", err)
	}

	// Verify header was created
	if conv.SchemaVersion != "1.0" {
		t.Errorf("expected schema_version 1.0, got %s", conv.SchemaVersion)
	}
}

func TestConvertHTMLToJSONL_FileNotFound(t *testing.T) {
	tmpDir := t.TempDir()
	htmlPath := filepath.Join(tmpDir, "nonexistent.html")
	jsonlPath := filepath.Join(tmpDir, "output.jsonl")

	err := ConvertHTMLToJSONL(htmlPath, jsonlPath)
	if err == nil {
		t.Fatal("expected error for nonexistent file")
	}
}
