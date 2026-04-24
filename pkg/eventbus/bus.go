// Package eventbus implements a unified event bus for inter-component communication.
//
// The event bus supports four channels with different delivery guarantees:
//   - telemetry: fire-and-forget, best-effort delivery
//   - notification: durable, persisted to JSONL before sink dispatch
//   - audit: durable, persisted to JSONL before sink dispatch
//   - heartbeat: fire-and-forget, best-effort delivery
//
// Events are routed by type prefix (e.g., "telemetry.*", "audit.session.end").
// Subscribers can use wildcard patterns for flexible matching.
//
// Sinks receive dispatched events and can write to files, logs, or external systems.
package eventbus

import (
	"context"
	"log/slog"
	"time"

	"github.com/google/uuid"
)

// Channel represents an event delivery channel with specific guarantees.
type Channel string

const (
	// ChannelTelemetry is for metrics and performance data (fire-and-forget).
	ChannelTelemetry Channel = "telemetry"
	// ChannelNotification is for user-facing notifications (durable).
	ChannelNotification Channel = "notification"
	// ChannelAudit is for compliance and audit trail events (durable).
	ChannelAudit Channel = "audit"
	// ChannelHeartbeat is for liveness signals (fire-and-forget).
	ChannelHeartbeat Channel = "heartbeat"
)

// IsDurable returns true if the channel requires persistence before dispatch.
func (c Channel) IsDurable() bool {
	return c == ChannelNotification || c == ChannelAudit
}

// ChannelFromType extracts the channel from an event type string.
// Event types use the format "channel.subtype" (e.g., "telemetry.agent.launch").
func ChannelFromType(eventType string) Channel {
	for i := 0; i < len(eventType); i++ {
		if eventType[i] == '.' {
			return Channel(eventType[:i])
		}
	}
	return Channel(eventType)
}

// Common event type constants.
const (
	// Telemetry events
	TypeTelemetryAgentLaunch  = "telemetry.agent.launch"
	TypeTelemetryPluginExec   = "telemetry.plugin.executed"
	TypeTelemetryBusPublish   = "telemetry.bus.publish"
	TypeTelemetryBusResponse  = "telemetry.bus.response"
	TypeTelemetryBusPanic     = "telemetry.bus.panic"
	TypeTelemetryBusLimitHit  = "telemetry.bus.limit_hit"
	TypeTelemetryBusSizeLimit = "telemetry.bus.size_limit"

	// Notification events
	TypeNotificationPhaseComplete = "notification.phase.complete"
	TypeNotificationSessionEnd    = "notification.session.end"

	// Audit events
	TypeAuditConfigChange = "audit.config.change"
	TypeAuditSessionStart = "audit.session.start"
	TypeAuditSessionEnd   = "audit.session.end"

	// Heartbeat events
	TypeHeartbeatAgent   = "heartbeat.agent"
	TypeHeartbeatSession = "heartbeat.session"
)

// Event represents a single event on the bus.
type Event struct {
	ID        string                 `json:"id"`
	Type      string                 `json:"type"`
	Channel   Channel                `json:"channel"`
	Source    string                 `json:"source"`
	Timestamp time.Time              `json:"timestamp"`
	Level     slog.Level             `json:"level"`
	Data      map[string]interface{} `json:"data,omitempty"`
}

// NewEvent creates a new event, deriving the channel from the type prefix.
func NewEvent(eventType string, source string, data map[string]interface{}) *Event {
	return &Event{
		ID:        uuid.New().String(),
		Type:      eventType,
		Channel:   ChannelFromType(eventType),
		Source:    source,
		Timestamp: time.Now(),
		Level:     slog.LevelInfo,
		Data:      data,
	}
}

// NewEventWithLevel creates a new event with an explicit severity level.
func NewEventWithLevel(eventType string, source string, level slog.Level, data map[string]interface{}) *Event {
	e := NewEvent(eventType, source, data)
	e.Level = level
	return e
}

// Response represents a subscriber's reply to an event.
type Response struct {
	ID        string                 `json:"id"`
	EventID   string                 `json:"event_id"`
	Responder string                 `json:"responder"`
	Timestamp time.Time              `json:"timestamp"`
	Data      map[string]interface{} `json:"data,omitempty"`
	Error     string                 `json:"error,omitempty"`
}

// NewResponse creates a new response to an event.
func NewResponse(eventID string, responder string, data map[string]interface{}) *Response {
	return &Response{
		ID:        uuid.New().String(),
		EventID:   eventID,
		Responder: responder,
		Timestamp: time.Now(),
		Data:      data,
	}
}

// Handler processes an event and optionally returns a response.
type Handler func(ctx context.Context, event *Event) (*Response, error)

// Filter controls which events a sink or subscriber receives.
type Filter struct {
	// Channels to accept (empty = all channels).
	Channels []Channel
	// Types to match (supports wildcards: "telemetry.*", "*"). Empty = all types.
	Types []string
	// MinLevel filters events below this severity level.
	MinLevel slog.Level
}

// Matches returns true if the event passes the filter.
func (f *Filter) Matches(event *Event) bool {
	if len(f.Channels) > 0 {
		found := false
		for _, ch := range f.Channels {
			if ch == event.Channel {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	if len(f.Types) > 0 {
		found := false
		for _, pattern := range f.Types {
			if matchesPattern(event.Type, pattern) {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	if event.Level < f.MinLevel {
		return false
	}
	return true
}

// Sink receives events after dispatch. Implementations write to files, logs, etc.
type Sink interface {
	// Name returns a human-readable sink identifier.
	Name() string
	// HandleEvent processes a dispatched event.
	HandleEvent(ctx context.Context, event *Event) error
	// Close releases sink resources.
	Close() error
}

// Bus is the interface for event bus implementations.
type Bus interface {
	// Emit publishes an event to all matching subscribers and sinks.
	Emit(ctx context.Context, event *Event) error
	// Subscribe registers a handler for events matching the given pattern.
	// Returns a subscriber ID for use with Unsubscribe.
	Subscribe(pattern string, subscriber string, handler Handler) string
	// Unsubscribe removes a handler by subscriber ID.
	Unsubscribe(pattern string, subscriberID string) error
	// AddSink registers a sink with an optional filter.
	AddSink(sink Sink, filter *Filter)
	// Close releases all resources.
	Close() error
}

// TelemetryRecorder is maintained for backward compatibility with consumers
// that pass a telemetry recorder to NewBus. New code should use Sinks instead.
type TelemetryRecorder interface {
	Record(eventType string, agent string, level slog.Level, data map[string]interface{}) error
}

// NewBus creates a new LocalBus. The telemetry parameter is accepted for
// backward compatibility but ignored — use AddSink for telemetry output.
func NewBus(_ TelemetryRecorder) *LocalBus {
	return NewLocalBus()
}

// NewBusWithLimits creates a new LocalBus with custom limits.
// The telemetry parameter is accepted for backward compatibility but ignored.
func NewBusWithLimits(_ TelemetryRecorder, maxSubscribersPerTopic, maxEventSize int) *LocalBus {
	return NewLocalBus(
		WithMaxSubscribersPerTopic(maxSubscribersPerTopic),
		WithMaxEventSize(maxEventSize),
	)
}

// EventResponse is maintained for backward compatibility.
type EventResponse struct {
	Subscriber string
	Timestamp  time.Time
	Action     string
	Success    bool
	Error      string
	Metadata   map[string]interface{}
}
