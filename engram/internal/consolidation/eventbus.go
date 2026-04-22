package consolidation

import (
	"context"
	"time"
)

// Memory lifecycle event topics for EventBus
const (
	// Memory operation events
	TopicMemoryStored    = "memory.stored"
	TopicMemoryRetrieved = "memory.retrieved"
	TopicMemoryUpdated   = "memory.updated"
	TopicMemoryDeleted   = "memory.deleted"

	// Artifact operation events
	TopicArtifactStored  = "memory.artifact.stored"
	TopicArtifactFetched = "memory.artifact.fetched"
	TopicArtifactDeleted = "memory.artifact.deleted"

	// Session operation events
	TopicSessionPersisted = "memory.session.persisted"
)

// EventBus is an interface for publishing events to the EventBus
// This matches the interface defined in core/pkg/eventbus
type EventBus interface {
	// Publish publishes an event to the bus (async)
	Publish(ctx context.Context, event *Event) error

	// PublishSync publishes an event synchronously (blocking)
	PublishSync(ctx context.Context, event *Event) error
}

// Event represents an EventBus event
// This matches the structure from core/pkg/eventbus/event.go
type Event struct {
	ID               string
	Topic            string
	Publisher        string
	Timestamp        time.Time
	Data             map[string]interface{}
	RequiresResponse bool
	ResponseTimeout  time.Duration
}

// NewMemoryEvent creates a new Event for memory operations
func NewMemoryEvent(topic string, data map[string]interface{}) *Event {
	return &Event{
		Topic:     topic,
		Publisher: "memory-consolidation",
		Timestamp: time.Now(),
		Data:      data,
	}
}

// PublishMemoryEvent is a helper to publish memory operation events
func PublishMemoryEvent(ctx context.Context, bus EventBus, topic string, data map[string]interface{}) {
	if bus == nil {
		return // No event bus configured
	}

	event := NewMemoryEvent(topic, data)

	// Publish asynchronously (non-blocking)
	// Errors are logged by eventbus telemetry
	_ = bus.Publish(ctx, event)
}

// eventBusKey is the context key for EventBus
type eventBusKey struct{}

// GetEventBus extracts EventBus from context
// Returns nil if not present
func GetEventBus(ctx context.Context) EventBus {
	if bus, ok := ctx.Value(eventBusKey{}).(EventBus); ok {
		return bus
	}
	return nil
}

// WithEventBus adds EventBus to context
func WithEventBus(ctx context.Context, bus EventBus) context.Context {
	return context.WithValue(ctx, eventBusKey{}, bus)
}
