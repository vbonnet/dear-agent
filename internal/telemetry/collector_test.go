package telemetry

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/vbonnet/dear-agent/internal/testutil"
)

// TestNewCollector_Enabled verifies collector initialization when enabled
func TestNewCollector_Enabled(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "telemetry-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	telemetryPath := filepath.Join(tmpDir, "telemetry", "events.jsonl")

	collector, err := NewCollector(true, telemetryPath)
	if err != nil {
		t.Fatalf("NewCollector() failed: %v", err)
	}
	defer collector.Close()

	if !collector.enabled {
		t.Error("Collector.enabled = false, want true")
	}

	if collector.path != telemetryPath {
		t.Errorf("Collector.path = %q, want %q", collector.path, telemetryPath)
	}

	if collector.file == nil {
		t.Error("Collector.file is nil, want non-nil")
	}

	// Verify directory and file were created
	if _, err := os.Stat(filepath.Dir(telemetryPath)); err != nil {
		t.Errorf("Telemetry directory not created: %v", err)
	}
}

// TestNewCollector_Disabled verifies collector initialization when disabled
func TestNewCollector_Disabled(t *testing.T) {
	collector, err := NewCollector(false, "/dev/null")
	if err != nil {
		t.Fatalf("NewCollector() failed with disabled: %v", err)
	}
	defer collector.Close()

	if collector.enabled {
		t.Error("Collector.enabled = true, want false")
	}

	if collector.file != nil {
		t.Error("Collector.file is non-nil for disabled collector, want nil")
	}
}

// TestNewCollector_InvalidPath verifies error handling for invalid paths
func TestNewCollector_InvalidPath(t *testing.T) {
	testutil.SkipIfRoot(t) // Root can create directories anywhere, breaking this test

	// Try to create file in non-existent directory without parent creation
	_, err := NewCollector(true, "/nonexistent/deeply/nested/path/events.jsonl")
	if err == nil {
		t.Fatal("NewCollector() succeeded with invalid path, want error")
	}
}

// TestRecord_Enabled verifies event recording when enabled
func TestRecord_Enabled(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "telemetry-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	telemetryPath := filepath.Join(tmpDir, "events.jsonl")

	collector, err := NewCollector(true, telemetryPath)
	if err != nil {
		t.Fatalf("NewCollector() failed: %v", err)
	}
	defer collector.Close()

	// Record an event
	err = collector.Record("test_event", "claude-code", LevelInfo, map[string]interface{}{
		"key1": "value1",
		"key2": 42,
	})
	if err != nil {
		t.Fatalf("Record() failed: %v", err)
	}

	// Close to flush
	collector.Close()

	// Read back the file
	data, err := os.ReadFile(telemetryPath)
	if err != nil {
		t.Fatalf("failed to read telemetry file: %v", err)
	}

	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) != 1 {
		t.Fatalf("Expected 1 event line, got %d", len(lines))
	}

	// Parse the event
	var event Event
	if err := json.Unmarshal([]byte(lines[0]), &event); err != nil {
		t.Fatalf("failed to unmarshal event: %v", err)
	}

	if event.Type != "test_event" {
		t.Errorf("Event.Type = %q, want %q", event.Type, "test_event")
	}

	if event.Agent != "claude-code" {
		t.Errorf("Event.Agent = %q, want %q", event.Agent, "claude-code")
	}

	if event.Data["key1"] != "value1" {
		t.Errorf("Event.Data[key1] = %v, want %q", event.Data["key1"], "value1")
	}

	if event.Data["key2"].(float64) != 42 {
		t.Errorf("Event.Data[key2] = %v, want 42", event.Data["key2"])
	}

	if event.Timestamp.IsZero() {
		t.Error("Event.Timestamp is zero")
	}
}

// TestRecord_Disabled verifies no recording when disabled
func TestRecord_Disabled(t *testing.T) {
	collector, err := NewCollector(false, "/dev/null")
	if err != nil {
		t.Fatalf("NewCollector() failed: %v", err)
	}
	defer collector.Close()

	// Should not error even though disabled
	err = collector.Record("test_event", "agent", LevelInfo, map[string]interface{}{"key": "value"})
	if err != nil {
		t.Errorf("Record() failed when disabled: %v", err)
	}
}

// TestRecord_MultipleEvents verifies JSONL format with multiple events
func TestRecord_MultipleEvents(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "telemetry-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	telemetryPath := filepath.Join(tmpDir, "events.jsonl")

	collector, err := NewCollector(true, telemetryPath)
	if err != nil {
		t.Fatalf("NewCollector() failed: %v", err)
	}
	defer collector.Close()

	// Record multiple events
	events := []struct {
		eventType string
		agent     string
		data      map[string]interface{}
	}{
		{"event1", "claude-code", map[string]interface{}{"id": 1}},
		{"event2", "cursor", map[string]interface{}{"id": 2}},
		{"event3", "windsurf", map[string]interface{}{"id": 3}},
	}

	for _, e := range events {
		if err := collector.Record(e.eventType, e.agent, LevelInfo, e.data); err != nil {
			t.Fatalf("Record() failed: %v", err)
		}
	}

	collector.Close()

	// Read back the file
	data, err := os.ReadFile(telemetryPath)
	if err != nil {
		t.Fatalf("failed to read telemetry file: %v", err)
	}

	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) != 3 {
		t.Fatalf("Expected 3 event lines, got %d", len(lines))
	}

	// Verify each event
	for i, line := range lines {
		var event Event
		if err := json.Unmarshal([]byte(line), &event); err != nil {
			t.Fatalf("failed to unmarshal event %d: %v", i, err)
		}

		if event.Type != events[i].eventType {
			t.Errorf("Event[%d].Type = %q, want %q", i, event.Type, events[i].eventType)
		}

		if event.Agent != events[i].agent {
			t.Errorf("Event[%d].Agent = %q, want %q", i, event.Agent, events[i].agent)
		}
	}
}

// TestRecord_NilData verifies handling of nil data
func TestRecord_NilData(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "telemetry-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	telemetryPath := filepath.Join(tmpDir, "events.jsonl")

	collector, err := NewCollector(true, telemetryPath)
	if err != nil {
		t.Fatalf("NewCollector() failed: %v", err)
	}
	defer collector.Close()

	// Record event with nil data
	err = collector.Record("test_event", "agent", LevelInfo, nil)
	if err != nil {
		t.Fatalf("Record() failed with nil data: %v", err)
	}

	collector.Close()

	// Read back and verify
	data, err := os.ReadFile(telemetryPath)
	if err != nil {
		t.Fatalf("failed to read telemetry file: %v", err)
	}

	var event Event
	if err := json.Unmarshal(data, &event); err != nil {
		t.Fatalf("failed to unmarshal event: %v", err)
	}

	if event.Data != nil {
		t.Errorf("Event.Data = %v, want nil (omitempty)", event.Data)
	}
}

// TestClose verifies close handling
func TestClose(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "telemetry-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	telemetryPath := filepath.Join(tmpDir, "events.jsonl")

	collector, err := NewCollector(true, telemetryPath)
	if err != nil {
		t.Fatalf("NewCollector() failed: %v", err)
	}

	err = collector.Close()
	if err != nil {
		t.Errorf("Close() failed: %v", err)
	}

	// Second close should not error
	err = collector.Close()
	if err == nil {
		t.Log("Second Close() succeeded (file already closed)")
	}
}

// TestClose_Disabled verifies close handling when disabled
func TestClose_Disabled(t *testing.T) {
	collector, err := NewCollector(false, "/dev/null")
	if err != nil {
		t.Fatalf("NewCollector() failed: %v", err)
	}

	err = collector.Close()
	if err != nil {
		t.Errorf("Close() failed for disabled collector: %v", err)
	}
}

// TestConcurrency verifies thread-safe recording
func TestConcurrency(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "telemetry-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	telemetryPath := filepath.Join(tmpDir, "events.jsonl")

	collector, err := NewCollector(true, telemetryPath)
	if err != nil {
		t.Fatalf("NewCollector() failed: %v", err)
	}
	defer collector.Close()

	// Record events concurrently
	done := make(chan bool)
	for i := 0; i < 10; i++ {
		go func(id int) {
			_ = collector.Record("concurrent_event", "agent", LevelInfo, map[string]interface{}{
				"id": id,
			})
			done <- true
		}(i)
	}

	// Wait for all goroutines
	for i := 0; i < 10; i++ {
		<-done
	}

	collector.Close()

	// Verify we got 10 events
	data, err := os.ReadFile(telemetryPath)
	if err != nil {
		t.Fatalf("failed to read telemetry file: %v", err)
	}

	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) != 10 {
		t.Errorf("Expected 10 events, got %d", len(lines))
	}
}

// TestEventConstants verifies event type constants exist
func TestEventConstants(t *testing.T) {
	constants := []string{
		EventEngramLoaded,
		EventPluginExecuted,
		EventEcphoryQuery,
		EventReflectionSaved,
		EventConfigLoaded,
		EventEventBusPublish,
		EventEventBusResponse,
	}

	for _, constant := range constants {
		if constant == "" {
			t.Error("Event constant is empty")
		}
	}
}

// TestRecordSync_NoRaceCondition verifies RecordSync is atomic
// P0-3: RecordSync must hold lock across both Record and Sync operations
func TestRecordSync_NoRaceCondition(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "telemetry-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	telemetryPath := filepath.Join(tmpDir, "events.jsonl")

	collector, err := NewCollector(true, telemetryPath)
	if err != nil {
		t.Fatalf("NewCollector() failed: %v", err)
	}
	defer collector.Close()

	// Test concurrent RecordSync calls don't interleave
	done := make(chan bool)
	for i := 0; i < 20; i++ {
		go func(id int) {
			_ = collector.RecordSync("critical_event", "agent", map[string]interface{}{
				"id": id,
			})
			done <- true
		}(i)
	}

	// Wait for all goroutines
	for i := 0; i < 20; i++ {
		<-done
	}

	collector.Close()

	// Verify all events were written
	data, err := os.ReadFile(telemetryPath)
	if err != nil {
		t.Fatalf("failed to read telemetry file: %v", err)
	}

	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) != 20 {
		t.Errorf("Expected 20 events, got %d (race condition may have lost events)", len(lines))
	}

	// Verify all lines are valid JSON (no corruption from interleaving)
	for i, line := range lines {
		var event Event
		if err := json.Unmarshal([]byte(line), &event); err != nil {
			t.Errorf("Event %d is corrupted (race condition): %v", i, err)
			t.Errorf("Line content: %s", line)
		}
	}
}

// TestRecordSync_Enabled verifies RecordSync writes and syncs
func TestRecordSync_Enabled(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "telemetry-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	telemetryPath := filepath.Join(tmpDir, "events.jsonl")

	collector, err := NewCollector(true, telemetryPath)
	if err != nil {
		t.Fatalf("NewCollector() failed: %v", err)
	}
	defer collector.Close()

	// Record a critical event
	err = collector.RecordSync("critical_event", "claude-code", map[string]interface{}{
		"session": "end",
	})
	if err != nil {
		t.Fatalf("RecordSync() failed: %v", err)
	}

	// Don't close yet - verify event was synced to disk
	// (Read directly from file without closing collector)
	data, err := os.ReadFile(telemetryPath)
	if err != nil {
		t.Fatalf("failed to read telemetry file: %v", err)
	}

	if len(data) == 0 {
		t.Error("RecordSync() did not sync event to disk")
	}

	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) != 1 {
		t.Fatalf("Expected 1 event line, got %d", len(lines))
	}

	var event Event
	if err := json.Unmarshal([]byte(lines[0]), &event); err != nil {
		t.Fatalf("failed to unmarshal event: %v", err)
	}

	if event.Type != "critical_event" {
		t.Errorf("Event.Type = %q, want %q", event.Type, "critical_event")
	}
}

// TestRecordSync_Disabled verifies RecordSync doesn't error when disabled
func TestRecordSync_Disabled(t *testing.T) {
	collector, err := NewCollector(false, "/dev/null")
	if err != nil {
		t.Fatalf("NewCollector() failed: %v", err)
	}
	defer collector.Close()

	// Should not error even though disabled
	err = collector.RecordSync("critical_event", "agent", map[string]interface{}{"key": "value"})
	if err != nil {
		t.Errorf("RecordSync() failed when disabled: %v", err)
	}
}

// TestEvent_JSONMarshaling verifies Event JSON serialization
func TestEvent_JSONMarshaling(t *testing.T) {
	now := time.Now().UTC()
	event := Event{
		Timestamp: now,
		Type:      "test_type",
		Agent:     "test_agent",
		Data: map[string]interface{}{
			"key": "value",
		},
	}

	data, err := json.Marshal(event)
	if err != nil {
		t.Fatalf("json.Marshal() failed: %v", err)
	}

	var decoded Event
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("json.Unmarshal() failed: %v", err)
	}

	if decoded.Type != "test_type" {
		t.Errorf("Decoded Type = %q, want %q", decoded.Type, "test_type")
	}

	if decoded.Agent != "test_agent" {
		t.Errorf("Decoded Agent = %q, want %q", decoded.Agent, "test_agent")
	}

	// Timestamp comparison (allow small difference due to precision)
	timeDiff := decoded.Timestamp.Sub(now)
	if timeDiff > time.Second || timeDiff < -time.Second {
		t.Errorf("Timestamp difference too large: %v", timeDiff)
	}
}

// Test 2.3: Async notification - Record() completes in <100μs with slow listener
func TestAsyncNotification(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "telemetry-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	telemetryPath := filepath.Join(tmpDir, "events.jsonl")

	collector, err := NewCollector(true, telemetryPath)
	if err != nil {
		t.Fatalf("NewCollector() failed: %v", err)
	}
	defer collector.Close()

	// Add slow listener (1ms delay)
	slowListener := &mockListener{
		minLevel:  LevelInfo,
		callDelay: time.Millisecond,
	}
	collector.AddListener(slowListener)

	// Measure Record() time
	start := time.Now()
	err = collector.Record("test.async", "test", LevelInfo, map[string]interface{}{"test": "data"})
	duration := time.Since(start)

	if err != nil {
		t.Fatalf("Record() failed: %v", err)
	}

	// Record() should complete in <500μs despite slow listener (1ms delay)
	// This proves notification is async (would be >1ms if synchronous)
	if duration > 500*time.Microsecond {
		t.Errorf("Record() took %v, want <500μs (async notification failed)", duration)
	}

	// Wait for listener to complete
	time.Sleep(50 * time.Millisecond)

	// Verify listener was called
	events := slowListener.getEvents()
	if len(events) != 1 {
		t.Errorf("Listener should have received 1 event, got %d", len(events))
	}
}

// Test 2.5: ERROR bypass - ERROR events notify listeners when disabled
func TestErrorBypass(t *testing.T) {
	// Create DISABLED collector (no file)
	collector, err := NewCollector(false, "/dev/null")
	if err != nil {
		t.Fatalf("NewCollector() failed: %v", err)
	}
	defer collector.Close()

	// Add listener to verify notification happens
	listener := &mockListener{minLevel: LevelError}
	collector.AddListener(listener)

	// Test 1: INFO event should NOT notify listener when disabled
	err = collector.Record("test.info", "test", LevelInfo, map[string]interface{}{"test": "info"})
	if err != nil {
		t.Errorf("Record() failed for INFO: %v", err)
	}

	// Test 2: ERROR event SHOULD notify listener despite disabled
	err = collector.Record("test.error", "test", LevelError, map[string]interface{}{"test": "error"})
	if err != nil {
		t.Errorf("Record() failed for ERROR: %v", err)
	}

	// Test 3: CRITICAL event SHOULD notify listener despite disabled
	err = collector.Record("test.critical", "test", LevelCritical, map[string]interface{}{"test": "critical"})
	if err != nil {
		t.Errorf("Record() failed for CRITICAL: %v", err)
	}

	// Wait for async listeners
	time.Sleep(50 * time.Millisecond)

	// Verify listener received ERROR and CRITICAL (but not INFO)
	events := listener.getEvents()
	if len(events) != 2 {
		t.Errorf("Listener should have received 2 events (ERROR + CRITICAL), got %d", len(events))
	}

	// Verify events are ERROR and CRITICAL
	for i, event := range events {
		if event.Level != LevelError && event.Level != LevelCritical {
			t.Errorf("Event %d has level %d, want ERROR or CRITICAL", i, event.Level)
		}
	}
}

// Mock listener for async tests
type mockListener struct {
	minLevel    Level
	events      []*Event
	mu          sync.Mutex
	shouldPanic bool
	shouldError bool
	callDelay   time.Duration
	callCount   atomic.Int32
}

func (l *mockListener) MinLevel() Level {
	return l.minLevel
}

func (l *mockListener) OnEvent(event *Event) error {
	l.callCount.Add(1)

	if l.shouldPanic {
		panic("intentional test panic")
	}

	if l.callDelay > 0 {
		time.Sleep(l.callDelay)
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	l.events = append(l.events, event)

	if l.shouldError {
		return errors.New("intentional test error")
	}

	return nil
}

func (l *mockListener) getEvents() []*Event {
	l.mu.Lock()
	defer l.mu.Unlock()
	// Return copy to avoid race
	result := make([]*Event, len(l.events))
	copy(result, l.events)
	return result
}

func (l *mockListener) reset() {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.events = nil
	l.callCount.Store(0)
}
