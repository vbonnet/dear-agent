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
//	tracker.StartPhase("PROBLEM")
//	// ... do work ...
//	tracker.CompletePhase("PROBLEM", "success", metadata)
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
	event := eventbus.NewEvent("wayfinder.session.started", "wayfinder", map[string]any{
		"session_id":   st.sessionID,
		"project_path": projectPath,
		"event_topic":  "wayfinder.session.started",
	})

	return st.publishEvent(event)
}

// StartPhase publishes a wayfinder.phase.started event.
//
// Parameters:
//   - phase: Canonical phase identifier (for example, "PROBLEM", "RESEARCH", "PLAN")
func (st *SessionTracker) StartPhase(phase string) error {
	st.phaseStartTime = time.Now()

	event := eventbus.NewEvent(eventbus.TypeWayfinderPhaseStarted, "wayfinder", map[string]any{
		"session_id":  st.sessionID,
		"phase":       phase,
		"event_topic": eventbus.TypeWayfinderPhaseStarted,
	})

	st.currentPhase = phase
	return st.publishEvent(event)
}

// CompletePhase publishes a wayfinder.phase.completed event.
//
// Parameters:
//   - phase: Phase identifier (should match StartPhase call)
//   - outcome: Phase result ("success", "failure", "partial", "skipped")
//   - metadata: Optional metadata (engrams loaded, tokens, etc.)
func (st *SessionTracker) CompletePhase(phase string, outcome string, metadata map[string]any) error {
	endTime := time.Now()
	duration := endTime.Sub(st.phaseStartTime)

	data := map[string]any{
		"session_id":  st.sessionID,
		"phase":       phase,
		"duration_ms": duration.Milliseconds(),
		"outcome":     outcome,
		"event_topic": eventbus.TypeWayfinderPhaseCompleted,
	}

	// Merge metadata if provided (files_modified, lines_added, etc.)
	for k, v := range metadata {
		data[k] = v
	}

	event := eventbus.NewEvent(eventbus.TypeWayfinderPhaseCompleted, "wayfinder", data)
	return st.publishEvent(event)
}

// EndSession publishes a session.completed event.
//
// Parameters:
//   - outcome: Session result ("success", "failed", "abandoned")
func (st *SessionTracker) EndSession(outcome string) error {
	endTime := time.Now()
	totalDuration := endTime.Sub(st.sessionStartTime)

	event := eventbus.NewEvent("wayfinder.session.completed", "wayfinder", map[string]any{
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
