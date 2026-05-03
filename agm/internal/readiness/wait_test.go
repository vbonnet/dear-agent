package readiness

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestWaitForReady_Success(t *testing.T) {
	// Create temporary directory for test
	tmpDir := t.TempDir()
	sessionName := "test-session-success"

	// Override home directory for test
	t.Setenv("HOME", tmpDir)

	// Create ready-file in background after 100ms
	go func() {
		time.Sleep(100 * time.Millisecond)
		payload := ReadyFilePayload{
			Status:      "ready",
			ReadyAt:     time.Now().Format(time.RFC3339),
			SessionName: sessionName,
		}
		data, _ := json.Marshal(payload)
		os.MkdirAll(filepath.Join(tmpDir, ".agm"), 0700)
		os.WriteFile(filepath.Join(tmpDir, ".agm", "ready-"+sessionName), data, 0600)
	}()

	// Wait for ready file
	err := WaitForReady(sessionName, 2*time.Second)
	if err != nil {
		t.Fatalf("Expected success, got error: %v", err)
	}

	// Verify ready-file was cleaned up
	if _, err := os.Stat(filepath.Join(tmpDir, ".agm", "ready-"+sessionName)); !os.IsNotExist(err) {
		t.Error("Expected ready-file to be cleaned up after detection")
	}
}

func TestWaitForReady_Timeout(t *testing.T) {
	// Create temporary directory for test
	tmpDir := t.TempDir()
	sessionName := "test-session-timeout"

	// Override home directory for test
	t.Setenv("HOME", tmpDir)

	// Wait for ready file with short timeout (no file will be created)
	err := WaitForReady(sessionName, 500*time.Millisecond)
	if err == nil {
		t.Fatal("Expected timeout error, got nil")
	}

	// Verify error message
	if err.Error() != "timeout waiting for ready-file" {
		t.Errorf("Expected timeout error message, got: %v", err)
	}
}

func TestWaitForReady_RaceCondition(t *testing.T) {
	// Create temporary directory for test
	tmpDir := t.TempDir()
	sessionName := "test-session-race"

	// Override home directory for test
	t.Setenv("HOME", tmpDir)

	// Create ready-file BEFORE calling WaitForReady (race condition simulation)
	os.MkdirAll(filepath.Join(tmpDir, ".agm"), 0700)
	payload := ReadyFilePayload{
		Status:      "ready",
		ReadyAt:     time.Now().Format(time.RFC3339),
		SessionName: sessionName,
	}
	data, _ := json.Marshal(payload)
	os.WriteFile(filepath.Join(tmpDir, ".agm", "ready-"+sessionName), data, 0600)

	// Wait for ready file (should detect immediately via pre-check)
	start := time.Now()
	err := WaitForReady(sessionName, 2*time.Second)
	elapsed := time.Since(start)

	if err != nil {
		t.Fatalf("Expected success, got error: %v", err)
	}

	// Should return immediately (<100ms) not wait for timeout
	if elapsed > 500*time.Millisecond {
		t.Errorf("Expected immediate return, took %v", elapsed)
	}
}

func TestWaitForReady_MalformedJSON(t *testing.T) {
	// Create temporary directory for test
	tmpDir := t.TempDir()
	sessionName := "test-session-malformed"

	// Override home directory for test
	t.Setenv("HOME", tmpDir)

	// Create malformed JSON ready-file, then valid one
	go func() {
		time.Sleep(100 * time.Millisecond)
		os.MkdirAll(filepath.Join(tmpDir, ".agm"), 0700)

		// First, create malformed JSON (should be ignored)
		os.WriteFile(filepath.Join(tmpDir, ".agm", "ready-"+sessionName), []byte("invalid json{"), 0600)

		// Wait a bit, then create valid JSON
		time.Sleep(200 * time.Millisecond)
		os.Remove(filepath.Join(tmpDir, ".agm", "ready-"+sessionName))

		payload := ReadyFilePayload{
			Status:      "ready",
			ReadyAt:     time.Now().Format(time.RFC3339),
			SessionName: sessionName,
		}
		data, _ := json.Marshal(payload)
		os.WriteFile(filepath.Join(tmpDir, ".agm", "ready-"+sessionName), data, 0600)
	}()

	// Wait for ready file (should skip malformed, wait for valid)
	err := WaitForReady(sessionName, 2*time.Second)
	if err != nil {
		t.Fatalf("Expected success after malformed JSON ignored, got error: %v", err)
	}
}

func TestWaitForReady_CrashedStatus(t *testing.T) {
	// Create temporary directory for test
	tmpDir := t.TempDir()
	sessionName := "test-session-crashed"

	// Override home directory for test
	t.Setenv("HOME", tmpDir)

	// Create ready-file with "crashed" status
	go func() {
		time.Sleep(100 * time.Millisecond)
		os.MkdirAll(filepath.Join(tmpDir, ".agm"), 0700)

		payload := ReadyFilePayload{
			Status:      "crashed",
			CrashedAt:   time.Now().Format(time.RFC3339),
			SessionName: sessionName,
			Error:       "test crash error",
		}
		data, _ := json.Marshal(payload)
		os.WriteFile(filepath.Join(tmpDir, ".agm", "ready-"+sessionName), data, 0600)
	}()

	// Wait for ready file (should return error for crashed status)
	err := WaitForReady(sessionName, 2*time.Second)
	if err == nil {
		t.Fatal("Expected error for crashed status, got nil")
	}

	if err.Error() != "Claude crashed during startup" {
		t.Errorf("Expected 'Claude crashed during startup' error, got: %v", err)
	}
}

func TestParseReadyFile_Valid(t *testing.T) {
	// Create temporary file with valid JSON
	tmpDir := t.TempDir()
	readyFile := filepath.Join(tmpDir, "ready-test")

	payload := ReadyFilePayload{
		Status:      "ready",
		ReadyAt:     time.Now().Format(time.RFC3339),
		SessionName: "test",
	}
	data, _ := json.Marshal(payload)
	os.WriteFile(readyFile, data, 0600)

	// Parse file
	status, err := parseReadyFile(readyFile)
	if err != nil {
		t.Fatalf("Expected success, got error: %v", err)
	}

	if status != "ready" {
		t.Errorf("Expected status 'ready', got '%s'", status)
	}
}

func TestParseReadyFile_Malformed(t *testing.T) {
	// Create temporary file with malformed JSON
	tmpDir := t.TempDir()
	readyFile := filepath.Join(tmpDir, "ready-test")

	os.WriteFile(readyFile, []byte("invalid json{"), 0600)

	// Parse file (should fail)
	status, err := parseReadyFile(readyFile)
	if err == nil {
		t.Fatal("Expected error for malformed JSON, got nil")
	}

	if status != "" {
		t.Errorf("Expected empty status on error, got '%s'", status)
	}

	// Verify error message contains "invalid JSON"
	if !strings.HasPrefix(err.Error(), "invalid JSON in ready-file") {
		t.Errorf("Expected 'invalid JSON' error, got: %v", err)
	}
}

func TestParseReadyFile_MissingStatus(t *testing.T) {
	// Create temporary file with JSON but missing status field
	tmpDir := t.TempDir()
	readyFile := filepath.Join(tmpDir, "ready-test")

	// JSON without status field
	os.WriteFile(readyFile, []byte(`{"session_name": "test"}`), 0600)

	// Parse file (should fail)
	status, err := parseReadyFile(readyFile)
	if err == nil {
		t.Fatal("Expected error for missing status field, got nil")
	}

	if status != "" {
		t.Errorf("Expected empty status on error, got '%s'", status)
	}

	// Verify error message
	if err.Error() != "missing status field in ready-file" {
		t.Errorf("Expected 'missing status field' error, got: %v", err)
	}
}

func TestCleanupStaleReadyFiles(t *testing.T) {
	// Create temporary directory
	tmpDir := t.TempDir()
	agmDir := filepath.Join(tmpDir, ".agm")
	os.MkdirAll(agmDir, 0700)

	// Create stale ready-file (11 minutes old)
	staleFile := filepath.Join(agmDir, "ready-stale-session")
	os.WriteFile(staleFile, []byte("{}"), 0600)
	oldTime := time.Now().Add(-11 * time.Minute)
	os.Chtimes(staleFile, oldTime, oldTime)

	// Run cleanup
	err := cleanupStaleReadyFiles(agmDir)
	if err != nil {
		t.Fatalf("Expected success, got error: %v", err)
	}

	// Verify stale file was removed
	if _, err := os.Stat(staleFile); !os.IsNotExist(err) {
		t.Error("Expected stale ready-file to be removed")
	}
}

func TestCleanupStaleReadyFiles_PreservesRecent(t *testing.T) {
	// Create temporary directory
	tmpDir := t.TempDir()
	agmDir := filepath.Join(tmpDir, ".agm")
	os.MkdirAll(agmDir, 0700)

	// Create recent ready-file (5 minutes old)
	recentFile := filepath.Join(agmDir, "ready-recent-session")
	os.WriteFile(recentFile, []byte("{}"), 0600)
	recentTime := time.Now().Add(-5 * time.Minute)
	os.Chtimes(recentFile, recentTime, recentTime)

	// Run cleanup
	err := cleanupStaleReadyFiles(agmDir)
	if err != nil {
		t.Fatalf("Expected success, got error: %v", err)
	}

	// Verify recent file was NOT removed
	if _, err := os.Stat(recentFile); os.IsNotExist(err) {
		t.Error("Expected recent ready-file to be preserved")
	}
}

func TestCleanupStaleReadyFiles_IgnoresNonReadyFiles(t *testing.T) {
	// Create temporary directory
	tmpDir := t.TempDir()
	agmDir := filepath.Join(tmpDir, ".agm")
	os.MkdirAll(agmDir, 0700)

	// Create stale file that doesn't match "ready-*" pattern
	otherFile := filepath.Join(agmDir, "other-file.txt")
	os.WriteFile(otherFile, []byte("{}"), 0600)
	oldTime := time.Now().Add(-11 * time.Minute)
	os.Chtimes(otherFile, oldTime, oldTime)

	// Run cleanup
	err := cleanupStaleReadyFiles(agmDir)
	if err != nil {
		t.Fatalf("Expected success, got error: %v", err)
	}

	// Verify non-ready file was NOT removed (glob pattern "ready-*" doesn't match)
	if _, err := os.Stat(otherFile); os.IsNotExist(err) {
		t.Error("Expected non-ready file to be preserved")
	}
}
