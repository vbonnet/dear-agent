package history

import "time"

// Event represents a single event in the history log
type Event struct {
	Timestamp time.Time              `json:"timestamp"`
	Type      string                 `json:"type"`
	Phase     string                 `json:"phase,omitempty"`
	Data      map[string]interface{} `json:"data,omitempty"`
}

// EventType constants
const (
	EventTypeSessionStarted   = "session.started"
	EventTypePhaseStarted     = "phase.started"
	EventTypePhaseCompleted   = "phase.completed"
	EventTypeValidationFailed = "validation.failed"
	EventTypeRewind           = "rewind"
	EventTypeSessionCompleted = "session.completed"
	EventTypeSessionAborted   = "session.aborted"
)

// HistoryFilename is the name of the history log file
const HistoryFilename = "WAYFINDER-HISTORY.md"
