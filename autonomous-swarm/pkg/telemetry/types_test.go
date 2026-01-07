package telemetry

import (
	"encoding/json"
	"testing"
	"time"
)

func TestExecutionEvent_JSONMarshal(t *testing.T) {
	event := ExecutionEvent{
		Timestamp: time.Date(2026, 1, 7, 0, 0, 0, 0, time.UTC),
		BeadID:    "bead-1",
		Event:     "claim",
		Details: map[string]interface{}{
			"session_name": "test-session",
			"iteration":    1,
		},
	}

	data, err := json.Marshal(&event)
	if err != nil {
		t.Fatalf("failed to marshal: %v", err)
	}

	var event2 ExecutionEvent
	if err := json.Unmarshal(data, &event2); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if event2.BeadID != event.BeadID {
		t.Errorf("bead_id mismatch: got %s, want %s", event2.BeadID, event.BeadID)
	}
	if event2.Event != event.Event {
		t.Errorf("event mismatch: got %s, want %s", event2.Event, event.Event)
	}
	if event2.Details["session_name"] != "test-session" {
		t.Errorf("details session_name mismatch: got %v", event2.Details["session_name"])
	}
}

func TestExecutionEvent_EventTypes(t *testing.T) {
	eventTypes := []string{
		"claim",
		"execute",
		"validate_s8",
		"validate_s9",
		"complete",
		"escalate",
		"error",
	}

	for _, eventType := range eventTypes {
		event := ExecutionEvent{
			Timestamp: time.Now(),
			BeadID:    "test-bead",
			Event:     eventType,
		}

		if event.Event != eventType {
			t.Errorf("event type mismatch: got %s, want %s", event.Event, eventType)
		}
	}
}

func TestRoadmapEntry_Sections(t *testing.T) {
	tests := []struct {
		section string
		count   int
		items   []string
	}{
		{"ready", 5, []string{"bead-1", "bead-2"}},
		{"in_progress", 2, []string{"bead-3"}},
		{"blocked", 1, []string{"bead-4"}},
		{"completed", 10, []string{}},
	}

	for _, tt := range tests {
		t.Run(tt.section, func(t *testing.T) {
			entry := RoadmapEntry{
				Section: tt.section,
				Count:   tt.count,
				Items:   tt.items,
			}

			if entry.Section != tt.section {
				t.Errorf("section mismatch: got %s, want %s", entry.Section, tt.section)
			}
			if entry.Count != tt.count {
				t.Errorf("count mismatch: got %d, want %d", entry.Count, tt.count)
			}
			if len(entry.Items) != len(tt.items) {
				t.Errorf("items length mismatch: got %d, want %d", len(entry.Items), len(tt.items))
			}
		})
	}
}

func TestExecutionEvent_WithoutDetails(t *testing.T) {
	event := ExecutionEvent{
		Timestamp: time.Now(),
		BeadID:    "simple-bead",
		Event:     "complete",
		Details:   nil,
	}

	data, err := json.Marshal(&event)
	if err != nil {
		t.Fatalf("failed to marshal event without details: %v", err)
	}

	var event2 ExecutionEvent
	if err := json.Unmarshal(data, &event2); err != nil {
		t.Fatalf("failed to unmarshal event without details: %v", err)
	}

	if event2.Event != "complete" {
		t.Errorf("event mismatch: got %s, want complete", event2.Event)
	}
}
