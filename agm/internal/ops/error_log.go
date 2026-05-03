package ops

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// ErrorCategory classifies the type of error recorded.
type ErrorCategory string

const (
	CategoryHookError         ErrorCategory = "hook-error"
	CategoryPermissionBlocked ErrorCategory = "permission-blocked"
	CategoryTestFailure       ErrorCategory = "test-failure"
	CategoryBuildFailure      ErrorCategory = "build-failure"
	CategoryMergeConflict     ErrorCategory = "merge-conflict"
)

// ValidCategories returns all recognized error categories.
func ValidCategories() []ErrorCategory {
	return []ErrorCategory{
		CategoryHookError,
		CategoryPermissionBlocked,
		CategoryTestFailure,
		CategoryBuildFailure,
		CategoryMergeConflict,
	}
}

// IsValidCategory reports whether cat is a recognized error category.
func IsValidCategory(cat string) bool {
	for _, valid := range ValidCategories() {
		if string(valid) == cat {
			return true
		}
	}
	return false
}

// ErrorEntry represents a single error recorded to the persistent log.
type ErrorEntry struct {
	Timestamp   time.Time     `json:"timestamp"`
	Category    ErrorCategory `json:"category"`
	Message     string        `json:"message"`
	Source      string        `json:"source"`
	SessionName string        `json:"session_name,omitempty"`
}

// DefaultErrorLogPath returns the default path for the error log file.
func DefaultErrorLogPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".agm", "hook-errors.jsonl")
}

// ErrorLog provides append/read access to the persistent JSONL error log.
type ErrorLog struct {
	path string
}

// NewErrorLog creates an ErrorLog backed by the given file path.
func NewErrorLog(path string) *ErrorLog {
	return &ErrorLog{path: path}
}

// AppendError writes a new error entry to the log.
func (el *ErrorLog) AppendError(category, message, source string) error {
	entry := ErrorEntry{
		Timestamp:   time.Now(),
		Category:    ErrorCategory(category),
		Message:     message,
		Source:      source,
		SessionName: os.Getenv("AGM_SESSION_NAME"),
	}
	return el.appendEntry(entry)
}

// appendEntry marshals and appends a single entry to the JSONL file.
func (el *ErrorLog) appendEntry(entry ErrorEntry) error {
	if err := os.MkdirAll(filepath.Dir(el.path), 0o700); err != nil {
		return fmt.Errorf("create error log directory: %w", err)
	}

	f, err := os.OpenFile(el.path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return fmt.Errorf("open error log: %w", err)
	}
	defer f.Close()

	data, err := json.Marshal(entry)
	if err != nil {
		return fmt.Errorf("marshal error entry: %w", err)
	}

	if _, err := f.Write(append(data, '\n')); err != nil {
		return fmt.Errorf("write error entry: %w", err)
	}

	return nil
}

// ReadErrors returns all error entries recorded since the given time.
// If since is zero, all entries are returned.
func (el *ErrorLog) ReadErrors(since time.Time) ([]ErrorEntry, error) {
	f, err := os.Open(el.path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("open error log: %w", err)
	}
	defer f.Close()

	var entries []ErrorEntry
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}

		var entry ErrorEntry
		if err := json.Unmarshal(line, &entry); err != nil {
			continue // skip malformed lines
		}

		if !since.IsZero() && entry.Timestamp.Before(since) {
			continue
		}

		entries = append(entries, entry)
	}

	if err := scanner.Err(); err != nil {
		return entries, fmt.Errorf("read error log: %w", err)
	}

	return entries, nil
}

// ReadErrorsByCategory returns entries matching the given category since the given time.
func (el *ErrorLog) ReadErrorsByCategory(category string, since time.Time) ([]ErrorEntry, error) {
	all, err := el.ReadErrors(since)
	if err != nil {
		return nil, err
	}

	var filtered []ErrorEntry
	for _, e := range all {
		if string(e.Category) == category {
			filtered = append(filtered, e)
		}
	}

	return filtered, nil
}
