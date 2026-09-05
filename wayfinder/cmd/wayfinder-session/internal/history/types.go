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
	EventTypePhaseStarted     = "wayfinder.phase.started"
	EventTypePhaseCompleted   = "wayfinder.phase.completed"
	EventTypeValidationFailed = "validation.failed"
	EventTypeRewind           = "rewind"
	EventTypeSessionCompleted = "session.completed"
	EventTypeSessionAborted   = "session.aborted"
)

const (
	// HistoryFilename is the name of the history log file.
	HistoryFilename = "WAYFINDER-HISTORY.jsonl"
	// LegacyHistoryFilename is retained only for lossless on-disk migration.
	LegacyHistoryFilename = "WAYFINDER-HISTORY.md"
)
