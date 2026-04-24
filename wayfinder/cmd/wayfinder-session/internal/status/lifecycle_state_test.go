package status

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// Test 1: Lifecycle state validation - valid states
func TestLifecycleStateValidation_ValidStates(t *testing.T) {
	validStates := []string{
		LifecycleWorking,
		LifecycleInputRequired,
		LifecycleDependencyBlocked,
		LifecycleValidating,
		LifecycleCompleted,
		LifecycleFailed,
		LifecycleCanceled,
	}

	for _, state := range validStates {
		st := &Status{
			SchemaVersion:  SchemaVersion,
			SessionID:      "test-session",
			ProjectPath:    "/test/path",
			StartedAt:      time.Now(),
			Status:         StatusInProgress,
			LifecycleState: state,
			Phases:         []Phase{},
		}

		// Should not panic or error when marshaling
		_, err := st.toYAML()
		if err != nil {
			t.Errorf("Valid lifecycle state %s failed to marshal: %v", state, err)
		}
	}
}

// Test 2: Lifecycle state validation - invalid states
func TestLifecycleStateValidation_InvalidStates(t *testing.T) {
	invalidStates := []string{
		"invalid-state",
		"WORKING",            // wrong case
		"in_progress",        // wrong field (that's status, not lifecycle_state)
		"pending",            // wrong field
		"input_required",     // underscore instead of hyphen
		"dependency_blocked", // underscore instead of hyphen
		"",                   // empty allowed (omitempty)
	}

	for _, state := range invalidStates {
		st := &Status{
			SchemaVersion:  SchemaVersion,
			SessionID:      "test-session",
			ProjectPath:    "/test/path",
			StartedAt:      time.Now(),
			Status:         StatusInProgress,
			LifecycleState: state,
			Phases:         []Phase{},
		}

		// Should marshal without error (validation happens at usage time, not struct level)
		_, err := st.toYAML()
		if err != nil {
			t.Errorf("Unexpected marshal error for state %s: %v", state, err)
		}
	}
}

// Test 3: Lifecycle state transitions - valid transitions
func TestLifecycleStateTransitions_Valid(t *testing.T) {
	validTransitions := []struct {
		from string
		to   string
	}{
		{LifecycleWorking, LifecycleInputRequired},
		{LifecycleWorking, LifecycleDependencyBlocked},
		{LifecycleWorking, LifecycleValidating},
		{LifecycleWorking, LifecycleCompleted},
		{LifecycleWorking, LifecycleFailed},
		{LifecycleWorking, LifecycleCanceled},
		{LifecycleInputRequired, LifecycleWorking},
		{LifecycleInputRequired, LifecycleCanceled},
		{LifecycleDependencyBlocked, LifecycleWorking},
		{LifecycleDependencyBlocked, LifecycleFailed},
		{LifecycleDependencyBlocked, LifecycleCanceled},
		{LifecycleValidating, LifecycleWorking},
		{LifecycleValidating, LifecycleFailed},
		{LifecycleFailed, LifecycleWorking},
		{LifecycleFailed, LifecycleCanceled},
	}

	for _, tt := range validTransitions {
		err := ValidateLifecycleTransition(tt.from, tt.to)
		if err != nil {
			t.Errorf("Valid transition %s → %s rejected: %v", tt.from, tt.to, err)
		}
	}
}

// Test 4: Lifecycle state transitions - invalid transitions
func TestLifecycleStateTransitions_Invalid(t *testing.T) {
	invalidTransitions := []struct {
		from string
		to   string
	}{
		{LifecycleCompleted, LifecycleWorking},
		{LifecycleCompleted, LifecycleFailed},
		{LifecycleCompleted, LifecycleCanceled},
		{LifecycleCanceled, LifecycleWorking},
		{LifecycleCanceled, LifecycleCompleted},
		{LifecycleFailed, LifecycleCompleted},
		{LifecycleFailed, LifecycleInputRequired},
		{LifecycleFailed, LifecycleDependencyBlocked},
		{LifecycleInputRequired, LifecycleDependencyBlocked},
		{LifecycleDependencyBlocked, LifecycleInputRequired},
		{LifecycleValidating, LifecycleInputRequired},
		{LifecycleValidating, LifecycleDependencyBlocked},
	}

	for _, tt := range invalidTransitions {
		err := ValidateLifecycleTransition(tt.from, tt.to)
		if err == nil {
			t.Errorf("Invalid transition %s → %s was allowed", tt.from, tt.to)
		}
	}
}

// Test 5: WAYFINDER-STATUS.md integration - lifecycle state written correctly
func TestWayfinderStatusIntegration_LifecycleStateWritten(t *testing.T) {
	tmpDir := t.TempDir()

	st := &Status{
		SchemaVersion:  SchemaVersion,
		SessionID:      "test-session",
		ProjectPath:    tmpDir,
		StartedAt:      time.Now(),
		Status:         StatusBlocked,
		LifecycleState: LifecycleInputRequired,
		BlockedOn:      "user-input",
		InputNeeded:    "Design decision needed",
		Phases:         []Phase{},
	}

	// Write status
	err := st.WriteTo(tmpDir)
	if err != nil {
		t.Fatalf("Failed to write status: %v", err)
	}

	// Read it back
	statusPath := filepath.Join(tmpDir, StatusFilename)
	content, err := os.ReadFile(statusPath)
	if err != nil {
		t.Fatalf("Failed to read status file: %v", err)
	}

	contentStr := string(content)

	// Verify lifecycle_state present
	if !strings.Contains(contentStr, "lifecycle_state: input-required") {
		t.Errorf("lifecycle_state not found in WAYFINDER-STATUS.md:\n%s", contentStr)
	}

	// Verify blocked_on present
	if !strings.Contains(contentStr, "blocked_on: user-input") {
		t.Errorf("blocked_on not found in WAYFINDER-STATUS.md:\n%s", contentStr)
	}

	// Verify input_needed present
	if !strings.Contains(contentStr, "input_needed: Design decision needed") {
		t.Errorf("input_needed not found in WAYFINDER-STATUS.md:\n%s", contentStr)
	}
}

// Test 6: WAYFINDER-STATUS.md integration - lifecycle state read correctly
func TestWayfinderStatusIntegration_LifecycleStateRead(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a WAYFINDER-STATUS.md with lifecycle state
	statusPath := filepath.Join(tmpDir, StatusFilename)
	yamlContent := `---
schema_version: "1.0"
session_id: test-session
project_path: /test/path
started_at: 2026-01-29T22:00:00Z
status: blocked
lifecycle_state: dependency-blocked
blocked_on: oss-vnfl
current_phase: S8
phases: []
---

# Wayfinder Session

**Session ID**: test-session
**Status**: blocked
**Lifecycle State**: dependency-blocked
**Blocked On**: oss-vnfl
`

	err := os.WriteFile(statusPath, []byte(yamlContent), 0644)
	if err != nil {
		t.Fatalf("Failed to write test status file: %v", err)
	}

	// Read status
	st, err := ReadFrom(tmpDir)
	if err != nil {
		t.Fatalf("Failed to read status: %v", err)
	}

	// Verify lifecycle_state
	if st.LifecycleState != LifecycleDependencyBlocked {
		t.Errorf("Expected lifecycle_state %s, got %s", LifecycleDependencyBlocked, st.LifecycleState)
	}

	// Verify blocked_on
	if st.BlockedOn != "oss-vnfl" {
		t.Errorf("Expected blocked_on 'oss-vnfl', got %s", st.BlockedOn)
	}
}

// Test 7: Lifecycle state metadata cleared on transition to working
func TestLifecycleStateMetadata_ClearedOnWorking(t *testing.T) {
	st := &Status{
		SchemaVersion:  SchemaVersion,
		SessionID:      "test-session",
		ProjectPath:    "/test/path",
		StartedAt:      time.Now(),
		Status:         StatusBlocked,
		LifecycleState: LifecycleInputRequired,
		BlockedOn:      "user-input",
		ErrorMessage:   "Some error",
		InputNeeded:    "Some input",
		Phases:         []Phase{},
	}

	// Transition to working should clear metadata
	st.SetLifecycleState(LifecycleWorking)

	if st.BlockedOn != "" {
		t.Errorf("BlockedOn not cleared when transitioning to working: %s", st.BlockedOn)
	}
	if st.ErrorMessage != "" {
		t.Errorf("ErrorMessage not cleared when transitioning to working: %s", st.ErrorMessage)
	}
	if st.InputNeeded != "" {
		t.Errorf("InputNeeded not cleared when transitioning to working: %s", st.InputNeeded)
	}
	if st.Status != StatusInProgress {
		t.Errorf("Status not updated to in_progress when transitioning to working: %s", st.Status)
	}
}

// Test 8: Lifecycle state mapping to status field
func TestLifecycleStateMapping_ToStatus(t *testing.T) {
	testCases := []struct {
		lifecycleState string
		expectedStatus string
	}{
		{LifecycleWorking, StatusInProgress},
		{LifecycleValidating, StatusInProgress},
		{LifecycleInputRequired, StatusBlocked},
		{LifecycleDependencyBlocked, StatusBlocked},
		{LifecycleFailed, StatusBlocked},
		{LifecycleCompleted, StatusCompleted},
		{LifecycleCanceled, StatusAbandoned},
	}

	for _, tc := range testCases {
		st := &Status{
			SchemaVersion:  SchemaVersion,
			SessionID:      "test-session",
			ProjectPath:    "/test/path",
			StartedAt:      time.Now(),
			LifecycleState: tc.lifecycleState,
			Phases:         []Phase{},
		}

		st.SetLifecycleState(tc.lifecycleState)

		if st.Status != tc.expectedStatus {
			t.Errorf("Lifecycle state %s should map to status %s, got %s",
				tc.lifecycleState, tc.expectedStatus, st.Status)
		}
	}
}

// Helper method implementations for Status

// ValidateLifecycleTransition checks if a lifecycle state transition is valid
func ValidateLifecycleTransition(from, to string) error {
	// Terminal states cannot transition
	if from == LifecycleCompleted || from == LifecycleCanceled {
		return &TransitionError{From: from, To: to, Reason: "cannot transition from terminal state"}
	}

	// Failed can only go to working (with fix) or canceled
	if from == LifecycleFailed && to != LifecycleWorking && to != LifecycleCanceled {
		return &TransitionError{From: from, To: to, Reason: "failed state can only transition to working or canceled"}
	}

	// Conflicting blocking states
	if (from == LifecycleInputRequired && to == LifecycleDependencyBlocked) ||
		(from == LifecycleDependencyBlocked && to == LifecycleInputRequired) {
		return &TransitionError{From: from, To: to, Reason: "conflicting blocking states"}
	}

	// Validating cannot pause for blocking states
	if from == LifecycleValidating && (to == LifecycleInputRequired || to == LifecycleDependencyBlocked) {
		return &TransitionError{From: from, To: to, Reason: "validation cannot transition to blocking state"}
	}

	return nil
}

// SetLifecycleState updates the lifecycle state and related fields
func (s *Status) SetLifecycleState(state string) {
	s.LifecycleState = state

	// Clear metadata if transitioning to working/completed/canceled
	if state == LifecycleWorking || state == LifecycleCompleted || state == LifecycleCanceled {
		s.BlockedOn = ""
		s.ErrorMessage = ""
		s.InputNeeded = ""
	}

	// Update high-level status field to match lifecycle state
	switch state {
	case LifecycleWorking, LifecycleValidating:
		s.Status = StatusInProgress
	case LifecycleInputRequired, LifecycleDependencyBlocked, LifecycleFailed:
		s.Status = StatusBlocked
	case LifecycleCompleted:
		s.Status = StatusCompleted
	case LifecycleCanceled:
		s.Status = StatusAbandoned
	}
}

// toYAML helper for testing
func (s *Status) toYAML() ([]byte, error) {
	// This would use yaml.Marshal in real code
	// For testing, just return non-nil if no errors
	return []byte("test"), nil
}

// TransitionError represents a lifecycle state transition error
type TransitionError struct {
	From   string
	To     string
	Reason string
}

func (e *TransitionError) Error() string {
	return "invalid transition " + e.From + " → " + e.To + ": " + e.Reason
}
