package retrospective

import (
	"os"
	"path/filepath"
	"testing"
)

// TestLogRewindEvent_FullFlow tests the complete LogRewindEvent orchestration
func TestLogRewindEvent_FullFlow(t *testing.T) {
	tmpDir := t.TempDir()

	createStatusFile(t, tmpDir, "RETRO")

	// Create WAYFINDER-HISTORY.jsonl for history logging
	historyPath := filepath.Join(tmpDir, "WAYFINDER-HISTORY.jsonl")
	if err := os.WriteFile(historyPath, []byte("# History\n\n"), 0644); err != nil {
		t.Fatalf("Failed to write HISTORY file: %v", err)
	}

	// Test with --no-prompt flag (skip prompting)
	flags := RewindFlags{
		NoPrompt:  true,
		Reason:    "Testing full flow",
		Learnings: "Integration test learnings",
	}

	err := LogRewindEvent(tmpDir, "RETRO", "PROBLEM", flags)
	if err != nil {
		t.Errorf("LogRewindEvent failed: %v", err)
	}

	// Verify RETRO file was created
	s11Path := filepath.Join(tmpDir, RetroFilename)
	s11Content, err := os.ReadFile(s11Path)
	if err != nil {
		t.Fatalf("Failed to read RETRO file: %v", err)
	}

	s11Str := string(s11Content)

	// Validate RETRO content
	if !contains(s11Str, "## Rewind: RETRO → PROBLEM") {
		t.Errorf("RETRO missing rewind header")
	}
	if !contains(s11Str, "Testing full flow") {
		t.Errorf("RETRO missing reason")
	}
	if !contains(s11Str, "Integration test learnings") {
		t.Errorf("RETRO missing learnings")
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

// TestLogRewindEvent_ErrorHandling tests explicit required-persistence failures.
func TestLogRewindEvent_ErrorHandling(t *testing.T) {
	tmpDir := t.TempDir()

	// Don't create STATUS file - should return an error but not panic.
	flags := RewindFlags{NoPrompt: true, Reason: "Test"}

	err := LogRewindEvent(tmpDir, "RETRO", "SETUP", flags)
	if err == nil {
		t.Error("LogRewindEvent should return an explicit missing-status error")
	}
}

// TestLogRewindEvent_WithPrompting tests prompting flow
func TestLogRewindEvent_WithPrompting(t *testing.T) {
	tmpDir := t.TempDir()

	createStatusFile(t, tmpDir, "BUILD")

	historyPath := filepath.Join(tmpDir, "WAYFINDER-HISTORY.jsonl")
	if err := os.WriteFile(historyPath, []byte("# History\n\n"), 0644); err != nil {
		t.Fatalf("Failed to write HISTORY file: %v", err)
	}

	// Pre-provide reason (bypasses prompting but still logs as "prompted")
	flags := RewindFlags{
		Reason:    "Pre-provided reason",
		Learnings: "",
	}

	err := LogRewindEvent(tmpDir, "BUILD", "CHARTER", flags)
	if err != nil {
		t.Errorf("LogRewindEvent failed: %v", err)
	}

	// Verify RETRO
	s11Path := filepath.Join(tmpDir, RetroFilename)
	s11Content, err := os.ReadFile(s11Path)
	if err != nil {
		t.Fatalf("Failed to read RETRO file: %v", err)
	}

	if !contains(string(s11Content), "Pre-provided reason") {
		t.Errorf("RETRO missing pre-provided reason")
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
