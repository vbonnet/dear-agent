// Package telemetry implements event collection and storage for observability
// and analytics.
//
// Telemetry provides insight into platform usage, plugin behavior, and system
// performance without compromising privacy. All telemetry is opt-in and stored
// locally by default.
//
// Collected event types:
//   - config.loaded: Configuration initialization
//   - plugin.loaded: Plugin discovery and loading
//   - plugin.executed: Plugin command execution
//   - engram.retrieved: Pattern retrieval operations
//   - eventbus.published: Inter-plugin event communication
//
// Events include:
//   - Timestamp: When the event occurred
//   - Event type: Category of event
//   - Agent: Which AI agent triggered the event
//   - Data: Event-specific metadata
//
// Example usage:
//
//	collector, err := telemetry.NewCollector(true, "~/.engram/telemetry.jsonl")
//	if err != nil {
//	    log.Fatal(err)
//	}
//	defer collector.Close()
//
//	err = collector.Record("plugin.executed", "claude-code", map[string]interface{}{
//	    "plugin": "multi-persona-review",
//	    "duration_ms": 1234,
//	})
//
// Privacy considerations:
//   - Opt-in by default (disabled unless explicitly enabled)
//   - Local storage only (no data sent to external services)
//   - No PII collected (paths, code content excluded)
//   - Retention controlled by user
package telemetry

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// Collector handles telemetry event collection and storage
type Collector struct {
	mu       sync.Mutex
	enabled  bool
	path     string
	file     *os.File
	registry *ListenerRegistry
}

// NewCollector creates a new telemetry collector
func NewCollector(enabled bool, path string) (*Collector, error) {
	c := &Collector{
		enabled:  enabled,
		path:     path,
		registry: NewListenerRegistry(),
	}

	if !enabled {
		return c, nil
	}

	// Create telemetry directory if needed
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0700); err != nil { // Owner-only permissions (S10 P0 security fix)
		return nil, fmt.Errorf("failed to create telemetry directory: %w", err)
	}

	// Open telemetry file in append mode
	file, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600) // Owner read/write only (S10 P0 security fix)
	if err != nil {
		return nil, fmt.Errorf("failed to open telemetry file: %w", err)
	}

	c.file = file
	return c, nil
}

// AddListener registers an event listener for async notification.
//
// Listeners are called asynchronously in goroutines when events are recorded.
// Level filtering is applied: only events with level >= listener.MinLevel()
// trigger OnEvent().
//
// Panics in listeners are recovered and logged.
// Errors from listeners are logged but don't block other listeners.
//
// Example:
//
//	listener := &MyListener{}
//	collector.AddListener(listener)
func (c *Collector) AddListener(listener EventListener) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.registry.Register(listener)
}

// Record records a telemetry event with severity level.
//
// Breaking change: Signature changed from Record(type, agent, data)
// to Record(type, agent, level, data).
//
// ERROR bypass: If enabled=false but level >= LevelError, the event
// is still recorded to file and notified to listeners.
//
// Example:
//
//	collector.Record("plugin.executed", "claude-code", LevelInfo, map[string]interface{}{
//	    "plugin": "multi-persona-review",
//	    "duration_ms": 1234,
//	})
func (c *Collector) Record(eventType, agent string, level Level, data map[string]interface{}) error {
	// ERROR bypass: skip INFO/WARN when disabled, always record ERROR/CRITICAL
	if !c.enabled && level < LevelError {
		return nil
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	event := Event{
		Timestamp:     time.Now(),
		Type:          eventType,
		Agent:         agent,
		Level:         level, // NEW: Severity level
		SchemaVersion: "1.0.0",
		Data:          data,
	}

	// Write event as JSONL (one JSON object per line)
	// Only write to file if file exists (enabled=true or ERROR bypass)
	if c.file != nil {
		line, err := json.Marshal(event)
		if err != nil {
			return fmt.Errorf("failed to marshal event: %w", err)
		}

		if _, err := c.file.Write(append(line, '\n')); err != nil {
			// Still notify listeners even if file write fails
			c.registry.Notify(&event)
			return fmt.Errorf("failed to write event: %w", err)
		}
	}

	// Notify listeners asynchronously (even if file is nil)
	c.registry.Notify(&event)

	return nil
}

// RecordSync records a critical telemetry event and ensures it's flushed to disk
//
// Use this for critical events that must be persisted immediately:
//   - Session end events
//   - Error/crash events
//   - Billing/compliance events
//
// For normal events, use Record() instead for better performance.
//
// Implementation note (P1-5):
// This addresses the "Add optional file sync for critical events" P1 issue.
// Ensures critical events survive process crashes or power failures.
//
// P0-3 Fix: Holds lock across both Record and Sync to prevent race condition
func (c *Collector) RecordSync(eventType string, agent string, data map[string]interface{}) error {
	if !c.enabled {
		return nil
	}

	// P0-3 Fix: Hold lock across both Record and Sync operations
	// This prevents another goroutine from writing between Record and Sync
	c.mu.Lock()
	defer c.mu.Unlock()

	// Create event
	event := Event{
		Timestamp:     time.Now(),
		Type:          eventType,
		Agent:         agent,
		SchemaVersion: "1.0.0",
		Data:          data,
	}

	// Write event as JSONL (one JSON object per line)
	line, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("failed to marshal event: %w", err)
	}

	if _, err := c.file.Write(append(line, '\n')); err != nil {
		return fmt.Errorf("failed to write event: %w", err)
	}

	// Sync to disk while still holding lock
	if err := c.file.Sync(); err != nil {
		return fmt.Errorf("failed to sync event to disk: %w", err)
	}

	return nil
}

// Close closes the telemetry collector
func (c *Collector) Close() error {
	if c.file != nil {
		return c.file.Close()
	}
	return nil
}
