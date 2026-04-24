package simple

import (
	"context"
	"testing"
	"time"

	"github.com/vbonnet/dear-agent/engram/internal/consolidation"
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

func TestSimpleFileProvider_StoreMemory_Telemetry(t *testing.T) {
	tempDir := t.TempDir()
	provider := &SimpleFileProvider{storagePath: tempDir}
	recorder := &mockTelemetryRecorder{}
	ctx := consolidation.WithTelemetryRecorder(context.Background(), recorder)

	namespace := []string{"user", "alice"}
	memory := consolidation.Memory{
		SchemaVersion: "1.0",
		ID:            "mem-telemetry-test",
		Type:          consolidation.Episodic,
		Namespace:     namespace,
		Content:       "Test content",
		Timestamp:     time.Now(),
		Importance:    0.8,
	}

	// Execute operation
	err := provider.StoreMemory(ctx, namespace, memory)
	if err != nil {
		t.Fatalf("StoreMemory failed: %v", err)
	}

	// Verify telemetry event was emitted
	if len(recorder.events) != 1 {
		t.Fatalf("Expected 1 telemetry event, got %d", len(recorder.events))
	}

	event := recorder.events[0]

	// Verify event type
	if event.eventType != consolidation.EventMemoryStored {
		t.Errorf("Event type = %s, want %s", event.eventType, consolidation.EventMemoryStored)
	}

	// Verify event data
	if provider := event.data["provider"].(string); provider != "simple" {
		t.Errorf("Provider = %s, want simple", provider)
	}

	if memoryID := event.data["memory_id"].(string); memoryID != "mem-telemetry-test" {
		t.Errorf("Memory ID = %s, want mem-telemetry-test", memoryID)
	}

	if memoryType := event.data["memory_type"].(string); memoryType != "episodic" {
		t.Errorf("Memory type = %s, want episodic", memoryType)
	}

	if success := event.data["success"].(bool); !success {
		t.Error("Success should be true")
	}

	if _, ok := event.data["error_msg"]; ok {
		t.Error("Error msg should not be present for successful operation")
	}
}

func TestSimpleFileProvider_RetrieveMemory_Telemetry(t *testing.T) {
	tempDir := t.TempDir()
	provider := &SimpleFileProvider{storagePath: tempDir}
	recorder := &mockTelemetryRecorder{}
	ctx := consolidation.WithTelemetryRecorder(context.Background(), recorder)

	namespace := []string{"user", "bob"}

	// Store some memories first
	for i := 0; i < 3; i++ {
		memory := consolidation.Memory{
			SchemaVersion: "1.0",
			ID:            "mem-" + string(rune('a'+i)),
			Type:          consolidation.Semantic,
			Namespace:     namespace,
			Content:       "Content",
			Timestamp:     time.Now(),
		}
		_ = provider.StoreMemory(context.Background(), namespace, memory)
	}

	// Clear events from setup
	recorder.events = nil

	// Execute retrieve operation
	query := consolidation.Query{Limit: 10}
	memories, err := provider.RetrieveMemory(ctx, namespace, query)
	if err != nil {
		t.Fatalf("RetrieveMemory failed: %v", err)
	}

	// Verify telemetry event
	if len(recorder.events) != 1 {
		t.Fatalf("Expected 1 telemetry event, got %d", len(recorder.events))
	}

	event := recorder.events[0]

	if event.eventType != consolidation.EventMemoryRetrieved {
		t.Errorf("Event type = %s, want %s", event.eventType, consolidation.EventMemoryRetrieved)
	}

	if success := event.data["success"].(bool); !success {
		t.Error("Success should be true")
	}

	if resultSize := event.data["result_size"].(int); resultSize != len(memories) {
		t.Errorf("Result size = %d, want %d", resultSize, len(memories))
	}
}

func TestSimpleFileProvider_UpdateMemory_Telemetry(t *testing.T) {
	tempDir := t.TempDir()
	provider := &SimpleFileProvider{storagePath: tempDir}
	recorder := &mockTelemetryRecorder{}
	ctx := consolidation.WithTelemetryRecorder(context.Background(), recorder)

	namespace := []string{"user", "charlie"}
	memory := consolidation.Memory{
		SchemaVersion: "1.0",
		ID:            "mem-update",
		Type:          consolidation.Procedural,
		Namespace:     namespace,
		Content:       "Original content",
		Timestamp:     time.Now(),
	}

	// Store memory first
	_ = provider.StoreMemory(context.Background(), namespace, memory)

	// Clear setup events
	recorder.events = nil

	// Update memory
	updates := consolidation.MemoryUpdate{
		SetContent: func() *interface{} {
			var c interface{} = "Updated content"
			return &c
		}(),
	}

	err := provider.UpdateMemory(ctx, namespace, "mem-update", updates)
	if err != nil {
		t.Fatalf("UpdateMemory failed: %v", err)
	}

	// Verify telemetry event
	if len(recorder.events) != 1 {
		t.Fatalf("Expected 1 telemetry event, got %d", len(recorder.events))
	}

	event := recorder.events[0]

	if event.eventType != consolidation.EventMemoryUpdated {
		t.Errorf("Event type = %s, want %s", event.eventType, consolidation.EventMemoryUpdated)
	}

	if memoryType := event.data["memory_type"].(string); memoryType != "procedural" {
		t.Errorf("Memory type = %s, want procedural", memoryType)
	}

	if success := event.data["success"].(bool); !success {
		t.Error("Success should be true")
	}
}

func TestSimpleFileProvider_DeleteMemory_Telemetry(t *testing.T) {
	tempDir := t.TempDir()
	provider := &SimpleFileProvider{storagePath: tempDir}
	recorder := &mockTelemetryRecorder{}
	ctx := consolidation.WithTelemetryRecorder(context.Background(), recorder)

	namespace := []string{"user", "dave"}
	memory := consolidation.Memory{
		SchemaVersion: "1.0",
		ID:            "mem-delete",
		Type:          consolidation.Working,
		Namespace:     namespace,
		Content:       "To delete",
		Timestamp:     time.Now(),
	}

	// Store memory first
	_ = provider.StoreMemory(context.Background(), namespace, memory)

	// Clear setup events
	recorder.events = nil

	// Delete memory
	err := provider.DeleteMemory(ctx, namespace, "mem-delete")
	if err != nil {
		t.Fatalf("DeleteMemory failed: %v", err)
	}

	// Verify telemetry event
	if len(recorder.events) != 1 {
		t.Fatalf("Expected 1 telemetry event, got %d", len(recorder.events))
	}

	event := recorder.events[0]

	if event.eventType != consolidation.EventMemoryDeleted {
		t.Errorf("Event type = %s, want %s", event.eventType, consolidation.EventMemoryDeleted)
	}

	if success := event.data["success"].(bool); !success {
		t.Error("Success should be true")
	}
}

func TestSimpleFileProvider_StoreMemory_TelemetryOnError(t *testing.T) {
	tempDir := t.TempDir()
	provider := &SimpleFileProvider{storagePath: tempDir}
	recorder := &mockTelemetryRecorder{}
	ctx := consolidation.WithTelemetryRecorder(context.Background(), recorder)

	// Invalid namespace (will cause error)
	namespace := []string{"user", ".."}
	memory := consolidation.Memory{
		SchemaVersion: "1.0",
		ID:            "mem-error",
		Type:          consolidation.Episodic,
		Content:       "Content",
		Timestamp:     time.Now(),
	}

	// Execute operation (should fail)
	err := provider.StoreMemory(ctx, namespace, memory)
	if err == nil {
		t.Fatal("Expected error for invalid namespace")
	}

	// Verify telemetry event was emitted even on error
	if len(recorder.events) != 1 {
		t.Fatalf("Expected 1 telemetry event, got %d", len(recorder.events))
	}

	event := recorder.events[0]

	// Verify error was recorded
	if success := event.data["success"].(bool); success {
		t.Error("Success should be false for failed operation")
	}

	if _, ok := event.data["error_msg"]; !ok {
		t.Error("Error msg should be present for failed operation")
	}
}

func TestSimpleFileProvider_StoreArtifact_Telemetry(t *testing.T) {
	tempDir := t.TempDir()
	provider := &SimpleFileProvider{storagePath: tempDir}
	recorder := &mockTelemetryRecorder{}
	ctx := consolidation.WithTelemetryRecorder(context.Background(), recorder)

	data := []byte("binary artifact data")
	err := provider.StoreArtifact(ctx, "artifact-123", data)
	if err != nil {
		t.Fatalf("StoreArtifact failed: %v", err)
	}

	// Verify telemetry event
	if len(recorder.events) != 1 {
		t.Fatalf("Expected 1 telemetry event, got %d", len(recorder.events))
	}

	event := recorder.events[0]

	if event.eventType != consolidation.EventArtifactStored {
		t.Errorf("Event type = %s, want %s", event.eventType, consolidation.EventArtifactStored)
	}

	if sizeBytes := event.data["size_bytes"].(int); sizeBytes != len(data) {
		t.Errorf("Size bytes = %d, want %d", sizeBytes, len(data))
	}

	if success := event.data["success"].(bool); !success {
		t.Error("Success should be true")
	}
}

func TestSimpleFileProvider_NoTelemetryRecorder(t *testing.T) {
	tempDir := t.TempDir()
	provider := &SimpleFileProvider{storagePath: tempDir}

	// Context without telemetry recorder
	ctx := context.Background()

	namespace := []string{"user", "eve"}
	memory := consolidation.Memory{
		SchemaVersion: "1.0",
		ID:            "mem-no-recorder",
		Type:          consolidation.Episodic,
		Namespace:     namespace,
		Content:       "Content",
		Timestamp:     time.Now(),
	}

	// Should not panic when no recorder is present
	err := provider.StoreMemory(ctx, namespace, memory)
	if err != nil {
		t.Fatalf("StoreMemory failed: %v", err)
	}

	// Operation should succeed normally
	memories, err := provider.RetrieveMemory(ctx, namespace, consolidation.Query{Limit: 10})
	if err != nil {
		t.Fatalf("RetrieveMemory failed: %v", err)
	}

	if len(memories) != 1 {
		t.Errorf("Expected 1 memory, got %d", len(memories))
	}
}
