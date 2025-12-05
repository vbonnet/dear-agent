package claude

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestCaptureLatestUUID(t *testing.T) {
	t.Run("success - recent entry", func(t *testing.T) {
		// Create temp history file with recent entry
		tmpDir := t.TempDir()
		claudeDir := filepath.Join(tmpDir, ".claude")
		err := os.MkdirAll(claudeDir, 0755)
		if err != nil {
			t.Fatalf("failed to create .claude dir: %v", err)
		}

		historyPath := filepath.Join(claudeDir, "history.jsonl")

		// Write entry with current timestamp
		now := float64(time.Now().UnixNano()) / float64(time.Millisecond)
		content := fmt.Sprintf(`{"sessionId":"test-uuid-123","project":"/tmp/test","timestamp":%.0f}`, now)

		err = os.WriteFile(historyPath, []byte(content+"\n"), 0644)
		if err != nil {
			t.Fatalf("failed to create test file: %v", err)
		}

		// Override HOME for test
		t.Setenv("HOME", tmpDir)

		// Capture UUID (minimal timeout since file already exists)
		uuid, err := CaptureLatestUUID(100 * time.Millisecond)

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if uuid != "test-uuid-123" {
			t.Errorf("got UUID %q, want %q", uuid, "test-uuid-123")
		}
	})

	t.Run("failure - no entries", func(t *testing.T) {
		tmpDir := t.TempDir()
		claudeDir := filepath.Join(tmpDir, ".claude")
		err := os.MkdirAll(claudeDir, 0755)
		if err != nil {
			t.Fatalf("failed to create .claude dir: %v", err)
		}

		historyPath := filepath.Join(claudeDir, "history.jsonl")

		// Create empty file
		err = os.WriteFile(historyPath, []byte(""), 0644)
		if err != nil {
			t.Fatalf("failed to create test file: %v", err)
		}

		t.Setenv("HOME", tmpDir)

		_, err = CaptureLatestUUID(100 * time.Millisecond)

		if err == nil {
			t.Fatal("expected error for empty history, got nil")
		}

		if !strings.Contains(err.Error(), "no entries") {
			t.Errorf("error %q does not contain 'no entries'", err.Error())
		}
	})

	t.Run("failure - entry too old", func(t *testing.T) {
		tmpDir := t.TempDir()
		claudeDir := filepath.Join(tmpDir, ".claude")
		err := os.MkdirAll(claudeDir, 0755)
		if err != nil {
			t.Fatalf("failed to create .claude dir: %v", err)
		}

		historyPath := filepath.Join(claudeDir, "history.jsonl")

		// Write entry with old timestamp (10 seconds ago)
		oldTime := time.Now().Add(-10 * time.Second)
		oldTS := float64(oldTime.UnixNano()) / float64(time.Millisecond)
		content := fmt.Sprintf(`{"sessionId":"old-uuid","project":"/tmp/test","timestamp":%.0f}`, oldTS)

		err = os.WriteFile(historyPath, []byte(content+"\n"), 0644)
		if err != nil {
			t.Fatalf("failed to create test file: %v", err)
		}

		t.Setenv("HOME", tmpDir)

		_, err = CaptureLatestUUID(100 * time.Millisecond)

		if err == nil {
			t.Fatal("expected error for old entry, got nil")
		}

		if !strings.Contains(err.Error(), "too old") {
			t.Errorf("error %q does not contain 'too old'", err.Error())
		}
	})
}
