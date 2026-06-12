package usage

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestNew_CustomPath(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "usage.jsonl")
	tracker, err := New(path)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if tracker.FilePath() != path {
		t.Errorf("FilePath = %q, want %q", tracker.FilePath(), path)
	}
}

func TestTrackSync_WritesEvent(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "usage.jsonl")
	tracker, err := New(path)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	ev := Event{
		Timestamp: time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC),
		Command:   "session",
		Args:      []string{"new"},
		Success:   true,
	}
	if err := tracker.TrackSync(ev); err != nil {
		t.Fatalf("TrackSync: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	var decoded Event
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if decoded.Command != "session" {
		t.Errorf("Command = %q, want session", decoded.Command)
	}
	if !decoded.Success {
		t.Error("Success should be true")
	}
}

func TestTrackSync_AppendsMultipleEvents(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "usage.jsonl")
	tracker, err := New(path)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	for i := range 3 {
		ev := Event{Command: "cmd", Args: []string{string(rune('a' + i))}, Success: true}
		if err := tracker.TrackSync(ev); err != nil {
			t.Fatalf("TrackSync %d: %v", i, err)
		}
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	// Each line is a JSON object; count newlines
	lines := 0
	for _, b := range data {
		if b == '\n' {
			lines++
		}
	}
	if lines != 3 {
		t.Errorf("want 3 JSONL lines, got %d", lines)
	}
}

func TestTrackSync_SetsTimestampWhenZero(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "usage.jsonl")
	tracker, err := New(path)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	before := time.Now()
	ev := Event{Command: "test"} // Timestamp is zero
	if err := tracker.TrackSync(ev); err != nil {
		t.Fatalf("TrackSync: %v", err)
	}

	data, _ := os.ReadFile(path)
	var decoded Event
	_ = json.Unmarshal(data, &decoded)
	if decoded.Timestamp.IsZero() {
		t.Error("Timestamp should be set by trackSync when zero")
	}
	if decoded.Timestamp.Before(before) {
		t.Error("Timestamp should be after the test started")
	}
}

func TestTrackSync_ErrorEvent(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "usage.jsonl")
	tracker, err := New(path)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	ev := Event{
		Command: "bad-cmd",
		Success: false,
		Error:   "unknown command",
	}
	if err := tracker.TrackSync(ev); err != nil {
		t.Fatalf("TrackSync: %v", err)
	}

	data, _ := os.ReadFile(path)
	var decoded Event
	_ = json.Unmarshal(data, &decoded)
	if decoded.Error != "unknown command" {
		t.Errorf("Error = %q, want 'unknown command'", decoded.Error)
	}
}
