package health

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// Logger handles JSONL logging for health checks
type Logger struct {
	logPath string
}

// NewLogger creates a new logger instance
func NewLogger() *Logger {
	logPath := filepath.Join(os.Getenv("HOME"), ".engram", "logs", "doctor.log.jsonl")
	return &Logger{
		logPath: logPath,
	}
}

// AppendToLog appends a health check log entry to the JSONL file
func (l *Logger) AppendToLog(entry *HealthCheckLog) error {
	// Marshal to JSON (single line, no indentation)
	data, err := json.Marshal(entry)
	if err != nil {
		return fmt.Errorf("marshal log entry: %w", err)
	}

	// Append newline
	data = append(data, '\n')

	// Create logs directory if it doesn't exist
	logsDir := filepath.Dir(l.logPath)
	if err := os.MkdirAll(logsDir, 0755); err != nil {
		return fmt.Errorf("create logs directory: %w", err)
	}

	// Open file for append (create if not exists)
	// POSIX guarantees: O_APPEND + single Write() ≤ PIPE_BUF (4096 bytes) is atomic
	f, err := os.OpenFile(l.logPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600)
	if err != nil {
		return fmt.Errorf("open log file: %w", err)
	}
	defer f.Close()

	// Atomic write (single Write() call)
	if _, err := f.Write(data); err != nil {
		return fmt.Errorf("write log entry: %w", err)
	}

	return nil
}
