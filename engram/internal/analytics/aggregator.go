// Package analytics provides analytics-related functionality.
package analytics

import (
	"fmt"
	"sort"
	"time"
)

// Aggregator converts raw events into Session structs
type Aggregator struct {
	waitDetector *WaitDetector
}

// NewAggregator creates a new aggregator with default settings
func NewAggregator() *Aggregator {
	return &Aggregator{
		waitDetector: NewWaitDetector(),
	}
}

// AggregateSessions converts parsed events into Session objects
func (a *Aggregator) AggregateSessions(eventsBySession map[string][]ParsedEvent) ([]Session, error) {
	sessions := make([]Session, 0, len(eventsBySession))

	for sessionID, events := range eventsBySession {
		session, err := a.AggregateSession(sessionID, events)
		if err != nil {
			// Log warning but continue processing other sessions
			fmt.Printf("Warning: Failed to aggregate session %s: %v\n", sessionID, err)
			continue
		}
		sessions = append(sessions, *session)
	}

	// Sort sessions by start time (newest first)
	sort.Slice(sessions, func(i, j int) bool {
		return sessions[i].StartTime.After(sessions[j].StartTime)
	})

	return sessions, nil
}

// AggregateSession converts events for a single session
func (a *Aggregator) AggregateSession(sessionID string, events []ParsedEvent) (*Session, error) {
	if len(events) == 0 {
		return nil, fmt.Errorf("no events for session %s", sessionID)
	}

	// Sort events by timestamp
	sort.Slice(events, func(i, j int) bool {
		return events[i].Timestamp.Before(events[j].Timestamp)
	})

	session := &Session{
		ID:     sessionID,
		Status: "incomplete", // Default, updated if we find session.completed
	}

	// Find session start/end events
	var sessionStartEvent *ParsedEvent
	var sessionEndEvent *ParsedEvent

	for i := range events {
		switch events[i].EventTopic {
		case "wayfinder.session.started":
			sessionStartEvent = &events[i]
		case "wayfinder.session.completed":
			sessionEndEvent = &events[i]
		}
	}

	if sessionStartEvent == nil {
		return nil, fmt.Errorf("session %s missing session.started event", sessionID)
	}

	session.StartTime = sessionStartEvent.Timestamp

	// Extract project path if available
	if projectPath, ok := sessionStartEvent.Data["project_path"].(string); ok {
		session.ProjectPath = projectPath
	}

	if sessionEndEvent != nil {
		session.EndTime = sessionEndEvent.Timestamp
		// Check status from event data
		if status, ok := sessionEndEvent.Data["status"].(string); ok {
			session.Status = status
		} else {
			session.Status = "completed"
		}
	} else {
		// No end event = session still in progress or crashed
		session.EndTime = time.Now()
		session.Status = "incomplete"
	}

	// Build phases from phase.started/completed events
	phases, err := a.buildPhases(events)
	if err != nil {
		return nil, err
	}
	session.Phases = phases

	// Calculate metrics
	session.Metrics = a.calculateMetrics(session)

	return session, nil
}

// buildPhases matches phase.started with phase.completed events
func (a *Aggregator) buildPhases(events []ParsedEvent) ([]Phase, error) {
	// Map of phase name → start event
	phaseStarts := make(map[string]*ParsedEvent)
	// Map of phase name → end event
	phaseEnds := make(map[string]*ParsedEvent)

	for i := range events {
		event := &events[i]
		if event.Phase == "" {
			// Not a phase event
			continue
		}

		switch event.EventTopic {
		case "wayfinder.phase.started":
			phaseStarts[event.Phase] = event
		case "wayfinder.phase.completed":
			phaseEnds[event.Phase] = event
		}
	}

	// Build Phase objects
	phases := []Phase{}
	for phaseName, startEvent := range phaseStarts {
		endEvent, hasEnd := phaseEnds[phaseName]

		phase := Phase{
			Name:      phaseName,
			StartTime: startEvent.Timestamp,
			Metadata:  make(map[string]interface{}),
		}

		if hasEnd {
			phase.EndTime = endEvent.Timestamp
			phase.Duration = endEvent.Timestamp.Sub(startEvent.Timestamp)

			// Extract metadata from end event (e.g., files_modified, lines_added)
			if endEvent.Data != nil {
				for key, value := range endEvent.Data {
					// Skip standard fields
					if key != "session_id" && key != "phase" && key != "event_topic" {
						phase.Metadata[key] = value
					}
				}
			}
		} else {
			// Phase not completed yet
			phase.EndTime = time.Now()
			phase.Duration = time.Since(startEvent.Timestamp)
		}

		phases = append(phases, phase)
	}

	// Sort phases by start time
	sort.Slice(phases, func(i, j int) bool {
		return phases[i].StartTime.Before(phases[j].StartTime)
	})

	return phases, nil
}

// calculateMetrics computes session metrics
func (a *Aggregator) calculateMetrics(session *Session) SessionMetrics {
	metrics := SessionMetrics{
		PhaseCount: len(session.Phases),
	}

	if len(session.Phases) == 0 {
		return metrics
	}

	// Calculate total duration
	metrics.TotalDuration = session.EndTime.Sub(session.StartTime)

	// Use WaitDetector to separate AI time vs. wait time
	aiTime, waitTime := a.waitDetector.DetectWaitTime(session.Phases)
	metrics.AITime = aiTime
	metrics.WaitTime = waitTime

	// TODO: Calculate cost if token data available
	// This would require extracting token counts from event metadata
	// For now, leave as 0 (will be implemented when token tracking is added)

	return metrics
}

// ComputeSummary calculates aggregate statistics across sessions
func (a *Aggregator) ComputeSummary(sessions []Session) SessionSummary {
	summary := SessionSummary{
		TotalSessions: len(sessions),
	}

	if len(sessions) == 0 {
		return summary
	}

	var totalDuration time.Duration
	var totalAITime time.Duration
	var totalWaitTime time.Duration
	var totalCost float64

	for _, session := range sessions {
		totalDuration += session.Metrics.TotalDuration
		totalAITime += session.Metrics.AITime
		totalWaitTime += session.Metrics.WaitTime
		totalCost += session.Metrics.EstimatedCost

		// Count completed vs. failed
		switch session.Status {
		case "completed", "success":
			summary.CompletedSessions++
		case "failed":
			summary.FailedSessions++
		}
	}

	summary.TotalDuration = totalDuration
	summary.TotalAITime = totalAITime
	summary.TotalWaitTime = totalWaitTime
	summary.TotalCost = totalCost

	// Calculate averages
	if summary.TotalSessions > 0 {
		summary.AverageDuration = totalDuration / time.Duration(summary.TotalSessions)
		summary.AverageCost = totalCost / float64(summary.TotalSessions)
	}

	return summary
}
