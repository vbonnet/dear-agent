package consolidation

import (
	"context"
	"time"

	"github.com/vbonnet/dear-agent/internal/telemetry"
)

// Memory operation event types for telemetry
const (
	EventMemoryStored    = "memory_stored"
	EventMemoryRetrieved = "memory_retrieved"
	EventMemoryUpdated   = "memory_updated"
	EventMemoryDeleted   = "memory_deleted"
	EventArtifactStored  = "artifact_stored"
	EventArtifactFetched = "artifact_fetched"
	EventArtifactDeleted = "artifact_deleted"
	EventSessionPersist  = "session_persisted"
)

// TelemetryRecorder is an interface for recording telemetry events
// This matches the interface defined in core/pkg/eventbus/telemetry.go
type TelemetryRecorder interface {
	// Record records a telemetry event with severity level
	Record(eventType string, agent string, level telemetry.Level, data map[string]interface{}) error
}

// MemoryEventData contains metadata for memory operation events
type MemoryEventData struct {
	Provider   string        `json:"provider"`
	Namespace  []string      `json:"namespace"`
	MemoryID   string        `json:"memory_id,omitempty"`
	MemoryType MemoryType    `json:"memory_type,omitempty"`
	Latency    time.Duration `json:"latency_ms"` // in milliseconds
	Success    bool          `json:"success"`
	ErrorMsg   string        `json:"error_msg,omitempty"`
	ResultSize int           `json:"result_size,omitempty"` // For retrieve operations
}

// RecordMemoryEvent is a helper to emit memory operation telemetry events
func RecordMemoryEvent(ctx context.Context, recorder TelemetryRecorder, eventType string, data MemoryEventData) {
	if recorder == nil {
		return // No telemetry recorder configured
	}

	// Convert MemoryEventData to map for telemetry
	eventData := map[string]interface{}{
		"provider":   data.Provider,
		"namespace":  data.Namespace,
		"latency_ms": data.Latency.Milliseconds(),
		"success":    data.Success,
	}

	if data.MemoryID != "" {
		eventData["memory_id"] = data.MemoryID
	}
	if data.MemoryType != "" {
		eventData["memory_type"] = string(data.MemoryType)
	}
	if data.ErrorMsg != "" {
		eventData["error_msg"] = data.ErrorMsg
	}
	if data.ResultSize > 0 {
		eventData["result_size"] = data.ResultSize
	}

	// Record with empty agent (provider doesn't know which agent is calling)
	// The caller can pass agent info through context if needed
	level := telemetry.LevelInfo
	if !data.Success {
		level = telemetry.LevelError
	}
	_ = recorder.Record(eventType, "", level, eventData)
}

// telemetryKey is the context key for TelemetryRecorder
type telemetryKey struct{}

// GetTelemetryRecorder extracts TelemetryRecorder from context
// Returns nil if not present
func GetTelemetryRecorder(ctx context.Context) TelemetryRecorder {
	if recorder, ok := ctx.Value(telemetryKey{}).(TelemetryRecorder); ok {
		return recorder
	}
	return nil
}

// WithTelemetryRecorder adds TelemetryRecorder to context
func WithTelemetryRecorder(ctx context.Context, recorder TelemetryRecorder) context.Context {
	return context.WithValue(ctx, telemetryKey{}, recorder)
}
