package ops

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestRetryTracker_LoadRetryState_NonExistent(t *testing.T) {
	tmpDir := t.TempDir()
	rt := NewRetryTracker(tmpDir)

	state, err := rt.LoadRetryState("nonexistent-session")
	if err != nil {
		t.Fatalf("LoadRetryState failed: %v", err)
	}

	if state.SessionName != "nonexistent-session" {
		t.Errorf("Expected session name to be 'nonexistent-session', got %q", state.SessionName)
	}

	if state.AttemptCount != 0 {
		t.Errorf("Expected attempt count 0, got %d", state.AttemptCount)
	}
}

func TestRetryTracker_SaveAndLoad(t *testing.T) {
	tmpDir := t.TempDir()
	rt := NewRetryTracker(tmpDir)

	state := &RetryState{
		SessionName:  "test-session",
		AttemptCount: 2,
		LastError:    "connection timeout",
		LastAttempt:  time.Now(),
	}

	if err := rt.SaveRetryState(state); err != nil {
		t.Fatalf("SaveRetryState failed: %v", err)
	}

	loaded, err := rt.LoadRetryState("test-session")
	if err != nil {
		t.Fatalf("LoadRetryState failed: %v", err)
	}

	if loaded.SessionName != state.SessionName {
		t.Errorf("Session name mismatch: expected %q, got %q", state.SessionName, loaded.SessionName)
	}

	if loaded.AttemptCount != state.AttemptCount {
		t.Errorf("Attempt count mismatch: expected %d, got %d", state.AttemptCount, loaded.AttemptCount)
	}

	if loaded.LastError != state.LastError {
		t.Errorf("Last error mismatch: expected %q, got %q", state.LastError, loaded.LastError)
	}
}

func TestRetryTracker_RecordRetryAttempt_FirstAttempt(t *testing.T) {
	tmpDir := t.TempDir()
	rt := NewRetryTracker(tmpDir)

	canRetry, state, err := rt.RecordRetryAttempt("test-session", "first error")
	if err != nil {
		t.Fatalf("RecordRetryAttempt failed: %v", err)
	}

	if !canRetry {
		t.Errorf("Expected canRetry to be true for first attempt")
	}

	if state.AttemptCount != 1 {
		t.Errorf("Expected attempt count 1, got %d", state.AttemptCount)
	}

	if state.LastError != "first error" {
		t.Errorf("Expected last error 'first error', got %q", state.LastError)
	}

	// Verify NextRetryAt is set correctly (should be ~1 minute from now)
	if state.NextRetryAt.IsZero() {
		t.Errorf("Expected NextRetryAt to be set")
	}

	expectedBackoff := 1 * time.Minute
	expectedTime := time.Now().Add(expectedBackoff)
	timeDiff := expectedTime.Sub(state.NextRetryAt).Abs()
	if timeDiff > 5*time.Second {
		t.Errorf("NextRetryAt too far from expected: diff = %v", timeDiff)
	}
}

func TestRetryTracker_RecordRetryAttempt_Backoff_Progression(t *testing.T) {
	tmpDir := t.TempDir()
	rt := NewRetryTracker(tmpDir)

	// First attempt: backoff = 1 minute
	_, state1, _ := rt.RecordRetryAttempt("test-session", "error 1")
	if state1.AttemptCount != 1 {
		t.Fatalf("Expected attempt count 1, got %d", state1.AttemptCount)
	}
	expectedBackoff1 := 1 * time.Minute
	backoffDiff1 := state1.NextRetryAt.Sub(state1.LastAttempt)
	if backoffDiff1 < expectedBackoff1-500*time.Millisecond || backoffDiff1 > expectedBackoff1+500*time.Millisecond {
		t.Errorf("First backoff not ~1 minute: %v", backoffDiff1)
	}

	// Second attempt: backoff = 3 minutes
	_, state2, _ := rt.RecordRetryAttempt("test-session", "error 2")
	if state2.AttemptCount != 2 {
		t.Fatalf("Expected attempt count 2, got %d", state2.AttemptCount)
	}
	expectedBackoff2 := 3 * time.Minute
	backoffDiff2 := state2.NextRetryAt.Sub(state2.LastAttempt)
	if backoffDiff2 < expectedBackoff2-500*time.Millisecond || backoffDiff2 > expectedBackoff2+500*time.Millisecond {
		t.Errorf("Second backoff not ~3 minutes: %v", backoffDiff2)
	}

	// Third attempt: backoff = 10 minutes
	_, state3, _ := rt.RecordRetryAttempt("test-session", "error 3")
	if state3.AttemptCount != 3 {
		t.Fatalf("Expected attempt count 3, got %d", state3.AttemptCount)
	}
	expectedBackoff3 := 10 * time.Minute
	backoffDiff3 := state3.NextRetryAt.Sub(state3.LastAttempt)
	if backoffDiff3 < expectedBackoff3-500*time.Millisecond || backoffDiff3 > expectedBackoff3+500*time.Millisecond {
		t.Errorf("Third backoff not ~10 minutes: %v", backoffDiff3)
	}
}

func TestRetryTracker_RecordRetryAttempt_MaxRetriesExceeded(t *testing.T) {
	tmpDir := t.TempDir()
	rt := NewRetryTracker(tmpDir)

	// Simulate hitting max retries
	for i := 1; i <= 3; i++ {
		_, _, _ = rt.RecordRetryAttempt("test-session", fmt.Sprintf("error %d", i))
	}

	// Fourth attempt should exceed max
	canRetry, state, err := rt.RecordRetryAttempt("test-session", "error 4")
	if err != nil {
		t.Fatalf("RecordRetryAttempt failed: %v", err)
	}

	if canRetry {
		t.Errorf("Expected canRetry to be false after max retries")
	}

	if state.AttemptCount != 4 {
		t.Errorf("Expected attempt count 4, got %d", state.AttemptCount)
	}
}

func TestRetryTracker_CanRetryNow(t *testing.T) {
	tmpDir := t.TempDir()
	rt := NewRetryTracker(tmpDir)

	// Create a retry state with NextRetryAt in the future
	state := &RetryState{
		SessionName:  "test-session",
		AttemptCount: 1,
		LastError:    "test error",
		NextRetryAt:  time.Now().Add(10 * time.Minute),
	}
	rt.SaveRetryState(state)

	canRetry, err := rt.CanRetryNow("test-session")
	if err != nil {
		t.Fatalf("CanRetryNow failed: %v", err)
	}

	if canRetry {
		t.Errorf("Expected canRetry to be false (backoff not elapsed)")
	}

	// Update state so NextRetryAt is in the past
	state.NextRetryAt = time.Now().Add(-1 * time.Minute)
	rt.SaveRetryState(state)

	canRetry, err = rt.CanRetryNow("test-session")
	if err != nil {
		t.Fatalf("CanRetryNow failed: %v", err)
	}

	if !canRetry {
		t.Errorf("Expected canRetry to be true (backoff elapsed)")
	}
}

func TestRetryTracker_CanRetryNow_NoAttempts(t *testing.T) {
	tmpDir := t.TempDir()
	rt := NewRetryTracker(tmpDir)

	canRetry, err := rt.CanRetryNow("nonexistent-session")
	if err != nil {
		t.Fatalf("CanRetryNow failed: %v", err)
	}

	if canRetry {
		t.Errorf("Expected canRetry to be false (no attempts recorded)")
	}
}

func TestRetryTracker_CanRetryNow_MaxRetriesExceeded(t *testing.T) {
	tmpDir := t.TempDir()
	rt := NewRetryTracker(tmpDir)

	state := &RetryState{
		SessionName:  "test-session",
		AttemptCount: 3, // Max retries
		LastError:    "test error",
	}
	rt.SaveRetryState(state)

	canRetry, err := rt.CanRetryNow("test-session")
	if err != nil {
		t.Fatalf("CanRetryNow failed: %v", err)
	}

	if canRetry {
		t.Errorf("Expected canRetry to be false (max retries exceeded)")
	}
}

func TestRetryTracker_ClearRetryState(t *testing.T) {
	tmpDir := t.TempDir()
	rt := NewRetryTracker(tmpDir)

	state := &RetryState{
		SessionName:  "test-session",
		AttemptCount: 2,
		LastError:    "test error",
	}
	rt.SaveRetryState(state)

	// Verify state exists
	_, err := os.Stat(rt.retryFilePath("test-session"))
	if err != nil {
		t.Fatalf("Retry state file not created")
	}

	// Clear state
	if err := rt.ClearRetryState("test-session"); err != nil {
		t.Fatalf("ClearRetryState failed: %v", err)
	}

	// Verify state is gone
	_, err = os.Stat(rt.retryFilePath("test-session"))
	if !os.IsNotExist(err) {
		t.Errorf("Expected retry state file to be deleted")
	}
}

func TestRetryTracker_GetRetryState(t *testing.T) {
	tmpDir := t.TempDir()
	rt := NewRetryTracker(tmpDir)

	state := &RetryState{
		SessionName:  "test-session",
		AttemptCount: 2,
		LastError:    "test error",
	}
	rt.SaveRetryState(state)

	retrieved, err := rt.GetRetryState("test-session")
	if err != nil {
		t.Fatalf("GetRetryState failed: %v", err)
	}

	if retrieved.SessionName != state.SessionName {
		t.Errorf("Expected session %q, got %q", state.SessionName, retrieved.SessionName)
	}

	if retrieved.AttemptCount != state.AttemptCount {
		t.Errorf("Expected attempt count %d, got %d", state.AttemptCount, retrieved.AttemptCount)
	}
}

func TestRetryTracker_DirectoryCreation(t *testing.T) {
	tmpDir := t.TempDir()
	rt := NewRetryTracker(tmpDir)

	state := &RetryState{
		SessionName:  "test-session",
		AttemptCount: 1,
		LastError:    "test error",
	}

	if err := rt.SaveRetryState(state); err != nil {
		t.Fatalf("SaveRetryState failed: %v", err)
	}

	// Verify directory structure was created
	retriesDir := filepath.Join(tmpDir, ".agm", "retries")
	if _, err := os.Stat(retriesDir); err != nil {
		t.Fatalf("Retries directory not created: %v", err)
	}
}

func TestDefaultRetryConfig(t *testing.T) {
	cfg := DefaultRetryConfig()

	if cfg.MaxRetries != 3 {
		t.Errorf("Expected MaxRetries 3, got %d", cfg.MaxRetries)
	}

	if len(cfg.Backoff) != 3 {
		t.Errorf("Expected 3 backoff durations, got %d", len(cfg.Backoff))
	}

	expectedBackoffs := []time.Duration{
		1 * time.Minute,
		3 * time.Minute,
		10 * time.Minute,
	}

	for i, expected := range expectedBackoffs {
		if cfg.Backoff[i] != expected {
			t.Errorf("Backoff[%d]: expected %v, got %v", i, expected, cfg.Backoff[i])
		}
	}
}
