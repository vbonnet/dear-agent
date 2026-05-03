// Package usage provides usage functionality.
package usage

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// Event represents a single CLI command execution event
type Event struct {
	Timestamp time.Time         `json:"timestamp"`
	Command   string            `json:"command"`
	Args      []string          `json:"args,omitempty"`
	Flags     map[string]string `json:"flags,omitempty"`
	Duration  int64             `json:"duration_ms,omitempty"` // milliseconds
	Success   bool              `json:"success"`
	Error     string            `json:"error,omitempty"`
}

// Tracker tracks CLI usage events
type Tracker struct {
	filePath string
	mu       sync.Mutex
}

// New creates a new usage tracker
// If filePath is empty, uses ~/.engram/usage.jsonl
func New(filePath string) (*Tracker, error) {
	if filePath == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, fmt.Errorf("failed to get home directory: %w", err)
		}

		engramDir := filepath.Join(home, ".engram")
		if err := os.MkdirAll(engramDir, 0o700); err != nil {
			return nil, fmt.Errorf("failed to create .engram directory: %w", err)
		}

		filePath = filepath.Join(engramDir, "usage.jsonl")
	}

	return &Tracker{
		filePath: filePath,
	}, nil
}

// Track records a usage event (async, non-blocking)
func (t *Tracker) Track(event Event) {
	// Run in goroutine to avoid blocking CLI execution
	go t.trackSync(event)
}

// trackSync records a usage event synchronously
func (t *Tracker) trackSync(event Event) error {
	t.mu.Lock()
	defer t.mu.Unlock()

	// Ensure timestamp is set
	if event.Timestamp.IsZero() {
		event.Timestamp = time.Now()
	}

	// Open file in append mode
	f, err := os.OpenFile(t.filePath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		// Silently fail - we don't want to break CLI if tracking fails
		return fmt.Errorf("failed to open usage file: %w", err)
	}
	defer f.Close()

	// Encode as JSON line
	encoder := json.NewEncoder(f)
	if err := encoder.Encode(event); err != nil {
		return fmt.Errorf("failed to encode event: %w", err)
	}

	return nil
}

// TrackSync records a usage event synchronously (blocking)
// Use this when you need to ensure the event is written before proceeding
func (t *Tracker) TrackSync(event Event) error {
	return t.trackSync(event)
}

// FilePath returns the path to the usage log file
func (t *Tracker) FilePath() string {
	return t.filePath
}
