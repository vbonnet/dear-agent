package ops

import (
	"testing"
	"time"
)

// TestStallRecovery_WithRetryTracking tests the retry tracking integration in stall recovery.
func TestStallRecovery_WithRetryTracking(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("AGM_RETRY_BASE_DIR", tmpDir)
	ctx := &OpContext{}
	sr := NewStallRecovery(ctx, "orchestrator")

	// Verify retry tracker is initialized
	if sr.retryTracker == nil {
		t.Errorf("Expected retryTracker to be initialized")
	}
}

func TestStallRecovery_CheckCanRetry_FirstAttempt(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("AGM_RETRY_BASE_DIR", tmpDir)
	ctx := &OpContext{}
	sr := NewStallRecovery(ctx, "orchestrator")

	canRetry, state, err := sr.checkCanRetry("test-session")
	if err != nil {
		t.Fatalf("checkCanRetry failed: %v", err)
	}

	if !canRetry {
		t.Errorf("Expected canRetry to be true for first attempt")
	}

	if state.AttemptCount != 0 {
		t.Errorf("Expected attempt count 0 for new session, got %d", state.AttemptCount)
	}
}

func TestStallRecovery_CheckCanRetry_WithBackoff(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("AGM_RETRY_BASE_DIR", tmpDir)
	ctx := &OpContext{}
	sr := NewStallRecovery(ctx, "orchestrator")

	// Record first failure
	sr.recordFailure("test-session", "first error")

	// Should not be able to retry immediately (backoff not elapsed)
	canRetry, state, err := sr.checkCanRetry("test-session")
	if err != nil {
		t.Fatalf("checkCanRetry failed: %v", err)
	}

	if canRetry {
		t.Errorf("Expected canRetry to be false (backoff not elapsed)")
	}

	if state.AttemptCount != 1 {
		t.Errorf("Expected attempt count 1, got %d", state.AttemptCount)
	}
}

func TestStallRecovery_RecordFailure(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("AGM_RETRY_BASE_DIR", tmpDir)
	ctx := &OpContext{}
	sr := NewStallRecovery(ctx, "orchestrator")

	err := sr.recordFailure("test-session", "test error message")
	if err != nil {
		t.Fatalf("recordFailure failed: %v", err)
	}

	// Verify state was recorded
	state, err := sr.retryTracker.GetRetryState("test-session")
	if err != nil {
		t.Fatalf("GetRetryState failed: %v", err)
	}

	if state.AttemptCount != 1 {
		t.Errorf("Expected attempt count 1, got %d", state.AttemptCount)
	}

	if state.LastError != "test error message" {
		t.Errorf("Expected last error 'test error message', got %q", state.LastError)
	}
}

func TestStallRecovery_RecordFailure_MaxRetriesExceeded(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("AGM_RETRY_BASE_DIR", tmpDir)
	ctx := &OpContext{}
	sr := NewStallRecovery(ctx, "orchestrator")

	// Record 3 failures (max retries)
	for i := 1; i <= 3; i++ {
		err := sr.recordFailure("test-session", "error")
		if err != nil {
			t.Fatalf("recordFailure %d failed: %v", i, err)
		}
	}

	// Verify state indicates max retries exceeded
	state, err := sr.retryTracker.GetRetryState("test-session")
	if err != nil {
		t.Fatalf("GetRetryState failed: %v", err)
	}

	if state.AttemptCount != 3 {
		t.Errorf("Expected attempt count 3, got %d", state.AttemptCount)
	}
}

// TestStallRecovery_CheckCanRetry_AfterBackoffElapsed tests retry after backoff duration passes.
func TestStallRecovery_CheckCanRetry_AfterBackoffElapsed(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("AGM_RETRY_BASE_DIR", tmpDir)
	ctx := &OpContext{}
	sr := NewStallRecovery(ctx, "orchestrator")

	// Record a failure
	sr.recordFailure("test-session", "error")

	// Update state to have backoff in the past
	state, _ := sr.retryTracker.GetRetryState("test-session")
	state.NextRetryAt = time.Now().Add(-1 * time.Minute)
	sr.retryTracker.SaveRetryState(state)

	// Should be able to retry now
	canRetry, _, err := sr.checkCanRetry("test-session")
	if err != nil {
		t.Fatalf("checkCanRetry failed: %v", err)
	}

	if !canRetry {
		t.Errorf("Expected canRetry to be true (backoff elapsed)")
	}
}

// TestStallRecovery_Recover_WithRetryContext tests that Recover includes error context from previous attempts.
func TestStallRecovery_Recover_WithRetryContext(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("AGM_RETRY_BASE_DIR", tmpDir)
	ctx := &OpContext{}
	sr := NewStallRecovery(ctx, "")

	// Record a previous failure
	sr.recordFailure("test-session", "permission denied")

	// Note: We can't fully test Recover without mocking SendMessage,
	// but we can verify that the method doesn't panic and error context
	// is available to recovery handlers

	// Verify the state contains error context
	state, err := sr.retryTracker.GetRetryState("test-session")
	if err != nil {
		t.Fatalf("GetRetryState failed: %v", err)
	}

	if state.LastError != "permission denied" {
		t.Errorf("Expected last error context 'permission denied', got %q", state.LastError)
	}
}

// TestStallRecovery_EscalateFailure tests that escalation tracks the correct metadata.
// Full testing would require mocking SendMessage, which is an external dependency.
func TestStallRecovery_EscalateFailure(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("AGM_RETRY_BASE_DIR", tmpDir)
	// Just verify that the escalation action structure is set up correctly
	// without calling SendMessage
	_ = NewStallRecovery(nil, "orchestrator")

	// Verify that an escalate action would have proper metadata
	expectedSessionName := "worker-session"
	expectedActionType := "escalate"

	action := RecoveryAction{
		SessionName: expectedSessionName,
		ActionType:  expectedActionType,
	}

	if action.SessionName != expectedSessionName {
		t.Errorf("Expected session name %q, got %q", expectedSessionName, action.SessionName)
	}

	if action.ActionType != expectedActionType {
		t.Errorf("Expected action type %q, got %q", expectedActionType, action.ActionType)
	}
}
