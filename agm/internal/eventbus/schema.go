package eventbus

import (
	"encoding/json"
	"fmt"
	"time"
)

// EventType represents the type of event
type EventType string

const (
	// EventSessionStuck indicates a session is stuck and requires intervention
	EventSessionStuck EventType = "session.stuck"

	// EventSessionEscalated indicates a session has been escalated
	EventSessionEscalated EventType = "session.escalated"

	// EventSessionRecovered indicates a session has recovered from stuck state
	EventSessionRecovered EventType = "session.recovered"

	// EventSessionStateChange indicates a session state transition
	EventSessionStateChange EventType = "session.state_change"

	// EventSessionCompleted indicates a session has completed successfully
	EventSessionCompleted EventType = "session.completed"

	// EventStallDetected indicates a stall condition was detected by the stall detector
	EventStallDetected EventType = "stall.detected"

	// EventStallRecovered indicates a stall was successfully recovered
	EventStallRecovered EventType = "stall.recovered"

	// EventStallEscalated indicates a stall recovery failed and was escalated
	EventStallEscalated EventType = "stall.escalated"
)

// Event represents a generic event in the system
type Event struct {
	Type      EventType       `json:"type"`       // Event type discriminator
	Timestamp time.Time       `json:"timestamp"`  // When the event occurred
	SessionID string          `json:"session_id"` // Session that triggered the event
	Payload   json.RawMessage `json:"payload"`    // Type-specific payload
}

// SessionStuckPayload contains data for session.stuck events
type SessionStuckPayload struct {
	Reason      string        `json:"reason"`                // Why the session is stuck
	Duration    time.Duration `json:"duration_seconds"`      // How long it's been stuck
	LastOutput  string        `json:"last_output,omitempty"` // Last output from session
	Suggestions []string      `json:"suggestions,omitempty"` // Suggested actions
}

// SessionEscalatedPayload contains data for session.escalated events
type SessionEscalatedPayload struct {
	EscalationType string    `json:"escalation_type"` // Type: error, prompt, warning
	Pattern        string    `json:"pattern"`         // Pattern that triggered escalation
	Line           string    `json:"line"`            // The actual line that triggered
	LineNumber     int       `json:"line_number"`     // Line number in output
	DetectedAt     time.Time `json:"detected_at"`     // When escalation was detected
	Description    string    `json:"description"`     // Human-readable description
	Severity       string    `json:"severity"`        // Severity: critical, high, medium, low
}

// SessionRecoveredPayload contains data for session.recovered events
type SessionRecoveredPayload struct {
	PreviousState string        `json:"previous_state"` // State before recovery
	RecoveryTime  time.Duration `json:"recovery_time"`  // Time taken to recover
	Action        string        `json:"action"`         // Action that led to recovery
}

// SessionStateChangePayload contains data for session.state_change events
type SessionStateChangePayload struct {
	OldState string `json:"old_state"` // Previous state
	NewState string `json:"new_state"` // New state
	Reason   string `json:"reason"`    // Reason for state change
}

// SessionCompletedPayload contains data for session.completed events
type SessionCompletedPayload struct {
	ExitCode     int           `json:"exit_code"`             // Exit code from session
	Duration     time.Duration `json:"duration_seconds"`      // Total session duration
	MessageCount int           `json:"message_count"`         // Total messages exchanged
	TokensUsed   int           `json:"tokens_used,omitempty"` // Total tokens used
	FinalState   string        `json:"final_state"`           // Final session state
}

// StallDetectedPayload contains data for stall.detected events
type StallDetectedPayload struct {
	StallType string        `json:"stall_type"` // "permission_prompt" | "no_commit" | "error_loop"
	Session   string        `json:"session"`    // Session name
	Duration  time.Duration `json:"duration"`   // How long stalled
	Details   string        `json:"details"`    // Evidence or description
	Severity  string        `json:"severity"`   // "warning" | "critical"
}

// StallRecoveredPayload contains data for stall.recovered events
type StallRecoveredPayload struct {
	StallType      string        `json:"stall_type"`      // Original stall type
	Session        string        `json:"session"`          // Session name
	RecoveryAction string        `json:"recovery_action"`  // Action taken
	Duration       time.Duration `json:"duration"`         // Stall duration before recovery
}

// StallEscalatedPayload contains data for stall.escalated events
type StallEscalatedPayload struct {
	StallType    string `json:"stall_type"`    // Original stall type
	Session      string `json:"session"`       // Session name
	Reason       string `json:"reason"`        // Why escalation occurred
	AttemptCount int    `json:"attempt_count"` // Number of recovery attempts before escalation
}

// ValidateEventType checks if an event type is valid
func ValidateEventType(eventType EventType) error {
	switch eventType {
	case EventSessionStuck,
		EventSessionEscalated,
		EventSessionRecovered,
		EventSessionStateChange,
		EventSessionCompleted,
		EventStallDetected,
		EventStallRecovered,
		EventStallEscalated:
		return nil
	default:
		return fmt.Errorf("invalid event type: %s", eventType)
	}
}

// NewEvent creates a new event with the given type and payload
func NewEvent(eventType EventType, sessionID string, payload any) (*Event, error) {
	// Validate event type
	if err := ValidateEventType(eventType); err != nil {
		return nil, err
	}

	// Validate session ID
	if sessionID == "" {
		return nil, fmt.Errorf("session_id cannot be empty")
	}

	// Marshal payload to JSON
	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal payload: %w", err)
	}

	return &Event{
		Type:      eventType,
		Timestamp: time.Now(),
		SessionID: sessionID,
		Payload:   payloadBytes,
	}, nil
}

// ParsePayload parses the event payload into the given type
func (e *Event) ParsePayload(v any) error {
	if err := json.Unmarshal(e.Payload, v); err != nil {
		return fmt.Errorf("failed to parse payload: %w", err)
	}
	return nil
}

// Validate checks if the event is well-formed
func (e *Event) Validate() error {
	// Validate event type
	if err := ValidateEventType(e.Type); err != nil {
		return err
	}

	// Validate session ID
	if e.SessionID == "" {
		return fmt.Errorf("session_id cannot be empty")
	}

	// Validate timestamp
	if e.Timestamp.IsZero() {
		return fmt.Errorf("timestamp cannot be zero")
	}

	// Validate payload is valid JSON
	if len(e.Payload) > 0 {
		var temp any
		if err := json.Unmarshal(e.Payload, &temp); err != nil {
			return fmt.Errorf("invalid payload JSON: %w", err)
		}
	}

	return nil
}

// ExampleEvents returns example events for each type (useful for documentation and testing)
func ExampleEvents() map[EventType]*Event {
	examples := make(map[EventType]*Event)

	// session.stuck example
	stuckEvent, _ := NewEvent(EventSessionStuck, "session-abc123", SessionStuckPayload{
		Reason:      "No output for 5 minutes",
		Duration:    5 * time.Minute,
		LastOutput:  "Waiting for user input...",
		Suggestions: []string{"Check if session is waiting for input", "Verify network connectivity"},
	})
	examples[EventSessionStuck] = stuckEvent

	// session.escalated example
	escalatedEvent, _ := NewEvent(EventSessionEscalated, "session-def456", SessionEscalatedPayload{
		EscalationType: "error",
		Pattern:        "(?i)error:",
		Line:           "Error: Permission denied",
		LineNumber:     42,
		DetectedAt:     time.Now(),
		Description:    "Permission error detected",
		Severity:       "high",
	})
	examples[EventSessionEscalated] = escalatedEvent

	// session.recovered example
	recoveredEvent, _ := NewEvent(EventSessionRecovered, "session-ghi789", SessionRecoveredPayload{
		PreviousState: "stuck",
		RecoveryTime:  2 * time.Minute,
		Action:        "User provided input",
	})
	examples[EventSessionRecovered] = recoveredEvent

	// session.state_change example
	stateChangeEvent, _ := NewEvent(EventSessionStateChange, "session-jkl012", SessionStateChangePayload{
		OldState: "active",
		NewState: "stopped",
		Reason:   "User requested stop",
	})
	examples[EventSessionStateChange] = stateChangeEvent

	// session.completed example
	completedEvent, _ := NewEvent(EventSessionCompleted, "session-mno345", SessionCompletedPayload{
		ExitCode:     0,
		Duration:     30 * time.Minute,
		MessageCount: 42,
		TokensUsed:   15000,
		FinalState:   "archived",
	})
	examples[EventSessionCompleted] = completedEvent

	// stall.detected example
	stallDetectedEvent, _ := NewEvent(EventStallDetected, "worker-1", StallDetectedPayload{
		StallType: "permission_prompt",
		Session:   "worker-1",
		Duration:  10 * time.Minute,
		Details:   "Permission dialog open for 10m",
		Severity:  "critical",
	})
	examples[EventStallDetected] = stallDetectedEvent

	// stall.recovered example
	stallRecoveredEvent, _ := NewEvent(EventStallRecovered, "worker-1", StallRecoveredPayload{
		StallType:      "permission_prompt",
		Session:        "worker-1",
		RecoveryAction: "alert_orchestrator",
		Duration:       10 * time.Minute,
	})
	examples[EventStallRecovered] = stallRecoveredEvent

	// stall.escalated example
	stallEscalatedEvent, _ := NewEvent(EventStallEscalated, "worker-2", StallEscalatedPayload{
		StallType:    "error_loop",
		Session:      "worker-2",
		Reason:       "max retries exceeded after 3 attempts",
		AttemptCount: 3,
	})
	examples[EventStallEscalated] = stallEscalatedEvent

	return examples
}
