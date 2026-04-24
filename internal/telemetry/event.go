package telemetry

import (
	"log/slog"
	"time"
)

// Level represents event severity level.
// Uses Go stdlib slog.Level for ecosystem compatibility.
type Level = slog.Level

const (
	// LevelInfo represents informational events (normal operations).
	LevelInfo = slog.LevelInfo // 0

	// LevelWarn represents warning events (degraded performance, non-critical issues).
	LevelWarn = slog.LevelWarn // 4

	// LevelError represents error events (operation failures).
	LevelError = slog.LevelError // 8
)

// LevelCritical represents critical system failures requiring immediate attention.
// This is a custom level higher than ERROR.
const LevelCritical Level = 12

// Event represents a single telemetry event
type Event struct {
	// Event ID (optional, for tracking)
	ID string `json:"id,omitempty"`

	// Event timestamp
	Timestamp time.Time `json:"timestamp"`

	// Event type (engram_loaded, plugin_executed, ecphory_query, etc.)
	Type string `json:"type"`

	// Agent platform
	Agent string `json:"agent"`

	// Severity level (INFO, WARN, ERROR, CRITICAL)
	Level Level `json:"level"`

	// Schema version (for backward compatibility)
	// Defaults to "1.0.0" if not set (S7 P1.15 fix)
	SchemaVersion string `json:"schema_version,omitempty"`

	// Event-specific data
	Data map[string]interface{} `json:"data,omitempty"`
}

// EventType constants for common event types
const (
	EventEngramLoaded     = "engram_loaded"
	EventPluginExecuted   = "plugin_executed"
	EventEcphoryQuery     = "ecphory_query"
	EventReflectionSaved  = "reflection_saved"
	EventConfigLoaded     = "config_loaded"
	EventEventBusPublish  = "eventbus_publish"
	EventEventBusResponse = "eventbus_response"

	// Telemetry and benchmarking events (S8 Group 1)
	EventEcphoryAuditCompleted    = "ecphory_audit_completed"
	EventPersonaReviewCompleted   = "persona_review_completed"
	EventPhaseTransitionCompleted = "phase_transition_completed"
	EventAgentLaunch              = "agent_launch"
)
