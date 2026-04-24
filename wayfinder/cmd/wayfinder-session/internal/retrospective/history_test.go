package retrospective

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestLogToHistory tests JSON marshaling and history logging
func TestLogToHistory(t *testing.T) {
	tmpDir := t.TempDir()

	// History file will be created automatically by O_CREATE flag

	// Create rewind event data
	data := &RewindEventData{
		FromPhase: "S7",
		ToPhase:   "S5",
		Magnitude: 2,
		Timestamp: time.Date(2024, 1, 7, 12, 0, 0, 0, time.UTC),
		Prompted:  true,
		Reason:    "Test reason",
		Learnings: "Test learnings",
		Context: ContextSnapshot{
			Git: GitContext{
				Branch:             "main",
				Commit:             "abc123",
				UncommittedChanges: false,
			},
			Deliverables: []string{"D1-problem.md"},
			PhaseState: PhaseContext{
				CurrentPhase:    "S7",
				CompletedPhases: []string{"W0", "D1"},
				SessionID:       "test-session",
			},
		},
	}

	// Test LogToHistory
	err := LogToHistory(tmpDir, data)
	if err != nil {
		t.Fatalf("LogToHistory failed: %v", err)
	}

	// Verify HISTORY file was created
	historyPath := filepath.Join(tmpDir, "WAYFINDER-HISTORY.md")
	historyContent, err := os.ReadFile(historyPath)
	if err != nil {
		t.Fatalf("Failed to read HISTORY file: %v", err)
	}

	historyStr := strings.TrimSpace(string(historyContent))

	// History file is JSON lines format (one JSON object per line)
	// Parse the JSON line
	var parsedEvent map[string]interface{}
	if err := json.Unmarshal([]byte(historyStr), &parsedEvent); err != nil {
		t.Fatalf("Failed to parse JSON from HISTORY: %v", err)
	}

	// Verify required fields
	if parsedEvent["type"] != "rewind.logged" {
		t.Errorf("Expected type 'rewind.logged', got: %v", parsedEvent["type"])
	}

	if parsedEvent["phase"] != "S5" {
		t.Errorf("Expected phase 'S5', got: %v", parsedEvent["phase"])
	}

	// Verify nested data structure
	dataField, ok := parsedEvent["data"].(map[string]interface{})
	if !ok {
		t.Fatalf("Expected 'data' field to be object, got: %T", parsedEvent["data"])
	}

	if dataField["from_phase"] != "S7" {
		t.Errorf("Expected from_phase 'S7', got: %v", dataField["from_phase"])
	}

	if dataField["to_phase"] != "S5" {
		t.Errorf("Expected to_phase 'S5', got: %v", dataField["to_phase"])
	}

	magnitude, ok := dataField["magnitude"].(float64) // JSON numbers are float64
	if !ok || magnitude != 2 {
		t.Errorf("Expected magnitude 2, got: %v", dataField["magnitude"])
	}

	if dataField["reason"] != "Test reason" {
		t.Errorf("Expected reason 'Test reason', got: %v", dataField["reason"])
	}
}

// TestLogToHistory_MultipleEvents tests appending multiple events
func TestLogToHistory_MultipleEvents(t *testing.T) {
	tmpDir := t.TempDir()

	// First event
	data1 := &RewindEventData{
		FromPhase: "S7",
		ToPhase:   "S6",
		Magnitude: 1,
		Timestamp: time.Now(),
		Reason:    "First rewind",
	}

	err := LogToHistory(tmpDir, data1)
	if err != nil {
		t.Fatalf("First LogToHistory failed: %v", err)
	}

	// Second event
	data2 := &RewindEventData{
		FromPhase: "S8",
		ToPhase:   "S5",
		Magnitude: 3,
		Timestamp: time.Now(),
		Reason:    "Second rewind",
	}

	err = LogToHistory(tmpDir, data2)
	if err != nil {
		t.Fatalf("Second LogToHistory failed: %v", err)
	}

	// Verify both events in file
	historyPath := filepath.Join(tmpDir, "WAYFINDER-HISTORY.md")
	historyContent, err := os.ReadFile(historyPath)
	if err != nil {
		t.Fatalf("Failed to read HISTORY file: %v", err)
	}

	lines := strings.Split(strings.TrimSpace(string(historyContent)), "\n")
	if len(lines) != 2 {
		t.Errorf("Expected 2 events in history, got %d", len(lines))
	}

	// Verify first event
	var event1 map[string]interface{}
	if err := json.Unmarshal([]byte(lines[0]), &event1); err != nil {
		t.Fatalf("Failed to parse first event: %v", err)
	}

	data1Field := event1["data"].(map[string]interface{})
	if data1Field["reason"] != "First rewind" {
		t.Errorf("First event has wrong reason: %v", data1Field["reason"])
	}

	// Verify second event
	var event2 map[string]interface{}
	if err := json.Unmarshal([]byte(lines[1]), &event2); err != nil {
		t.Fatalf("Failed to parse second event: %v", err)
	}

	data2Field := event2["data"].(map[string]interface{})
	if data2Field["reason"] != "Second rewind" {
		t.Errorf("Second event has wrong reason: %v", data2Field["reason"])
	}
}

// TestLogToHistory_JSONMarshaling tests edge cases in JSON marshaling
func TestLogToHistory_JSONMarshaling(t *testing.T) {
	tmpDir := t.TempDir()

	// Test with minimal data (omitempty fields)
	data := &RewindEventData{
		FromPhase: "S5",
		ToPhase:   "S4",
		Magnitude: 1,
		Timestamp: time.Now(),
		Prompted:  false,
		// Reason and Learnings omitted (should use omitempty)
		Context: ContextSnapshot{
			Git:          GitContext{Error: "git not available"},
			Deliverables: []string{},
			PhaseState: PhaseContext{
				CurrentPhase: "S5",
			},
		},
	}

	err := LogToHistory(tmpDir, data)
	if err != nil {
		t.Fatalf("LogToHistory failed: %v", err)
	}

	// Verify file was written
	historyPath := filepath.Join(tmpDir, "WAYFINDER-HISTORY.md")
	historyContent, err := os.ReadFile(historyPath)
	if err != nil {
		t.Fatalf("Failed to read HISTORY file: %v", err)
	}

	historyStr := strings.TrimSpace(string(historyContent))

	// Parse JSON to verify omitempty worked
	var parsedEvent map[string]interface{}
	if err := json.Unmarshal([]byte(historyStr), &parsedEvent); err != nil {
		t.Fatalf("Failed to parse JSON: %v", err)
	}

	if parsedEvent["type"] != "rewind.logged" {
		t.Errorf("HISTORY missing event type")
	}

	// Verify data field exists
	dataField, ok := parsedEvent["data"].(map[string]interface{})
	if !ok {
		t.Fatalf("Expected 'data' field to be object, got: %T", parsedEvent["data"])
	}

	// Verify reason and learnings are omitted (omitempty)
	if _, exists := dataField["reason"]; exists && dataField["reason"] == "" {
		t.Errorf("Expected empty reason to be omitted, but it exists")
	}

	if _, exists := dataField["learnings"]; exists && dataField["learnings"] == "" {
		t.Errorf("Expected empty learnings to be omitted, but it exists")
	}
}
