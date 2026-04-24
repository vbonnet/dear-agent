package conversation

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCountMessages_EmptyFile(t *testing.T) {
	// Create temporary empty file
	tmpDir := t.TempDir()
	emptyFile := filepath.Join(tmpDir, "conversation.jsonl")

	if err := os.WriteFile(emptyFile, []byte(""), 0644); err != nil {
		t.Fatalf("failed to create empty file: %v", err)
	}

	count, err := CountMessages(emptyFile)
	if err != nil {
		t.Fatalf("CountMessages failed: %v", err)
	}

	if count != 0 {
		t.Errorf("CountMessages() = %d, want 0", count)
	}
}

func TestCountMessages_SingleMessage(t *testing.T) {
	// Create temporary file with single message
	tmpDir := t.TempDir()
	singleFile := filepath.Join(tmpDir, "conversation.jsonl")

	content := `{"type":"user","content":"Hello world","timestamp":"2024-01-01T00:00:00Z"}
`
	if err := os.WriteFile(singleFile, []byte(content), 0644); err != nil {
		t.Fatalf("failed to create single message file: %v", err)
	}

	count, err := CountMessages(singleFile)
	if err != nil {
		t.Fatalf("CountMessages failed: %v", err)
	}

	if count != 1 {
		t.Errorf("CountMessages() = %d, want 1", count)
	}
}

func TestCountMessages_MultipleMessages(t *testing.T) {
	// Create temporary file with 5+ messages
	tmpDir := t.TempDir()
	multiFile := filepath.Join(tmpDir, "conversation.jsonl")

	content := `{"type":"user","content":"Message 1","timestamp":"2024-01-01T00:00:00Z"}
{"type":"assistant","content":"Response 1","timestamp":"2024-01-01T00:01:00Z"}
{"type":"user","content":"Message 2","timestamp":"2024-01-01T00:02:00Z"}
{"type":"assistant","content":"Response 2","timestamp":"2024-01-01T00:03:00Z"}
{"type":"user","content":"Message 3","timestamp":"2024-01-01T00:04:00Z"}
{"type":"assistant","content":"Response 3","timestamp":"2024-01-01T00:05:00Z"}
{"type":"system_reminder","content":"Token usage: 1000/200000","timestamp":"2024-01-01T00:06:00Z"}
`
	if err := os.WriteFile(multiFile, []byte(content), 0644); err != nil {
		t.Fatalf("failed to create multiple messages file: %v", err)
	}

	count, err := CountMessages(multiFile)
	if err != nil {
		t.Fatalf("CountMessages failed: %v", err)
	}

	if count != 7 {
		t.Errorf("CountMessages() = %d, want 7", count)
	}
}

func TestCountMessages_InvalidJSON(t *testing.T) {
	// Create temporary file with some invalid JSON lines
	tmpDir := t.TempDir()
	invalidFile := filepath.Join(tmpDir, "conversation.jsonl")

	content := `{"type":"user","content":"Valid message 1","timestamp":"2024-01-01T00:00:00Z"}
this is not valid JSON
{"type":"assistant","content":"Valid message 2","timestamp":"2024-01-01T00:01:00Z"}
{broken json
{"type":"user","content":"Valid message 3","timestamp":"2024-01-01T00:02:00Z"}
`
	if err := os.WriteFile(invalidFile, []byte(content), 0644); err != nil {
		t.Fatalf("failed to create invalid JSON file: %v", err)
	}

	count, err := CountMessages(invalidFile)
	if err != nil {
		t.Fatalf("CountMessages failed: %v", err)
	}

	// Should skip invalid lines and count only valid JSON
	if count != 3 {
		t.Errorf("CountMessages() = %d, want 3 (should skip invalid lines)", count)
	}
}

func TestCountMessages_FileNotFound(t *testing.T) {
	// Test with non-existent file
	tmpDir := t.TempDir()
	nonExistentFile := filepath.Join(tmpDir, "does-not-exist.jsonl")

	count, err := CountMessages(nonExistentFile)
	if err != nil {
		t.Fatalf("CountMessages should return nil error for non-existent file, got: %v", err)
	}

	if count != 0 {
		t.Errorf("CountMessages() = %d, want 0 for non-existent file", count)
	}
}

func TestCountMessages_EmptyLines(t *testing.T) {
	// Create temporary file with empty lines
	tmpDir := t.TempDir()
	emptyLinesFile := filepath.Join(tmpDir, "conversation.jsonl")

	content := `{"type":"user","content":"Message 1","timestamp":"2024-01-01T00:00:00Z"}

{"type":"assistant","content":"Message 2","timestamp":"2024-01-01T00:01:00Z"}


{"type":"user","content":"Message 3","timestamp":"2024-01-01T00:02:00Z"}
`
	if err := os.WriteFile(emptyLinesFile, []byte(content), 0644); err != nil {
		t.Fatalf("failed to create file with empty lines: %v", err)
	}

	count, err := CountMessages(emptyLinesFile)
	if err != nil {
		t.Fatalf("CountMessages failed: %v", err)
	}

	// Should skip empty lines and count only valid messages
	if count != 3 {
		t.Errorf("CountMessages() = %d, want 3 (should skip empty lines)", count)
	}
}

func TestIsTrivialSession_BelowThreshold(t *testing.T) {
	// Create a session directory with few messages
	tmpHome := t.TempDir()
	sessionID := "test-session-trivial"
	sessionDir := filepath.Join(tmpHome, ".claude", "sessions", sessionID)

	if err := os.MkdirAll(sessionDir, 0755); err != nil {
		t.Fatalf("failed to create session directory: %v", err)
	}

	conversationPath := filepath.Join(sessionDir, "conversation.jsonl")
	content := `{"type":"user","content":"Message 1","timestamp":"2024-01-01T00:00:00Z"}
{"type":"assistant","content":"Response 1","timestamp":"2024-01-01T00:01:00Z"}
`
	if err := os.WriteFile(conversationPath, []byte(content), 0644); err != nil {
		t.Fatalf("failed to create conversation file: %v", err)
	}

	// Override HOME for this test
	originalHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpHome)
	defer os.Setenv("HOME", originalHome)

	threshold := 5
	isTrivial, err := IsTrivialSession(sessionID, threshold)
	if err != nil {
		t.Fatalf("IsTrivialSession failed: %v", err)
	}

	if !isTrivial {
		t.Errorf("IsTrivialSession() = false, want true (2 messages < threshold of 5)")
	}
}

func TestIsTrivialSession_AboveThreshold(t *testing.T) {
	// Create a session directory with many messages
	tmpHome := t.TempDir()
	sessionID := "test-session-nontrivial"
	sessionDir := filepath.Join(tmpHome, ".claude", "sessions", sessionID)

	if err := os.MkdirAll(sessionDir, 0755); err != nil {
		t.Fatalf("failed to create session directory: %v", err)
	}

	conversationPath := filepath.Join(sessionDir, "conversation.jsonl")
	content := `{"type":"user","content":"Message 1","timestamp":"2024-01-01T00:00:00Z"}
{"type":"assistant","content":"Response 1","timestamp":"2024-01-01T00:01:00Z"}
{"type":"user","content":"Message 2","timestamp":"2024-01-01T00:02:00Z"}
{"type":"assistant","content":"Response 2","timestamp":"2024-01-01T00:03:00Z"}
{"type":"user","content":"Message 3","timestamp":"2024-01-01T00:04:00Z"}
{"type":"assistant","content":"Response 3","timestamp":"2024-01-01T00:05:00Z"}
{"type":"user","content":"Message 4","timestamp":"2024-01-01T00:06:00Z"}
{"type":"assistant","content":"Response 4","timestamp":"2024-01-01T00:07:00Z"}
`
	if err := os.WriteFile(conversationPath, []byte(content), 0644); err != nil {
		t.Fatalf("failed to create conversation file: %v", err)
	}

	// Override HOME for this test
	originalHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpHome)
	defer os.Setenv("HOME", originalHome)

	threshold := 5
	isTrivial, err := IsTrivialSession(sessionID, threshold)
	if err != nil {
		t.Fatalf("IsTrivialSession failed: %v", err)
	}

	if isTrivial {
		t.Errorf("IsTrivialSession() = true, want false (8 messages >= threshold of 5)")
	}
}

func TestIsTrivialSession_ExactThreshold(t *testing.T) {
	// Create a session directory with exactly threshold messages
	tmpHome := t.TempDir()
	sessionID := "test-session-exact"
	sessionDir := filepath.Join(tmpHome, ".claude", "sessions", sessionID)

	if err := os.MkdirAll(sessionDir, 0755); err != nil {
		t.Fatalf("failed to create session directory: %v", err)
	}

	conversationPath := filepath.Join(sessionDir, "conversation.jsonl")
	content := `{"type":"user","content":"Message 1","timestamp":"2024-01-01T00:00:00Z"}
{"type":"assistant","content":"Response 1","timestamp":"2024-01-01T00:01:00Z"}
{"type":"user","content":"Message 2","timestamp":"2024-01-01T00:02:00Z"}
{"type":"assistant","content":"Response 2","timestamp":"2024-01-01T00:03:00Z"}
{"type":"user","content":"Message 3","timestamp":"2024-01-01T00:04:00Z"}
`
	if err := os.WriteFile(conversationPath, []byte(content), 0644); err != nil {
		t.Fatalf("failed to create conversation file: %v", err)
	}

	// Override HOME for this test
	originalHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpHome)
	defer os.Setenv("HOME", originalHome)

	threshold := 5
	isTrivial, err := IsTrivialSession(sessionID, threshold)
	if err != nil {
		t.Fatalf("IsTrivialSession failed: %v", err)
	}

	if isTrivial {
		t.Errorf("IsTrivialSession() = true, want false (5 messages == threshold of 5, not < threshold)")
	}
}

func TestIsTrivialSession_NonExistentSession(t *testing.T) {
	// Test with non-existent session (should be considered trivial)
	tmpHome := t.TempDir()
	sessionID := "non-existent-session"

	// Override HOME for this test
	originalHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpHome)
	defer os.Setenv("HOME", originalHome)

	threshold := 5
	isTrivial, err := IsTrivialSession(sessionID, threshold)
	if err != nil {
		t.Fatalf("IsTrivialSession should not error for non-existent session, got: %v", err)
	}

	if !isTrivial {
		t.Errorf("IsTrivialSession() = false, want true for non-existent session (0 messages)")
	}
}
