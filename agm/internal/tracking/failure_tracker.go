// Package tracking provides task failure tracking with auto-skip support.
package tracking

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// FailureRecord stores failure information for a single task.
type FailureRecord struct {
	Count    int       `json:"count"`
	LastFail time.Time `json:"last_fail"`
}

// FailureTracker tracks task failures and supports auto-skip after N failures.
type FailureTracker struct {
	mu       sync.Mutex
	path     string
	failures map[string]*FailureRecord
}

// NewFailureTracker creates a tracker that persists to the given file path.
// If path is empty, it defaults to ~/.agm/failure-tracking.json.
func NewFailureTracker(path string) (*FailureTracker, error) {
	if path == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, err
		}
		path = filepath.Join(home, ".agm", "failure-tracking.json")
	}

	ft := &FailureTracker{
		path:     path,
		failures: make(map[string]*FailureRecord),
	}

	if err := ft.load(); err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	return ft, nil
}

// RecordFailure increments the failure count for the given task ID.
func (ft *FailureTracker) RecordFailure(taskID string) error {
	ft.mu.Lock()
	defer ft.mu.Unlock()

	rec, ok := ft.failures[taskID]
	if !ok {
		rec = &FailureRecord{}
		ft.failures[taskID] = rec
	}
	rec.Count++
	rec.LastFail = time.Now()

	return ft.save()
}

// ShouldSkip returns true if the task has reached or exceeded maxFailures.
func (ft *FailureTracker) ShouldSkip(taskID string, maxFailures int) bool {
	ft.mu.Lock()
	defer ft.mu.Unlock()

	rec, ok := ft.failures[taskID]
	if !ok {
		return false
	}
	return rec.Count >= maxFailures
}

// GetFailures returns the current failure count for the given task ID.
func (ft *FailureTracker) GetFailures(taskID string) int {
	ft.mu.Lock()
	defer ft.mu.Unlock()

	rec, ok := ft.failures[taskID]
	if !ok {
		return 0
	}
	return rec.Count
}

// Reset clears the failure count for the given task ID.
func (ft *FailureTracker) Reset(taskID string) error {
	ft.mu.Lock()
	defer ft.mu.Unlock()

	delete(ft.failures, taskID)
	return ft.save()
}

func (ft *FailureTracker) load() error {
	data, err := os.ReadFile(ft.path)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, &ft.failures)
}

func (ft *FailureTracker) save() error {
	if err := os.MkdirAll(filepath.Dir(ft.path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(ft.failures, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(ft.path, data, 0o600)
}
