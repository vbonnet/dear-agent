package telemetry

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// EcphoryAuditLogger logs ecphory correctness audit results to JSONL.
//
// Event format (Data field):
//
//	{
//	  "session_id": "abc123",
//	  "timestamp": "2024-01-01T12:00:00Z",
//	  "total_retrievals": 10,
//	  "appropriate_count": 9,
//	  "inappropriate_count": 1,
//	  "correctness_score": 0.90,
//	  "audit_duration_ms": 450,
//	  "context": {
//	    "language": "python",
//	    "framework": "django",
//	    "task_type": "debugging"
//	  }
//	}
//
// Output: ~/.engram/logs/ecphory-audit.jsonl
type EcphoryAuditLogger struct {
	mu       sync.Mutex
	logDir   string
	filename string
}

// NewEcphoryAuditLogger creates a new JSONL logger for ecphory audits.
func NewEcphoryAuditLogger(logDir string) *EcphoryAuditLogger {
	return &EcphoryAuditLogger{
		logDir:   logDir,
		filename: "ecphory-audit.jsonl",
	}
}

// MinLevel returns INFO (log all audit events).
func (l *EcphoryAuditLogger) MinLevel() Level {
	return LevelInfo
}

// OnEvent appends ecphory audit events to JSONL file.
func (l *EcphoryAuditLogger) OnEvent(event *Event) error {
	// Filter: only process ecphory_audit_completed events
	if event.Type != EventEcphoryAuditCompleted {
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
		return fmt.Errorf("failed to marshal audit event: %w", err)
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
