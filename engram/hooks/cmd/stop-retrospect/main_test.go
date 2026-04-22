package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestAnalyzeConversation_Empty(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.jsonl")
	os.WriteFile(path, []byte(""), 0o644)

	findings := analyzeConversation(path)
	if findings.undoneCount != 0 {
		t.Errorf("expected 0 undone, got %d", findings.undoneCount)
	}
	if findings.brokenPromises != 0 {
		t.Errorf("expected 0 broken promises, got %d", findings.brokenPromises)
	}
	if findings.failedTools != 0 {
		t.Errorf("expected 0 failed tools, got %d", findings.failedTools)
	}
}

func TestAnalyzeConversation_WithPromises(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.jsonl")

	// Write a few JSONL lines with promises
	lines := []string{
		`{"type":"assistant","message":{"role":"assistant","content":"I'll fix the bug"}}`,
		`{"type":"assistant","message":{"role":"assistant","content":"Let me update the docs"}}`,
		`{"type":"assistant","message":{"role":"assistant","content":"I will add tests"}}`,
	}
	content := ""
	for _, l := range lines {
		content += l + "\n"
	}
	os.WriteFile(path, []byte(content), 0o644)

	findings := analyzeConversation(path)
	// Late promises (last 30%) should be detected
	if findings.brokenPromises == 0 {
		t.Error("expected some broken promises from late conversation")
	}
}

func TestAnalyzeConversation_ToolErrors(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.jsonl")

	content := `{"type":"tool_error","message":{}}
{"type":"tool_error","message":{}}
{"type":"assistant","message":{"role":"assistant","content":"ok"}}
`
	os.WriteFile(path, []byte(content), 0o644)

	findings := analyzeConversation(path)
	if findings.failedTools != 2 {
		t.Errorf("expected 2 failed tools, got %d", findings.failedTools)
	}
}

func TestExtractText_String(t *testing.T) {
	raw := []byte(`{"role":"assistant","content":"hello world"}`)
	text := extractText(raw)
	if text != "hello world" {
		t.Errorf("expected 'hello world', got %q", text)
	}
}

func TestExtractText_ContentBlocks(t *testing.T) {
	raw := []byte(`{"role":"assistant","content":[{"type":"text","text":"hello"},{"type":"text","text":"world"}]}`)
	text := extractText(raw)
	if text != "hello world" {
		t.Errorf("expected 'hello world', got %q", text)
	}
}

func TestFindConversationFile_NoHome(t *testing.T) {
	// Should return empty gracefully
	result := findConversationFile("/nonexistent", "")
	// This might find files from the actual home dir, so just check it doesn't panic
	_ = result
}
