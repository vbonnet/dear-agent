package ops

import (
	"testing"
	"time"
)

func TestRetrySession_NoAttemptsRecorded(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("AGM_RETRY_BASE_DIR", tmpDir)
	ctx := &OpContext{}

	result, err := RetrySession(ctx, &RetrySessionRequest{
		SessionName: "test-session-unique-12345",
	})

	if err != nil {
		t.Fatalf("RetrySession failed: %v", err)
	}

	if result.Status != "ready" {
		t.Errorf("Expected status 'ready', got %q", result.Status)
	}

	if result.RetryState.AttemptCount != 0 {
		t.Errorf("Expected attempt count 0, got %d", result.RetryState.AttemptCount)
	}
}

func TestRetrySession_MaxRetriesExceeded(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("AGM_RETRY_BASE_DIR", tmpDir)
	ctx := &OpContext{}

	// Create a state with max retries reached
	rt := NewRetryTracker(tmpDir)
	state := &RetryState{
		SessionName:  "test-session",
		AttemptCount: 3,
		LastError:    "max attempts reached",
		LastAttempt:  time.Now(),
	}
	rt.SaveRetryState(state)

	result, err := RetrySession(ctx, &RetrySessionRequest{
		SessionName: "test-session",
	})

	if err != nil {
		t.Fatalf("RetrySession failed: %v", err)
	}

	if result.Status != "max_retries_exceeded" {
		t.Errorf("Expected status 'max_retries_exceeded', got %q", result.Status)
	}
}

func TestRetrySession_BackoffNotElapsed(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("AGM_RETRY_BASE_DIR", tmpDir)
	ctx := &OpContext{}

	// Create a state with backoff in the future
	rt := NewRetryTracker(tmpDir)
	state := &RetryState{
		SessionName:  "test-session",
		AttemptCount: 1,
		LastError:    "test error",
		LastAttempt:  time.Now(),
		NextRetryAt:  time.Now().Add(10 * time.Minute),
	}
	rt.SaveRetryState(state)

	result, err := RetrySession(ctx, &RetrySessionRequest{
		SessionName: "test-session",
	})

	if err != nil {
		t.Fatalf("RetrySession failed: %v", err)
	}

	if result.Status != "backoff_not_elapsed" {
		t.Errorf("Expected status 'backoff_not_elapsed', got %q", result.Status)
	}
}

func TestRetrySession_ReadyForRetry(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("AGM_RETRY_BASE_DIR", tmpDir)
	ctx := &OpContext{}

	// Create a state with backoff in the past
	rt := NewRetryTracker(tmpDir)
	state := &RetryState{
		SessionName:  "test-session",
		AttemptCount: 1,
		LastError:    "permission denied",
		LastAttempt:  time.Now().Add(-10 * time.Minute),
		NextRetryAt:  time.Now().Add(-1 * time.Minute),
	}
	rt.SaveRetryState(state)

	result, err := RetrySession(ctx, &RetrySessionRequest{
		SessionName: "test-session",
	})

	if err != nil {
		t.Fatalf("RetrySession failed: %v", err)
	}

	if result.Status != "ready_for_retry" {
		t.Errorf("Expected status 'ready_for_retry', got %q", result.Status)
	}

	if result.RetryState.LastError != "permission denied" {
		t.Errorf("Expected last error 'permission denied', got %q", result.RetryState.LastError)
	}
}

func TestRetrySession_EmptySessionName(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("AGM_RETRY_BASE_DIR", tmpDir)
	ctx := &OpContext{}

	_, err := RetrySession(ctx, &RetrySessionRequest{
		SessionName: "",
	})

	if err == nil {
		t.Errorf("Expected error for empty session name")
	}
}

func TestRetrySession_SecondAttempt(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("AGM_RETRY_BASE_DIR", tmpDir)
	ctx := &OpContext{}

	// Create a state for second attempt with backoff elapsed
	rt := NewRetryTracker(tmpDir)
	state := &RetryState{
		SessionName:  "test-session",
		AttemptCount: 1,
		LastError:    "first error",
		LastAttempt:  time.Now().Add(-5 * time.Minute),
		NextRetryAt:  time.Now().Add(-1 * time.Minute),
	}
	rt.SaveRetryState(state)

	result, err := RetrySession(ctx, &RetrySessionRequest{
		SessionName: "test-session",
	})

	if err != nil {
		t.Fatalf("RetrySession failed: %v", err)
	}

	if result.Status != "ready_for_retry" {
		t.Errorf("Expected status 'ready_for_retry', got %q", result.Status)
	}

	if result.RetryState.AttemptCount != 1 {
		t.Errorf("Expected attempt count 1, got %d", result.RetryState.AttemptCount)
	}
}

func TestRetrySession_ThirdAttemptBackoffNotElapsed(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("AGM_RETRY_BASE_DIR", tmpDir)
	ctx := &OpContext{}

	// Create a state for third attempt with backoff not elapsed
	rt := NewRetryTracker(tmpDir)
	state := &RetryState{
		SessionName:  "test-session",
		AttemptCount: 2,
		LastError:    "second error",
		LastAttempt:  time.Now().Add(-5 * time.Minute),
		NextRetryAt:  time.Now().Add(5 * time.Minute), // 5 minutes in future
	}
	rt.SaveRetryState(state)

	result, err := RetrySession(ctx, &RetrySessionRequest{
		SessionName: "test-session",
	})

	if err != nil {
		t.Fatalf("RetrySession failed: %v", err)
	}

	if result.Status != "backoff_not_elapsed" {
		t.Errorf("Expected status 'backoff_not_elapsed', got %q", result.Status)
	}

	if result.RetryState.AttemptCount != 2 {
		t.Errorf("Expected attempt count 2, got %d", result.RetryState.AttemptCount)
	}
}

func TestRetrySessionWithMessage_ReadyForRetry(t *testing.T) {
	// This test would require mocking SendMessage which requires deeper integration
	// For now, we test the basic retry message construction via RetrySession alone
	tmpDir := t.TempDir()
	t.Setenv("AGM_RETRY_BASE_DIR", tmpDir)
	ctx := &OpContext{}

	// Create a state ready for retry
	rt := NewRetryTracker(tmpDir)
	state := &RetryState{
		SessionName:  "test-session-msg-12345",
		AttemptCount: 1,
		LastError:    "test error",
		LastAttempt:  time.Now().Add(-10 * time.Minute),
		NextRetryAt:  time.Now().Add(-1 * time.Minute),
	}
	rt.SaveRetryState(state)

	// Test that RetrySession correctly reports ready_for_retry status
	result, err := RetrySession(ctx, &RetrySessionRequest{
		SessionName: "test-session-msg-12345",
	})

	if err != nil {
		t.Fatalf("RetrySession failed: %v", err)
	}

	if result.Status != "ready_for_retry" {
		t.Errorf("Expected status 'ready_for_retry', got %q", result.Status)
	}

	// Verify that the result indicates message content is available
	if result.Description == "" {
		t.Errorf("Expected non-empty description for ready_for_retry status")
	}
}
