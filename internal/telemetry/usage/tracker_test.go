package usage

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestNew(t *testing.T) {
	// Test with custom path
	tmpDir := t.TempDir()
	customPath := filepath.Join(tmpDir, "custom.jsonl")

	tracker, err := New(customPath)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	if tracker.FilePath() != customPath {
		t.Errorf("FilePath() = %v, want %v", tracker.FilePath(), customPath)
	}

	// Test with default path (empty string)
	tracker, err = New("")
	if err != nil {
		t.Fatalf("New() with default path error = %v", err)
	}

	home, _ := os.UserHomeDir()
	expectedPath := filepath.Join(home, ".engram", "usage.jsonl")
	if tracker.FilePath() != expectedPath {
		t.Errorf("FilePath() = %v, want %v", tracker.FilePath(), expectedPath)
	}
}

func TestTrackSync(t *testing.T) {
	tmpDir := t.TempDir()
	logFile := filepath.Join(tmpDir, "usage.jsonl")

	tracker, err := New(logFile)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	// Track a simple event
	event := Event{
		Timestamp: time.Date(2026, 2, 12, 10, 30, 0, 0, time.UTC),
		Command:   "engram analytics summary",
		Args:      []string{"--format", "json"},
		Success:   true,
	}

	if err := tracker.TrackSync(event); err != nil {
		t.Fatalf("TrackSync() error = %v", err)
	}

	// Verify file was created and contains the event
	data, err := os.ReadFile(logFile)
	if err != nil {
		t.Fatalf("Failed to read log file: %v", err)
	}

	// Parse JSON line
	var parsed Event
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("Failed to parse event: %v", err)
	}

	if parsed.Command != event.Command {
		t.Errorf("Command = %v, want %v", parsed.Command, event.Command)
	}

	if parsed.Success != event.Success {
		t.Errorf("Success = %v, want %v", parsed.Success, event.Success)
	}

	if len(parsed.Args) != len(event.Args) {
		t.Errorf("Args length = %v, want %v", len(parsed.Args), len(event.Args))
	}
}

func TestTrackSync_MultipleEvents(t *testing.T) {
	tmpDir := t.TempDir()
	logFile := filepath.Join(tmpDir, "usage.jsonl")

	tracker, err := New(logFile)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	// Track multiple events
	events := []Event{
		{Command: "engram analytics summary", Success: true},
		{Command: "engram memory retrieve", Success: true},
		{Command: "engram hash", Success: false, Error: "invalid input"},
	}

	for _, event := range events {
		if err := tracker.TrackSync(event); err != nil {
			t.Fatalf("TrackSync() error = %v", err)
		}
	}

	// Verify all events were written
	data, err := os.ReadFile(logFile)
	if err != nil {
		t.Fatalf("Failed to read log file: %v", err)
	}

	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) != len(events) {
		t.Errorf("Got %d lines, want %d", len(lines), len(events))
	}

	// Verify each line is valid JSON
	for i, line := range lines {
		var parsed Event
		if err := json.Unmarshal([]byte(line), &parsed); err != nil {
			t.Errorf("Line %d: failed to parse JSON: %v", i, err)
		}

		if parsed.Command != events[i].Command {
			t.Errorf("Line %d: Command = %v, want %v", i, parsed.Command, events[i].Command)
		}
	}
}

func TestTrackSync_WithFlags(t *testing.T) {
	tmpDir := t.TempDir()
	logFile := filepath.Join(tmpDir, "usage.jsonl")

	tracker, err := New(logFile)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	// Track event with flags
	event := Event{
		Command: "engram analytics summary",
		Flags: map[string]string{
			"format":  "json",
			"verbose": "true",
		},
		Success: true,
	}

	if err := tracker.TrackSync(event); err != nil {
		t.Fatalf("TrackSync() error = %v", err)
	}

	// Verify flags were saved
	data, err := os.ReadFile(logFile)
	if err != nil {
		t.Fatalf("Failed to read log file: %v", err)
	}

	var parsed Event
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("Failed to parse event: %v", err)
	}

	if len(parsed.Flags) != len(event.Flags) {
		t.Errorf("Flags length = %v, want %v", len(parsed.Flags), len(event.Flags))
	}

	for k, v := range event.Flags {
		if parsed.Flags[k] != v {
			t.Errorf("Flags[%s] = %v, want %v", k, parsed.Flags[k], v)
		}
	}
}

func TestTrackSync_AutoTimestamp(t *testing.T) {
	tmpDir := t.TempDir()
	logFile := filepath.Join(tmpDir, "usage.jsonl")

	tracker, err := New(logFile)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	// Track event without timestamp
	event := Event{
		Command: "engram test",
		Success: true,
	}

	before := time.Now()
	if err := tracker.TrackSync(event); err != nil {
		t.Fatalf("TrackSync() error = %v", err)
	}
	after := time.Now()

	// Verify timestamp was set automatically
	data, err := os.ReadFile(logFile)
	if err != nil {
		t.Fatalf("Failed to read log file: %v", err)
	}

	var parsed Event
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("Failed to parse event: %v", err)
	}

	if parsed.Timestamp.IsZero() {
		t.Error("Timestamp should be set automatically")
	}

	if parsed.Timestamp.Before(before) || parsed.Timestamp.After(after) {
		t.Errorf("Timestamp %v not in expected range [%v, %v]", parsed.Timestamp, before, after)
	}
}

func TestTrack_Async(t *testing.T) {
	tmpDir := t.TempDir()
	logFile := filepath.Join(tmpDir, "usage.jsonl")

	tracker, err := New(logFile)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	// Track event asynchronously
	event := Event{
		Command: "engram test",
		Success: true,
	}

	tracker.Track(event)

	// Wait a bit for async write to complete
	time.Sleep(100 * time.Millisecond)

	// Verify event was written
	data, err := os.ReadFile(logFile)
	if err != nil {
		t.Fatalf("Failed to read log file: %v", err)
	}

	if len(data) == 0 {
		t.Error("Expected event to be written asynchronously")
	}

	var parsed Event
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("Failed to parse event: %v", err)
	}

	if parsed.Command != event.Command {
		t.Errorf("Command = %v, want %v", parsed.Command, event.Command)
	}
}
