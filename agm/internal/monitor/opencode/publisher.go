// Package opencode provides opencode functionality.
package opencode

import (
	"context"
	"errors"
	"fmt"
	"sync/atomic"
	"time"

	"github.com/vbonnet/dear-agent/agm/internal/eventbus"
)

var (
	// ErrQueueFull indicates the EventBus queue is full
	ErrQueueFull = errors.New("eventbus queue full")
)

// SessionStateChangeEvent represents a session state change event payload
// This matches the schema from the SPEC and includes the sequence field from P0 fixes
type SessionStateChangeEvent struct {
	SessionID string                 `json:"session_id"`
	State     string                 `json:"state"`
	Timestamp int64                  `json:"timestamp"`
	Sequence  uint64                 `json:"sequence"`           // Monotonic sequence counter (P0 fix)
	Source    string                 `json:"source"`             // "opencode-sse"
	Harness   string                 `json:"harness"`            // "opencode-cli"
	Metadata  map[string]interface{} `json:"metadata,omitempty"` // Event-specific metadata
}

// Publisher publishes parsed OpenCode events to AGM's EventBus
type Publisher struct {
	eventBus     EventBusPublisher
	sessionID    string
	sequence     atomic.Uint64     // Monotonic sequence counter
	failureCount atomic.Uint64     // Consecutive failure counter
	adapter      AdapterController // For circuit breaker (to stop adapter)
}

// NewPublisher creates a new EventBus publisher
func NewPublisher(eventBus EventBusPublisher, sessionID string, adapter AdapterController) *Publisher {
	return &Publisher{
		eventBus:  eventBus,
		sessionID: sessionID,
		adapter:   adapter,
	}
}

// PublishWithBackpressure publishes an event with retry logic and backpressure handling
// Retries 3 times with exponential backoff (100ms, 200ms, 400ms) if publish fails
// Activates circuit breaker after 10 consecutive failures
//
// Note: EventBus Broadcast is non-blocking, so this method primarily handles
// event creation errors and provides circuit breaker protection.
func (p *Publisher) PublishWithBackpressure(event *AGMEvent) error {
	maxRetries := 3
	baseDelay := 100 * time.Millisecond

	var lastErr error
	for i := 0; i < maxRetries; i++ {
		err := p.Publish(event)
		if err == nil {
			// Success - reset failure counter
			p.failureCount.Store(0)
			return nil
		}

		lastErr = err

		// Retry with exponential backoff
		delay := baseDelay * time.Duration(1<<uint(i)) // Exponential backoff: 100ms, 200ms, 400ms
		logger.Warn("EventBus publish failed, retrying", "delay", delay, "attempt", i+1, "max_retries", maxRetries, "error", err)
		incrementMetric("opencode.eventbus.backpressure_delays")
		time.Sleep(delay)
	}

	// All retries failed
	incrementMetric("opencode.eventbus.publish_failures")

	// Increment consecutive failure counter
	failures := p.failureCount.Add(1)

	logger.Error("Failed to publish event after retries", "max_retries", maxRetries, "error", lastErr, "consecutive_failures", failures)

	// Circuit breaker: stop adapter after 10 consecutive failures
	if failures >= 10 {
		logger.Error("Circuit breaker activated: 10 consecutive publish failures, stopping adapter")
		incrementMetric("opencode.eventbus.circuit_breaker_trips")

		// Stop the adapter asynchronously to avoid deadlock
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			if err := p.adapter.Stop(ctx); err != nil {
				logger.Error("Failed to stop adapter during circuit breaker", "error", err)
			}
		}()

		return ErrCircuitBreakerOpen
	}

	return lastErr
}

// Publish publishes a single event to the EventBus
// Returns error only if event creation fails (validation, marshaling)
// Broadcast itself is non-blocking and best-effort
func (p *Publisher) Publish(event *AGMEvent) error {
	if event == nil {
		return fmt.Errorf("event cannot be nil")
	}

	// Increment sequence number (monotonic, thread-safe)
	seq := p.sequence.Add(1)

	// Create SessionStateChangeEvent payload
	payload := SessionStateChangeEvent{
		SessionID: p.sessionID,
		State:     event.State,
		Timestamp: event.Timestamp,
		Sequence:  seq,
		Source:    "opencode-sse",
		Harness:   "opencode-cli",
		Metadata:  event.Metadata,
	}

	// Create EventBus event with SessionStateChange type
	busEvent, err := eventbus.NewEvent(
		eventbus.EventSessionStateChange,
		p.sessionID,
		payload,
	)
	if err != nil {
		return fmt.Errorf("failed to create eventbus event: %w", err)
	}

	// Broadcast to EventBus (non-blocking, best-effort delivery)
	// If the channel is full, the Hub will drop the event and log a warning
	p.eventBus.Broadcast(busEvent)

	logger.Info("Published state change", "session", p.sessionID, "state", event.State, "sequence", seq)
	return nil
}

// GetSequence returns the current sequence number (for testing)
func (p *Publisher) GetSequence() uint64 {
	return p.sequence.Load()
}

// GetFailureCount returns the current consecutive failure count (for testing)
func (p *Publisher) GetFailureCount() uint64 {
	return p.failureCount.Load()
}

// incrementMetric increments a metric counter
// This is a placeholder - in production, this would integrate with Prometheus/metrics system
func incrementMetric(name string) {
	// TODO: Integrate with actual metrics system
	logger.Info("[METRICS]", "metric", name, "operation", "increment")
}
