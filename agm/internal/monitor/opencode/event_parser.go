package opencode

import (
	"encoding/json"
	"fmt"

	"github.com/vbonnet/dear-agent/agm/internal/logging"
)

var logger = logging.DefaultLogger()

// OpenCodeEvent represents a raw event from OpenCode SSE stream.
type OpenCodeEvent struct {
	Type       string                 `json:"type"`
	Timestamp  int64                  `json:"timestamp"`
	Properties map[string]interface{} `json:"properties"`
}

// EventParser parses OpenCode SSE events and maps them to AGM state transitions.
type EventParser struct{}

// NewEventParser creates a new EventParser instance.
func NewEventParser() *EventParser {
	return &EventParser{}
}

// Parse parses OpenCode event JSON and maps it to an AGM event.
func (p *EventParser) Parse(data []byte) (*AGMEvent, error) {
	// 1. Unmarshal JSON
	var rawEvent OpenCodeEvent
	if err := json.Unmarshal(data, &rawEvent); err != nil {
		return nil, fmt.Errorf("invalid JSON: %w", err)
	}

	// 2. Validate schema
	if err := p.validate(rawEvent); err != nil {
		return nil, fmt.Errorf("validation failed: %w", err)
	}

	// 3. Map event type to state
	agmState := p.mapState(rawEvent.Type)

	// 4. Extract metadata
	metadata := p.extractMetadata(rawEvent)

	return &AGMEvent{
		State:     agmState,
		Timestamp: rawEvent.Timestamp,
		Metadata:  metadata,
	}, nil
}

// mapState maps OpenCode event type to AGM state.
func (p *EventParser) mapState(eventType string) string {
	switch eventType {
	case "permission.asked":
		return "AWAITING_PERMISSION"
	case "tool.execute.before":
		return "WORKING"
	case "tool.execute.after":
		return "IDLE"
	case "session.created":
		return "DONE"
	case "session.closed":
		return "TERMINATED"
	default:
		logger.Warn("Unknown OpenCode event type, defaulting to WORKING", "event_type", eventType)
		return "WORKING" // Safe default
	}
}

// extractMetadata extracts event-specific metadata from OpenCode event.
func (p *EventParser) extractMetadata(event OpenCodeEvent) map[string]interface{} {
	meta := map[string]interface{}{
		"event_type": event.Type,
	}

	// Extract permission metadata
	if perm, ok := event.Properties["permission"].(map[string]interface{}); ok {
		if id, ok := perm["id"].(string); ok {
			meta["permission_id"] = id
		}
		if action, ok := perm["action"].(string); ok {
			meta["action"] = action
		}
		if path, ok := perm["path"].(string); ok {
			meta["path"] = path
		}
	}

	// Extract tool metadata
	if tool, ok := event.Properties["tool"].(string); ok {
		meta["tool_name"] = tool
	}

	// Extract session metadata
	if sessionID, ok := event.Properties["session_id"].(string); ok {
		meta["session_id"] = sessionID
	}

	return meta
}

// validate checks that the event has required fields.
func (p *EventParser) validate(event OpenCodeEvent) error {
	if event.Type == "" {
		return fmt.Errorf("missing event type")
	}
	if event.Timestamp == 0 {
		return fmt.Errorf("missing timestamp")
	}
	return nil
}
