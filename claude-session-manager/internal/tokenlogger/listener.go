package tokenlogger

import (
	"os"
	"path/filepath"
	"sync"

	"github.com/vbonnet/engram/core/pkg/telemetry"
)

// TokenLogger implements telemetry.EventListener to log events to CSM session directories.
type TokenLogger struct {
	mu           sync.Mutex
	sessionDir   string
	cacheChecked bool
	minLevel     telemetry.Level
}

// NewTokenLogger creates a new TokenLogger instance with default settings.
func NewTokenLogger() *TokenLogger {
	return &TokenLogger{
		minLevel: telemetry.LevelError,
	}
}

// MinLevel returns the minimum telemetry level this listener handles.
func (l *TokenLogger) MinLevel() telemetry.Level {
	return l.minLevel
}

// OnEvent handles incoming telemetry events by writing them to the session log file.
func (l *TokenLogger) OnEvent(event *telemetry.Event) error {
	l.mu.Lock()
	defer l.mu.Unlock()

	// Get or detect session directory (cached)
	sessionDir := l.getSessionDirLocked()
	if sessionDir == "" {
		// No active session - gracefully drop event
		return nil
	}

	// Check if session directory exists
	if _, err := os.Stat(sessionDir); os.IsNotExist(err) {
		// Session directory missing - gracefully drop event
		// CSM creates directories, plugin should not
		return nil
	}

	// Write event to session log file
	logPath := filepath.Join(sessionDir, "token-usage.jsonl")
	return writeEvent(logPath, event)
}
