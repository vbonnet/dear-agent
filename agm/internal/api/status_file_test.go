package api

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/vbonnet/dear-agent/agm/internal/state"
)

func TestStatusFileWriter_WriteAndRead(t *testing.T) {
	tempDir := t.TempDir()
	writer, err := NewStatusFileWriter(tempDir)
	if err != nil {
		t.Fatalf("Failed to create writer: %v", err)
	}

	// Write status
	result := state.DetectionResult{
		State:      state.StateReady,
		Timestamp:  time.Now(),
		Evidence:   "Claude prompt detected",
		Confidence: "high",
	}

	if err := writer.WriteStatus("test-session", result); err != nil {
		t.Fatalf("Failed to write status: %v", err)
	}

	// Read status
	status, err := writer.ReadStatus("test-session")
	if err != nil {
		t.Fatalf("Failed to read status: %v", err)
	}

	// Verify
	if status.SessionName != "test-session" {
		t.Errorf("Expected session name 'test-session', got %s", status.SessionName)
	}

	if status.State != state.StateReady {
		t.Errorf("Expected StateReady, got %s", status.State)
	}

	if status.Evidence != "Claude prompt detected" {
		t.Errorf("Expected correct evidence, got %s", status.Evidence)
	}

	if status.Confidence != "high" {
		t.Errorf("Expected high confidence, got %s", status.Confidence)
	}
}

func TestStatusFileWriter_ReadNonexistent(t *testing.T) {
	tempDir := t.TempDir()
	writer, _ := NewStatusFileWriter(tempDir)

	_, err := writer.ReadStatus("nonexistent-session")
	if err == nil {
		t.Error("Expected error for nonexistent session, got nil")
	}
}

func TestStatusFileWriter_DeleteStatus(t *testing.T) {
	tempDir := t.TempDir()
	writer, _ := NewStatusFileWriter(tempDir)

	// Write a status
	result := state.DetectionResult{
		State:      state.StateThinking,
		Timestamp:  time.Now(),
		Evidence:   "Spinner detected",
		Confidence: "high",
	}

	writer.WriteStatus("delete-me", result)

	// Verify it exists
	filePath := writer.getStatusFilePath("delete-me")
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		t.Fatal("Status file should exist before deletion")
	}

	// Delete
	if err := writer.DeleteStatus("delete-me"); err != nil {
		t.Fatalf("Failed to delete status: %v", err)
	}

	// Verify it's gone
	if _, err := os.Stat(filePath); !os.IsNotExist(err) {
		t.Error("Status file should not exist after deletion")
	}
}

func TestStatusFileWriter_ListSessions(t *testing.T) {
	tempDir := t.TempDir()
	writer, _ := NewStatusFileWriter(tempDir)

	// Write multiple statuses
	sessions := []string{"session-1", "session-2", "session-3"}
	result := state.DetectionResult{
		State:      state.StateReady,
		Timestamp:  time.Now(),
		Evidence:   "Test",
		Confidence: "high",
	}

	for _, session := range sessions {
		writer.WriteStatus(session, result)
	}

	// List sessions
	found, err := writer.ListSessions()
	if err != nil {
		t.Fatalf("Failed to list sessions: %v", err)
	}

	if len(found) != len(sessions) {
		t.Errorf("Expected %d sessions, found %d", len(sessions), len(found))
	}

	// Verify all sessions found
	sessionMap := make(map[string]bool)
	for _, s := range found {
		sessionMap[s] = true
	}

	for _, expected := range sessions {
		if !sessionMap[expected] {
			t.Errorf("Expected session %s not found in list", expected)
		}
	}
}

func TestStatusFileWriter_AtomicWrite(t *testing.T) {
	tempDir := t.TempDir()
	writer, _ := NewStatusFileWriter(tempDir)

	// Write first version
	result1 := state.DetectionResult{
		State:      state.StateReady,
		Timestamp:  time.Now(),
		Evidence:   "Version 1",
		Confidence: "high",
	}

	writer.WriteStatus("atomic-test", result1)

	// Overwrite with second version
	result2 := state.DetectionResult{
		State:      state.StateThinking,
		Timestamp:  time.Now(),
		Evidence:   "Version 2",
		Confidence: "high",
	}

	writer.WriteStatus("atomic-test", result2)

	// Read should get version 2
	status, _ := writer.ReadStatus("atomic-test")

	if status.State != state.StateThinking {
		t.Errorf("Expected StateThinking (version 2), got %s", status.State)
	}

	if status.Evidence != "Version 2" {
		t.Errorf("Expected version 2 evidence, got %s", status.Evidence)
	}

	// Temp file should not exist
	tempFile := filepath.Join(tempDir, "atomic-test.json.tmp")
	if _, err := os.Stat(tempFile); !os.IsNotExist(err) {
		t.Error("Temp file should not exist after atomic write")
	}
}

func TestNewStatusFileWriter_CreatesDirectory(t *testing.T) {
	tempDir := t.TempDir()
	statusDir := filepath.Join(tempDir, "nested", "status")

	writer, err := NewStatusFileWriter(statusDir)
	if err != nil {
		t.Fatalf("Failed to create writer with nested dir: %v", err)
	}

	// Verify directory was created
	if _, err := os.Stat(statusDir); os.IsNotExist(err) {
		t.Error("Status directory should have been created")
	}

	if writer.GetBaseDir() != statusDir {
		t.Errorf("Expected base dir %s, got %s", statusDir, writer.GetBaseDir())
	}
}
