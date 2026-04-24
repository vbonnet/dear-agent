package astrocyte

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/vbonnet/dear-agent/agm/internal/eventbus"
	"github.com/vbonnet/dear-agent/agm/internal/logging"
)

const (
	// Default escalation window (15 minutes)
	defaultEscalationWindow = 15 * time.Minute

	// Default polling interval for checking the incidents file
	defaultPollInterval = 5 * time.Second

	// Size of the event buffer channel
	eventBufferSize = 100
)

// AstrocyteIncident represents an incident record from Astrocyte's JSONL log
type AstrocyteIncident struct {
	Timestamp               string   `json:"timestamp"`
	SessionName             string   `json:"session_name"`
	SessionID               string   `json:"session_id"`
	Symptom                 string   `json:"symptom"`
	DurationMinutes         int      `json:"duration_minutes"`
	DetectionHeuristic      string   `json:"detection_heuristic"`
	PaneSnapshot            string   `json:"pane_snapshot"`
	CursorPosition          string   `json:"cursor_position"`
	RecoveryAttempted       bool     `json:"recovery_attempted"`
	RecoveryMethod          *string  `json:"recovery_method"`
	RecoverySuccess         *bool    `json:"recovery_success"`
	RecoveryDurationSeconds *float64 `json:"recovery_duration_seconds"`
	DiagnosisFiled          bool     `json:"diagnosis_filed"`
	DiagnosisFile           *string  `json:"diagnosis_file"`
	CascadeDepth            int      `json:"cascade_depth"`
	CircuitBreakerTriggered bool     `json:"circuit_breaker_triggered"`
}

// EscalationTracker tracks the last escalation time per session to implement time-windowing
type EscalationTracker struct {
	lastEscalations sync.Map // sessionID -> time.Time
	window          time.Duration
}

// NewEscalationTracker creates a new EscalationTracker with the specified time window
func NewEscalationTracker(window time.Duration) *EscalationTracker {
	if window <= 0 {
		window = defaultEscalationWindow
	}
	return &EscalationTracker{
		window: window,
	}
}

// ShouldPublish checks if an escalation event should be published for the given session.
// Returns true if the window has elapsed since the last escalation, or if this is the first escalation.
func (t *EscalationTracker) ShouldPublish(sessionID string) bool {
	now := time.Now()

	if val, ok := t.lastEscalations.Load(sessionID); ok {
		lastTime := val.(time.Time)
		elapsed := now.Sub(lastTime)
		return elapsed >= t.window
	}

	// First escalation for this session
	return true
}

// RecordEscalation records that an escalation event was published for the given session
func (t *EscalationTracker) RecordEscalation(sessionID string) {
	t.lastEscalations.Store(sessionID, time.Now())
}

// Watcher monitors the Astrocyte incidents file and publishes events to the eventbus
type Watcher struct {
	hub           eventbus.Broadcaster
	tracker       *EscalationTracker
	incidentsFile string
	pollInterval  time.Duration
	lastPosition  int64
	shutdown      chan struct{}
	wg            sync.WaitGroup
	mu            sync.Mutex
	logger        *slog.Logger
}

// NewWatcher creates a new Astrocyte incidents watcher
func NewWatcher(hub eventbus.Broadcaster, incidentsFile string, escalationWindow time.Duration) *Watcher {
	return NewWatcherWithPollInterval(hub, incidentsFile, escalationWindow, defaultPollInterval)
}

// NewWatcherWithPollInterval creates a new Astrocyte incidents watcher with a custom poll interval
func NewWatcherWithPollInterval(hub eventbus.Broadcaster, incidentsFile string, escalationWindow time.Duration, pollInterval time.Duration) *Watcher {
	logger := logging.DefaultLogger()

	if incidentsFile == "" {
		// Default to ~/.agm/astrocyte/incidents.jsonl
		home, err := os.UserHomeDir()
		if err != nil {
			logger.Warn("Failed to get home directory", "error", err)
			incidentsFile = "/tmp/agm-astrocyte-incidents.jsonl"
		} else {
			incidentsFile = filepath.Join(home, ".agm", "logs", "astrocyte", "incidents.jsonl")
		}
	}

	if pollInterval <= 0 {
		pollInterval = defaultPollInterval
	}

	return &Watcher{
		hub:           hub,
		tracker:       NewEscalationTracker(escalationWindow),
		incidentsFile: incidentsFile,
		pollInterval:  pollInterval,
		lastPosition:  0,
		shutdown:      make(chan struct{}),
		logger:        logger,
	}
}

// Start begins watching the incidents file
func (w *Watcher) Start() error {
	// Ensure the incidents file exists and get its initial size
	if err := w.initializeFilePosition(); err != nil {
		return fmt.Errorf("failed to initialize file position: %w", err)
	}

	w.logger.Info("Astrocyte watcher started", "file", w.incidentsFile, "window", w.tracker.window)

	w.wg.Add(1)
	go w.watchLoop()

	return nil
}

// Stop gracefully stops the watcher
func (w *Watcher) Stop() {
	close(w.shutdown)
	w.wg.Wait()
	w.logger.Info("Astrocyte watcher stopped")
}

// initializeFilePosition sets the initial file position to the end of the file
// so we only process new incidents, not historical ones
func (w *Watcher) initializeFilePosition() error {
	w.mu.Lock()
	defer w.mu.Unlock()

	file, err := os.Open(w.incidentsFile)
	if err != nil {
		if os.IsNotExist(err) {
			// File doesn't exist yet, will be created by Astrocyte
			w.lastPosition = 0
			w.logger.Info("Incidents file does not exist yet, will watch for creation", "file", w.incidentsFile)
			return nil
		}
		return fmt.Errorf("failed to open incidents file: %w", err)
	}
	defer file.Close()

	// Seek to the end of the file
	pos, err := file.Seek(0, io.SeekEnd)
	if err != nil {
		return fmt.Errorf("failed to seek to end of file: %w", err)
	}

	w.lastPosition = pos
	w.logger.Info("Initialized file position", "bytes", pos)

	return nil
}

// watchLoop continuously monitors the incidents file for new entries
func (w *Watcher) watchLoop() {
	defer w.wg.Done()

	ticker := time.NewTicker(w.pollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-w.shutdown:
			return
		case <-ticker.C:
			if err := w.checkForNewIncidents(); err != nil {
				w.logger.Warn("Error checking for new incidents", "error", err)
			}
		}
	}
}

// checkForNewIncidents reads new lines from the incidents file and processes them
func (w *Watcher) checkForNewIncidents() error {
	w.mu.Lock()
	defer w.mu.Unlock()

	file, err := os.Open(w.incidentsFile)
	if err != nil {
		if os.IsNotExist(err) {
			// File doesn't exist yet, nothing to do
			return nil
		}
		return fmt.Errorf("failed to open incidents file: %w", err)
	}
	defer file.Close()

	// Get current file size
	stat, err := file.Stat()
	if err != nil {
		return fmt.Errorf("failed to stat incidents file: %w", err)
	}

	currentSize := stat.Size()

	// If file was truncated, reset to beginning
	if currentSize < w.lastPosition {
		w.logger.Warn("Incidents file truncated, resetting position")
		w.lastPosition = 0
	}

	// If no new data, nothing to do
	if currentSize == w.lastPosition {
		return nil
	}

	// Seek to last position
	if _, err := file.Seek(w.lastPosition, io.SeekStart); err != nil {
		return fmt.Errorf("failed to seek to last position: %w", err)
	}

	// Read new lines
	scanner := bufio.NewScanner(file)
	lineCount := 0
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			continue
		}

		var incident AstrocyteIncident
		if err := json.Unmarshal([]byte(line), &incident); err != nil {
			w.logger.Warn("Failed to parse incident", "error", err)
			continue
		}

		// Process the incident
		if err := w.processIncident(&incident); err != nil {
			w.logger.Warn("Failed to process incident", "error", err)
			continue
		}

		lineCount++
	}

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("error reading incidents file: %w", err)
	}

	// Update last position
	w.lastPosition = currentSize

	if lineCount > 0 {
		w.logger.Info("Processed new incidents", "count", lineCount)
	}

	return nil
}

// processIncident converts an Astrocyte incident to an eventbus event and publishes it
func (w *Watcher) processIncident(incident *AstrocyteIncident) error {
	// Skip if no session ID
	if incident.SessionID == "" {
		w.logger.Warn("Skipping incident with empty session_id")
		return nil
	}

	// Check if we should publish based on time-windowing
	if !w.tracker.ShouldPublish(incident.SessionID) {
		w.logger.Info("Skipping duplicate escalation", "session_id", incident.SessionID, "window", w.tracker.window)
		return nil
	}

	// Map Astrocyte incident to eventbus event
	payload := eventbus.SessionEscalatedPayload{
		EscalationType: mapSymptomToEscalationType(incident.Symptom),
		Pattern:        incident.DetectionHeuristic,
		Line:           truncateString(incident.PaneSnapshot, 200),
		LineNumber:     0, // Not available from Astrocyte
		DetectedAt:     parseTimestamp(incident.Timestamp),
		Description:    formatDescription(incident),
		Severity:       mapSymptomToSeverity(incident.Symptom),
	}

	// Create event
	event, err := eventbus.NewEvent(
		eventbus.EventSessionEscalated,
		incident.SessionID,
		payload,
	)
	if err != nil {
		return fmt.Errorf("failed to create event: %w", err)
	}

	// Validate event
	if err := event.Validate(); err != nil {
		return fmt.Errorf("event validation failed: %w", err)
	}

	// Broadcast event
	w.hub.Broadcast(event)

	// Record escalation
	w.tracker.RecordEscalation(incident.SessionID)

	w.logger.Info("Published escalation event", "session_id", incident.SessionID, "type", payload.EscalationType, "symptom", incident.Symptom)

	return nil
}

// mapSymptomToEscalationType maps Astrocyte symptom types to eventbus escalation types
func mapSymptomToEscalationType(symptom string) string {
	switch symptom {
	case "permission_prompt":
		return "prompt"
	case "ask_question_violation":
		return "warning"
	case "bash_violation":
		return "error"
	case "stuck_mustering", "stuck_waiting":
		return "error"
	case "cursor_frozen":
		return "error"
	default:
		return "warning"
	}
}

// mapSymptomToSeverity maps Astrocyte symptom types to severity levels
func mapSymptomToSeverity(symptom string) string {
	switch symptom {
	case "stuck_mustering", "stuck_waiting", "cursor_frozen":
		return "high"
	case "permission_prompt":
		return "medium"
	case "ask_question_violation", "bash_violation":
		return "low"
	default:
		return "medium"
	}
}

// formatDescription creates a human-readable description from the incident
func formatDescription(incident *AstrocyteIncident) string {
	desc := fmt.Sprintf("Session stuck: %s detected", incident.Symptom)

	if incident.DurationMinutes > 0 {
		desc += fmt.Sprintf(" (duration: %d min)", incident.DurationMinutes)
	}

	if incident.RecoveryAttempted && incident.RecoveryMethod != nil {
		desc += fmt.Sprintf(", recovery: %s", *incident.RecoveryMethod)
		if incident.RecoverySuccess != nil {
			if *incident.RecoverySuccess {
				desc += " (success)"
			} else {
				desc += " (failed)"
			}
		}
	}

	return desc
}

// parseTimestamp parses the ISO 8601 timestamp from Astrocyte
func parseTimestamp(timestamp string) time.Time {
	t, err := time.Parse(time.RFC3339, timestamp)
	if err != nil {
		// Try without timezone
		t, err = time.Parse("2006-01-02T15:04:05", timestamp)
		if err != nil {
			return time.Now()
		}
	}
	return t
}

// truncateString truncates a string to the specified length
func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
