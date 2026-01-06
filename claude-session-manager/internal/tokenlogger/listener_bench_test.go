package tokenlogger

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/vbonnet/engram/core/pkg/telemetry"
)

// BenchmarkWriteEvent measures file write performance
// Target: <500μs per write (from D4 requirements)
func BenchmarkWriteEvent(b *testing.B) {
	tmpDir := b.TempDir()
	logPath := filepath.Join(tmpDir, "bench-token-usage.jsonl")

	event := &telemetry.Event{
		Timestamp: time.Now(),
		Type:      "benchmark.event",
		Agent:     "bench",
		Level:     telemetry.LevelError,
		Data:      map[string]interface{}{"tokens": 1000},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := writeEvent(logPath, event); err != nil {
			b.Fatalf("writeEvent failed: %v", err)
		}
	}
}

// BenchmarkSessionDetection_FirstCall measures initial session detection
// Target: <10ms first call (from D4 requirements)
func BenchmarkSessionDetection_FirstCall(b *testing.B) {
	// Override getSessionUUID to simulate CSM command execution
	oldGetSessionUUID := getSessionUUID
	defer func() { getSessionUUID = oldGetSessionUUID }()
	getSessionUUID = func() string {
		// Simulate subprocess overhead (~5ms)
		time.Sleep(5 * time.Millisecond)
		return "bench-session-uuid"
	}

	for i := 0; i < b.N; i++ {
		logger := NewTokenLogger()
		_ = logger.getSessionDirLocked()
	}
}

// BenchmarkSessionDetection_Cached measures cached session detection
// Target: <1μs cached access (from D4 requirements)
func BenchmarkSessionDetection_Cached(b *testing.B) {
	oldGetSessionUUID := getSessionUUID
	defer func() { getSessionUUID = oldGetSessionUUID }()
	getSessionUUID = func() string {
		return "bench-session-uuid"
	}

	logger := NewTokenLogger()
	// Prime the cache
	_ = logger.getSessionDirLocked()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = logger.getSessionDirLocked()
	}
}

// BenchmarkOnEvent_WithSession measures full OnEvent cycle
func BenchmarkOnEvent_WithSession(b *testing.B) {
	tmpDir := b.TempDir()
	sessionUUID := "bench-session-uuid"
	sessionDir := filepath.Join(tmpDir, sessionUUID)
	if err := os.MkdirAll(sessionDir, 0755); err != nil {
		b.Fatalf("Failed to create session directory: %v", err)
	}

	oldGetSessionUUID := getSessionUUID
	defer func() { getSessionUUID = oldGetSessionUUID }()
	getSessionUUID = func() string { return sessionUUID }

	logger := NewTokenLogger()
	logger.sessionDir = sessionDir
	logger.cacheChecked = true

	event := &telemetry.Event{
		Timestamp: time.Now(),
		Type:      "benchmark.event",
		Agent:     "bench",
		Level:     telemetry.LevelError,
		Data:      map[string]interface{}{"tokens": 1000},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := logger.OnEvent(event); err != nil {
			b.Fatalf("OnEvent failed: %v", err)
		}
	}
}

// BenchmarkOnEvent_NoSession measures graceful degradation performance
func BenchmarkOnEvent_NoSession(b *testing.B) {
	oldGetSessionUUID := getSessionUUID
	defer func() { getSessionUUID = oldGetSessionUUID }()
	getSessionUUID = func() string { return "" }

	logger := NewTokenLogger()

	event := &telemetry.Event{
		Timestamp: time.Now(),
		Type:      "benchmark.event",
		Agent:     "bench",
		Level:     telemetry.LevelError,
		Data:      map[string]interface{}{"tokens": 1000},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := logger.OnEvent(event); err != nil {
			b.Fatalf("OnEvent failed: %v", err)
		}
	}
}
