package reaper

import (
	"testing"
	"time"
)

func TestReaperTimeout_ConstantValue(t *testing.T) {
	if ReaperTimeout != 10*time.Minute {
		t.Errorf("ReaperTimeout = %v, want 10m", ReaperTimeout)
	}
}

func TestReaperTimeout_ExceedsPhase1Budgets(t *testing.T) {
	// The overall timeout must exceed the sum of all Phase 1 step timeouts
	// to avoid false zombie detection during normal operation.
	phase1Max := PromptDetectionTimeout + FallbackWaitTime +
		PaneCloseTimeout + SIGTERMGracePeriod + PostKillPaneTimeout
	if ReaperTimeout <= phase1Max {
		t.Errorf("ReaperTimeout (%v) should exceed Phase 1 budget (%v)", ReaperTimeout, phase1Max)
	}
}

func TestTimeRemaining_FullBudget(t *testing.T) {
	r := New("test", "/tmp")
	startTime := time.Now()

	remaining := r.timeRemaining(startTime)
	// Should be very close to ReaperTimeout (within 1 second)
	if remaining < ReaperTimeout-time.Second || remaining > ReaperTimeout {
		t.Errorf("timeRemaining() = %v, want ~%v", remaining, ReaperTimeout)
	}
}

func TestTimeRemaining_PartiallyElapsed(t *testing.T) {
	r := New("test", "/tmp")
	// Simulate a start time 5 minutes ago
	startTime := time.Now().Add(-5 * time.Minute)

	remaining := r.timeRemaining(startTime)
	expected := 5 * time.Minute
	tolerance := time.Second

	if remaining < expected-tolerance || remaining > expected+tolerance {
		t.Errorf("timeRemaining() = %v, want ~%v", remaining, expected)
	}
}

func TestTimeRemaining_Exceeded(t *testing.T) {
	r := New("test", "/tmp")
	// Simulate a start time 11 minutes ago (past the 10-min timeout)
	startTime := time.Now().Add(-11 * time.Minute)

	remaining := r.timeRemaining(startTime)
	if remaining > 0 {
		t.Errorf("timeRemaining() = %v, should be <= 0 after timeout exceeded", remaining)
	}
}

func TestTimeRemaining_ExactlyAtLimit(t *testing.T) {
	r := New("test", "/tmp")
	// Simulate start exactly ReaperTimeout ago
	startTime := time.Now().Add(-ReaperTimeout)

	remaining := r.timeRemaining(startTime)
	// Should be at or just below zero
	if remaining > time.Millisecond {
		t.Errorf("timeRemaining() = %v, should be ~0 at exact timeout", remaining)
	}
}

func TestTimeRemainingMethod_Exists(t *testing.T) {
	r := New("test", "/tmp")
	// Compile-time check that the method exists
	var _ = r.timeRemaining
}

func TestStopProcess_Exists(t *testing.T) {
	r := New("test", "/tmp")
	// Compile-time check that the method exists
	var _ = r.stopProcess
}
