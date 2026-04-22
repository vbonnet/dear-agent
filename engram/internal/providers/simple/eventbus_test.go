package simple

import (
	"context"
	"testing"
	"time"

	"github.com/vbonnet/dear-agent/engram/internal/consolidation"
)

// mockEventBus implements EventBus for testing
type mockEventBus struct {
	events []consolidation.Event
}

func (m *mockEventBus) Publish(ctx context.Context, event *consolidation.Event) error {
	m.events = append(m.events, *event)
	return nil
}

func (m *mockEventBus) PublishSync(ctx context.Context, event *consolidation.Event) error {
	m.events = append(m.events, *event)
	return nil
}

func TestSimpleFileProvider_StoreMemory_EventBus(t *testing.T) {
	tempDir := t.TempDir()
	provider := &SimpleFileProvider{storagePath: tempDir}
	bus := &mockEventBus{}
	ctx := consolidation.WithEventBus(context.Background(), bus)

	namespace := []string{"user", "alice"}
	memory := consolidation.Memory{
		SchemaVersion: "1.0",
		ID:            "mem-eventbus-test",
		Type:          consolidation.Episodic,
		Namespace:     namespace,
		Content:       "Test content",
		Timestamp:     time.Now(),
	}

	// Execute operation
	err := provider.StoreMemory(ctx, namespace, memory)
	if err != nil {
		t.Fatalf("StoreMemory failed: %v", err)
	}

	// Verify event was published
	if len(bus.events) != 1 {
		t.Fatalf("Expected 1 event, got %d", len(bus.events))
	}

	event := bus.events[0]

	// Verify event properties
	if event.Topic != consolidation.TopicMemoryStored {
		t.Errorf("Topic = %s, want %s", event.Topic, consolidation.TopicMemoryStored)
	}

	if event.Publisher != "memory-consolidation" {
		t.Errorf("Publisher = %s, want memory-consolidation", event.Publisher)
	}

	// Verify event data
	if provider := event.Data["provider"].(string); provider != "simple" {
		t.Errorf("Data[provider] = %s, want simple", provider)
	}

	if memoryID := event.Data["memory_id"].(string); memoryID != "mem-eventbus-test" {
		t.Errorf("Data[memory_id] = %s, want mem-eventbus-test", memoryID)
	}

	if memoryType := event.Data["memory_type"].(string); memoryType != "episodic" {
		t.Errorf("Data[memory_type] = %s, want episodic", memoryType)
	}
}

func TestSimpleFileProvider_UpdateMemory_EventBus(t *testing.T) {
	tempDir := t.TempDir()
	provider := &SimpleFileProvider{storagePath: tempDir}
	bus := &mockEventBus{}
	ctx := consolidation.WithEventBus(context.Background(), bus)

	namespace := []string{"user", "bob"}
	memory := consolidation.Memory{
		SchemaVersion: "1.0",
		ID:            "mem-update-eventbus",
		Type:          consolidation.Semantic,
		Namespace:     namespace,
		Content:       "Original",
		Timestamp:     time.Now(),
	}

	// Store memory first
	_ = provider.StoreMemory(context.Background(), namespace, memory)

	// Clear events from setup
	bus.events = nil

	// Update memory
	updates := consolidation.MemoryUpdate{
		SetContent: func() *interface{} {
			var c interface{} = "Updated"
			return &c
		}(),
	}

	err := provider.UpdateMemory(ctx, namespace, "mem-update-eventbus", updates)
	if err != nil {
		t.Fatalf("UpdateMemory failed: %v", err)
	}

	// Verify event was published
	if len(bus.events) != 1 {
		t.Fatalf("Expected 1 event, got %d", len(bus.events))
	}

	event := bus.events[0]

	if event.Topic != consolidation.TopicMemoryUpdated {
		t.Errorf("Topic = %s, want %s", event.Topic, consolidation.TopicMemoryUpdated)
	}

	if memoryType := event.Data["memory_type"].(string); memoryType != "semantic" {
		t.Errorf("Data[memory_type] = %s, want semantic", memoryType)
	}
}

func TestSimpleFileProvider_DeleteMemory_EventBus(t *testing.T) {
	tempDir := t.TempDir()
	provider := &SimpleFileProvider{storagePath: tempDir}
	bus := &mockEventBus{}
	ctx := consolidation.WithEventBus(context.Background(), bus)

	namespace := []string{"user", "charlie"}
	memory := consolidation.Memory{
		SchemaVersion: "1.0",
		ID:            "mem-delete-eventbus",
		Type:          consolidation.Working,
		Namespace:     namespace,
		Content:       "To delete",
		Timestamp:     time.Now(),
	}

	// Store memory first
	_ = provider.StoreMemory(context.Background(), namespace, memory)

	// Clear events from setup
	bus.events = nil

	// Delete memory
	err := provider.DeleteMemory(ctx, namespace, "mem-delete-eventbus")
	if err != nil {
		t.Fatalf("DeleteMemory failed: %v", err)
	}

	// Verify event was published
	if len(bus.events) != 1 {
		t.Fatalf("Expected 1 event, got %d", len(bus.events))
	}

	event := bus.events[0]

	if event.Topic != consolidation.TopicMemoryDeleted {
		t.Errorf("Topic = %s, want %s", event.Topic, consolidation.TopicMemoryDeleted)
	}

	if memoryID := event.Data["memory_id"].(string); memoryID != "mem-delete-eventbus" {
		t.Errorf("Data[memory_id] = %s, want mem-delete-eventbus", memoryID)
	}
}

func TestSimpleFileProvider_NoEventBus(t *testing.T) {
	tempDir := t.TempDir()
	provider := &SimpleFileProvider{storagePath: tempDir}

	// Context without event bus
	ctx := context.Background()

	namespace := []string{"user", "dave"}
	memory := consolidation.Memory{
		SchemaVersion: "1.0",
		ID:            "mem-no-bus",
		Type:          consolidation.Episodic,
		Namespace:     namespace,
		Content:       "Content",
		Timestamp:     time.Now(),
	}

	// Should not panic when no bus is present
	err := provider.StoreMemory(ctx, namespace, memory)
	if err != nil {
		t.Fatalf("StoreMemory failed: %v", err)
	}

	// Operation should succeed normally
	err = provider.DeleteMemory(ctx, namespace, "mem-no-bus")
	if err != nil {
		t.Fatalf("DeleteMemory failed: %v", err)
	}
}

func TestSimpleFileProvider_EventBusOnError(t *testing.T) {
	tempDir := t.TempDir()
	provider := &SimpleFileProvider{storagePath: tempDir}
	bus := &mockEventBus{}
	ctx := consolidation.WithEventBus(context.Background(), bus)

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

	// Event should NOT be published on error (operation didn't complete)
	if len(bus.events) != 0 {
		t.Errorf("Expected 0 events on error, got %d", len(bus.events))
	}
}

func TestSimpleFileProvider_MultipleOperations_EventBus(t *testing.T) {
	tempDir := t.TempDir()
	provider := &SimpleFileProvider{storagePath: tempDir}
	bus := &mockEventBus{}
	ctx := consolidation.WithEventBus(context.Background(), bus)

	namespace := []string{"user", "eve"}

	// Store 3 memories
	for i := 0; i < 3; i++ {
		memory := consolidation.Memory{
			SchemaVersion: "1.0",
			ID:            string(rune('a' + i)),
			Type:          consolidation.Semantic,
			Namespace:     namespace,
			Content:       "Content",
			Timestamp:     time.Now(),
		}
		_ = provider.StoreMemory(ctx, namespace, memory)
	}

	// Should have 3 store events
	if len(bus.events) != 3 {
		t.Errorf("Expected 3 store events, got %d", len(bus.events))
	}

	// All should be memory.stored events
	for _, event := range bus.events {
		if event.Topic != consolidation.TopicMemoryStored {
			t.Errorf("Event topic = %s, want %s", event.Topic, consolidation.TopicMemoryStored)
		}
	}
}

func TestSimpleFileProvider_EventBusWithTelemetry(t *testing.T) {
	tempDir := t.TempDir()
	provider := &SimpleFileProvider{storagePath: tempDir}

	// Setup both telemetry and eventbus
	telemetryRecorder := &mockTelemetryRecorder{}
	bus := &mockEventBus{}

	ctx := consolidation.WithTelemetryRecorder(context.Background(), telemetryRecorder)
	ctx = consolidation.WithEventBus(ctx, bus)

	namespace := []string{"user", "frank"}
	memory := consolidation.Memory{
		SchemaVersion: "1.0",
		ID:            "mem-both",
		Type:          consolidation.Episodic,
		Namespace:     namespace,
		Content:       "Test",
		Timestamp:     time.Now(),
	}

	// Execute operation
	err := provider.StoreMemory(ctx, namespace, memory)
	if err != nil {
		t.Fatalf("StoreMemory failed: %v", err)
	}

	// Both telemetry and eventbus should be triggered
	if len(telemetryRecorder.events) != 1 {
		t.Errorf("Expected 1 telemetry event, got %d", len(telemetryRecorder.events))
	}

	if len(bus.events) != 1 {
		t.Errorf("Expected 1 eventbus event, got %d", len(bus.events))
	}

	// Verify telemetry event
	telemetryEvent := telemetryRecorder.events[0]
	if telemetryEvent.eventType != consolidation.EventMemoryStored {
		t.Errorf("Telemetry event type = %s, want %s", telemetryEvent.eventType, consolidation.EventMemoryStored)
	}

	// Verify eventbus event
	busEvent := bus.events[0]
	if busEvent.Topic != consolidation.TopicMemoryStored {
		t.Errorf("EventBus topic = %s, want %s", busEvent.Topic, consolidation.TopicMemoryStored)
	}
}
