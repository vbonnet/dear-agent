// Package history provides history-related functionality.
package history

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// ErrAmbiguousHistory means both the legacy and canonical append-only logs
// exist. Lifecycle transitions must stop before mutating status until an
// operator reconciles the two histories.
var ErrAmbiguousHistory = errors.New("ambiguous history state")

// History manages append-only event logging to WAYFINDER-HISTORY.jsonl
type History struct {
	path string
}

// New creates a new History for the given directory
func New(dir string) *History {
	return &History{
		path: filepath.Join(dir, HistoryFilename),
	}
}

// EnsureCurrentFile atomically migrates the legacy JSON Lines filename before
// the history is read, appended, or archived.
func (h *History) EnsureCurrentFile() error {
	legacyPath := filepath.Join(filepath.Dir(h.path), LegacyHistoryFilename)
	if _, err := os.Stat(h.path); err == nil {
		if _, legacyErr := os.Stat(legacyPath); legacyErr == nil {
			return fmt.Errorf("%w: both %s and %s exist; reconcile them before continuing", ErrAmbiguousHistory, h.path, legacyPath)
		} else if !os.IsNotExist(legacyErr) {
			return fmt.Errorf("stat legacy history file: %w", legacyErr)
		}
		return nil
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("stat history file: %w", err)
	}

	if _, err := os.Stat(legacyPath); err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("stat legacy history file: %w", err)
	}
	if err := os.Rename(legacyPath, h.path); err != nil {
		if _, statErr := os.Stat(h.path); statErr == nil {
			return nil
		}
		return fmt.Errorf("migrate legacy history file: %w", err)
	}
	return nil
}

// AppendEvent appends a new event to the history log
// Uses O_APPEND flag for concurrent-safe writes
func (h *History) AppendEvent(eventType, phase string, data map[string]interface{}) error {
	if err := h.EnsureCurrentFile(); err != nil {
		return err
	}
	event := Event{
		Timestamp: time.Now(),
		Type:      eventType,
		Phase:     phase,
		Data:      data,
	}

	// Marshal event to JSON
	jsonData, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("failed to marshal event: %w", err)
	}

	// Open file with O_APPEND for concurrent-safe writes
	file, err := os.OpenFile(h.path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return fmt.Errorf("failed to open history file: %w", err)
	}
	defer func() { _ = file.Close() }()

	// Write event as single line (JSON + newline)
	if _, err := file.Write(append(jsonData, '\n')); err != nil {
		return fmt.Errorf("failed to write event: %w", err)
	}

	return nil
}

// Read reads all events from the history log
func (h *History) Read() ([]Event, error) {
	if err := h.EnsureCurrentFile(); err != nil {
		return nil, err
	}
	// Check if file exists
	if _, err := os.Stat(h.path); os.IsNotExist(err) {
		return []Event{}, nil // Empty history
	}

	file, err := os.Open(h.path)
	if err != nil {
		return nil, fmt.Errorf("failed to open history file: %w", err)
	}
	defer func() { _ = file.Close() }()

	var events []Event
	scanner := bufio.NewScanner(file)
	lineNum := 0

	for scanner.Scan() {
		lineNum++
		line := strings.TrimSpace(scanner.Text())

		// Skip empty lines
		if line == "" {
			continue
		}

		var event Event
		if err := json.Unmarshal([]byte(line), &event); err != nil {
			// Log warning but continue reading other events
			fmt.Fprintf(os.Stderr, "Warning: failed to parse event at line %d: %v\n", lineNum, err)
			continue
		}

		events = append(events, event)
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("error reading history file: %w", err)
	}

	return events, nil
}

// GetEventsByPhase returns all events for a specific phase
func (h *History) GetEventsByPhase(phaseName string) ([]Event, error) {
	allEvents, err := h.Read()
	if err != nil {
		return nil, err
	}

	var phaseEvents []Event
	for _, event := range allEvents {
		if event.Phase == phaseName {
			phaseEvents = append(phaseEvents, event)
		}
	}

	return phaseEvents, nil
}

// GetEventsByType returns all events of a specific type
func (h *History) GetEventsByType(eventType string) ([]Event, error) {
	allEvents, err := h.Read()
	if err != nil {
		return nil, err
	}

	var typeEvents []Event
	for _, event := range allEvents {
		if event.Type == eventType {
			typeEvents = append(typeEvents, event)
		}
	}

	return typeEvents, nil
}
