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

func TestCaptureLatestUUIDWithRetry(t *testing.T) {
	t.Run("success - first attempt", func(t *testing.T) {
		// UUID exists immediately, no retries needed
		tmpDir := t.TempDir()
		claudeDir := filepath.Join(tmpDir, ".claude")
		err := os.MkdirAll(claudeDir, 0755)
		if err != nil {
			t.Fatalf("failed to create .claude dir: %v", err)
		}

		historyPath := filepath.Join(tmpDir, ".claude", "history.jsonl")

		// Write entry with current timestamp
		now := float64(time.Now().UnixNano()) / float64(time.Millisecond)
		content := fmt.Sprintf(`{"sessionId":"test-uuid-first","project":"/tmp/test","timestamp":%.0f}`, now)

		err = os.WriteFile(historyPath, []byte(content+"\n"), 0644)
		if err != nil {
			t.Fatalf("failed to create test file: %v", err)
		}

		t.Setenv("HOME", tmpDir)

		// Call retry function
		uuid, err := CaptureLatestUUIDWithRetry(5, 10*time.Millisecond)

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if uuid != "test-uuid-first" {
			t.Errorf("got UUID %q, want %q", uuid, "test-uuid-first")
		}
	})

	t.Run("success - after retries", func(t *testing.T) {
		// UUID appears after a delay, retries succeed
		tmpDir := t.TempDir()
		claudeDir := filepath.Join(tmpDir, ".claude")
		err := os.MkdirAll(claudeDir, 0755)
		if err != nil {
			t.Fatalf("failed to create .claude dir: %v", err)
		}

		historyPath := filepath.Join(tmpDir, ".claude", "history.jsonl")

		// Initially create empty file
		err = os.WriteFile(historyPath, []byte(""), 0644)
		if err != nil {
			t.Fatalf("failed to create test file: %v", err)
		}

		t.Setenv("HOME", tmpDir)

		// Simulate delayed UUID write in background
		go func() {
			time.Sleep(50 * time.Millisecond)
			now := float64(time.Now().UnixNano()) / float64(time.Millisecond)
			content := fmt.Sprintf(`{"sessionId":"test-uuid-delayed","project":"/tmp/test","timestamp":%.0f}`, now)
			_ = os.WriteFile(historyPath, []byte(content+"\n"), 0644)
		}()

		// Call retry function (should succeed after retries)
		uuid, err := CaptureLatestUUIDWithRetry(10, 20*time.Millisecond)

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if uuid != "test-uuid-delayed" {
			t.Errorf("got UUID %q, want %q", uuid, "test-uuid-delayed")
		}
	})

	t.Run("failure - max retries exceeded", func(t *testing.T) {
		// UUID never appears, all retries exhausted
		tmpDir := t.TempDir()
		claudeDir := filepath.Join(tmpDir, ".claude")
		err := os.MkdirAll(claudeDir, 0755)
		if err != nil {
			t.Fatalf("failed to create .claude dir: %v", err)
		}

		historyPath := filepath.Join(tmpDir, ".claude", "history.jsonl")

		// Create empty file (UUID never written)
		err = os.WriteFile(historyPath, []byte(""), 0644)
		if err != nil {
			t.Fatalf("failed to create test file: %v", err)
		}

		t.Setenv("HOME", tmpDir)

		// Call retry function with short retry config
		_, err = CaptureLatestUUIDWithRetry(3, 10*time.Millisecond)

		if err == nil {
			t.Fatal("expected error for exhausted retries, got nil")
		}

		if !strings.Contains(err.Error(), "UUID not found after") {
			t.Errorf("error %q does not contain 'UUID not found after'", err.Error())
		}

		if !strings.Contains(err.Error(), "3 retries") {
			t.Errorf("error %q does not contain '3 retries'", err.Error())
		}
	})

	t.Run("failure - entry too old", func(t *testing.T) {
		// History has old entry, should keep retrying and fail
		tmpDir := t.TempDir()
		claudeDir := filepath.Join(tmpDir, ".claude")
		err := os.MkdirAll(claudeDir, 0755)
		if err != nil {
			t.Fatalf("failed to create .claude dir: %v", err)
		}

		historyPath := filepath.Join(tmpDir, ".claude", "history.jsonl")

		// Write entry with old timestamp (10 seconds ago)
		oldTime := time.Now().Add(-10 * time.Second)
		oldTS := float64(oldTime.UnixNano()) / float64(time.Millisecond)
		content := fmt.Sprintf(`{"sessionId":"old-uuid","project":"/tmp/test","timestamp":%.0f}`, oldTS)

		err = os.WriteFile(historyPath, []byte(content+"\n"), 0644)
		if err != nil {
			t.Fatalf("failed to create test file: %v", err)
		}

		t.Setenv("HOME", tmpDir)

		// Call retry function (should reject old entry and retry, then timeout)
		_, err = CaptureLatestUUIDWithRetry(3, 10*time.Millisecond)

		if err == nil {
			t.Fatal("expected error for old entry, got nil")
		}

		// Should fail with retry exhaustion, not "too old" (that's internal logic)
		if !strings.Contains(err.Error(), "UUID not found after") {
			t.Errorf("error %q does not contain 'UUID not found after'", err.Error())
		}
	})

	t.Run("success - rejects old then accepts new", func(t *testing.T) {
		// History has old entry initially, then new entry appears
		tmpDir := t.TempDir()
		claudeDir := filepath.Join(tmpDir, ".claude")
		err := os.MkdirAll(claudeDir, 0755)
		if err != nil {
			t.Fatalf("failed to create .claude dir: %v", err)
		}

		historyPath := filepath.Join(tmpDir, ".claude", "history.jsonl")

		// Write old entry initially
		oldTime := time.Now().Add(-10 * time.Second)
		oldTS := float64(oldTime.UnixNano()) / float64(time.Millisecond)
		content := fmt.Sprintf(`{"sessionId":"old-uuid","project":"/tmp/test","timestamp":%.0f}`, oldTS)

		err = os.WriteFile(historyPath, []byte(content+"\n"), 0644)
		if err != nil {
			t.Fatalf("failed to create test file: %v", err)
		}

		t.Setenv("HOME", tmpDir)

		// Simulate new UUID appearing after delay
		go func() {
			time.Sleep(50 * time.Millisecond)
			now := float64(time.Now().UnixNano()) / float64(time.Millisecond)
			newContent := fmt.Sprintf(`{"sessionId":"new-uuid","project":"/tmp/test","timestamp":%.0f}`, now)
			// Append new entry
			_ = os.WriteFile(historyPath, []byte(content+"\n"+newContent+"\n"), 0644)
		}()

		// Call retry function (should skip old, find new)
		uuid, err := CaptureLatestUUIDWithRetry(10, 20*time.Millisecond)

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if uuid != "new-uuid" {
			t.Errorf("got UUID %q, want %q", uuid, "new-uuid")
		}
	})

	t.Run("failure - missing history file", func(t *testing.T) {
		// history.jsonl doesn't exist
		tmpDir := t.TempDir()
		claudeDir := filepath.Join(tmpDir, ".claude")
		err := os.MkdirAll(claudeDir, 0755)
		if err != nil {
			t.Fatalf("failed to create .claude dir: %v", err)
		}

		t.Setenv("HOME", tmpDir)
		// Note: Not creating history.jsonl

		// Call retry function
		_, err = CaptureLatestUUIDWithRetry(3, 10*time.Millisecond)

		if err == nil {
			t.Fatal("expected error for missing file, got nil")
		}

		// Should fail with retry exhaustion
		if !strings.Contains(err.Error(), "UUID not found after") {
			t.Errorf("error %q does not contain 'UUID not found after'", err.Error())
		}
	})
}
