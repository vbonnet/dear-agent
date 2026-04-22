package analytics

import (
	"encoding/csv"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

// TestMarkdownFormatter_Format tests Markdown list formatting
func TestMarkdownFormatter_Format(t *testing.T) {
	sessions := []Session{
		{
			ID:          "session-123",
			ProjectPath: "/tmp/test/project",
			StartTime:   parseTime("2025-12-09T10:00:00Z"),
			EndTime:     parseTime("2025-12-09T12:00:00Z"),
			Status:      "completed",
			Metrics: SessionMetrics{
				TotalDuration: 2 * time.Hour,
				PhaseCount:    4,
			},
		},
	}

	formatter := &MarkdownFormatter{}
	output, err := formatter.Format(sessions)
	if err != nil {
		t.Fatalf("Format() failed: %v", err)
	}

	// Verify output contains expected elements
	if !strings.Contains(output, "# Wayfinder Sessions") {
		t.Error("Expected header '# Wayfinder Sessions' not found")
	}

	if !strings.Contains(output, "session-123") {
		t.Error("Expected session ID not found in output")
	}

	if !strings.Contains(output, "2h 0m") {
		t.Error("Expected duration '2h 0m' not found in output")
	}

	if !strings.Contains(output, "✅") {
		t.Error("Expected completed status emoji not found")
	}
}

// TestMarkdownFormatter_Format_Empty tests empty sessions
func TestMarkdownFormatter_Format_Empty(t *testing.T) {
	formatter := &MarkdownFormatter{}
	output, err := formatter.Format([]Session{})
	if err != nil {
		t.Fatalf("Format() failed: %v", err)
	}

	if !strings.Contains(output, "No Wayfinder sessions found") {
		t.Error("Expected 'No Wayfinder sessions found' message")
	}
}

// TestMarkdownFormatter_FormatSession tests detailed session formatting
func TestMarkdownFormatter_FormatSession(t *testing.T) {
	session := &Session{
		ID:          "test-session-456",
		ProjectPath: "/tmp/test/my-project",
		StartTime:   parseTime("2025-12-09T10:00:00Z"),
		EndTime:     parseTime("2025-12-09T12:30:00Z"),
		Status:      "success",
		Phases: []Phase{
			{
				Name:      "D1",
				StartTime: parseTime("2025-12-09T10:00:05Z"),
				EndTime:   parseTime("2025-12-09T10:15:00Z"),
				Duration:  14*time.Minute + 55*time.Second,
			},
			{
				Name:      "D2",
				StartTime: parseTime("2025-12-09T10:15:05Z"),
				EndTime:   parseTime("2025-12-09T10:30:00Z"),
				Duration:  14*time.Minute + 55*time.Second,
			},
		},
		Metrics: SessionMetrics{
			TotalDuration: 2*time.Hour + 30*time.Minute,
			AITime:        2 * time.Hour,
			WaitTime:      30 * time.Minute,
			PhaseCount:    2,
			EstimatedCost: 1.25,
		},
	}

	formatter := &MarkdownFormatter{}
	output, err := formatter.FormatSession(session)
	if err != nil {
		t.Fatalf("FormatSession() failed: %v", err)
	}

	// Verify header
	if !strings.Contains(output, "# Session: test-session-456") {
		t.Error("Expected session header not found")
	}

	// Verify phase timeline
	if !strings.Contains(output, "## Phase Timeline") {
		t.Error("Expected phase timeline section not found")
	}

	if !strings.Contains(output, "D1") || !strings.Contains(output, "D2") {
		t.Error("Expected phase names not found")
	}

	// Verify metrics
	if !strings.Contains(output, "## Metrics") {
		t.Error("Expected metrics section not found")
	}

	if !strings.Contains(output, "2h 30m") {
		t.Error("Expected total duration not found")
	}

	if !strings.Contains(output, "$1.25") {
		t.Error("Expected cost not found")
	}

	// Verify percentages
	if !strings.Contains(output, "80%") { // AI time percentage (2h / 2.5h)
		t.Error("Expected AI time percentage not found")
	}

	if !strings.Contains(output, "20%") { // Wait time percentage (0.5h / 2.5h)
		t.Error("Expected wait time percentage not found")
	}
}

// TestMarkdownFormatter_FormatSummary tests summary formatting
func TestMarkdownFormatter_FormatSummary(t *testing.T) {
	summary := SessionSummary{
		TotalSessions:     10,
		CompletedSessions: 8,
		FailedSessions:    2,
		TotalDuration:     20 * time.Hour,
		AverageDuration:   2 * time.Hour,
		TotalCost:         15.50,
		AverageCost:       1.55,
	}

	formatter := &MarkdownFormatter{}
	output, err := formatter.FormatSummary(summary)
	if err != nil {
		t.Fatalf("FormatSummary() failed: %v", err)
	}

	// Verify header
	if !strings.Contains(output, "# Wayfinder Session Summary") {
		t.Error("Expected summary header not found")
	}

	// Verify counts
	if !strings.Contains(output, "Total Sessions**: 10") {
		t.Error("Expected total sessions count not found")
	}

	if !strings.Contains(output, "Completed**: 8") {
		t.Error("Expected completed count not found")
	}

	// Verify completion rate
	if !strings.Contains(output, "80.0%") { // 8/10 = 80%
		t.Error("Expected completion rate not found")
	}

	// Verify durations
	if !strings.Contains(output, "20h 0m") {
		t.Error("Expected total duration not found")
	}

	// Verify costs
	if !strings.Contains(output, "$15.50") {
		t.Error("Expected total cost not found")
	}

	if !strings.Contains(output, "$1.55") {
		t.Error("Expected average cost not found")
	}
}

// TestJSONFormatter_Format tests JSON formatting
func TestJSONFormatter_Format(t *testing.T) {
	sessions := []Session{
		{
			ID:          "json-test",
			ProjectPath: "/tmp/test/project",
			StartTime:   parseTime("2025-12-09T10:00:00Z"),
			EndTime:     parseTime("2025-12-09T11:00:00Z"),
			Status:      "completed",
			Phases:      []Phase{},
			Metrics: SessionMetrics{
				TotalDuration: 1 * time.Hour,
				PhaseCount:    3,
			},
		},
	}

	formatter := &JSONFormatter{Pretty: false}
	output, err := formatter.Format(sessions)
	if err != nil {
		t.Fatalf("Format() failed: %v", err)
	}

	// Verify valid JSON
	var parsed []Session
	if err := json.Unmarshal([]byte(output), &parsed); err != nil {
		t.Fatalf("Output is not valid JSON: %v", err)
	}

	// Verify data
	if len(parsed) != 1 {
		t.Errorf("Expected 1 session, got %d", len(parsed))
	}

	if parsed[0].ID != "json-test" {
		t.Errorf("Expected session ID 'json-test', got '%s'", parsed[0].ID)
	}
}

// TestJSONFormatter_Format_Pretty tests pretty JSON formatting
func TestJSONFormatter_Format_Pretty(t *testing.T) {
	sessions := []Session{
		{
			ID:     "pretty-json-test",
			Status: "completed",
		},
	}

	formatter := &JSONFormatter{Pretty: true}
	output, err := formatter.Format(sessions)
	if err != nil {
		t.Fatalf("Format() failed: %v", err)
	}

	// Verify indentation (pretty printing)
	if !strings.Contains(output, "  ") {
		t.Error("Expected indented JSON (pretty printing)")
	}

	// Verify valid JSON
	var parsed []Session
	if err := json.Unmarshal([]byte(output), &parsed); err != nil {
		t.Fatalf("Pretty output is not valid JSON: %v", err)
	}
}

// TestCSVFormatter_Format tests CSV formatting
func TestCSVFormatter_Format(t *testing.T) {
	sessions := []Session{
		{
			ID:          "csv-test-1",
			ProjectPath: "/tmp/test/project1",
			StartTime:   parseTime("2025-12-09T10:00:00Z"),
			EndTime:     parseTime("2025-12-09T11:30:00Z"),
			Status:      "completed",
			Metrics: SessionMetrics{
				TotalDuration: 90 * time.Minute,
				AITime:        60 * time.Minute,
				WaitTime:      30 * time.Minute,
				PhaseCount:    4,
				EstimatedCost: 1.50,
			},
		},
		{
			ID:          "csv-test-2",
			ProjectPath: "/tmp/test/project2",
			StartTime:   parseTime("2025-12-09T12:00:00Z"),
			EndTime:     parseTime("2025-12-09T13:00:00Z"),
			Status:      "failed",
			Metrics: SessionMetrics{
				TotalDuration: 60 * time.Minute,
				AITime:        50 * time.Minute,
				WaitTime:      10 * time.Minute,
				PhaseCount:    2,
				EstimatedCost: 0.75,
			},
		},
	}

	formatter := &CSVFormatter{}
	output, err := formatter.Format(sessions)
	if err != nil {
		t.Fatalf("Format() failed: %v", err)
	}

	// Parse CSV
	reader := csv.NewReader(strings.NewReader(output))
	records, err := reader.ReadAll()
	if err != nil {
		t.Fatalf("Failed to parse CSV: %v", err)
	}

	// Verify header + 2 rows
	if len(records) != 3 {
		t.Errorf("Expected 3 rows (header + 2 data), got %d", len(records))
	}

	// Verify header
	header := records[0]
	if header[0] != "session_id" {
		t.Errorf("Expected first column 'session_id', got '%s'", header[0])
	}

	// Verify first data row
	row1 := records[1]
	if row1[0] != "csv-test-1" {
		t.Errorf("Expected session ID 'csv-test-1', got '%s'", row1[0])
	}

	if row1[4] != "90.00" { // duration_minutes
		t.Errorf("Expected duration '90.00', got '%s'", row1[4])
	}

	if row1[8] != "1.50" { // cost_usd
		t.Errorf("Expected cost '1.50', got '%s'", row1[8])
	}

	if row1[9] != "completed" { // status
		t.Errorf("Expected status 'completed', got '%s'", row1[9])
	}
}

// TestFormatDuration tests duration formatting helper
func TestFormatDuration(t *testing.T) {
	tests := []struct {
		duration time.Duration
		expected string
	}{
		{0, "0s"},
		{30 * time.Second, "30s"},
		{2 * time.Minute, "2m 0s"},
		{2*time.Minute + 30*time.Second, "2m 30s"},
		{1 * time.Hour, "1h 0m"},
		{2*time.Hour + 15*time.Minute, "2h 15m"},
		{24*time.Hour + 30*time.Minute, "24h 30m"},
	}

	for _, tt := range tests {
		result := formatDuration(tt.duration)
		if result != tt.expected {
			t.Errorf("formatDuration(%v) = %s, expected %s", tt.duration, result, tt.expected)
		}
	}
}

// TestFormatStatus tests status formatting helper
func TestFormatStatus(t *testing.T) {
	tests := []struct {
		status   string
		expected string
	}{
		{"completed", "✅"},
		{"success", "✅"},
		{"failed", "❌"},
		{"incomplete", "⏸️"},
		{"unknown", "unknown"},
	}

	for _, tt := range tests {
		result := formatStatus(tt.status)
		if result != tt.expected {
			t.Errorf("formatStatus(%s) = %s, expected %s", tt.status, result, tt.expected)
		}
	}
}
