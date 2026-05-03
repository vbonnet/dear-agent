package ops

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/vbonnet/dear-agent/agm/internal/contracts"
)

// RetryConfig defines retry behavior.
type RetryConfig struct {
	MaxRetries int             // Maximum number of retries
	Backoff    []time.Duration // Backoff delays per attempt
}

// DefaultRetryConfig returns the default retry configuration from contracts.
func DefaultRetryConfig() RetryConfig {
	slo := contracts.Load()
	backoffs := make([]time.Duration, len(slo.Retry.BackoffDelays))
	for i, d := range slo.Retry.BackoffDelays {
		backoffs[i] = d.Duration
	}
	return RetryConfig{
		MaxRetries: slo.Retry.MaxRetries,
		Backoff:    backoffs,
	}
}

// RetryState tracks retry attempts for a session.
type RetryState struct {
	SessionName  string    `json:"session_name"`
	AttemptCount int       `json:"attempt_count"`
	LastAttempt  time.Time `json:"last_attempt,omitempty"`
	LastError    string    `json:"last_error,omitempty"`
	NextRetryAt  time.Time `json:"next_retry_at,omitempty"`
}

// RetryTracker manages retry state and persistence.
type RetryTracker struct {
	config  RetryConfig
	baseDir string // Base directory for .agm/retries
}

// NewRetryTracker creates a new retry tracker.
func NewRetryTracker(baseDir string) *RetryTracker {
	return &RetryTracker{
		config:  DefaultRetryConfig(),
		baseDir: baseDir,
	}
}

// retryFilePath returns the path to the retry state file for a session.
func (rt *RetryTracker) retryFilePath(sessionName string) string {
	retriesDir := filepath.Join(rt.baseDir, ".agm", "retries")
	return filepath.Join(retriesDir, sessionName+".json")
}

// LoadRetryState loads the retry state for a session from disk.
func (rt *RetryTracker) LoadRetryState(sessionName string) (*RetryState, error) {
	path := rt.retryFilePath(sessionName)

	// If file doesn't exist, return empty state
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return &RetryState{
			SessionName:  sessionName,
			AttemptCount: 0,
		}, nil
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read retry state: %w", err)
	}

	var state RetryState
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, fmt.Errorf("failed to parse retry state: %w", err)
	}

	return &state, nil
}

// SaveRetryState saves the retry state for a session to disk.
func (rt *RetryTracker) SaveRetryState(state *RetryState) error {
	path := rt.retryFilePath(state.SessionName)

	// Ensure directory exists
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("failed to create retry directory: %w", err)
	}

	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal retry state: %w", err)
	}

	if err := os.WriteFile(path, data, 0o600); err != nil {
		return fmt.Errorf("failed to write retry state: %w", err)
	}

	return nil
}

// RecordRetryAttempt records a failed attempt and updates retry state.
// Returns true if another retry should be attempted, false if max retries exceeded.
func (rt *RetryTracker) RecordRetryAttempt(sessionName string, lastError string) (bool, *RetryState, error) {
	state, err := rt.LoadRetryState(sessionName)
	if err != nil {
		return false, nil, fmt.Errorf("failed to load retry state: %w", err)
	}

	now := time.Now()
	state.SessionName = sessionName
	state.AttemptCount++
	state.LastAttempt = now
	state.LastError = lastError

	// Calculate next retry time (even for the last attempt)
	if state.AttemptCount <= len(rt.config.Backoff) {
		backoffIdx := state.AttemptCount - 1
		backoffDuration := rt.config.Backoff[backoffIdx]
		state.NextRetryAt = now.Add(backoffDuration)
	}

	// Check if we can retry (after setting backoff for tracking)
	if state.AttemptCount >= rt.config.MaxRetries {
		// Save final state before giving up
		if err := rt.SaveRetryState(state); err != nil {
			return false, nil, err
		}
		return false, state, nil
	}

	if err := rt.SaveRetryState(state); err != nil {
		return false, nil, err
	}

	return true, state, nil
}

// CanRetryNow checks if a session is ready for retry (backoff elapsed).
func (rt *RetryTracker) CanRetryNow(sessionName string) (bool, error) {
	state, err := rt.LoadRetryState(sessionName)
	if err != nil {
		return false, err
	}

	// No retries recorded yet
	if state.AttemptCount == 0 {
		return false, nil
	}

	// Max retries exceeded
	if state.AttemptCount >= rt.config.MaxRetries {
		return false, nil
	}

	// Check if backoff has elapsed
	if state.NextRetryAt.IsZero() {
		return true, nil
	}

	return time.Now().After(state.NextRetryAt), nil
}

// ClearRetryState removes retry state for a session.
func (rt *RetryTracker) ClearRetryState(sessionName string) error {
	path := rt.retryFilePath(sessionName)
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to clear retry state: %w", err)
	}
	return nil
}

// GetRetryState returns the current retry state for a session.
func (rt *RetryTracker) GetRetryState(sessionName string) (*RetryState, error) {
	return rt.LoadRetryState(sessionName)
}
