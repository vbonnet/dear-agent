// Package debug provides debug functionality.
package debug

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"time"
)

var (
	globalLogger *Logger
	mu           sync.Mutex
)

// Logger provides debug logging to a file
type Logger struct {
	file      *os.File
	enabled   bool
	startTime time.Time
}

// Init initializes the global debug logger
func Init(enabled bool, sessionName string) error {
	mu.Lock()
	defer mu.Unlock()

	if !enabled {
		globalLogger = &Logger{enabled: false}
		return nil
	}

	// Create debug directory
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("failed to get home dir: %w", err)
	}

	debugDir := filepath.Join(homeDir, ".agm", "debug")
	if err := os.MkdirAll(debugDir, 0755); err != nil {
		return fmt.Errorf("failed to create debug dir: %w", err)
	}

	// Create log file with timestamp
	timestamp := time.Now().Format("20060102-150405")
	logPath := filepath.Join(debugDir, fmt.Sprintf("new-%s-%s.log", sessionName, timestamp))

	file, err := os.Create(logPath)
	if err != nil {
		return fmt.Errorf("failed to create debug log: %w", err)
	}

	slog.Info("Debug log initialized", "path", logPath)

	globalLogger = &Logger{
		file:      file,
		enabled:   true,
		startTime: time.Now(),
	}

	// Write initial log entries directly (can't call Log() while holding mutex)
	now := time.Now().Format("15:04:05.000")
	file.WriteString(fmt.Sprintf("[%s] +%7dms | Debug logging initialized\n", now, 0))
	file.WriteString(fmt.Sprintf("[%s] +%7dms | Log file: %s\n", now, 0, logPath))
	file.Sync()

	return nil
}

// Close closes the debug logger
func Close() {
	mu.Lock()
	defer mu.Unlock()

	if globalLogger != nil && globalLogger.file != nil {
		// Write final log entry directly (can't call Log() while holding mutex)
		elapsed := time.Since(globalLogger.startTime)
		timestamp := time.Now().Format("15:04:05.000")
		globalLogger.file.WriteString(fmt.Sprintf("[%s] +%7dms | Debug session ended (total: %v)\n",
			timestamp, elapsed.Milliseconds(), elapsed))
		globalLogger.file.Close()
	}
}

// Log writes a timestamped debug message
func Log(format string, args ...interface{}) {
	mu.Lock()
	defer mu.Unlock()

	if globalLogger == nil || !globalLogger.enabled {
		return
	}

	elapsed := time.Since(globalLogger.startTime)
	timestamp := time.Now().Format("15:04:05.000")
	msg := fmt.Sprintf(format, args...)

	line := fmt.Sprintf("[%s] +%7dms | %s\n", timestamp, elapsed.Milliseconds(), msg)
	globalLogger.file.WriteString(line)
	globalLogger.file.Sync() // Ensure it's written immediately
}

// Phase logs a phase transition with a separator
func Phase(name string) {
	Log("=" + "=" + "=" + "=" + "=" + "=" + "=" + "=" + "=" + "=")
	Log("PHASE: %s", name)
	Log("=" + "=" + "=" + "=" + "=" + "=" + "=" + "=" + "=" + "=")
}
