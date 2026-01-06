package tokenlogger

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/vbonnet/engram/core/pkg/telemetry"
)

// Test 1: MinLevel() returns LevelError
func TestMinLevel_ReturnsError(t *testing.T) {
	logger := NewTokenLogger()
	if logger.MinLevel() != telemetry.LevelError {
		t.Errorf("Expected MinLevel() = LevelError, got %v", logger.MinLevel())
	}
}

// Test 2: OnEvent with no session returns nil
func TestOnEvent_NoSession_ReturnsNil(t *testing.T) {
	// Override getSessionUUID to return empty string
	oldGetSessionUUID := getSessionUUID
	defer func() { getSessionUUID = oldGetSessionUUID }()
	getSessionUUID = func() string { return "" }

	logger := NewTokenLogger()
	event := &telemetry.Event{
		Type:  "test.event",
		Level: telemetry.LevelError,
		Data:  map[string]interface{}{"key": "value"},
	}

	err := logger.OnEvent(event)
	if err != nil {
		t.Errorf("Expected nil error, got %v", err)
	}
}

// Test 3: OnEvent with session writes JSONL file
func TestOnEvent_WithSession_WritesFile(t *testing.T) {
	// Create temp directory for session
	tmpDir := t.TempDir()
	sessionUUID := "test-session-uuid"
	sessionDir := filepath.Join(tmpDir, sessionUUID)
	if err := os.MkdirAll(sessionDir, 0755); err != nil {
		t.Fatalf("Failed to create session directory: %v", err)
	}

	// Override getSessionUUID and os.UserHomeDir
	oldGetSessionUUID := getSessionUUID
	defer func() { getSessionUUID = oldGetSessionUUID }()
	getSessionUUID = func() string { return sessionUUID }

	// Create logger with custom session dir path
	logger := NewTokenLogger()
	// Manually set session dir for testing
	logger.sessionDir = sessionDir
	logger.cacheChecked = true

	event := &telemetry.Event{
		Timestamp: time.Now(),
		Type:      "test.event",
		Agent:     "test",
		Level:     telemetry.LevelError,
		Data:      map[string]interface{}{"key": "value"},
	}

	err := logger.OnEvent(event)
	if err != nil {
		t.Fatalf("OnEvent failed: %v", err)
	}

	// Verify file exists
	logPath := filepath.Join(sessionDir, "token-usage.jsonl")
	if _, err := os.Stat(logPath); os.IsNotExist(err) {
		t.Fatal("Log file not created")
	}

	// Verify JSONL format
	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("Failed to read log file: %v", err)
	}

	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) != 1 {
		t.Errorf("Expected 1 line, got %d", len(lines))
	}

	// Verify JSON is valid
	var parsedEvent telemetry.Event
	if err := json.Unmarshal([]byte(lines[0]), &parsedEvent); err != nil {
		t.Errorf("Failed to parse JSON: %v", err)
	}

	// Verify event data
	if parsedEvent.Type != "test.event" {
		t.Errorf("Expected type 'test.event', got '%s'", parsedEvent.Type)
	}
}

// Test 4: Concurrent OnEvent calls (no races)
func TestOnEvent_Concurrent_NoRaces(t *testing.T) {
	// Create temp directory
	tmpDir := t.TempDir()
	sessionUUID := "test-concurrent-uuid"
	sessionDir := filepath.Join(tmpDir, sessionUUID)
	if err := os.MkdirAll(sessionDir, 0755); err != nil {
		t.Fatalf("Failed to create session directory: %v", err)
	}

	// Override getSessionUUID
	oldGetSessionUUID := getSessionUUID
	defer func() { getSessionUUID = oldGetSessionUUID }()
	getSessionUUID = func() string { return sessionUUID }

	logger := NewTokenLogger()
	logger.sessionDir = sessionDir
	logger.cacheChecked = true

	// Launch 10 goroutines writing events
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			event := &telemetry.Event{
				Timestamp: time.Now(),
				Type:      "test.concurrent",
				Agent:     "test",
				Level:     telemetry.LevelError,
				Data:      map[string]interface{}{"id": id},
			}
			if err := logger.OnEvent(event); err != nil {
				t.Errorf("OnEvent failed: %v", err)
			}
		}(i)
	}

	wg.Wait()

	// Verify all 10 events written
	logPath := filepath.Join(sessionDir, "token-usage.jsonl")
	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("Failed to read log file: %v", err)
	}

	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) != 10 {
		t.Errorf("Expected 10 lines, got %d", len(lines))
	}

	// Verify each line is valid JSON
	for i, line := range lines {
		var event telemetry.Event
		if err := json.Unmarshal([]byte(line), &event); err != nil {
			t.Errorf("Line %d: failed to parse JSON: %v", i+1, err)
		}
	}
}

// Test 5: Session caching (getSessionUUID called once)
func TestSessionCaching_CallsOnce(t *testing.T) {
	tmpDir := t.TempDir()
	sessionUUID := "test-cache-uuid"
	sessionDir := filepath.Join(tmpDir, sessionUUID)
	if err := os.MkdirAll(sessionDir, 0755); err != nil {
		t.Fatalf("Failed to create session directory: %v", err)
	}

	// Track calls to getSessionUUID
	callCount := 0
	oldGetSessionUUID := getSessionUUID
	defer func() { getSessionUUID = oldGetSessionUUID }()
	getSessionUUID = func() string {
		callCount++
		return sessionUUID
	}

	logger := NewTokenLogger()
	logger.sessionDir = sessionDir
	// Don't set cacheChecked - let getSessionDirLocked() handle it

	event := &telemetry.Event{
		Timestamp: time.Now(),
		Type:      "test.cache",
		Agent:     "test",
		Level:     telemetry.LevelError,
		Data:      map[string]interface{}{"key": "value"},
	}

	// Call OnEvent multiple times
	for i := 0; i < 5; i++ {
		if err := logger.OnEvent(event); err != nil {
			t.Fatalf("OnEvent call %d failed: %v", i+1, err)
		}
	}

	// Verify getSessionUUID was called only once
	if callCount != 1 {
		t.Errorf("Expected getSessionUUID called 1 time, got %d", callCount)
	}
}

// Test 6: JSONL format valid (parseable by json.Unmarshal)
func TestJSONL_FormatValid(t *testing.T) {
	tmpDir := t.TempDir()
	sessionUUID := "test-jsonl-uuid"
	sessionDir := filepath.Join(tmpDir, sessionUUID)
	if err := os.MkdirAll(sessionDir, 0755); err != nil {
		t.Fatalf("Failed to create session directory: %v", err)
	}

	logger := NewTokenLogger()
	logger.sessionDir = sessionDir
	logger.cacheChecked = true

	// Write multiple events
	for i := 0; i < 3; i++ {
		event := &telemetry.Event{
			Timestamp: time.Now(),
			Type:      "test.jsonl",
			Agent:     "test",
			Level:     telemetry.LevelError,
			Data:      map[string]interface{}{"index": i},
		}
		if err := logger.OnEvent(event); err != nil {
			t.Fatalf("OnEvent failed: %v", err)
		}
	}

	// Read file and validate JSONL format
	logPath := filepath.Join(sessionDir, "token-usage.jsonl")
	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("Failed to read log file: %v", err)
	}

	// Each line should be valid JSON
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) != 3 {
		t.Errorf("Expected 3 lines, got %d", len(lines))
	}

	for i, line := range lines {
		var event telemetry.Event
		if err := json.Unmarshal([]byte(line), &event); err != nil {
			t.Errorf("Line %d: invalid JSON: %v", i+1, err)
		}
		// Verify format: should have timestamp, type, level, data
		if event.Type != "test.jsonl" {
			t.Errorf("Line %d: expected type 'test.jsonl', got '%s'", i+1, event.Type)
		}
	}
}

// Test 7: File permissions 0600
func TestFilePermissions_0600(t *testing.T) {
	tmpDir := t.TempDir()
	sessionUUID := "test-perms-uuid"
	sessionDir := filepath.Join(tmpDir, sessionUUID)
	if err := os.MkdirAll(sessionDir, 0755); err != nil {
		t.Fatalf("Failed to create session directory: %v", err)
	}

	logger := NewTokenLogger()
	logger.sessionDir = sessionDir
	logger.cacheChecked = true

	event := &telemetry.Event{
		Timestamp: time.Now(),
		Type:      "test.perms",
		Agent:     "test",
		Level:     telemetry.LevelError,
		Data:      map[string]interface{}{"key": "value"},
	}

	if err := logger.OnEvent(event); err != nil {
		t.Fatalf("OnEvent failed: %v", err)
	}

	// Check file permissions
	logPath := filepath.Join(sessionDir, "token-usage.jsonl")
	info, err := os.Stat(logPath)
	if err != nil {
		t.Fatalf("Failed to stat file: %v", err)
	}

	mode := info.Mode().Perm()
	expected := os.FileMode(0600)
	if mode != expected {
		t.Errorf("Expected permissions %o, got %o", expected, mode)
	}
}

// Test 8: Directory missing returns nil (graceful degradation)
func TestOnEvent_DirectoryMissing_ReturnsNil(t *testing.T) {
	tmpDir := t.TempDir()
	sessionUUID := "test-missing-uuid"
	sessionDir := filepath.Join(tmpDir, "nonexistent", sessionUUID)

	// Override getSessionUUID
	oldGetSessionUUID := getSessionUUID
	defer func() { getSessionUUID = oldGetSessionUUID }()
	getSessionUUID = func() string { return sessionUUID }

	logger := NewTokenLogger()
	logger.sessionDir = sessionDir
	logger.cacheChecked = true

	event := &telemetry.Event{
		Timestamp: time.Now(),
		Type:      "test.missing",
		Agent:     "test",
		Level:     telemetry.LevelError,
		Data:      map[string]interface{}{"key": "value"},
	}

	// Should return nil without creating directory
	err := logger.OnEvent(event)
	if err != nil {
		t.Errorf("Expected nil error, got %v", err)
	}

	// Verify directory was not created
	if _, err := os.Stat(sessionDir); !os.IsNotExist(err) {
		t.Error("Directory should not have been created")
	}
}
