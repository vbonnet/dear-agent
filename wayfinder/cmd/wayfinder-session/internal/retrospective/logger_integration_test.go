package retrospective

import (
	"os"
	"path/filepath"
	"testing"
)

// TestLogRewindEvent_FullFlow tests the complete LogRewindEvent orchestration
func TestLogRewindEvent_FullFlow(t *testing.T) {
	tmpDir := t.TempDir()

	// Create WAYFINDER-STATUS.md for ReadFrom to work
	statusContent := `---
session_id: test-session-123
current_phase: S7
phases:
  - name: W0
    status: completed
    started_at: "2024-01-01T10:00:00Z"
    completed_at: "2024-01-01T11:00:00Z"
  - name: D1
    status: completed
    started_at: "2024-01-01T12:00:00Z"
    completed_at: "2024-01-01T13:00:00Z"
  - name: S7
    status: in_progress
    started_at: "2024-01-01T14:00:00Z"
---
`
	statusPath := filepath.Join(tmpDir, "WAYFINDER-STATUS.md")
	if err := os.WriteFile(statusPath, []byte(statusContent), 0644); err != nil {
		t.Fatalf("Failed to write STATUS file: %v", err)
	}

	// Create WAYFINDER-HISTORY.md for history logging
	historyPath := filepath.Join(tmpDir, "WAYFINDER-HISTORY.md")
	if err := os.WriteFile(historyPath, []byte("# History\n\n"), 0644); err != nil {
		t.Fatalf("Failed to write HISTORY file: %v", err)
	}

	// Test with --no-prompt flag (skip prompting)
	flags := RewindFlags{
		NoPrompt:  true,
		Reason:    "Testing full flow",
		Learnings: "Integration test learnings",
	}

	err := LogRewindEvent(tmpDir, "S7", "D1", flags)
	if err != nil {
		t.Errorf("LogRewindEvent failed: %v", err)
	}

	// Verify S11 file was created
	s11Path := filepath.Join(tmpDir, S11Filename)
	s11Content, err := os.ReadFile(s11Path)
	if err != nil {
		t.Fatalf("Failed to read S11 file: %v", err)
	}

	s11Str := string(s11Content)

	// Validate S11 content
	if !contains(s11Str, "## Rewind: S7 → D1") {
		t.Errorf("S11 missing rewind header")
	}
	if !contains(s11Str, "Testing full flow") {
		t.Errorf("S11 missing reason")
	}
	if !contains(s11Str, "Integration test learnings") {
		t.Errorf("S11 missing learnings")
	}

	// Verify HISTORY file was appended
	historyContent, err := os.ReadFile(historyPath)
	if err != nil {
		t.Fatalf("Failed to read HISTORY file: %v", err)
	}

	historyStr := string(historyContent)
	if !contains(historyStr, "rewind.logged") {
		t.Errorf("HISTORY missing rewind.logged event")
	}
}

// TestLogRewindEvent_ErrorHandling tests fail-gracefully behavior
func TestLogRewindEvent_ErrorHandling(t *testing.T) {
	tmpDir := t.TempDir()

	// Don't create STATUS file - should trigger error but not panic
	flags := RewindFlags{NoPrompt: true, Reason: "Test"}

	// Should return nil (fail-gracefully), log warning to stderr
	err := LogRewindEvent(tmpDir, "S7", "S5", flags)
	if err != nil {
		t.Errorf("LogRewindEvent should return nil even on errors (fail-gracefully), got: %v", err)
	}
}

// TestLogRewindEvent_WithPrompting tests prompting flow
func TestLogRewindEvent_WithPrompting(t *testing.T) {
	tmpDir := t.TempDir()

	// Create minimal STATUS
	statusContent := `---
session_id: test-session-456
current_phase: S6
phases:
  - name: W0
    status: completed
  - name: S6
    status: in_progress
---
`
	statusPath := filepath.Join(tmpDir, "WAYFINDER-STATUS.md")
	if err := os.WriteFile(statusPath, []byte(statusContent), 0644); err != nil {
		t.Fatalf("Failed to write STATUS file: %v", err)
	}

	historyPath := filepath.Join(tmpDir, "WAYFINDER-HISTORY.md")
	if err := os.WriteFile(historyPath, []byte("# History\n\n"), 0644); err != nil {
		t.Fatalf("Failed to write HISTORY file: %v", err)
	}

	// Pre-provide reason (bypasses prompting but still logs as "prompted")
	flags := RewindFlags{
		Reason:    "Pre-provided reason",
		Learnings: "",
	}

	err := LogRewindEvent(tmpDir, "S6", "W0", flags)
	if err != nil {
		t.Errorf("LogRewindEvent failed: %v", err)
	}

	// Verify S11
	s11Path := filepath.Join(tmpDir, S11Filename)
	s11Content, err := os.ReadFile(s11Path)
	if err != nil {
		t.Fatalf("Failed to read S11 file: %v", err)
	}

	if !contains(string(s11Content), "Pre-provided reason") {
		t.Errorf("S11 missing pre-provided reason")
	}
}

func contains(haystack, needle string) bool {
	return len(haystack) >= len(needle) && haystack != "" && needle != "" &&
		(haystack == needle || len(haystack) > len(needle) && stringContains(haystack, needle))
}

func stringContains(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
