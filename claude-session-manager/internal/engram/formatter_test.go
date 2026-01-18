package engram

import (
	"strings"
	"testing"
)

func TestFormatSystemMessage_Empty(t *testing.T) {
	results := []EngramResult{}
	msg := FormatSystemMessage(results)

	if msg != "" {
		t.Errorf("Expected empty string for empty results, got %q", msg)
	}
}

func TestFormatSystemMessage_SingleEngram(t *testing.T) {
	results := []EngramResult{
		{
			Hash:    "sha256:abc12345def",
			Title:   "Test Engram",
			Score:   0.95,
			Tags:    []string{"test", "example"},
			Content: "Test content here",
		},
	}

	msg := FormatSystemMessage(results)

	if !strings.Contains(msg, "<system>") {
		t.Errorf("Expected <system> tag in message")
	}
	if !strings.Contains(msg, `<engram id="abc12345"`) {
		t.Errorf("Expected engram tag with ID, got %s", msg)
	}
	if !strings.Contains(msg, `score="0.95"`) {
		t.Errorf("Expected score in engram tag")
	}
	if !strings.Contains(msg, `tags="test,example"`) {
		t.Errorf("Expected tags in engram tag")
	}
	if !strings.Contains(msg, "Test Engram") {
		t.Errorf("Expected title in message")
	}
	if !strings.Contains(msg, "Test content here") {
		t.Errorf("Expected content in message")
	}
	if !strings.Contains(msg, "Note: This context was automatically loaded") {
		t.Errorf("Expected note at end of message")
	}
}

func TestFormatSystemMessage_MultipleEngrams(t *testing.T) {
	results := []EngramResult{
		{Hash: "sha256:abc12345", Title: "First", Score: 0.9, Tags: []string{"tag1"}, Content: "Content 1"},
		{Hash: "sha256:def67890", Title: "Second", Score: 0.8, Tags: []string{"tag2"}, Content: "Content 2"},
	}

	msg := FormatSystemMessage(results)

	if !strings.Contains(msg, "First") {
		t.Errorf("Expected first engram title")
	}
	if !strings.Contains(msg, "Second") {
		t.Errorf("Expected second engram title")
	}
	if !strings.Contains(msg, "Content 1") {
		t.Errorf("Expected first engram content")
	}
	if !strings.Contains(msg, "Content 2") {
		t.Errorf("Expected second engram content")
	}
}

func TestTruncateContent_NoTruncation(t *testing.T) {
	content := "Short content"
	truncated := truncateContent(content, MaxContentLength)

	if truncated != content {
		t.Errorf("Expected no truncation for short content, got %q", truncated)
	}
}

func TestTruncateContent_WithTruncation(t *testing.T) {
	content := strings.Repeat("a", MaxContentLength+100)
	truncated := truncateContent(content, MaxContentLength)

	if len(truncated) > MaxContentLength+20 {
		t.Errorf("Expected truncated content length ≤%d+marker, got %d", MaxContentLength, len(truncated))
	}
	if !strings.Contains(truncated, "... [truncated]") {
		t.Errorf("Expected truncation marker in truncated content")
	}
	if !strings.HasPrefix(truncated, strings.Repeat("a", MaxContentLength)) {
		t.Errorf("Expected truncated content to preserve first %d chars", MaxContentLength)
	}
}

func TestExtractID_ValidHash(t *testing.T) {
	hash := "sha256:abc12345def67890"
	id := extractID(hash)

	if id != "abc12345" {
		t.Errorf("Expected ID=abc12345, got %s", id)
	}
}

func TestExtractID_ShortHash(t *testing.T) {
	hash := "short"
	id := extractID(hash)

	if id != "short" {
		t.Errorf("Expected ID=short for short hash, got %s", id)
	}
}
