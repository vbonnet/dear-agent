package telemetry

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func TestNewLogger(t *testing.T) {
	logger := NewLogger("test.jsonl")

	if logger == nil {
		t.Fatal("NewLogger should not return nil")
	}

	if logger.filePath != "test.jsonl" {
		t.Errorf("filePath: got %q, want %q", logger.filePath, "test.jsonl")
	}
}

func TestLogEvent(t *testing.T) {
	tmpDir := t.TempDir()
	logFile := filepath.Join(tmpDir, "execution.jsonl")

	logger := NewLogger(logFile)

	// Create test event
	event := &ExecutionEvent{
		Timestamp: time.Now(),
		BeadID:    "bead-1",
		Event:     "claim",
		Details: map[string]interface{}{
			"session": "session-1",
		},
	}

	// Log event
	if err := logger.LogEvent(event); err != nil {
		t.Fatalf("LogEvent failed: %v", err)
	}

	// Verify file exists
	if _, err := os.Stat(logFile); err != nil {
		t.Errorf("log file not created: %v", err)
	}

	// Read and verify content
	data, err := os.ReadFile(logFile)
	if err != nil {
		t.Fatalf("failed to read log file: %v", err)
	}

	// Verify JSON format
	var logged ExecutionEvent
	if err := json.Unmarshal(data, &logged); err != nil {
		t.Errorf("invalid JSON in log file: %v", err)
	}

	// Verify event data
	if logged.BeadID != "bead-1" {
		t.Errorf("bead ID: got %q, want %q", logged.BeadID, "bead-1")
	}

	if logged.Event != "claim" {
		t.Errorf("event: got %q, want %q", logged.Event, "claim")
	}
}

func TestLogEvent_MultipleEvents(t *testing.T) {
	tmpDir := t.TempDir()
	logFile := filepath.Join(tmpDir, "execution.jsonl")

	logger := NewLogger(logFile)

	// Log multiple events
	events := []*ExecutionEvent{
		{Timestamp: time.Now(), BeadID: "bead-1", Event: "claim"},
		{Timestamp: time.Now(), BeadID: "bead-1", Event: "execute"},
		{Timestamp: time.Now(), BeadID: "bead-1", Event: "complete"},
	}

	for _, event := range events {
		if err := logger.LogEvent(event); err != nil {
			t.Fatalf("LogEvent failed: %v", err)
		}
	}

	// Read all lines
	file, err := os.Open(logFile)
	if err != nil {
		t.Fatalf("failed to open log file: %v", err)
	}
	defer func() {
		_ = file.Close()
	}()

	scanner := bufio.NewScanner(file)
	lineCount := 0
	for scanner.Scan() {
		lineCount++

		// Verify each line is valid JSON
		var event ExecutionEvent
		if err := json.Unmarshal(scanner.Bytes(), &event); err != nil {
			t.Errorf("line %d: invalid JSON: %v", lineCount, err)
		}
	}

	if lineCount != 3 {
		t.Errorf("line count: got %d, want 3", lineCount)
	}
}

func TestLogEvent_ConcurrentWrites(t *testing.T) {
	tmpDir := t.TempDir()
	logFile := filepath.Join(tmpDir, "execution.jsonl")

	logger := NewLogger(logFile)

	// Concurrent writes
	var wg sync.WaitGroup
	eventCount := 10

	for i := 0; i < eventCount; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()

			event := &ExecutionEvent{
				Timestamp: time.Now(),
				BeadID:    "bead-concurrent",
				Event:     "test",
				Details: map[string]interface{}{
					"id": id,
				},
			}

			if err := logger.LogEvent(event); err != nil {
				t.Errorf("concurrent LogEvent %d failed: %v", id, err)
			}
		}(i)
	}

	wg.Wait()

	// Verify all events written
	file, err := os.Open(logFile)
	if err != nil {
		t.Fatalf("failed to open log file: %v", err)
	}
	defer func() {
		_ = file.Close()
	}()

	scanner := bufio.NewScanner(file)
	lineCount := 0
	for scanner.Scan() {
		lineCount++

		// Verify each line is valid JSON
		var event ExecutionEvent
		if err := json.Unmarshal(scanner.Bytes(), &event); err != nil {
			t.Errorf("line %d: invalid JSON: %v", lineCount, err)
		}
	}

	if lineCount != eventCount {
		t.Errorf("concurrent writes: got %d events, want %d", lineCount, eventCount)
	}
}

func TestLogEvent_InvalidPath(t *testing.T) {
	// Use invalid path (directory doesn't exist)
	logger := NewLogger("/nonexistent/directory/execution.jsonl")

	event := &ExecutionEvent{
		Timestamp: time.Now(),
		BeadID:    "bead-1",
		Event:     "claim",
	}

	// Should fail to create file
	err := logger.LogEvent(event)
	if err == nil {
		t.Error("expected error for invalid path")
	}
}
