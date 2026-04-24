package tracker

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/vbonnet/dear-agent/pkg/eventbus"
	"github.com/vbonnet/dear-agent/pkg/telemetry"
	"github.com/vbonnet/dear-agent/wayfinder/internal/analytics"
)

// Tracker wraps SessionTracker with EventBus initialization
type Tracker struct {
	sessionTracker *analytics.SessionTracker
	bus            *eventbus.LocalBus
	telemetry      *simpleTelemetry
	sessionID      string
}

// simpleTelemetry implements eventbus.TelemetryRecorder interface
type simpleTelemetry struct {
	path string
	file *os.File
}

func (st *simpleTelemetry) Record(topic, agent string, level telemetry.Level, data map[string]interface{}) error {
	event := map[string]interface{}{
		"timestamp": time.Now().Format(time.RFC3339),
		"type":      topic,
		"agent":     agent,
		"level":     level.String(),
		"data":      data,
	}

	jsonData, err := json.Marshal(event)
	if err != nil {
		return err
	}

	_, err = st.file.Write(append(jsonData, '\n'))
	return err
}

func (st *simpleTelemetry) Close() error {
	if st.file != nil {
		return st.file.Close()
	}
	return nil
}

// New creates a new Tracker with initialized EventBus
func New(sessionID string) (*Tracker, error) {
	// Get telemetry path
	telemetryPath := os.Getenv("ENGRAM_TELEMETRY_PATH")
	if telemetryPath == "" {
		telemetryPath = os.ExpandEnv("$HOME/.claude/telemetry.jsonl")
	}

	// Open telemetry file for appending
	file, err := os.OpenFile(telemetryPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600)
	if err != nil {
		return nil, fmt.Errorf("failed to open telemetry file: %w", err)
	}

	tel := &simpleTelemetry{
		path: telemetryPath,
		file: file,
	}

	// Create EventBus
	bus := eventbus.NewBus(tel)

	// Create SessionTracker
	tracker := analytics.NewSessionTracker(bus)

	return &Tracker{
		sessionTracker: tracker,
		bus:            bus,
		telemetry:      tel,
		sessionID:      sessionID,
	}, nil
}

// StartSession publishes session.started event
func (t *Tracker) StartSession(projectPath string) error {
	return t.sessionTracker.StartSession(projectPath)
}

// StartPhase publishes phase.started event
func (t *Tracker) StartPhase(phase string) error {
	return t.sessionTracker.StartPhase(phase)
}

// CompletePhase publishes phase.completed event
func (t *Tracker) CompletePhase(phase, outcome string, metadata map[string]interface{}) error {
	return t.sessionTracker.CompletePhase(phase, outcome, metadata)
}

// EndSession publishes session.completed event
func (t *Tracker) EndSession(outcome string) error {
	return t.sessionTracker.EndSession(outcome)
}

// SessionID returns the session ID
func (t *Tracker) SessionID() string {
	return t.sessionTracker.SessionID()
}

// Close closes the telemetry file
func (t *Tracker) Close(ctx context.Context) error {
	if t.telemetry != nil {
		return t.telemetry.Close()
	}
	return nil
}
