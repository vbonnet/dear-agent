package history

import (
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func TestNew(t *testing.T) {
	h := New("/tmp/test")
	expected := filepath.Join("/tmp/test", HistoryFilename)
	if h.path != expected {
		t.Errorf("New() path = %q, want %q", h.path, expected)
	}
}

func TestAppendEvent(t *testing.T) {
	// Create temp directory
	tmpDir := t.TempDir()
	h := New(tmpDir)

	// Append event
	data := map[string]interface{}{
		"key": "value",
		"num": 42,
	}
	err := h.AppendEvent(EventTypePhaseStarted, "D1", data)
	if err != nil {
		t.Fatalf("AppendEvent() error = %v", err)
	}

	// Verify file exists
	if _, err := os.Stat(h.path); os.IsNotExist(err) {
		t.Fatal("History file was not created")
	}

	// Read back events
	events, err := h.Read()
	if err != nil {
		t.Fatalf("Read() error = %v", err)
	}

	if len(events) != 1 {
		t.Fatalf("Read() returned %d events, want 1", len(events))
	}

	event := events[0]
	if event.Type != EventTypePhaseStarted {
		t.Errorf("event.Type = %q, want %q", event.Type, EventTypePhaseStarted)
	}
	if event.Phase != "D1" {
		t.Errorf("event.Phase = %q, want %q", event.Phase, "D1")
	}
	if event.Data["key"] != "value" {
		t.Errorf("event.Data[key] = %v, want %q", event.Data["key"], "value")
	}
	// JSON numbers are unmarshaled as float64
	if event.Data["num"] != float64(42) {
		t.Errorf("event.Data[num] = %v, want %v", event.Data["num"], float64(42))
	}
}

func TestAppendEvent_Multiple(t *testing.T) {
	tmpDir := t.TempDir()
	h := New(tmpDir)

	// Append multiple events
	events := []struct {
		eventType string
		phase     string
	}{
		{EventTypeSessionStarted, ""},
		{EventTypePhaseStarted, "D1"},
		{EventTypePhaseCompleted, "D1"},
		{EventTypePhaseStarted, "D2"},
	}

	for _, e := range events {
		if err := h.AppendEvent(e.eventType, e.phase, nil); err != nil {
			t.Fatalf("AppendEvent(%q, %q) error = %v", e.eventType, e.phase, err)
		}
	}

	// Read all events
	readEvents, err := h.Read()
	if err != nil {
		t.Fatalf("Read() error = %v", err)
	}

	if len(readEvents) != len(events) {
		t.Fatalf("Read() returned %d events, want %d", len(readEvents), len(events))
	}

	// Verify events in order
	for i, expected := range events {
		if readEvents[i].Type != expected.eventType {
			t.Errorf("event[%d].Type = %q, want %q", i, readEvents[i].Type, expected.eventType)
		}
		if readEvents[i].Phase != expected.phase {
			t.Errorf("event[%d].Phase = %q, want %q", i, readEvents[i].Phase, expected.phase)
		}
	}
}

func TestRead_EmptyHistory(t *testing.T) {
	tmpDir := t.TempDir()
	h := New(tmpDir)

	// Read without creating file
	events, err := h.Read()
	if err != nil {
		t.Fatalf("Read() error = %v", err)
	}

	if len(events) != 0 {
		t.Errorf("Read() on non-existent file returned %d events, want 0", len(events))
	}
}

func TestRead_CorruptedLine(t *testing.T) {
	tmpDir := t.TempDir()
	h := New(tmpDir)

	// Write valid event
	if err := h.AppendEvent(EventTypePhaseStarted, "D1", nil); err != nil {
		t.Fatalf("AppendEvent() error = %v", err)
	}

	// Manually append corrupted line
	file, err := os.OpenFile(h.path, os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		t.Fatalf("OpenFile() error = %v", err)
	}
	file.WriteString("{ corrupted json\n")
	file.Close()

	// Write another valid event
	if err := h.AppendEvent(EventTypePhaseCompleted, "D1", nil); err != nil {
		t.Fatalf("AppendEvent() error = %v", err)
	}

	// Read should skip corrupted line and return valid events
	events, err := h.Read()
	if err != nil {
		t.Fatalf("Read() error = %v", err)
	}

	// Should have 2 valid events (corrupted line skipped)
	if len(events) != 2 {
		t.Errorf("Read() returned %d events, want 2 (corrupted line should be skipped)", len(events))
	}
}

func TestAppendEvent_Concurrent(t *testing.T) {
	tmpDir := t.TempDir()
	h := New(tmpDir)

	// Concurrent writes using O_APPEND should be safe
	const numGoroutines = 10
	const eventsPerGoroutine = 10

	var wg sync.WaitGroup
	wg.Add(numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		go func(id int) {
			defer wg.Done()
			for j := 0; j < eventsPerGoroutine; j++ {
				data := map[string]interface{}{
					"goroutine": id,
					"event":     j,
				}
				if err := h.AppendEvent(EventTypePhaseStarted, "D1", data); err != nil {
					t.Errorf("AppendEvent() error in goroutine %d: %v", id, err)
				}
			}
		}(i)
	}

	wg.Wait()

	// Read all events
	events, err := h.Read()
	if err != nil {
		t.Fatalf("Read() error = %v", err)
	}

	expectedCount := numGoroutines * eventsPerGoroutine
	if len(events) != expectedCount {
		t.Errorf("Read() returned %d events, want %d", len(events), expectedCount)
	}
}

func TestGetEventsByPhase(t *testing.T) {
	tmpDir := t.TempDir()
	h := New(tmpDir)

	// Add events for different phases
	h.AppendEvent(EventTypePhaseStarted, "D1", nil)
	h.AppendEvent(EventTypePhaseCompleted, "D1", nil)
	h.AppendEvent(EventTypePhaseStarted, "D2", nil)
	h.AppendEvent(EventTypePhaseCompleted, "D2", nil)
	h.AppendEvent(EventTypePhaseStarted, "D3", nil)

	// Get events for D2
	events, err := h.GetEventsByPhase("D2")
	if err != nil {
		t.Fatalf("GetEventsByPhase() error = %v", err)
	}

	if len(events) != 2 {
		t.Fatalf("GetEventsByPhase(D2) returned %d events, want 2", len(events))
	}

	// Verify all events are for D2
	for i, event := range events {
		if event.Phase != "D2" {
			t.Errorf("event[%d].Phase = %q, want %q", i, event.Phase, "D2")
		}
	}
}

func TestGetEventsByType(t *testing.T) {
	tmpDir := t.TempDir()
	h := New(tmpDir)

	// Add events of different types
	h.AppendEvent(EventTypeSessionStarted, "", nil)
	h.AppendEvent(EventTypePhaseStarted, "D1", nil)
	h.AppendEvent(EventTypePhaseCompleted, "D1", nil)
	h.AppendEvent(EventTypePhaseStarted, "D2", nil)
	h.AppendEvent(EventTypeValidationFailed, "D3", nil)

	// Get all phase.started events
	events, err := h.GetEventsByType(EventTypePhaseStarted)
	if err != nil {
		t.Fatalf("GetEventsByType() error = %v", err)
	}

	if len(events) != 2 {
		t.Fatalf("GetEventsByType(phase.started) returned %d events, want 2", len(events))
	}

	// Verify all events are phase.started
	for i, event := range events {
		if event.Type != EventTypePhaseStarted {
			t.Errorf("event[%d].Type = %q, want %q", i, event.Type, EventTypePhaseStarted)
		}
	}
}

func TestEvent_Timestamp(t *testing.T) {
	tmpDir := t.TempDir()
	h := New(tmpDir)

	before := time.Now()
	time.Sleep(1 * time.Millisecond) // Ensure timestamp is after 'before'

	if err := h.AppendEvent(EventTypePhaseStarted, "D1", nil); err != nil {
		t.Fatalf("AppendEvent() error = %v", err)
	}

	time.Sleep(1 * time.Millisecond) // Ensure timestamp is before 'after'
	after := time.Now()

	events, err := h.Read()
	if err != nil {
		t.Fatalf("Read() error = %v", err)
	}

	if len(events) != 1 {
		t.Fatalf("Read() returned %d events, want 1", len(events))
	}

	timestamp := events[0].Timestamp
	if timestamp.Before(before) || timestamp.After(after) {
		t.Errorf("event.Timestamp = %v, want between %v and %v", timestamp, before, after)
	}
}
