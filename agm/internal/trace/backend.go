package trace

import (
	"context"
	"time"

	"github.com/vbonnet/dear-agent/agm/internal/eventbus"
)

// TraceRecord is the canonical representation of an event for storage/export.
type TraceRecord struct {
	Timestamp time.Time              `json:"timestamp"`
	EventType eventbus.EventType     `json:"event_type"`
	SessionID string                 `json:"session_id"`
	Payload   map[string]interface{} `json:"payload"`
}

// Backend persists or exports TraceRecords.
type Backend interface {
	// Write persists a single trace record.
	Write(ctx context.Context, rec *TraceRecord) error

	// Flush ensures all buffered records are persisted.
	Flush(ctx context.Context) error

	// Close releases resources held by the backend.
	Close() error
}

// RecordFromEvent converts an eventbus.Event to a TraceRecord.
func RecordFromEvent(ev *eventbus.Event) (*TraceRecord, error) {
	var payload map[string]interface{}
	if len(ev.Payload) > 0 {
		if err := ev.ParsePayload(&payload); err != nil {
			return nil, err
		}
	}
	return &TraceRecord{
		Timestamp: ev.Timestamp,
		EventType: ev.Type,
		SessionID: ev.SessionID,
		Payload:   payload,
	}, nil
}
