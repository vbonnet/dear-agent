package telemetry

import (
	"encoding/json"
	"fmt"
	"os"
	"sync"
)

// Logger manages append-only JSON Lines logging to EXECUTION-LOG.jsonl
type Logger struct {
	filePath string
	mu       sync.Mutex
}

// NewLogger creates a new execution event logger
func NewLogger(filePath string) *Logger {
	return &Logger{
		filePath: filePath,
	}
}

// LogEvent appends an ExecutionEvent to the log file as a JSON line
func (l *Logger) LogEvent(event *ExecutionEvent) error {
	l.mu.Lock()
	defer l.mu.Unlock()

	// Marshal event to JSON
	data, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("failed to marshal event: %w", err)
	}

	// Append newline for JSON Lines format
	data = append(data, '\n')

	// Open file in append mode (create if not exists)
	file, err := os.OpenFile(l.filePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("failed to open log file: %w", err)
	}
	defer func() {
		_ = file.Close() // Explicitly ignore close error (write error already handled)
	}()

	// Write JSON line
	if _, err := file.Write(data); err != nil {
		return fmt.Errorf("failed to write to log file: %w", err)
	}

	return nil
}
