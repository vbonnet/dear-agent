package status

import (
	"testing"
)

func TestNextPhase(t *testing.T) {
	tests := []struct {
		name        string
		current     string
		phases      []Phase // phases with completion status
		expected    string
		expectError bool
		errorMsg    string
	}{
		{
			name:        "start of workflow (no current phase)",
			current:     "",
			phases:      []Phase{},
			expected:    "W0",
			expectError: false,
		},
		{
			name:    "D1 to D2",
			current: "D1",
			phases: []Phase{
				{Name: "D1", Status: PhaseStatusCompleted}, // D1 is completed
			},
			expected:    "D2",
			expectError: false,
		},
		{
			name:    "D2 to D3",
			current: "D2",
			phases: []Phase{
				{Name: "D1", Status: PhaseStatusCompleted},
				{Name: "D2", Status: PhaseStatusCompleted}, // D2 is completed
			},
			expected:    "D3",
			expectError: false,
		},
		{
			name:    "D3 to D4",
			current: "D3",
			phases: []Phase{
				{Name: "D1", Status: PhaseStatusCompleted},
				{Name: "D2", Status: PhaseStatusCompleted},
				{Name: "D3", Status: PhaseStatusCompleted}, // D3 is completed
			},
			expected:    "D4",
			expectError: false,
		},
		{
			name:    "D4 to S4 (discovery to SDLC transition)",
			current: "D4",
			phases: []Phase{
				{Name: "D1", Status: PhaseStatusCompleted},
				{Name: "D2", Status: PhaseStatusCompleted},
				{Name: "D3", Status: PhaseStatusCompleted},
				{Name: "D4", Status: PhaseStatusCompleted}, // D4 is completed
			},
			expected:    "S4",
			expectError: false,
		},
		{
			name:    "S4 to S5",
			current: "S4",
			phases: []Phase{
				{Name: "S4", Status: PhaseStatusCompleted}, // S4 is completed
			},
			expected:    "S5",
			expectError: false,
		},
		{
			name:    "S9 to S10",
			current: "S9",
			phases: []Phase{
				{Name: "S9", Status: PhaseStatusCompleted}, // S9 is completed
			},
			expected:    "S10",
			expectError: false,
		},
		{
			name:    "S10 to S11",
			current: "S10",
			phases: []Phase{
				{Name: "S10", Status: PhaseStatusCompleted}, // S10 is completed
			},
			expected:    "S11",
			expectError: false,
		},
		{
			name:    "current phase not completed returns same phase",
			current: "D2",
			phases: []Phase{
				{Name: "D1", Status: PhaseStatusCompleted},
				{Name: "D2", Status: PhaseStatusInProgress}, // D2 not completed
			},
			expected:    "D2",
			expectError: false,
		},
		{
			name:        "current phase set but Phases array empty",
			current:     "D3",
			phases:      []Phase{}, // Empty phases array
			expected:    "D3",
			expectError: false,
		},
		{
			name:    "at final phase S11",
			current: "S11",
			phases: []Phase{
				{Name: "S11", Status: PhaseStatusCompleted},
			},
			expected:    "",
			expectError: true,
			errorMsg:    "already at final phase S11",
		},
		{
			name:        "invalid phase",
			current:     "X99",
			phases:      []Phase{},
			expected:    "",
			expectError: true,
			errorMsg:    "invalid current phase: X99",
		},
		{
			name:        "another invalid phase",
			current:     "S99",
			phases:      []Phase{},
			expected:    "",
			expectError: true,
			errorMsg:    "invalid current phase: S99",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := &Status{
				CurrentPhase: tt.current,
				Phases:       tt.phases,
			}

			result, err := s.NextPhase()

			if tt.expectError {
				if err == nil {
					t.Errorf("expected error but got none")
					return
				}
				if err.Error() != tt.errorMsg {
					t.Errorf("expected error message %q, got %q", tt.errorMsg, err.Error())
				}
			} else {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
					return
				}
				if result != tt.expected {
					t.Errorf("expected %q, got %q", tt.expected, result)
				}
			}
		})
	}
}

// TestNextPhase_AllPhases verifies we can traverse the entire phase sequence
func TestNextPhase_AllPhases(t *testing.T) {
	expectedSequence := []string{"W0", "D1", "D2", "D3", "D4", "S4", "S5", "S6", "S7", "S8", "S9", "S10", "S11"}

	s := &Status{
		CurrentPhase: "",
		Phases:       []Phase{},
	}

	for i, expected := range expectedSequence {
		next, err := s.NextPhase()
		if err != nil {
			t.Fatalf("unexpected error at step %d: %v", i, err)
		}
		if next != expected {
			t.Errorf("step %d: expected %q, got %q", i, expected, next)
		}

		// Advance to next phase and mark current phase as completed
		if i < len(expectedSequence)-1 {
			s.CurrentPhase = next
			// Mark the phase as completed so NextPhase() will advance
			s.UpdatePhase(next, PhaseStatusCompleted, "success")
		}
	}

	// Verify S11 is truly final
	s.CurrentPhase = "S11"
	s.UpdatePhase("S11", PhaseStatusCompleted, "success")
	_, err := s.NextPhase()
	if err == nil {
		t.Error("expected error when advancing from S11, got none")
		return
	}
	if err.Error() != "already at final phase S11" {
		t.Errorf("unexpected error message: %v", err)
	}
}

// TestAllPhases_IncludesW0 verifies W0 is included in AllPhases()
func TestAllPhases_IncludesW0(t *testing.T) {
	phases := AllPhases()

	// Verify W0 is first
	if len(phases) == 0 {
		t.Fatal("AllPhases() returned empty slice")
	}

	if phases[0] != "W0" {
		t.Errorf("AllPhases()[0] = %q, want %q", phases[0], "W0")
	}

	// Verify total count is 13 (W0 + D1-D4 + S4-S11)
	if len(phases) != 13 {
		t.Errorf("AllPhases() length = %d, want 13", len(phases))
	}

	// Verify exact sequence
	expected := []string{"W0", "D1", "D2", "D3", "D4", "S4", "S5", "S6", "S7", "S8", "S9", "S10", "S11"}
	for i, phase := range phases {
		if phase != expected[i] {
			t.Errorf("AllPhases()[%d] = %q, want %q", i, phase, expected[i])
		}
	}
}

// TestFormatPhaseList_NilPointers verifies formatPhaseList doesn't crash on nil pointers
func TestFormatPhaseList_NilPointers(t *testing.T) {
	// Test 1: nil Status should not crash
	var nilStatus *Status
	result := nilStatus.formatPhaseList()
	if result != "" {
		t.Errorf("nil Status formatPhaseList() = %q, want empty string", result)
	}

	// Test 2: Phase with nil StartedAt/CompletedAt should not crash
	s := &Status{
		Phases: []Phase{
			{
				Name:        "D1",
				Status:      PhaseStatusCompleted,
				StartedAt:   nil, // nil pointer
				CompletedAt: nil, // nil pointer
			},
		},
	}

	// Should not crash
	result = s.formatPhaseList()
	if result == "" {
		t.Error("formatPhaseList() returned empty string for valid status with nil timestamps")
	}

	// Should contain the phase name
	if !containsSubstr(result, "D1") {
		t.Errorf("formatPhaseList() does not contain phase name D1: %q", result)
	}
}

// containsSubstr is a helper function to check if a string contains a substring
func containsSubstr(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 ||
		(len(s) > 0 && (s[:len(substr)] == substr || containsSubstr(s[1:], substr))))
}
