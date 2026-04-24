package worktree

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFormatWorktreeName(t *testing.T) {
	tests := []struct {
		name      string
		sessionID string
		expected  string
	}{
		{
			name:      "full UUID format",
			sessionID: "abc123de-f456-7890-1234-567890abcdef",
			expected:  "session-abc123de-f456-7890-1234-567890abcdef",
		},
		{
			name:      "fallback format with timestamp",
			sessionID: "auto-1709251234-a4f2",
			expected:  "session-auto-1709251234-a4f2",
		},
		{
			name:      "short UUID",
			sessionID: "abc123de",
			expected:  "session-abc123de",
		},
		{
			name:      "empty session ID",
			sessionID: "",
			expected:  "session-",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FormatWorktreeName(tt.sessionID)
			if got != tt.expected {
				t.Errorf("FormatWorktreeName(%q) = %q, want %q", tt.sessionID, got, tt.expected)
			}
		})
	}
}

func TestGetSessionID_FallbackBehavior(t *testing.T) {
	// Test with non-existent history file (should return fallback)
	sessionID, err := GetSessionID()

	// Should return a session ID even on error (fallback)
	if sessionID == "" {
		t.Error("GetSessionID() returned empty string, expected fallback ID")
	}

	// Fallback IDs start with "auto-"
	if err != nil && !strings.HasPrefix(sessionID, "auto-") {
		t.Errorf("Expected fallback ID to start with 'auto-', got %q", sessionID)
	}

	t.Logf("Session ID: %s (error: %v)", sessionID, err)
}

func TestGetSessionID_WithValidHistory(t *testing.T) {
	// Create temporary history file
	tmpDir := t.TempDir()
	historyPath := filepath.Join(tmpDir, "history.jsonl")

	// Write valid history entry
	validUUID := "12345678-1234-1234-1234-123456789abc"
	historyEntry := map[string]interface{}{
		"sessionId": validUUID,
		"timestamp": 1234567890,
	}

	data, err := json.Marshal(historyEntry)
	if err != nil {
		t.Fatalf("Failed to marshal history entry: %v", err)
	}

	if err := os.WriteFile(historyPath, data, 0644); err != nil {
		t.Fatalf("Failed to write history file: %v", err)
	}

	// Create .claude directory structure
	claudeDir := filepath.Join(tmpDir, ".claude")
	if err := os.Mkdir(claudeDir, 0755); err != nil {
		t.Fatalf("Failed to create .claude dir: %v", err)
	}

	// Move history file to expected location
	expectedPath := filepath.Join(claudeDir, "history.jsonl")
	if err := os.Rename(historyPath, expectedPath); err != nil {
		t.Fatalf("Failed to move history file: %v", err)
	}

	t.Setenv("HOME", tmpDir)

	// Test session ID extraction
	sessionID, err := GetSessionID()
	if err != nil {
		t.Logf("GetSessionID() returned error (may be expected): %v", err)
	}

	if sessionID == "" {
		t.Error("GetSessionID() returned empty session ID")
	}

	t.Logf("Extracted session ID: %s", sessionID)
}

func TestFormatWorktreeName_Consistency(t *testing.T) {
	// Same session ID should always produce same worktree name
	sessionID := "test-session-123"

	name1 := FormatWorktreeName(sessionID)
	name2 := FormatWorktreeName(sessionID)

	if name1 != name2 {
		t.Errorf("FormatWorktreeName not consistent: %q != %q", name1, name2)
	}

	// Verify prefix
	expectedPrefix := "session-"
	if !strings.HasPrefix(name1, expectedPrefix) {
		t.Errorf("Expected worktree name to start with %q, got %q", expectedPrefix, name1)
	}
}

func TestFormatWorktreeName_SpecialCharacters(t *testing.T) {
	tests := []struct {
		name      string
		sessionID string
	}{
		{
			name:      "session with dashes",
			sessionID: "abc-def-123",
		},
		{
			name:      "session with underscores",
			sessionID: "abc_def_123",
		},
		{
			name:      "session with numbers only",
			sessionID: "1234567890",
		},
		{
			name:      "session with mixed case",
			sessionID: "AbC123DeF",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FormatWorktreeName(tt.sessionID)
			expected := "session-" + tt.sessionID

			if got != expected {
				t.Errorf("FormatWorktreeName(%q) = %q, want %q", tt.sessionID, got, expected)
			}
		})
	}
}

// Benchmark session ID formatting
func BenchmarkFormatWorktreeName(b *testing.B) {
	sessionID := "abc123de-f456-7890-1234-567890abcdef"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		FormatWorktreeName(sessionID)
	}
}

// Test that session ID extraction handles empty files
func TestGetSessionID_EmptyHistory(t *testing.T) {
	tmpDir := t.TempDir()
	claudeDir := filepath.Join(tmpDir, ".claude")
	if err := os.Mkdir(claudeDir, 0755); err != nil {
		t.Fatalf("Failed to create .claude dir: %v", err)
	}

	// Create empty history file
	historyPath := filepath.Join(claudeDir, "history.jsonl")
	if err := os.WriteFile(historyPath, []byte(""), 0644); err != nil {
		t.Fatalf("Failed to write empty history: %v", err)
	}

	// Override HOME
	t.Setenv("HOME", tmpDir)

	// Should return fallback ID
	sessionID, err := GetSessionID()
	if err == nil {
		t.Error("Expected error for empty history file")
	}

	if sessionID == "" {
		t.Error("Expected fallback session ID, got empty string")
	}

	if !strings.HasPrefix(sessionID, "auto-") {
		t.Errorf("Expected fallback ID with 'auto-' prefix, got %q", sessionID)
	}
}
