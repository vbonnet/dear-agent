package consolidation

import (
	"context"
	"testing"
	"time"
)

// mockEventBus implements EventBus for testing
type mockEventBus struct {
	events []Event
}

func (m *mockEventBus) Publish(ctx context.Context, event *Event) error {
	m.events = append(m.events, *event)
	return nil
}

func (m *mockEventBus) PublishSync(ctx context.Context, event *Event) error {
	m.events = append(m.events, *event)
	return nil
}

func TestNewMemoryEvent(t *testing.T) {
	data := map[string]interface{}{
		"provider":  "simple",
		"namespace": []string{"user", "alice"},
		"memory_id": "mem-123",
	}

	event := NewMemoryEvent(TopicMemoryStored, data)

	if event.Topic != TopicMemoryStored {
		t.Errorf("Topic = %s, want %s", event.Topic, TopicMemoryStored)
	}

	if event.Publisher != "memory-consolidation" {
		t.Errorf("Publisher = %s, want memory-consolidation", event.Publisher)
	}

	if event.Data == nil {
		t.Fatal("Data should not be nil")
	}

	if provider := event.Data["provider"].(string); provider != "simple" {
		t.Errorf("Data[provider] = %s, want simple", provider)
	}

	if event.Timestamp.IsZero() {
		t.Error("Timestamp should be set")
	}
}

func TestPublishMemoryEvent(t *testing.T) {
	bus := &mockEventBus{}
	ctx := WithEventBus(context.Background(), bus)

	data := map[string]interface{}{
		"provider":  "test-provider",
		"namespace": []string{"user", "bob"},
		"memory_id": "mem-456",
	}

	PublishMemoryEvent(ctx, bus, TopicMemoryUpdated, data)

	if len(bus.events) != 1 {
		t.Fatalf("Expected 1 event, got %d", len(bus.events))
	}

	event := bus.events[0]

	if event.Topic != TopicMemoryUpdated {
		t.Errorf("Topic = %s, want %s", event.Topic, TopicMemoryUpdated)
	}

	if event.Publisher != "memory-consolidation" {
		t.Errorf("Publisher = %s, want memory-consolidation", event.Publisher)
	}

	if provider := event.Data["provider"].(string); provider != "test-provider" {
		t.Errorf("Data[provider] = %s, want test-provider", provider)
	}
}

func TestPublishMemoryEvent_NilBus(t *testing.T) {
	ctx := context.Background() // No bus in context

	data := map[string]interface{}{
		"provider": "test",
	}

	// Should not panic with nil bus
	PublishMemoryEvent(ctx, nil, TopicMemoryStored, data)

	// Should not panic when bus not in context
	PublishMemoryEvent(ctx, GetEventBus(ctx), TopicMemoryStored, data)
}

func TestGetEventBus(t *testing.T) {
	// Context without bus
	ctx := context.Background()
	if bus := GetEventBus(ctx); bus != nil {
		t.Error("Expected nil bus from empty context")
	}

	// Context with bus
	mockBus := &mockEventBus{}
	ctx = WithEventBus(ctx, mockBus)

	if bus := GetEventBus(ctx); bus == nil {
		t.Error("Expected non-nil bus from context")
	} else if bus != mockBus {
		t.Error("Expected same bus instance")
	}
}

func TestWithEventBus(t *testing.T) {
	ctx := context.Background()
	bus1 := &mockEventBus{}
	bus2 := &mockEventBus{}

	// Add first bus
	ctx1 := WithEventBus(ctx, bus1)
	if GetEventBus(ctx1) != bus1 {
		t.Error("Context should contain bus1")
	}

	// Replace with second bus
	ctx2 := WithEventBus(ctx1, bus2)
	if GetEventBus(ctx2) != bus2 {
		t.Error("Context should contain bus2")
	}

	// Original context unchanged
	if GetEventBus(ctx1) != bus1 {
		t.Error("Original context should still contain bus1")
	}
}

func TestEventTopics(t *testing.T) {
	// Verify topic constants are defined
	topics := []string{
		TopicMemoryStored,
		TopicMemoryRetrieved,
		TopicMemoryUpdated,
		TopicMemoryDeleted,
		TopicArtifactStored,
		TopicArtifactFetched,
		TopicArtifactDeleted,
		TopicSessionPersisted,
	}

	for _, topic := range topics {
		if topic == "" {
			t.Error("Topic constant should not be empty")
		}
	}

	// Verify topics follow naming convention (memory.* or memory.artifact.* or memory.session.*)
	expectedPrefixes := map[string]bool{
		"memory.":          true,
		"memory.artifact.": true,
		"memory.session.":  true,
	}

	for _, topic := range topics {
		hasValidPrefix := false
		for prefix := range expectedPrefixes {
			if len(topic) >= len(prefix) && topic[:len(prefix)] == prefix {
				hasValidPrefix = true
				break
			}
		}
		if !hasValidPrefix {
			t.Errorf("Topic %s does not follow naming convention", topic)
		}
	}
}

func TestEventStructure(t *testing.T) {
	// Verify Event struct has required fields
	event := NewMemoryEvent(TopicMemoryStored, map[string]interface{}{
		"test": "data",
	})

	// Verify all fields are accessible
	_ = event.ID
	_ = event.Topic
	_ = event.Publisher
	_ = event.Timestamp
	_ = event.Data
	_ = event.RequiresResponse
	_ = event.ResponseTimeout

	// Verify defaults
	if event.RequiresResponse {
		t.Error("RequiresResponse should default to false")
	}

	if event.ResponseTimeout != 0 {
		t.Error("ResponseTimeout should default to zero")
	}
}

func TestPublishMemoryEvent_AsyncPublish(t *testing.T) {
	bus := &mockEventBus{}
	ctx := WithEventBus(context.Background(), bus)

	// Publish multiple events rapidly
	for i := 0; i < 10; i++ {
		PublishMemoryEvent(ctx, bus, TopicMemoryStored, map[string]interface{}{
			"index": i,
		})
	}

	// All events should be published
	if len(bus.events) != 10 {
		t.Errorf("Expected 10 events, got %d", len(bus.events))
	}

	// Events should have unique timestamps
	timestamps := make(map[time.Time]bool)
	for _, event := range bus.events {
		timestamps[event.Timestamp] = true
	}

	// Note: timestamps might not be unique due to high frequency,
	// but we verify they're all set
	for _, event := range bus.events {
		if event.Timestamp.IsZero() {
			t.Error("Event timestamp should be set")
		}
	}
}
