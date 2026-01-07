package telemetry

import "time"

// ExecutionEvent represents a single event in the execution log
type ExecutionEvent struct {
	Timestamp time.Time              `json:"timestamp"`
	BeadID    string                 `json:"bead_id"`
	Event     string                 `json:"event"` // claim, execute, validate_s8, validate_s9, complete, escalate, error
	Details   map[string]interface{} `json:"details,omitempty"`
}

// RoadmapEntry represents a summary entry for the ROADMAP.md
type RoadmapEntry struct {
	Section string   `json:"section"` // ready, in_progress, blocked, completed
	Count   int      `json:"count"`
	Items   []string `json:"items,omitempty"`
}
