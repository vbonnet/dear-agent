package telemetry

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// WayfinderROILogger logs Wayfinder phase transitions and ROI metrics to JSONL.
//
// Event format (Data field):
//
//	{
//	  "session_id": "abc123",
//	  "timestamp": "2024-01-01T12:00:00Z",
//	  "phase_name": "D3",
//	  "outcome": "success",
//	  "duration_ms": 120000,
//	  "error_count": 0,
//	  "rework_count": 0,
//	  "quality_score": 1.0,
//	  "metrics": {
//	    "ai_time_ms": 100000,
//	    "wait_time_ms": 20000,
//	    "estimated_cost_usd": 0.15
//	  }
//	}
//
// Output: ~/.engram/logs/wayfinder-roi.jsonl
type WayfinderROILogger struct {
	mu       sync.Mutex
	logDir   string
	filename string
}

// NewWayfinderROILogger creates a new JSONL logger for Wayfinder ROI tracking.
func NewWayfinderROILogger(logDir string) *WayfinderROILogger {
	return &WayfinderROILogger{
		logDir:   logDir,
		filename: "wayfinder-roi.jsonl",
	}
}

// MinLevel returns INFO (log all phase transition events).
func (l *WayfinderROILogger) MinLevel() Level {
	return LevelInfo
}

// OnEvent appends phase transition events to JSONL file.
func (l *WayfinderROILogger) OnEvent(event *Event) error {
	// Filter: only process phase_transition_completed events
	if event.Type != EventPhaseTransitionCompleted {
		return nil
	}

	// Ensure log directory exists
	if err := os.MkdirAll(l.logDir, 0755); err != nil {
		return fmt.Errorf("failed to create log directory: %w", err)
	}

	// Prepare JSONL entry
	entry := map[string]interface{}{
		"timestamp":  event.Timestamp.Format(time.RFC3339),
		"event_type": event.Type,
		"data":       event.Data,
	}

	// Marshal to JSON
	jsonBytes, err := json.Marshal(entry)
	if err != nil {
		return fmt.Errorf("failed to marshal phase event: %w", err)
	}

	// Append to file (thread-safe)
	l.mu.Lock()
	defer l.mu.Unlock()

	logPath := filepath.Join(l.logDir, l.filename)
	f, err := os.OpenFile(logPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("failed to open log file: %w", err)
	}
	defer f.Close()

	// Write JSON line
	if _, err := f.Write(append(jsonBytes, '\n')); err != nil {
		return fmt.Errorf("failed to write to log file: %w", err)
	}

	return nil
}
