package telemetry

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// jsonlEventLogger appends one event type to one JSONL file under a log
// directory. It is the shared body behind the per-subject loggers in this
// package, which previously existed as byte-identical copies differing only in
// their name, their filename, and the event type they filter on.
//
// Writes are serialized so concurrent OnEvent callers cannot interleave a
// partial line into the file.
type jsonlEventLogger struct {
	logDir    string
	filename  string
	eventType string

	// subject names what the logger records, and appears in marshal errors so
	// a failure points at the right stream.
	subject string

	mu sync.Mutex
}

// MinLevel returns INFO: every matching event is recorded.
func (l *jsonlEventLogger) MinLevel() Level {
	return LevelInfo
}

// OnEvent appends a matching event to the logger's JSONL file. A
// non-matching event is ignored, which is how several loggers share one bus.
func (l *jsonlEventLogger) OnEvent(event *Event) error {
	if event.Type != l.eventType {
		return nil
	}

	if err := os.MkdirAll(l.logDir, 0o700); err != nil {
		return fmt.Errorf("failed to create log directory: %w", err)
	}

	entry := map[string]any{
		"timestamp":  event.Timestamp.Format(time.RFC3339),
		"event_type": event.Type,
		"data":       event.Data,
	}
	jsonBytes, err := json.Marshal(entry)
	if err != nil {
		return fmt.Errorf("failed to marshal %s event: %w", l.subject, err)
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	logPath := filepath.Join(l.logDir, l.filename)
	f, err := os.OpenFile(logPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return fmt.Errorf("failed to open log file: %w", err)
	}
	defer f.Close()

	if _, err := f.Write(append(jsonBytes, '\n')); err != nil {
		return fmt.Errorf("failed to write to log file: %w", err)
	}

	return nil
}
