// Package analytics provides session tracking and telemetry for Wayfinder.
//
// SessionTracker integrates with EventBus to record phase lifecycle events
// that enable session analytics (duration, cost, bottlenecks).
package analytics

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/vbonnet/dear-agent/pkg/eventbus"
)

// SessionTracker tracks Wayfinder session lifecycle and publishes events to EventBus.
//
// Usage:
//
//	tracker := analytics.NewSessionTracker(eventBus)
//	tracker.StartSession("/path/to/project")
//	tracker.StartPhase("D1")
//	// ... do work ...
//	tracker.CompletePhase("D1", "success", metadata)
//	tracker.EndSession("success")
type SessionTracker struct {
	sessionID        string
	sessionStartTime time.Time
	eventBus         *eventbus.LocalBus
	currentPhase     string
	phaseStartTime   time.Time
}

// NewSessionTracker creates a new session tracker.
func NewSessionTracker(bus *eventbus.LocalBus) *SessionTracker {
	return &SessionTracker{
		sessionID:        uuid.New().String(),
		sessionStartTime: time.Now(),
		eventBus:         bus,
	}
}

// SessionID returns the current session ID.
func (st *SessionTracker) SessionID() string {
	return st.sessionID
}

// StartSession publishes a session.started event.
//
// Parameters:
//   - projectPath: Working directory for the session
func (st *SessionTracker) StartSession(projectPath string) error {
	event := eventbus.NewEvent("wayfinder.session.started", "wayfinder", map[string]interface{}{
		"session_id":   st.sessionID,
		"project_path": projectPath,
		"event_topic":  "wayfinder.session.started",
	})

	return st.publishEvent(event)
}

// StartPhase publishes a phase.started event.
//
// Parameters:
//   - phase: Phase identifier (e.g., "D1", "D2", "S6")
func (st *SessionTracker) StartPhase(phase string) error {
	st.phaseStartTime = time.Now()

	event := eventbus.NewEvent("wayfinder.phase.started", "wayfinder", map[string]interface{}{
		"session_id":  st.sessionID,
		"phase":       phase,
		"event_topic": "wayfinder.phase.started",
	})

	st.currentPhase = phase
	return st.publishEvent(event)
}

// CompletePhase publishes a phase.completed event.
//
// Parameters:
//   - phase: Phase identifier (should match StartPhase call)
//   - outcome: Phase result ("success", "failure", "partial", "skipped")
//   - metadata: Optional metadata (engrams loaded, tokens, etc.)
func (st *SessionTracker) CompletePhase(phase string, outcome string, metadata map[string]interface{}) error {
	endTime := time.Now()
	duration := endTime.Sub(st.phaseStartTime)

	data := map[string]interface{}{
		"session_id":  st.sessionID,
		"phase":       phase,
		"duration_ms": duration.Milliseconds(),
		"outcome":     outcome,
		"event_topic": "wayfinder.phase.completed",
	}

	// Merge metadata if provided (files_modified, lines_added, etc.)
	if metadata != nil {
		for k, v := range metadata {
			data[k] = v
		}
	}

	event := eventbus.NewEvent("wayfinder.phase.completed", "wayfinder", data)
	return st.publishEvent(event)
}

// EndSession publishes a session.completed event.
//
// Parameters:
//   - outcome: Session result ("success", "failed", "abandoned")
func (st *SessionTracker) EndSession(outcome string) error {
	endTime := time.Now()
	totalDuration := endTime.Sub(st.sessionStartTime)

	event := eventbus.NewEvent("wayfinder.session.completed", "wayfinder", map[string]interface{}{
		"session_id":        st.sessionID,
		"total_duration_ms": totalDuration.Milliseconds(),
		"status":            outcome,
		"event_topic":       "wayfinder.session.completed",
	})

	return st.publishEvent(event)
}

// publishEvent publishes an event to EventBus and handles errors gracefully.
//
// Analytics events are non-critical - we log errors but don't fail the session.
func (st *SessionTracker) publishEvent(event *eventbus.Event) error {
	if st.eventBus == nil {
		// No EventBus configured - analytics disabled
		return nil
	}

	// Publish event - EventBus automatically records to telemetry
	err := st.eventBus.Publish(context.Background(), event)
	if err != nil {
		// TODO: Log error (don't fail session)
		// For now, swallow error - analytics is non-critical
		return err
	}

	return nil
}
