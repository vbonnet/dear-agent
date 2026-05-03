package telemetry

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

// TestEcphoryAuditLogger_New verifies logger construction
func TestEcphoryAuditLogger_New(t *testing.T) {
	logger := NewEcphoryAuditLogger("/tmp/test-logs")

	if logger == nil {
		t.Fatal("NewEcphoryAuditLogger() returned nil")
	}

	if logger.logDir != "/tmp/test-logs" {
		t.Errorf("logDir = %q, want %q", logger.logDir, "/tmp/test-logs")
	}

	if logger.filename != "ecphory-audit.jsonl" {
		t.Errorf("filename = %q, want %q", logger.filename, "ecphory-audit.jsonl")
	}
}

// TestEcphoryAuditLogger_MinLevel verifies level filtering
func TestEcphoryAuditLogger_MinLevel(t *testing.T) {
	logger := NewEcphoryAuditLogger("/tmp/test-logs")

	if logger.MinLevel() != LevelInfo {
		t.Errorf("MinLevel() = %v, want %v", logger.MinLevel(), LevelInfo)
	}
}

// TestEcphoryAuditLogger_OnEvent_Success verifies event logging
func TestEcphoryAuditLogger_OnEvent_Success(t *testing.T) {
	tmpDir := t.TempDir()

	logger := NewEcphoryAuditLogger(tmpDir)

	// Create test event
	event := &Event{
		Type:      EventEcphoryAuditCompleted,
		Agent:     "test",
		Level:     LevelInfo,
		Timestamp: time.Now(),
		Data: map[string]interface{}{
			"session_id":          "test-123",
			"total_retrievals":    10,
			"appropriate_count":   9,
			"inappropriate_count": 1,
			"correctness_score":   0.90,
		},
	}

	// Log event
	if err := logger.OnEvent(event); err != nil {
		t.Fatalf("OnEvent() failed: %v", err)
	}

	// Verify file exists
	logPath := filepath.Join(tmpDir, "ecphory-audit.jsonl")
	if _, err := os.Stat(logPath); err != nil {
		t.Fatalf("log file not created: %v", err)
	}

	// Read and verify content
	content, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("failed to read log file: %v", err)
	}

	var entry map[string]interface{}
	if err := json.Unmarshal(content, &entry); err != nil {
		t.Fatalf("failed to unmarshal log entry: %v", err)
	}

	if entry["event_type"] != EventEcphoryAuditCompleted {
		t.Errorf("event_type = %v, want %v", entry["event_type"], EventEcphoryAuditCompleted)
	}

	data, ok := entry["data"].(map[string]interface{})
	if !ok {
		t.Fatal("data field is not a map")
	}

	if data["session_id"] != "test-123" {
		t.Errorf("session_id = %v, want %v", data["session_id"], "test-123")
	}

	if data["correctness_score"] != 0.90 {
		t.Errorf("correctness_score = %v, want %v", data["correctness_score"], 0.90)
	}
}

// TestEcphoryAuditLogger_OnEvent_IgnoresOtherEvents verifies event filtering
func TestEcphoryAuditLogger_OnEvent_IgnoresOtherEvents(t *testing.T) {
	tmpDir := t.TempDir()

	logger := NewEcphoryAuditLogger(tmpDir)

	// Create wrong event type
	event := &Event{
		Type:      EventPersonaReviewCompleted,
		Agent:     "test",
		Level:     LevelInfo,
		Timestamp: time.Now(),
		Data:      map[string]interface{}{"test": "data"},
	}

	// Should not log anything
	if err := logger.OnEvent(event); err != nil {
		t.Fatalf("OnEvent() failed: %v", err)
	}

	// Verify no file created
	logPath := filepath.Join(tmpDir, "ecphory-audit.jsonl")
	if _, err := os.Stat(logPath); err == nil {
		t.Error("log file created for wrong event type, want no file")
	}
}

// TestEcphoryAuditLogger_OnEvent_Concurrent verifies thread safety
func TestEcphoryAuditLogger_OnEvent_Concurrent(t *testing.T) {
	tmpDir := t.TempDir()

	logger := NewEcphoryAuditLogger(tmpDir)

	// Write concurrently from multiple goroutines
	const numGoroutines = 10
	var wg sync.WaitGroup
	wg.Add(numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		go func(id int) {
			defer wg.Done()
			event := &Event{
				Type:      EventEcphoryAuditCompleted,
				Agent:     "test",
				Level:     LevelInfo,
				Timestamp: time.Now(),
				Data:      map[string]interface{}{"goroutine_id": id},
			}
			if err := logger.OnEvent(event); err != nil {
				t.Errorf("OnEvent() failed in goroutine %d: %v", id, err)
			}
		}(i)
	}

	wg.Wait()

	// Verify all events were logged (10 lines)
	logPath := filepath.Join(tmpDir, "ecphory-audit.jsonl")
	content, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("failed to read log file: %v", err)
	}

	lines := 0
	for _, b := range content {
		if b == '\n' {
			lines++
		}
	}

	if lines != numGoroutines {
		t.Errorf("got %d log lines, want %d", lines, numGoroutines)
	}
}

// TestPersonaEffectivenessLogger_New verifies logger construction
func TestPersonaEffectivenessLogger_New(t *testing.T) {
	logger := NewPersonaEffectivenessLogger("/tmp/test-logs")

	if logger == nil {
		t.Fatal("NewPersonaEffectivenessLogger() returned nil")
	}

	if logger.logDir != "/tmp/test-logs" {
		t.Errorf("logDir = %q, want %q", logger.logDir, "/tmp/test-logs")
	}

	if logger.filename != "persona-effectiveness.jsonl" {
		t.Errorf("filename = %q, want %q", logger.filename, "persona-effectiveness.jsonl")
	}
}

// TestPersonaEffectivenessLogger_MinLevel verifies level filtering
func TestPersonaEffectivenessLogger_MinLevel(t *testing.T) {
	logger := NewPersonaEffectivenessLogger("/tmp/test-logs")

	if logger.MinLevel() != LevelInfo {
		t.Errorf("MinLevel() = %v, want %v", logger.MinLevel(), LevelInfo)
	}
}

// TestPersonaEffectivenessLogger_OnEvent_Success verifies event logging
func TestPersonaEffectivenessLogger_OnEvent_Success(t *testing.T) {
	tmpDir := t.TempDir()

	logger := NewPersonaEffectivenessLogger(tmpDir)

	// Create test event
	event := &Event{
		Type:      EventPersonaReviewCompleted,
		Agent:     "test",
		Level:     LevelInfo,
		Timestamp: time.Now(),
		Data: map[string]interface{}{
			"session_id":       "test-456",
			"persona_id":       "reuse-advocate",
			"issues_found":     2,
			"severity":         "medium",
			"time_overhead_ms": 350,
		},
	}

	// Log event
	if err := logger.OnEvent(event); err != nil {
		t.Fatalf("OnEvent() failed: %v", err)
	}

	// Verify file exists and content
	logPath := filepath.Join(tmpDir, "persona-effectiveness.jsonl")
	content, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("failed to read log file: %v", err)
	}

	var entry map[string]interface{}
	if err := json.Unmarshal(content, &entry); err != nil {
		t.Fatalf("failed to unmarshal log entry: %v", err)
	}

	if entry["event_type"] != EventPersonaReviewCompleted {
		t.Errorf("event_type = %v, want %v", entry["event_type"], EventPersonaReviewCompleted)
	}

	data, ok := entry["data"].(map[string]interface{})
	if !ok {
		t.Fatal("data field is not a map")
	}

	if data["persona_id"] != "reuse-advocate" {
		t.Errorf("persona_id = %v, want %v", data["persona_id"], "reuse-advocate")
	}
}

// TestPersonaEffectivenessLogger_OnEvent_IgnoresOtherEvents verifies filtering
func TestPersonaEffectivenessLogger_OnEvent_IgnoresOtherEvents(t *testing.T) {
	tmpDir := t.TempDir()

	logger := NewPersonaEffectivenessLogger(tmpDir)

	// Wrong event type
	event := &Event{
		Type:      EventEcphoryAuditCompleted,
		Agent:     "test",
		Level:     LevelInfo,
		Timestamp: time.Now(),
		Data:      map[string]interface{}{"test": "data"},
	}

	if err := logger.OnEvent(event); err != nil {
		t.Fatalf("OnEvent() failed: %v", err)
	}

	// Verify no file created
	logPath := filepath.Join(tmpDir, "persona-effectiveness.jsonl")
	if _, err := os.Stat(logPath); err == nil {
		t.Error("log file created for wrong event type")
	}
}

// TestWayfinderROILogger_New verifies logger construction
func TestWayfinderROILogger_New(t *testing.T) {
	logger := NewWayfinderROILogger("/tmp/test-logs")

	if logger == nil {
		t.Fatal("NewWayfinderROILogger() returned nil")
	}

	if logger.logDir != "/tmp/test-logs" {
		t.Errorf("logDir = %q, want %q", logger.logDir, "/tmp/test-logs")
	}

	if logger.filename != "wayfinder-roi.jsonl" {
		t.Errorf("filename = %q, want %q", logger.filename, "wayfinder-roi.jsonl")
	}
}

// TestWayfinderROILogger_MinLevel verifies level filtering
func TestWayfinderROILogger_MinLevel(t *testing.T) {
	logger := NewWayfinderROILogger("/tmp/test-logs")

	if logger.MinLevel() != LevelInfo {
		t.Errorf("MinLevel() = %v, want %v", logger.MinLevel(), LevelInfo)
	}
}

// TestWayfinderROILogger_OnEvent_Success verifies event logging
func TestWayfinderROILogger_OnEvent_Success(t *testing.T) {
	tmpDir := t.TempDir()

	logger := NewWayfinderROILogger(tmpDir)

	// Create test event
	event := &Event{
		Type:      EventPhaseTransitionCompleted,
		Agent:     "test",
		Level:     LevelInfo,
		Timestamp: time.Now(),
		Data: map[string]interface{}{
			"session_id":   "test-789",
			"phase_name":   "D3",
			"outcome":      "success",
			"duration_ms":  120000,
			"error_count":  0,
			"rework_count": 0,
		},
	}

	// Log event
	if err := logger.OnEvent(event); err != nil {
		t.Fatalf("OnEvent() failed: %v", err)
	}

	// Verify file exists and content
	logPath := filepath.Join(tmpDir, "wayfinder-roi.jsonl")
	content, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("failed to read log file: %v", err)
	}

	var entry map[string]interface{}
	if err := json.Unmarshal(content, &entry); err != nil {
		t.Fatalf("failed to unmarshal log entry: %v", err)
	}

	if entry["event_type"] != EventPhaseTransitionCompleted {
		t.Errorf("event_type = %v, want %v", entry["event_type"], EventPhaseTransitionCompleted)
	}

	data, ok := entry["data"].(map[string]interface{})
	if !ok {
		t.Fatal("data field is not a map")
	}

	if data["phase_name"] != "D3" {
		t.Errorf("phase_name = %v, want %v", data["phase_name"], "D3")
	}
}

// TestWayfinderROILogger_OnEvent_IgnoresOtherEvents verifies filtering
func TestWayfinderROILogger_OnEvent_IgnoresOtherEvents(t *testing.T) {
	tmpDir := t.TempDir()

	logger := NewWayfinderROILogger(tmpDir)

	// Wrong event type
	event := &Event{
		Type:      EventEcphoryAuditCompleted,
		Agent:     "test",
		Level:     LevelInfo,
		Timestamp: time.Now(),
		Data:      map[string]interface{}{"test": "data"},
	}

	if err := logger.OnEvent(event); err != nil {
		t.Fatalf("OnEvent() failed: %v", err)
	}

	// Verify no file created
	logPath := filepath.Join(tmpDir, "wayfinder-roi.jsonl")
	if _, err := os.Stat(logPath); err == nil {
		t.Error("log file created for wrong event type")
	}
}
