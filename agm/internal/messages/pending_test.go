package messages

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWritePendingFileToDir_CreatesFile(t *testing.T) {
	tmpDir := t.TempDir()

	err := WritePendingFileToDir(tmpDir, "test-session", "1234567890-sender-001", "Hello from sender")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify directory was created
	sessionDir := filepath.Join(tmpDir, "test-session")
	info, err := os.Stat(sessionDir)
	if err != nil {
		t.Fatalf("session directory not created: %v", err)
	}
	if !info.IsDir() {
		t.Fatal("expected directory, got file")
	}

	// Verify a .msg file was created
	entries, err := os.ReadDir(sessionDir)
	if err != nil {
		t.Fatalf("failed to read session dir: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 file, got %d", len(entries))
	}

	entry := entries[0]
	if !strings.HasSuffix(entry.Name(), ".msg") {
		t.Errorf("expected .msg suffix, got %s", entry.Name())
	}

	// Verify file content
	content, err := os.ReadFile(filepath.Join(sessionDir, entry.Name()))
	if err != nil {
		t.Fatalf("failed to read file: %v", err)
	}
	if string(content) != "Hello from sender" {
		t.Errorf("unexpected content: %q", string(content))
	}
}

func TestWritePendingFileToDir_EmptySessionName(t *testing.T) {
	tmpDir := t.TempDir()

	err := WritePendingFileToDir(tmpDir, "", "id-001", "msg")
	if err == nil {
		t.Fatal("expected error for empty session name")
	}
	if !strings.Contains(err.Error(), "session name is required") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestWritePendingFileToDir_EmptyMessage(t *testing.T) {
	tmpDir := t.TempDir()

	err := WritePendingFileToDir(tmpDir, "sess", "id-001", "")
	if err == nil {
		t.Fatal("expected error for empty message")
	}
	if !strings.Contains(err.Error(), "message content is required") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestWritePendingFileToDir_MultipleFiles(t *testing.T) {
	tmpDir := t.TempDir()

	// Write two messages to the same session
	err := WritePendingFileToDir(tmpDir, "sess", "id-001", "msg1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	err = WritePendingFileToDir(tmpDir, "sess", "id-002", "msg2")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	entries, err := os.ReadDir(filepath.Join(tmpDir, "sess"))
	if err != nil {
		t.Fatalf("failed to read dir: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("expected 2 files, got %d", len(entries))
	}
}

func TestWritePendingFileToDir_LongMessageID(t *testing.T) {
	tmpDir := t.TempDir()

	// Message ID longer than 20 chars should be truncated in filename
	longID := "1234567890123456789012345-sender-001"
	err := WritePendingFileToDir(tmpDir, "sess", longID, "test")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	entries, err := os.ReadDir(filepath.Join(tmpDir, "sess"))
	if err != nil {
		t.Fatalf("failed to read dir: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 file, got %d", len(entries))
	}

	// Filename should contain truncated ID prefix
	name := entries[0].Name()
	if !strings.Contains(name, "12345678901234567890") {
		t.Errorf("expected truncated ID in filename, got %s", name)
	}
}

func TestWritePendingFileToDir_DirectoryPermissions(t *testing.T) {
	tmpDir := t.TempDir()

	err := WritePendingFileToDir(tmpDir, "secure-session", "id-001", "secret msg")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	info, err := os.Stat(filepath.Join(tmpDir, "secure-session"))
	if err != nil {
		t.Fatalf("failed to stat dir: %v", err)
	}

	// Directory should be owner-only (0700)
	perm := info.Mode().Perm()
	if perm != 0700 {
		t.Errorf("expected directory permissions 0700, got %04o", perm)
	}
}
