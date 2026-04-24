package telemetry

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// PersonaEffectivenessLogger logs persona review results to JSONL.
//
// Event format (Data field):
//
//	{
//	  "session_id": "abc123",
//	  "timestamp": "2024-01-01T12:00:00Z",
//	  "persona_id": "reuse-advocate",
//	  "issues_found": 2,
//	  "severity": "medium",
//	  "time_overhead_ms": 350,
//	  "false_positives": 0,
//	  "classification_metadata": {
//	    "language": "go",
//	    "pattern": "duplicate_code",
//	    "confidence": 0.95
//	  }
//	}
//
// Output: ~/.engram/logs/persona-effectiveness.jsonl
type PersonaEffectivenessLogger struct {
	mu       sync.Mutex
	logDir   string
	filename string
}

// NewPersonaEffectivenessLogger creates a new JSONL logger for persona reviews.
func NewPersonaEffectivenessLogger(logDir string) *PersonaEffectivenessLogger {
	return &PersonaEffectivenessLogger{
		logDir:   logDir,
		filename: "persona-effectiveness.jsonl",
	}
}

// MinLevel returns INFO (log all persona review events).
func (l *PersonaEffectivenessLogger) MinLevel() Level {
	return LevelInfo
}

// OnEvent appends persona review events to JSONL file.
func (l *PersonaEffectivenessLogger) OnEvent(event *Event) error {
	// Filter: only process persona_review_completed events
	if event.Type != EventPersonaReviewCompleted {
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
		return fmt.Errorf("failed to marshal persona event: %w", err)
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
