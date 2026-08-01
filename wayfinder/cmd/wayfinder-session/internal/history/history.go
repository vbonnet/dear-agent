// Package history provides history-related functionality.
package history

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"
)

// ErrAmbiguousHistory means both the legacy and canonical append-only logs
// exist. Lifecycle transitions must stop before mutating status until an
// operator reconciles the two histories.
var ErrAmbiguousHistory = errors.New("ambiguous history state")

// History manages append-only event logging to WAYFINDER-HISTORY.jsonl
type History struct {
	path        string
	userHomeDir func() (string, error)
}

// New creates a new History for the given directory
func New(dir string) *History {
	return &History{
		path:        filepath.Join(dir, HistoryFilename),
		userHomeDir: os.UserHomeDir,
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
	sanitizedData, err := h.sanitizeEventData(data)
	if err != nil {
		return fmt.Errorf("failed to sanitize event data: %w", err)
	}
	event := Event{
		Timestamp: time.Now(),
		Type:      eventType,
		Phase:     phase,
		Data:      sanitizedData,
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

func (h *History) sanitizeEventData(data map[string]any) (map[string]any, error) {
	if data == nil {
		return nil, nil
	}

	projectDir, err := filepath.Abs(filepath.Dir(h.path))
	if err != nil {
		return nil, fmt.Errorf("resolve project directory: %w", err)
	}
	homeDir, err := h.userHomeDir()
	if err != nil {
		return nil, fmt.Errorf("resolve home directory: %w", err)
	}

	encoded, err := json.Marshal(data)
	if err != nil {
		return nil, err
	}
	decoder := json.NewDecoder(bytes.NewReader(encoded))
	decoder.UseNumber()
	var cloned map[string]any
	if err := decoder.Decode(&cloned); err != nil {
		return nil, err
	}

	return sanitizeMapValues(cloned, projectDir, homeDir), nil
}

func sanitizeMapValues(data map[string]any, projectDir, homeDir string) map[string]any {
	result := make(map[string]any, len(data))
	for key, value := range data {
		result[key] = sanitizeValue(value, projectDir, homeDir)
	}
	return result
}

func sanitizeValue(value any, projectDir, homeDir string) any {
	switch typed := value.(type) {
	case string:
		typed = replacePathRoot(typed, projectDir, "$PROJECT_DIR")
		return replacePathRoot(typed, homeDir, "$HOME")
	case map[string]any:
		return sanitizeMapValues(typed, projectDir, homeDir)
	case []any:
		result := make([]any, len(typed))
		for i, item := range typed {
			result[i] = sanitizeValue(item, projectDir, homeDir)
		}
		return result
	default:
		return value
	}
}

func replacePathRoot(value, root, placeholder string) string {
	root = filepath.Clean(root)
	if root == "." || root == string(filepath.Separator) || root == "" {
		return value
	}

	var result strings.Builder
	lastWritten := 0
	searchFrom := 0
	replaced := false
	for searchFrom < len(value) {
		relativeIndex, matchedLength := indexPathRoot(value[searchFrom:], root)
		if relativeIndex == -1 {
			break
		}
		index := searchFrom + relativeIndex
		end := index + matchedLength
		if hasPathStartBoundary(value, index) && hasPathEndBoundary(value, end) {
			result.WriteString(value[lastWritten:index])
			result.WriteString(placeholder)
			lastWritten = end
			replaced = true
		}
		searchFrom = end
	}
	if !replaced {
		return value
	}
	result.WriteString(value[lastWritten:])
	return result.String()
}

func indexPathRoot(value, root string) (int, int) {
	if filepath.Separator != '\\' {
		return strings.Index(value, root), len(root)
	}
	for index := range value {
		if matchedLength, ok := matchWindowsPathRoot(value[index:], root); ok {
			return index, matchedLength
		}
	}
	return -1, 0
}

func matchWindowsPathRoot(value, root string) (int, bool) {
	consumed := 0
	for _, rootRune := range root {
		if value == "" {
			return 0, false
		}
		valueRune, valueRuneSize := utf8.DecodeRuneInString(value)
		if !windowsPathRunesEqual(valueRune, rootRune) {
			return 0, false
		}
		consumed += valueRuneSize
		value = value[valueRuneSize:]
	}
	return consumed, true
}

func windowsPathRunesEqual(left, right rune) bool {
	leftIsSeparator := left == '/' || left == '\\'
	rightIsSeparator := right == '/' || right == '\\'
	if leftIsSeparator || rightIsSeparator {
		return leftIsSeparator && rightIsSeparator
	}
	return strings.EqualFold(string(left), string(right))
}

func hasPathStartBoundary(value string, index int) bool {
	if index == 0 {
		return true
	}
	previous := value[index-1]
	if (previous >= 'a' && previous <= 'z') ||
		(previous >= 'A' && previous <= 'Z') ||
		(previous >= '0' && previous <= '9') {
		return false
	}
	return !strings.ContainsRune("_.-~", rune(previous))
}

func hasPathEndBoundary(value string, index int) bool {
	if index == len(value) {
		return true
	}
	if os.IsPathSeparator(value[index]) {
		return true
	}
	if value[index] == '.' && index+1 == len(value) {
		return true
	}

	// An exact root is commonly embedded in prose or key/value diagnostics. This
	// is a privacy-first boundary: common diagnostic punctuation is treated as a
	// delimiter even though POSIX permits it in a filename. Keep the list narrow;
	// backslash and non-terminal '.', '_', and '-' remain path continuations.
	switch value[index] {
	case ' ', '\t', '\r', '\n', ',', ':', ';', '!', '?', ')', ']', '}', '>', '"', '\'':
		return true
	default:
		return false
	}
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
