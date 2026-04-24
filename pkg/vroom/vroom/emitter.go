package vroom

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// EventPublisher abstracts the event bus so this package has no direct dependency
// on pkg/events. Implementations must be safe for concurrent use.
type EventPublisher interface {
	// Publish sends an event asynchronously. The caller does not wait for
	// subscribers to finish processing.
	Publish(topic string, data map[string]interface{}) error
}

// Emitter provides typed, fire-and-forget methods for VROOM decision events.
type Emitter struct {
	publisher EventPublisher
	role      string // VROOM role name used as the publisher identity
}

// NewEmitter creates an Emitter that publishes through the given EventPublisher.
// role identifies the VROOM role (e.g. "orchestrator", "overseer").
func NewEmitter(publisher EventPublisher, role string) *Emitter {
	return &Emitter{publisher: publisher, role: role}
}

// EmitDispatched fires a TopicDecisionDispatched event.
func (e *Emitter) EmitDispatched(p DispatchedPayload) {
	e.emit(TopicDecisionDispatched, p)
}

// EmitEscalated fires a TopicDecisionEscalated event.
func (e *Emitter) EmitEscalated(p EscalatedPayload) {
	e.emit(TopicDecisionEscalated, p)
}

// EmitEvaluated fires a TopicDecisionEvaluated event.
func (e *Emitter) EmitEvaluated(p EvaluatedPayload) {
	e.emit(TopicDecisionEvaluated, p)
}

// EmitGated fires a TopicDecisionGated event.
func (e *Emitter) EmitGated(p GatedPayload) {
	e.emit(TopicDecisionGated, p)
}

// emit marshals the payload and publishes asynchronously. Errors are silently
// dropped (fire-and-forget).
func (e *Emitter) emit(topic string, payload interface{}) {
	go func() {
		data, err := structToMap(payload)
		if err != nil {
			return
		}
		data["event_id"] = uuid.New().String()
		data["role"] = e.role
		data["timestamp"] = time.Now().UTC().Format(time.RFC3339Nano)
		_ = e.publisher.Publish(topic, data)
	}()
}

// structToMap converts a struct to map[string]interface{} via JSON round-trip.
func structToMap(v interface{}) (map[string]interface{}, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return nil, fmt.Errorf("marshal payload: %w", err)
	}
	var m map[string]interface{}
	if err := json.Unmarshal(b, &m); err != nil {
		return nil, fmt.Errorf("unmarshal payload: %w", err)
	}
	return m, nil
}
