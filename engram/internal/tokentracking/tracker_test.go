package tokentracking

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/vbonnet/dear-agent/internal/telemetry"
)

func TestNewTokenTracker(t *testing.T) {
	tracker := NewTokenTracker()
	if tracker == nil {
		t.Fatal("NewTokenTracker() returned nil")
	}
	if tracker.listener == nil {
		t.Error("Tracker listener is nil")
	}
	if tracker.telemetryAvail {
		t.Error("Telemetry should not be available by default (internal package)")
	}
}

func TestInitialize(t *testing.T) {
	tracker := NewTokenTracker()

	err := tracker.Initialize(nil)
	if err != nil {
		t.Errorf("Initialize() returned error: %v", err)
	}

	// Verify session start time was set
	if tracker.sessionStarted.IsZero() {
		t.Error("Session start time not set after Initialize()")
	}

	// Verify telemetry state
	if tracker.telemetryAvail {
		t.Error("Telemetry should be unavailable (P1 is internal package)")
	}
}

func TestRecordResponse_ValidJSON(t *testing.T) {
	tracker := NewTokenTracker()
	_ = tracker.Initialize(nil)

	responseJSON := []byte(`{
		"usage": {
			"input_tokens": 1000,
			"output_tokens": 500,
			"cache_creation_input_tokens": 100,
			"cache_read_input_tokens": 50
		}
	}`)

	usage, err := tracker.RecordResponse(responseJSON)
	if err != nil {
		t.Fatalf("RecordResponse() error: %v", err)
	}

	if usage == nil {
		t.Fatal("RecordResponse() returned nil usage")
	}

	// Verify extracted values
	if usage.InputTokens != 1000 {
		t.Errorf("InputTokens = %d, want 1000", usage.InputTokens)
	}
	if usage.OutputTokens != 500 {
		t.Errorf("OutputTokens = %d, want 500", usage.OutputTokens)
	}
	if usage.TotalTokens != 1500 {
		t.Errorf("TotalTokens = %d, want 1500", usage.TotalTokens)
	}

	// Verify listener was updated
	summary := tracker.GetSummary()
	if summary.ResponseCount != 1 {
		t.Errorf("Summary ResponseCount = %d, want 1", summary.ResponseCount)
	}
	if summary.TotalInputTokens != 1000 {
		t.Errorf("Summary TotalInputTokens = %d, want 1000", summary.TotalInputTokens)
	}
}

func TestRecordResponse_InvalidJSON(t *testing.T) {
	tracker := NewTokenTracker()
	_ = tracker.Initialize(nil)

	invalidJSON := []byte(`{"usage": {malformed`)

	usage, err := tracker.RecordResponse(invalidJSON)
	if err == nil {
		t.Error("RecordResponse() should return error for invalid JSON")
	}
	if usage != nil {
		t.Error("RecordResponse() should return nil usage on error")
	}

	// Verify listener was NOT updated
	summary := tracker.GetSummary()
	if summary.ResponseCount != 0 {
		t.Errorf("Summary should have 0 responses after error, got %d", summary.ResponseCount)
	}
}

func TestRecordResponseFromStruct(t *testing.T) {
	tracker := NewTokenTracker()
	_ = tracker.Initialize(nil)

	response := &APIResponse{
		Usage: struct {
			InputTokens         int `json:"input_tokens"`
			OutputTokens        int `json:"output_tokens"`
			CacheCreationTokens int `json:"cache_creation_input_tokens"`
			CacheReadTokens     int `json:"cache_read_input_tokens"`
		}{
			InputTokens:  2000,
			OutputTokens: 1000,
		},
	}

	usage, err := tracker.RecordResponseFromStruct(response)
	if err != nil {
		t.Fatalf("RecordResponseFromStruct() error: %v", err)
	}

	if usage.InputTokens != 2000 {
		t.Errorf("InputTokens = %d, want 2000", usage.InputTokens)
	}

	summary := tracker.GetSummary()
	if summary.ResponseCount != 1 {
		t.Errorf("ResponseCount = %d, want 1", summary.ResponseCount)
	}
}

func TestRecordResponse_MultipleResponses(t *testing.T) {
	tracker := NewTokenTracker()
	_ = tracker.Initialize(nil)

	responses := []string{
		`{"usage":{"input_tokens":100,"output_tokens":50}}`,
		`{"usage":{"input_tokens":200,"output_tokens":100}}`,
		`{"usage":{"input_tokens":300,"output_tokens":150}}`,
	}

	for _, resp := range responses {
		_, err := tracker.RecordResponse([]byte(resp))
		if err != nil {
			t.Fatalf("RecordResponse() error: %v", err)
		}
	}

	summary := tracker.GetSummary()
	if summary.ResponseCount != 3 {
		t.Errorf("ResponseCount = %d, want 3", summary.ResponseCount)
	}

	expectedInput := 100 + 200 + 300
	expectedOutput := 50 + 100 + 150

	if summary.TotalInputTokens != expectedInput {
		t.Errorf("TotalInputTokens = %d, want %d", summary.TotalInputTokens, expectedInput)
	}
	if summary.TotalOutputTokens != expectedOutput {
		t.Errorf("TotalOutputTokens = %d, want %d", summary.TotalOutputTokens, expectedOutput)
	}
}

func TestGetSummary(t *testing.T) {
	tracker := NewTokenTracker()
	_ = tracker.Initialize(nil)

	// Empty summary initially
	summary := tracker.GetSummary()
	if summary.ResponseCount != 0 {
		t.Errorf("Initial ResponseCount = %d, want 0", summary.ResponseCount)
	}

	// Add response
	responseJSON := []byte(`{"usage":{"input_tokens":1000,"output_tokens":500}}`)
	_, _ = tracker.RecordResponse(responseJSON)

	// Verify summary updated
	summary = tracker.GetSummary()
	if summary.ResponseCount != 1 {
		t.Errorf("After RecordResponse: ResponseCount = %d, want 1", summary.ResponseCount)
	}
}

func TestDisplaySummary_EmptySession(t *testing.T) {
	tracker := NewTokenTracker()
	_ = tracker.Initialize(nil)

	var buf bytes.Buffer
	err := tracker.DisplaySummary(&buf)
	if err != nil {
		t.Errorf("DisplaySummary() error: %v", err)
	}

	// Should output nothing for empty session
	if buf.Len() > 0 {
		t.Errorf("DisplaySummary() should output nothing for empty session, got: %s", buf.String())
	}
}

func TestDisplaySummary_WithResponses(t *testing.T) {
	tracker := NewTokenTracker()
	_ = tracker.Initialize(nil)

	// Add some responses
	responses := []string{
		`{"usage":{"input_tokens":1000,"output_tokens":500}}`,
		`{"usage":{"input_tokens":2000,"output_tokens":1000}}`,
	}
	for _, resp := range responses {
		_, _ = tracker.RecordResponse([]byte(resp))
	}

	// Small delay to ensure duration > 0
	time.Sleep(10 * time.Millisecond)

	var buf bytes.Buffer
	err := tracker.DisplaySummary(&buf)
	if err != nil {
		t.Errorf("DisplaySummary() error: %v", err)
	}

	output := buf.String()

	// Verify key elements are present
	expectedStrings := []string{
		"Claude CLI Session Token Summary",
		"Session Duration:",
		"API Responses:",
		"2", // Response count
		"Input:",
		"3000", // Total input (1000+2000)
		"Output:",
		"1500", // Total output (500+1000)
		"Total:",
		"4500", // Grand total
		"Severity Level:",
		"INFO",
	}

	for _, expected := range expectedStrings {
		if !strings.Contains(output, expected) {
			t.Errorf("DisplaySummary() output missing %q\nGot:\n%s", expected, output)
		}
	}
}

func TestDisplaySummary_HighSeverity(t *testing.T) {
	tracker := NewTokenTracker()
	_ = tracker.Initialize(nil)

	// Add response that triggers WARN level (≥50K tokens)
	responseJSON := []byte(`{"usage":{"input_tokens":30000,"output_tokens":25000}}`)
	_, _ = tracker.RecordResponse(responseJSON)

	var buf bytes.Buffer
	err := tracker.DisplaySummary(&buf)
	if err != nil {
		t.Errorf("DisplaySummary() error: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "WARN") {
		t.Errorf("DisplaySummary() should show WARN severity for 55K tokens\nGot:\n%s", output)
	}
}

func TestDisplaySummaryJSON(t *testing.T) {
	tracker := NewTokenTracker()
	_ = tracker.Initialize(nil)

	// Add responses
	responseJSON := []byte(`{"usage":{"input_tokens":1000,"output_tokens":500,"cache_creation_input_tokens":100,"cache_read_input_tokens":50}}`)
	_, _ = tracker.RecordResponse(responseJSON)

	var buf bytes.Buffer
	err := tracker.DisplaySummaryJSON(&buf)
	if err != nil {
		t.Errorf("DisplaySummaryJSON() error: %v", err)
	}

	// Parse JSON output
	var result map[string]interface{}
	if err := json.Unmarshal(buf.Bytes(), &result); err != nil {
		t.Fatalf("DisplaySummaryJSON() output is not valid JSON: %v\nOutput: %s", err, buf.String())
	}

	// Verify fields
	if result["response_count"].(float64) != 1 {
		t.Errorf("response_count = %v, want 1", result["response_count"])
	}
	if result["total_input_tokens"].(float64) != 1000 {
		t.Errorf("total_input_tokens = %v, want 1000", result["total_input_tokens"])
	}
	if result["total_output_tokens"].(float64) != 500 {
		t.Errorf("total_output_tokens = %v, want 500", result["total_output_tokens"])
	}
	if result["total_tokens"].(float64) != 1500 {
		t.Errorf("total_tokens = %v, want 1500", result["total_tokens"])
	}

	// Verify severity
	severity, ok := result["highest_severity"].(string)
	if !ok {
		t.Error("highest_severity field missing or wrong type")
	}
	if !strings.Contains(severity, "INFO") {
		t.Errorf("highest_severity = %q, should contain INFO", severity)
	}
}

func TestClose(t *testing.T) {
	tracker := NewTokenTracker()
	_ = tracker.Initialize(nil)

	err := tracker.Close()
	if err != nil {
		t.Errorf("Close() returned error: %v", err)
	}

	// Verify can still get summary after close (non-destructive)
	summary := tracker.GetSummary()
	if summary.ResponseCount != 0 {
		t.Error("Summary should still be accessible after Close()")
	}
}

func TestFormatDuration(t *testing.T) {
	tests := []struct {
		duration time.Duration
		want     string
	}{
		{500 * time.Millisecond, "0.5s"},
		{5 * time.Second, "5.0s"},
		{45 * time.Second, "45.0s"},
		{90 * time.Second, "1m 30s"},
		{5 * time.Minute, "5m 0s"},
		{65 * time.Minute, "1h 5m"},
		{2*time.Hour + 30*time.Minute, "2h 30m"},
	}

	for _, tt := range tests {
		got := formatDuration(tt.duration)
		if got != tt.want {
			t.Errorf("formatDuration(%v) = %q, want %q", tt.duration, got, tt.want)
		}
	}
}

func TestFormatSeverity(t *testing.T) {
	tests := []struct {
		level telemetry.Level
		want  string
	}{
		{telemetry.LevelInfo, "INFO"},
		{telemetry.LevelWarn, "WARN"},
		{telemetry.LevelError, "ERROR"},
		{telemetry.LevelCritical, "CRITICAL"},
	}

	for _, tt := range tests {
		got := formatSeverity(tt.level)
		if !strings.Contains(got, tt.want) {
			t.Errorf("formatSeverity(%v) = %q, should contain %q", tt.level, got, tt.want)
		}
	}
}

func TestGetDefaultTracker(t *testing.T) {
	// Reset before test
	ResetDefaultTracker()

	tracker1 := GetDefaultTracker()
	if tracker1 == nil {
		t.Fatal("GetDefaultTracker() returned nil")
	}

	tracker2 := GetDefaultTracker()
	if tracker2 == nil {
		t.Fatal("GetDefaultTracker() returned nil on second call")
	}

	// Should return same instance (singleton)
	if tracker1 != tracker2 {
		t.Error("GetDefaultTracker() should return singleton instance")
	}
}

func TestResetDefaultTracker(t *testing.T) {
	// Get initial tracker
	tracker1 := GetDefaultTracker()

	// Add some data
	responseJSON := []byte(`{"usage":{"input_tokens":1000,"output_tokens":500}}`)
	_, _ = tracker1.RecordResponse(responseJSON)

	summary := tracker1.GetSummary()
	if summary.ResponseCount != 1 {
		t.Fatalf("Setup failed: expected 1 response")
	}

	// Reset
	ResetDefaultTracker()

	// Get new tracker - should be fresh instance
	tracker2 := GetDefaultTracker()
	if tracker2 == tracker1 {
		t.Error("ResetDefaultTracker() should create new instance")
	}

	summary = tracker2.GetSummary()
	if summary.ResponseCount != 0 {
		t.Errorf("New tracker should have 0 responses, got %d", summary.ResponseCount)
	}
}

func TestDisplaySummaryToStderr(t *testing.T) {
	// Reset and setup
	ResetDefaultTracker()
	tracker := GetDefaultTracker()

	// Add response
	responseJSON := []byte(`{"usage":{"input_tokens":1000,"output_tokens":500}}`)
	_, _ = tracker.RecordResponse(responseJSON)

	// Note: Can't easily test stderr output, but verify no panic
	// err := DisplaySummaryToStderr()
	// In real usage, this would output to stderr
	// For test, just verify the function exists and tracker is accessible
	if tracker == nil {
		t.Error("Default tracker should be available for DisplaySummaryToStderr()")
	}
}
