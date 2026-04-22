package consolidation

import (
	"context"
	"testing"
	"time"

	"github.com/vbonnet/dear-agent/internal/telemetry"
)

// mockTelemetryRecorder implements TelemetryRecorder for testing
type mockTelemetryRecorder struct {
	events []telemetryEvent
}

type telemetryEvent struct {
	eventType string
	agent     string
	level     telemetry.Level
	data      map[string]interface{}
}

func (m *mockTelemetryRecorder) Record(eventType string, agent string, level telemetry.Level, data map[string]interface{}) error {
	m.events = append(m.events, telemetryEvent{
		eventType: eventType,
		agent:     agent,
		level:     level,
		data:      data,
	})
	return nil
}

func (m *mockTelemetryRecorder) getLastEvent() telemetryEvent {
	if len(m.events) == 0 {
		return telemetryEvent{}
	}
	return m.events[len(m.events)-1]
}

func TestRecordMemoryEvent(t *testing.T) {
	recorder := &mockTelemetryRecorder{}
	ctx := WithTelemetryRecorder(context.Background(), recorder)

	eventData := MemoryEventData{
		Provider:   "test-provider",
		Namespace:  []string{"user", "alice"},
		MemoryID:   "mem-123",
		MemoryType: Episodic,
		Latency:    50 * time.Millisecond,
		Success:    true,
		ResultSize: 5,
	}

	RecordMemoryEvent(ctx, recorder, EventMemoryStored, eventData)

	if len(recorder.events) != 1 {
		t.Fatalf("Expected 1 event, got %d", len(recorder.events))
	}

	event := recorder.events[0]
	if event.eventType != EventMemoryStored {
		t.Errorf("Event type = %s, want %s", event.eventType, EventMemoryStored)
	}

	// Check event data fields
	if provider := event.data["provider"].(string); provider != "test-provider" {
		t.Errorf("Provider = %s, want test-provider", provider)
	}

	if latency := event.data["latency_ms"].(int64); latency != 50 {
		t.Errorf("Latency = %d ms, want 50 ms", latency)
	}

	if success := event.data["success"].(bool); success != true {
		t.Errorf("Success = %v, want true", success)
	}

	if memoryType := event.data["memory_type"].(string); memoryType != "episodic" {
		t.Errorf("Memory type = %s, want episodic", memoryType)
	}
}

func TestRecordMemoryEvent_WithError(t *testing.T) {
	recorder := &mockTelemetryRecorder{}
	ctx := WithTelemetryRecorder(context.Background(), recorder)

	eventData := MemoryEventData{
		Provider:  "test-provider",
		Namespace: []string{"user", "bob"},
		MemoryID:  "mem-456",
		Latency:   25 * time.Millisecond,
		Success:   false,
		ErrorMsg:  "file not found",
	}

	RecordMemoryEvent(ctx, recorder, EventMemoryRetrieved, eventData)

	event := recorder.getLastEvent()
	if event.eventType != EventMemoryRetrieved {
		t.Errorf("Event type = %s, want %s", event.eventType, EventMemoryRetrieved)
	}

	if success := event.data["success"].(bool); success != false {
		t.Errorf("Success = %v, want false", success)
	}

	if errMsg := event.data["error_msg"].(string); errMsg != "file not found" {
		t.Errorf("Error msg = %s, want 'file not found'", errMsg)
	}
}

func TestRecordMemoryEvent_NilRecorder(t *testing.T) {
	ctx := context.Background() // No recorder

	eventData := MemoryEventData{
		Provider:  "test",
		Namespace: []string{"test"},
		Success:   true,
	}

	// Should not panic with nil recorder
	RecordMemoryEvent(ctx, nil, EventMemoryStored, eventData)

	// Should not panic when recorder not in context
	RecordMemoryEvent(ctx, GetTelemetryRecorder(ctx), EventMemoryStored, eventData)
}

func TestGetTelemetryRecorder(t *testing.T) {
	// Context without recorder
	ctx := context.Background()
	if recorder := GetTelemetryRecorder(ctx); recorder != nil {
		t.Error("Expected nil recorder from empty context")
	}

	// Context with recorder
	mockRecorder := &mockTelemetryRecorder{}
	ctx = WithTelemetryRecorder(ctx, mockRecorder)

	if recorder := GetTelemetryRecorder(ctx); recorder == nil {
		t.Error("Expected non-nil recorder from context")
	} else if recorder != mockRecorder {
		t.Error("Expected same recorder instance")
	}
}

func TestWithTelemetryRecorder(t *testing.T) {
	ctx := context.Background()
	recorder1 := &mockTelemetryRecorder{}
	recorder2 := &mockTelemetryRecorder{}

	// Add first recorder
	ctx1 := WithTelemetryRecorder(ctx, recorder1)
	if GetTelemetryRecorder(ctx1) != recorder1 {
		t.Error("Context should contain recorder1")
	}

	// Replace with second recorder
	ctx2 := WithTelemetryRecorder(ctx1, recorder2)
	if GetTelemetryRecorder(ctx2) != recorder2 {
		t.Error("Context should contain recorder2")
	}

	// Original context unchanged
	if GetTelemetryRecorder(ctx1) != recorder1 {
		t.Error("Original context should still contain recorder1")
	}
}

func TestMemoryEventData_OmitEmpty(t *testing.T) {
	recorder := &mockTelemetryRecorder{}
	ctx := WithTelemetryRecorder(context.Background(), recorder)

	// Event with minimal data (no optional fields)
	eventData := MemoryEventData{
		Provider:  "simple",
		Namespace: []string{"test"},
		Latency:   10 * time.Millisecond,
		Success:   true,
	}

	RecordMemoryEvent(ctx, recorder, EventMemoryDeleted, eventData)

	event := recorder.getLastEvent()

	// Optional fields should not be present when empty
	if _, ok := event.data["memory_id"]; ok {
		t.Error("memory_id should not be present when empty")
	}
	if _, ok := event.data["memory_type"]; ok {
		t.Error("memory_type should not be present when empty")
	}
	if _, ok := event.data["error_msg"]; ok {
		t.Error("error_msg should not be present when empty")
	}
	if _, ok := event.data["result_size"]; ok {
		t.Error("result_size should not be present when zero")
	}

	// Required fields should be present
	if _, ok := event.data["provider"]; !ok {
		t.Error("provider should be present")
	}
	if _, ok := event.data["namespace"]; !ok {
		t.Error("namespace should be present")
	}
	if _, ok := event.data["latency_ms"]; !ok {
		t.Error("latency_ms should be present")
	}
	if _, ok := event.data["success"]; !ok {
		t.Error("success should be present")
	}
}
