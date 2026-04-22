package analytics

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestParseAll_Success tests parsing valid telemetry with Wayfinder events
func TestParseAll_Success(t *testing.T) {
	// Create temporary telemetry file
	tmpDir := t.TempDir()
	telemetryPath := filepath.Join(tmpDir, "telemetry.jsonl")

	// Write mock telemetry data
	content := `{"timestamp":"2025-12-09T10:00:00Z","type":"eventbus_publish","agent":"wayfinder","data":{"event_topic":"wayfinder.session.started","session_id":"test-session-123","project_path":"/tmp/test/project"}}
{"timestamp":"2025-12-09T10:00:05Z","type":"eventbus_publish","agent":"wayfinder","data":{"event_topic":"wayfinder.phase.started","session_id":"test-session-123","phase":"D1"}}
{"timestamp":"2025-12-09T10:15:00Z","type":"eventbus_publish","agent":"wayfinder","data":{"event_topic":"wayfinder.phase.completed","session_id":"test-session-123","phase":"D1"}}
{"timestamp":"2025-12-09T10:15:05Z","type":"eventbus_publish","agent":"claude-code","data":{"some":"data"}}
{"timestamp":"2025-12-09T10:16:00Z","type":"eventbus_publish","agent":"wayfinder","data":{"event_topic":"wayfinder.session.completed","session_id":"test-session-123","status":"success"}}
`
	if err := os.WriteFile(telemetryPath, []byte(content), 0600); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	// Parse events
	parser := NewParser(telemetryPath)
	eventsBySession, err := parser.ParseAll()
	if err != nil {
		t.Fatalf("ParseAll() failed: %v", err)
	}

	// Verify results
	if len(eventsBySession) != 1 {
		t.Errorf("Expected 1 session, got %d", len(eventsBySession))
	}

	events, ok := eventsBySession["test-session-123"]
	if !ok {
		t.Fatal("Expected session test-session-123 not found")
	}

	expectedEvents := 4 // session.started, phase.started, phase.completed, session.completed
	if len(events) != expectedEvents {
		t.Errorf("Expected %d events, got %d", expectedEvents, len(events))
	}

	// Verify first event
	if events[0].EventTopic != "wayfinder.session.started" {
		t.Errorf("Expected first event topic 'wayfinder.session.started', got '%s'", events[0].EventTopic)
	}

	if events[0].SessionID != "test-session-123" {
		t.Errorf("Expected session ID 'test-session-123', got '%s'", events[0].SessionID)
	}

	// Verify phase event
	if events[1].Phase != "D1" {
		t.Errorf("Expected phase 'D1', got '%s'", events[1].Phase)
	}
}

// TestParseAll_FileNotFound tests error handling when file doesn't exist
func TestParseAll_FileNotFound(t *testing.T) {
	parser := NewParser("/nonexistent/path/telemetry.jsonl")
	_, err := parser.ParseAll()
	if err == nil {
		t.Error("Expected error for nonexistent file, got nil")
	}
	// Check error message contains expected text
	if !strings.Contains(err.Error(), "telemetry file not found") {
		t.Errorf("Expected file not found error, got: %v", err)
	}
}

// TestParseAll_MalformedJSON tests skipping malformed JSON lines
func TestParseAll_MalformedJSON(t *testing.T) {
	tmpDir := t.TempDir()
	telemetryPath := filepath.Join(tmpDir, "telemetry.jsonl")

	// Include malformed JSON line
	content := `{"timestamp":"2025-12-09T10:00:00Z","type":"eventbus_publish","agent":"wayfinder","data":{"event_topic":"wayfinder.session.started","session_id":"test-123"}}
{invalid json line here}
{"timestamp":"2025-12-09T10:00:05Z","type":"eventbus_publish","agent":"wayfinder","data":{"event_topic":"wayfinder.phase.started","session_id":"test-123","phase":"D1"}}
`
	if err := os.WriteFile(telemetryPath, []byte(content), 0600); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	parser := NewParser(telemetryPath)
	eventsBySession, err := parser.ParseAll()
	if err != nil {
		t.Fatalf("ParseAll() failed: %v", err)
	}

	// Should skip malformed line and parse the valid ones
	events := eventsBySession["test-123"]
	if len(events) != 2 {
		t.Errorf("Expected 2 valid events (malformed line skipped), got %d", len(events))
	}
}

// TestParseAll_NoWayfinderEvents tests telemetry with no Wayfinder events
func TestParseAll_NoWayfinderEvents(t *testing.T) {
	tmpDir := t.TempDir()
	telemetryPath := filepath.Join(tmpDir, "telemetry.jsonl")

	// Only non-Wayfinder events
	content := `{"timestamp":"2025-12-09T10:00:00Z","type":"eventbus_publish","agent":"claude-code","data":{"some":"data"}}
{"timestamp":"2025-12-09T10:00:05Z","type":"plugin_executed","agent":"other","data":{"plugin":"test"}}
`
	if err := os.WriteFile(telemetryPath, []byte(content), 0600); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	parser := NewParser(telemetryPath)
	eventsBySession, err := parser.ParseAll()
	if err != nil {
		t.Fatalf("ParseAll() failed: %v", err)
	}

	if len(eventsBySession) != 0 {
		t.Errorf("Expected 0 sessions, got %d", len(eventsBySession))
	}
}

// TestParseAll_EmptyFile tests parsing empty telemetry file
func TestParseAll_EmptyFile(t *testing.T) {
	tmpDir := t.TempDir()
	telemetryPath := filepath.Join(tmpDir, "empty.jsonl")

	if err := os.WriteFile(telemetryPath, []byte(""), 0600); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	parser := NewParser(telemetryPath)
	eventsBySession, err := parser.ParseAll()
	if err != nil {
		t.Fatalf("ParseAll() failed: %v", err)
	}

	if len(eventsBySession) != 0 {
		t.Errorf("Expected 0 sessions, got %d", len(eventsBySession))
	}
}

// TestParseSession_Success tests parsing specific session
func TestParseSession_Success(t *testing.T) {
	tmpDir := t.TempDir()
	telemetryPath := filepath.Join(tmpDir, "telemetry.jsonl")

	// Multiple sessions
	content := `{"timestamp":"2025-12-09T10:00:00Z","type":"eventbus_publish","agent":"wayfinder","data":{"event_topic":"wayfinder.session.started","session_id":"session-1"}}
{"timestamp":"2025-12-09T11:00:00Z","type":"eventbus_publish","agent":"wayfinder","data":{"event_topic":"wayfinder.session.started","session_id":"session-2"}}
{"timestamp":"2025-12-09T12:00:00Z","type":"eventbus_publish","agent":"wayfinder","data":{"event_topic":"wayfinder.phase.started","session_id":"session-1","phase":"D1"}}
`
	if err := os.WriteFile(telemetryPath, []byte(content), 0600); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	parser := NewParser(telemetryPath)
	events, err := parser.ParseSession("session-1")
	if err != nil {
		t.Fatalf("ParseSession() failed: %v", err)
	}

	// session-1 should have 2 events (started + phase started)
	if len(events) != 2 {
		t.Errorf("Expected 2 events for session-1, got %d", len(events))
	}
}

// TestParseSession_NotFound tests error when session doesn't exist
func TestParseSession_NotFound(t *testing.T) {
	tmpDir := t.TempDir()
	telemetryPath := filepath.Join(tmpDir, "telemetry.jsonl")

	content := `{"timestamp":"2025-12-09T10:00:00Z","type":"eventbus_publish","agent":"wayfinder","data":{"event_topic":"wayfinder.session.started","session_id":"session-1"}}
`
	if err := os.WriteFile(telemetryPath, []byte(content), 0600); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	parser := NewParser(telemetryPath)
	_, err := parser.ParseSession("nonexistent-session")
	if err == nil {
		t.Error("Expected error for nonexistent session, got nil")
	}
}

// TestParseAll_MultipleSessionsMultipleSessions tests multiple sessions
func TestParseAll_MultipleSessions(t *testing.T) {
	tmpDir := t.TempDir()
	telemetryPath := filepath.Join(tmpDir, "telemetry.jsonl")

	// Two complete sessions
	content := `{"timestamp":"2025-12-09T10:00:00Z","type":"eventbus_publish","agent":"wayfinder","data":{"event_topic":"wayfinder.session.started","session_id":"session-1"}}
{"timestamp":"2025-12-09T10:15:00Z","type":"eventbus_publish","agent":"wayfinder","data":{"event_topic":"wayfinder.session.completed","session_id":"session-1"}}
{"timestamp":"2025-12-09T11:00:00Z","type":"eventbus_publish","agent":"wayfinder","data":{"event_topic":"wayfinder.session.started","session_id":"session-2"}}
{"timestamp":"2025-12-09T11:30:00Z","type":"eventbus_publish","agent":"wayfinder","data":{"event_topic":"wayfinder.session.completed","session_id":"session-2"}}
`
	if err := os.WriteFile(telemetryPath, []byte(content), 0600); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	parser := NewParser(telemetryPath)
	eventsBySession, err := parser.ParseAll()
	if err != nil {
		t.Fatalf("ParseAll() failed: %v", err)
	}

	if len(eventsBySession) != 2 {
		t.Errorf("Expected 2 sessions, got %d", len(eventsBySession))
	}

	if len(eventsBySession["session-1"]) != 2 {
		t.Errorf("Expected 2 events for session-1, got %d", len(eventsBySession["session-1"]))
	}

	if len(eventsBySession["session-2"]) != 2 {
		t.Errorf("Expected 2 events for session-2, got %d", len(eventsBySession["session-2"]))
	}
}

// Benchmark_ParseAll benchmarks parsing performance
func Benchmark_ParseAll(b *testing.B) {
	tmpDir := b.TempDir()
	telemetryPath := filepath.Join(tmpDir, "telemetry.jsonl")

	// Generate 1000 events
	var content string
	timestamp := time.Now()
	for i := 0; i < 1000; i++ {
		content += `{"timestamp":"` + timestamp.Format(time.RFC3339) + `","type":"eventbus_publish","agent":"wayfinder","data":{"event_topic":"wayfinder.phase.started","session_id":"bench-session","phase":"D1"}}` + "\n"
		timestamp = timestamp.Add(time.Second)
	}

	if err := os.WriteFile(telemetryPath, []byte(content), 0600); err != nil {
		b.Fatalf("Failed to create test file: %v", err)
	}

	parser := NewParser(telemetryPath)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := parser.ParseAll()
		if err != nil {
			b.Fatalf("ParseAll() failed: %v", err)
		}
	}
}
