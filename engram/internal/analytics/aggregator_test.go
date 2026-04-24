package analytics

import (
	"testing"
	"time"
)

// TestAggregateSession_Complete tests aggregating a complete session
func TestAggregateSession_Complete(t *testing.T) {
	// Create mock events for a complete session
	events := []ParsedEvent{
		{
			EventTopic: "wayfinder.session.started",
			Timestamp:  parseTime("2025-12-09T10:00:00Z"),
			SessionID:  "test-session",
			Data: map[string]interface{}{
				"project_path": "/tmp/test/project",
			},
		},
		{
			EventTopic: "wayfinder.phase.started",
			Timestamp:  parseTime("2025-12-09T10:00:05Z"),
			SessionID:  "test-session",
			Phase:      "D1",
			Data:       map[string]interface{}{},
		},
		{
			EventTopic: "wayfinder.phase.completed",
			Timestamp:  parseTime("2025-12-09T10:15:00Z"),
			SessionID:  "test-session",
			Phase:      "D1",
			Data: map[string]interface{}{
				"files_modified": 5,
			},
		},
		{
			EventTopic: "wayfinder.session.completed",
			Timestamp:  parseTime("2025-12-09T10:20:00Z"),
			SessionID:  "test-session",
			Data: map[string]interface{}{
				"status": "success",
			},
		},
	}

	aggregator := NewAggregator()
	session, err := aggregator.AggregateSession("test-session", events)
	if err != nil {
		t.Fatalf("AggregateSession() failed: %v", err)
	}

	// Verify session fields
	if session.ID != "test-session" {
		t.Errorf("Expected session ID 'test-session', got '%s'", session.ID)
	}

	if session.ProjectPath != "/tmp/test/project" {
		t.Errorf("Expected project path '/tmp/test/project', got '%s'", session.ProjectPath)
	}

	if session.Status != "success" {
		t.Errorf("Expected status 'success', got '%s'", session.Status)
	}

	// Verify phases
	if len(session.Phases) != 1 {
		t.Fatalf("Expected 1 phase, got %d", len(session.Phases))
	}

	phase := session.Phases[0]
	if phase.Name != "D1" {
		t.Errorf("Expected phase name 'D1', got '%s'", phase.Name)
	}

	expectedDuration := 14*time.Minute + 55*time.Second // 10:00:05 → 10:15:00
	if phase.Duration != expectedDuration {
		t.Errorf("Expected phase duration %v, got %v", expectedDuration, phase.Duration)
	}

	// Verify metadata
	if filesModified, ok := phase.Metadata["files_modified"].(int); !ok || filesModified != 5 {
		t.Errorf("Expected files_modified=5, got %v", phase.Metadata["files_modified"])
	}

	// Verify metrics
	if session.Metrics.PhaseCount != 1 {
		t.Errorf("Expected phase count 1, got %d", session.Metrics.PhaseCount)
	}

	if session.Metrics.TotalDuration != 20*time.Minute {
		t.Errorf("Expected total duration 20min, got %v", session.Metrics.TotalDuration)
	}
}

// TestAggregateSession_Incomplete tests session missing end event
func TestAggregateSession_Incomplete(t *testing.T) {
	events := []ParsedEvent{
		{
			EventTopic: "wayfinder.session.started",
			Timestamp:  parseTime("2025-12-09T10:00:00Z"),
			SessionID:  "incomplete-session",
			Data:       map[string]interface{}{},
		},
		{
			EventTopic: "wayfinder.phase.started",
			Timestamp:  parseTime("2025-12-09T10:00:05Z"),
			SessionID:  "incomplete-session",
			Phase:      "D1",
			Data:       map[string]interface{}{},
		},
		// Missing phase.completed and session.completed
	}

	aggregator := NewAggregator()
	session, err := aggregator.AggregateSession("incomplete-session", events)
	if err != nil {
		t.Fatalf("AggregateSession() failed: %v", err)
	}

	// Verify status is incomplete
	if session.Status != "incomplete" {
		t.Errorf("Expected status 'incomplete', got '%s'", session.Status)
	}

	// Should still have 1 phase (even without end event)
	if len(session.Phases) != 1 {
		t.Errorf("Expected 1 phase, got %d", len(session.Phases))
	}
}

// TestAggregateSession_NoEvents tests error when no events
func TestAggregateSession_NoEvents(t *testing.T) {
	aggregator := NewAggregator()
	_, err := aggregator.AggregateSession("empty-session", []ParsedEvent{})
	if err == nil {
		t.Error("Expected error for session with no events, got nil")
	}
}

// TestAggregateSession_MultiplePhases tests session with multiple phases
func TestAggregateSession_MultiplePhases(t *testing.T) {
	events := []ParsedEvent{
		{EventTopic: "wayfinder.session.started", Timestamp: parseTime("2025-12-09T10:00:00Z"), SessionID: "multi-phase", Data: map[string]interface{}{}},
		{EventTopic: "wayfinder.phase.started", Timestamp: parseTime("2025-12-09T10:00:05Z"), SessionID: "multi-phase", Phase: "D1", Data: map[string]interface{}{}},
		{EventTopic: "wayfinder.phase.completed", Timestamp: parseTime("2025-12-09T10:15:00Z"), SessionID: "multi-phase", Phase: "D1", Data: map[string]interface{}{}},
		{EventTopic: "wayfinder.phase.started", Timestamp: parseTime("2025-12-09T10:15:05Z"), SessionID: "multi-phase", Phase: "D2", Data: map[string]interface{}{}},
		{EventTopic: "wayfinder.phase.completed", Timestamp: parseTime("2025-12-09T10:30:00Z"), SessionID: "multi-phase", Phase: "D2", Data: map[string]interface{}{}},
		{EventTopic: "wayfinder.session.completed", Timestamp: parseTime("2025-12-09T10:35:00Z"), SessionID: "multi-phase", Data: map[string]interface{}{"status": "success"}},
	}

	aggregator := NewAggregator()
	session, err := aggregator.AggregateSession("multi-phase", events)
	if err != nil {
		t.Fatalf("AggregateSession() failed: %v", err)
	}

	// Verify 2 phases
	if len(session.Phases) != 2 {
		t.Errorf("Expected 2 phases, got %d", len(session.Phases))
	}

	// Verify phases are sorted by start time
	if session.Phases[0].Name != "D1" || session.Phases[1].Name != "D2" {
		t.Errorf("Expected phases D1, D2 in order, got %s, %s", session.Phases[0].Name, session.Phases[1].Name)
	}

	// Verify phase count metric
	if session.Metrics.PhaseCount != 2 {
		t.Errorf("Expected phase count 2, got %d", session.Metrics.PhaseCount)
	}
}

// TestAggregateSessions_Multiple tests aggregating multiple sessions
func TestAggregateSessions_Multiple(t *testing.T) {
	eventsBySession := map[string][]ParsedEvent{
		"session-1": {
			{EventTopic: "wayfinder.session.started", Timestamp: parseTime("2025-12-09T10:00:00Z"), SessionID: "session-1", Data: map[string]interface{}{}},
			{EventTopic: "wayfinder.session.completed", Timestamp: parseTime("2025-12-09T10:30:00Z"), SessionID: "session-1", Data: map[string]interface{}{"status": "success"}},
		},
		"session-2": {
			{EventTopic: "wayfinder.session.started", Timestamp: parseTime("2025-12-09T11:00:00Z"), SessionID: "session-2", Data: map[string]interface{}{}},
			{EventTopic: "wayfinder.session.completed", Timestamp: parseTime("2025-12-09T11:45:00Z"), SessionID: "session-2", Data: map[string]interface{}{"status": "success"}},
		},
	}

	aggregator := NewAggregator()
	sessions, err := aggregator.AggregateSessions(eventsBySession)
	if err != nil {
		t.Fatalf("AggregateSessions() failed: %v", err)
	}

	if len(sessions) != 2 {
		t.Errorf("Expected 2 sessions, got %d", len(sessions))
	}

	// Verify sessions are sorted by start time (newest first)
	if sessions[0].StartTime.Before(sessions[1].StartTime) {
		t.Error("Expected sessions sorted by start time (newest first)")
	}
}

// TestComputeSummary tests summary computation
func TestComputeSummary(t *testing.T) {
	sessions := []Session{
		{
			ID:        "session-1",
			Status:    "completed",
			StartTime: parseTime("2025-12-09T10:00:00Z"),
			EndTime:   parseTime("2025-12-09T12:00:00Z"),
			Metrics: SessionMetrics{
				TotalDuration: 2 * time.Hour,
				AITime:        1*time.Hour + 30*time.Minute,
				WaitTime:      30 * time.Minute,
				PhaseCount:    5,
				EstimatedCost: 1.50,
			},
		},
		{
			ID:        "session-2",
			Status:    "failed",
			StartTime: parseTime("2025-12-09T13:00:00Z"),
			EndTime:   parseTime("2025-12-09T14:00:00Z"),
			Metrics: SessionMetrics{
				TotalDuration: 1 * time.Hour,
				AITime:        45 * time.Minute,
				WaitTime:      15 * time.Minute,
				PhaseCount:    3,
				EstimatedCost: 0.75,
			},
		},
	}

	aggregator := NewAggregator()
	summary := aggregator.ComputeSummary(sessions)

	// Verify counts
	if summary.TotalSessions != 2 {
		t.Errorf("Expected total sessions 2, got %d", summary.TotalSessions)
	}

	if summary.CompletedSessions != 1 {
		t.Errorf("Expected completed sessions 1, got %d", summary.CompletedSessions)
	}

	if summary.FailedSessions != 1 {
		t.Errorf("Expected failed sessions 1, got %d", summary.FailedSessions)
	}

	// Verify totals
	expectedTotalDuration := 3 * time.Hour
	if summary.TotalDuration != expectedTotalDuration {
		t.Errorf("Expected total duration %v, got %v", expectedTotalDuration, summary.TotalDuration)
	}

	expectedTotalCost := 2.25
	if summary.TotalCost != expectedTotalCost {
		t.Errorf("Expected total cost %.2f, got %.2f", expectedTotalCost, summary.TotalCost)
	}

	// Verify averages
	expectedAvgDuration := 90 * time.Minute // 3h / 2
	if summary.AverageDuration != expectedAvgDuration {
		t.Errorf("Expected average duration %v, got %v", expectedAvgDuration, summary.AverageDuration)
	}

	expectedAvgCost := 1.125 // 2.25 / 2
	if summary.AverageCost != expectedAvgCost {
		t.Errorf("Expected average cost %.3f, got %.3f", expectedAvgCost, summary.AverageCost)
	}
}

// TestComputeSummary_Empty tests summary with no sessions
func TestComputeSummary_Empty(t *testing.T) {
	aggregator := NewAggregator()
	summary := aggregator.ComputeSummary([]Session{})

	if summary.TotalSessions != 0 {
		t.Errorf("Expected total sessions 0, got %d", summary.TotalSessions)
	}

	if summary.TotalDuration != 0 {
		t.Errorf("Expected total duration 0, got %v", summary.TotalDuration)
	}
}

// TestWaitDetector_AllShortGaps tests wait detection with short gaps
func TestWaitDetector_AllShortGaps(t *testing.T) {
	detector := NewWaitDetector()

	phases := []Phase{
		{StartTime: parseTime("2025-12-09T10:00:00Z"), EndTime: parseTime("2025-12-09T10:10:00Z"), Duration: 10 * time.Minute},
		{StartTime: parseTime("2025-12-09T10:10:30Z"), EndTime: parseTime("2025-12-09T10:20:00Z"), Duration: 9*time.Minute + 30*time.Second}, // Gap: 30s
		{StartTime: parseTime("2025-12-09T10:21:00Z"), EndTime: parseTime("2025-12-09T10:30:00Z"), Duration: 9 * time.Minute},                // Gap: 1min
	}

	aiTime, waitTime := detector.DetectWaitTime(phases)

	// All gaps < 5min threshold → all AI time
	expectedAITime := 10*time.Minute + 9*time.Minute + 30*time.Second + 9*time.Minute + 30*time.Second + 1*time.Minute
	if aiTime != expectedAITime {
		t.Errorf("Expected AI time %v, got %v", expectedAITime, aiTime)
	}

	if waitTime != 0 {
		t.Errorf("Expected wait time 0, got %v", waitTime)
	}
}

// TestWaitDetector_OneLongGap tests wait detection with one long gap
func TestWaitDetector_OneLongGap(t *testing.T) {
	detector := NewWaitDetector()

	phases := []Phase{
		{StartTime: parseTime("2025-12-09T10:00:00Z"), EndTime: parseTime("2025-12-09T10:10:00Z"), Duration: 10 * time.Minute},
		{StartTime: parseTime("2025-12-09T10:20:00Z"), EndTime: parseTime("2025-12-09T10:30:00Z"), Duration: 10 * time.Minute}, // Gap: 10min (>5min threshold)
	}

	aiTime, waitTime := detector.DetectWaitTime(phases)

	// Phase durations: 10min + 10min = 20min AI time
	// Gap: 10min wait time
	expectedAITime := 20 * time.Minute
	expectedWaitTime := 10 * time.Minute

	if aiTime != expectedAITime {
		t.Errorf("Expected AI time %v, got %v", expectedAITime, aiTime)
	}

	if waitTime != expectedWaitTime {
		t.Errorf("Expected wait time %v, got %v", expectedWaitTime, waitTime)
	}
}

// TestWaitDetector_CustomThreshold tests custom threshold
func TestWaitDetector_CustomThreshold(t *testing.T) {
	// Custom threshold: 2 minutes
	detector := NewWaitDetectorWithThreshold(2 * time.Minute)

	phases := []Phase{
		{StartTime: parseTime("2025-12-09T10:00:00Z"), EndTime: parseTime("2025-12-09T10:10:00Z"), Duration: 10 * time.Minute},
		{StartTime: parseTime("2025-12-09T10:13:00Z"), EndTime: parseTime("2025-12-09T10:20:00Z"), Duration: 7 * time.Minute}, // Gap: 3min (>2min threshold)
	}

	aiTime, waitTime := detector.DetectWaitTime(phases)

	// Phase durations: 10min + 7min = 17min AI time
	expectedAITime := 17 * time.Minute
	if aiTime != expectedAITime {
		t.Errorf("Expected AI time %v, got %v", expectedAITime, aiTime)
	}

	// Gap is 3min, which is >2min threshold → wait time
	expectedWaitTime := 3 * time.Minute
	if waitTime != expectedWaitTime {
		t.Errorf("Expected wait time %v, got %v", expectedWaitTime, waitTime)
	}
}

// Helper function to parse RFC3339 timestamps
func parseTime(s string) time.Time {
	t, _ := time.Parse(time.RFC3339, s)
	return t
}
