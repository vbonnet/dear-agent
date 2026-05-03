package analytics

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestE2E_CompleteWayfinderSession tests the complete analytics flow
// with a full Wayfinder session (D1-D4, S5-S11)
// setupE2ETelemetry creates mock telemetry file and returns path
func setupE2ETelemetry(t *testing.T) (string, string, string) {
	t.Helper()
	tmpDir := t.TempDir()
	telemetryPath := filepath.Join(tmpDir, "telemetry.jsonl")
	sessionID := "test-wayfinder-session-e2e"
	projectPath := "/tmp/test/src/engram"

	sessionStart := parseTime("2025-12-09T10:00:00Z")
	content := generateMockWayfinderSession(sessionID, projectPath, sessionStart)

	if err := os.WriteFile(telemetryPath, []byte(content), 0600); err != nil {
		t.Fatalf("Failed to create test telemetry file: %v", err)
	}

	return telemetryPath, sessionID, projectPath
}

// parseAndVerifyEvents parses telemetry and verifies event count
func parseAndVerifyEvents(t *testing.T, telemetryPath, sessionID string) []ParsedEvent {
	t.Helper()
	parser := NewParser(telemetryPath)
	eventsBySession, err := parser.ParseAll()
	if err != nil {
		t.Fatalf("Parser.ParseAll() failed: %v", err)
	}

	if len(eventsBySession) != 1 {
		t.Fatalf("Expected 1 session, got %d", len(eventsBySession))
	}

	events, ok := eventsBySession[sessionID]
	if !ok {
		t.Fatalf("Session %s not found", sessionID)
	}

	expectedEvents := 24
	if len(events) != expectedEvents {
		t.Errorf("Expected %d events, got %d", expectedEvents, len(events))
	}

	return events
}

// aggregateAndVerifySession aggregates events into session and verifies structure
func aggregateAndVerifySession(t *testing.T, sessionID, projectPath string, events []ParsedEvent) *Session {
	t.Helper()
	aggregator := NewAggregator()
	session, err := aggregator.AggregateSession(sessionID, events)
	if err != nil {
		t.Fatalf("Aggregator.AggregateSession() failed: %v", err)
	}

	if session.ID != sessionID {
		t.Errorf("Expected session ID %s, got %s", sessionID, session.ID)
	}
	if session.ProjectPath != projectPath {
		t.Errorf("Expected project path %s, got %s", projectPath, session.ProjectPath)
	}
	if session.Status != "success" {
		t.Errorf("Expected status 'success', got '%s'", session.Status)
	}

	return session
}

// verifyPhases checks phase count and names
func verifyPhases(t *testing.T, session *Session) {
	t.Helper()
	expectedPhaseCount := 11
	if len(session.Phases) != expectedPhaseCount {
		t.Errorf("Expected %d phases, got %d", expectedPhaseCount, len(session.Phases))
	}

	expectedPhaseNames := []string{"D1", "D2", "D3", "D4", "S5", "S6", "S7", "S8", "S9", "S10", "S11"}
	for i, expectedName := range expectedPhaseNames {
		if i >= len(session.Phases) {
			break
		}
		if session.Phases[i].Name != expectedName {
			t.Errorf("Phase %d: expected %s, got %s", i, expectedName, session.Phases[i].Name)
		}
	}
}

// verifyMetrics checks session metrics are non-zero
func verifyMetrics(t *testing.T, session *Session) {
	t.Helper()
	if session.Metrics.TotalDuration == 0 {
		t.Error("Expected non-zero total duration")
	}
	if session.Metrics.PhaseCount != 11 {
		t.Errorf("Expected phase count 11, got %d", session.Metrics.PhaseCount)
	}
	if session.Metrics.AITime == 0 {
		t.Error("Expected non-zero AI time")
	}
}

// verifyFormatters tests all output formatters
func verifyFormatters(t *testing.T, session *Session, sessionID string) {
	t.Helper()
	expectedPhaseNames := []string{"D1", "D2", "D3", "D4", "S5", "S6", "S7", "S8", "S9", "S10", "S11"}

	// Markdown formatter
	mdFormatter := &MarkdownFormatter{}
	mdOutput, err := mdFormatter.FormatSession(session)
	if err != nil {
		t.Fatalf("MarkdownFormatter.FormatSession() failed: %v", err)
	}
	if !strings.Contains(mdOutput, "# Session") || !strings.Contains(mdOutput, "## Phase Timeline") {
		t.Error("Markdown output missing expected headers")
	}
	for _, phaseName := range expectedPhaseNames {
		if !strings.Contains(mdOutput, phaseName) {
			t.Errorf("Markdown output missing phase %s", phaseName)
		}
	}

	// JSON formatter
	jsonFormatter := &JSONFormatter{Pretty: true}
	jsonOutput, err := jsonFormatter.Format([]Session{*session})
	if err != nil {
		t.Fatalf("JSONFormatter.Format() failed: %v", err)
	}
	if !strings.Contains(jsonOutput, sessionID) {
		t.Error("JSON output missing session ID")
	}

	// CSV formatter
	csvFormatter := &CSVFormatter{}
	csvOutput, err := csvFormatter.Format([]Session{*session})
	if err != nil {
		t.Fatalf("CSVFormatter.Format() failed: %v", err)
	}
	if !strings.Contains(csvOutput, sessionID) {
		t.Error("CSV output missing session ID")
	}
}

// verifySummary tests aggregation summary
func verifySummary(t *testing.T, session *Session) {
	t.Helper()
	aggregator := NewAggregator()
	sessions := []Session{*session}
	summary := aggregator.ComputeSummary(sessions)

	if summary.TotalSessions != 1 {
		t.Errorf("Expected 1 session in summary, got %d", summary.TotalSessions)
	}
	if summary.CompletedSessions != 1 {
		t.Errorf("Expected 1 completed session, got %d", summary.CompletedSessions)
	}

	mdFormatter := &MarkdownFormatter{}
	summaryOutput, err := mdFormatter.FormatSummary(summary)
	if err != nil {
		t.Fatalf("MarkdownFormatter.FormatSummary() failed: %v", err)
	}
	if !strings.Contains(summaryOutput, "# Wayfinder Session Summary") {
		t.Error("Summary output missing header")
	}
}

func TestE2E_CompleteWayfinderSession(t *testing.T) {
	telemetryPath, sessionID, projectPath := setupE2ETelemetry(t)
	events := parseAndVerifyEvents(t, telemetryPath, sessionID)
	session := aggregateAndVerifySession(t, sessionID, projectPath, events)

	verifyPhases(t, session)
	verifyMetrics(t, session)
	verifyFormatters(t, session, sessionID)
	verifySummary(t, session)

	t.Logf("E2E Test Successful!")
	t.Logf("Session: %d phases, %v duration, %v AI time, %v wait time",
		session.Metrics.PhaseCount,
		session.Metrics.TotalDuration,
		session.Metrics.AITime,
		session.Metrics.WaitTime)
}

// generateMockWayfinderSession creates a complete mock telemetry session
func generateMockWayfinderSession(sessionID, projectPath string, startTime time.Time) string {
	var sb strings.Builder

	// Session started
	sb.WriteString(mockEvent("wayfinder.session.started", sessionID, "", startTime, map[string]interface{}{
		"project_path": projectPath,
	}))

	currentTime := startTime

	// All 11 Wayfinder phases (D1-D4, S5-S11)
	phases := []struct {
		name     string
		duration time.Duration
	}{
		{"D1", 15 * time.Minute},
		{"D2", 20 * time.Minute},
		{"D3", 10 * time.Minute},
		{"D4", 5 * time.Minute},
		{"S5", 30 * time.Minute},
		{"S6", 25 * time.Minute},
		{"S7", 20 * time.Minute},
		{"S8", 2 * time.Hour},     // Implementation phase (longest)
		{"S9", 30 * time.Minute},  // Validation
		{"S10", 15 * time.Minute}, // Documentation
		{"S11", 10 * time.Minute}, // Retrospective
	}

	for _, phase := range phases {
		// Phase started
		currentTime = currentTime.Add(5 * time.Second) // Small gap between phases
		sb.WriteString(mockEvent("wayfinder.phase.started", sessionID, phase.name, currentTime, nil))

		// Phase completed
		currentTime = currentTime.Add(phase.duration)
		sb.WriteString(mockEvent("wayfinder.phase.completed", sessionID, phase.name, currentTime, map[string]interface{}{
			"files_modified": 3,
			"lines_added":    150,
		}))
	}

	// Session completed
	currentTime = currentTime.Add(10 * time.Second)
	sb.WriteString(mockEvent("wayfinder.session.completed", sessionID, "", currentTime, map[string]interface{}{
		"status": "success",
	}))

	return sb.String()
}

// mockEvent creates a single telemetry event line
func mockEvent(topic, sessionID, phase string, timestamp time.Time, extraData map[string]interface{}) string {
	data := map[string]interface{}{
		"event_topic": topic,
		"session_id":  sessionID,
	}

	if phase != "" {
		data["phase"] = phase
	}

	// Merge extra data
	for k, v := range extraData {
		data[k] = v
	}

	// Convert data to JSON string (simplified - not production quality)
	var dataStr strings.Builder
	dataStr.WriteString("{")
	first := true
	for k, v := range data {
		if !first {
			dataStr.WriteString(",")
		}
		first = false

		switch val := v.(type) {
		case string:
			fmt.Fprintf(&dataStr, `"%s":"%s"`, k, val)
		case int:
			fmt.Fprintf(&dataStr, `"%s":%d`, k, val)
		default:
			fmt.Fprintf(&dataStr, `"%s":"%v"`, k, val)
		}
	}
	dataStr.WriteString("}")

	return fmt.Sprintf(`{"timestamp":"%s","type":"eventbus_publish","agent":"wayfinder","data":%s}%s`,
		timestamp.Format(time.RFC3339),
		dataStr.String(),
		"\n")
}
