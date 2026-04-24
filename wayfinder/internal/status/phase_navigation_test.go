package status

import (
	"testing"
)

func TestNextPhase(t *testing.T) {
	tests := []struct {
		name        string
		current     string
		expected    string
		expectError bool
		errorMsg    string
	}{
		{
			name:        "start of discovery (no current phase)",
			current:     "",
			expected:    "D1",
			expectError: false,
		},
		{
			name:        "D1 to D2",
			current:     "D1",
			expected:    "D2",
			expectError: false,
		},
		{
			name:        "D2 to D3",
			current:     "D2",
			expected:    "D3",
			expectError: false,
		},
		{
			name:        "D3 to D4",
			current:     "D3",
			expected:    "D4",
			expectError: false,
		},
		{
			name:        "D4 to S4 (discovery to SDLC transition)",
			current:     "D4",
			expected:    "S4",
			expectError: false,
		},
		{
			name:        "S4 to S5",
			current:     "S4",
			expected:    "S5",
			expectError: false,
		},
		{
			name:        "S9 to S10",
			current:     "S9",
			expected:    "S10",
			expectError: false,
		},
		{
			name:        "S10 to S11",
			current:     "S10",
			expected:    "S11",
			expectError: false,
		},
		{
			name:        "at final phase S11",
			current:     "S11",
			expected:    "",
			expectError: true,
			errorMsg:    "already at final phase S11",
		},
		{
			name:        "invalid phase",
			current:     "X99",
			expected:    "",
			expectError: true,
			errorMsg:    "invalid current phase: X99",
		},
		{
			name:        "another invalid phase",
			current:     "S99",
			expected:    "",
			expectError: true,
			errorMsg:    "invalid current phase: S99",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := &Status{
				CurrentPhase: tt.current,
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
	expectedSequence := []string{"D1", "D2", "D3", "D4", "S4", "S5", "S6", "S7", "S8", "S9", "S10", "S11"}

	s := &Status{
		CurrentPhase: "",
	}

	for i, expected := range expectedSequence {
		next, err := s.NextPhase()
		if err != nil {
			t.Fatalf("unexpected error at step %d: %v", i, err)
		}
		if next != expected {
			t.Errorf("step %d: expected %q, got %q", i, expected, next)
		}

		// Advance to next phase (except for S11 since it's the last)
		if i < len(expectedSequence)-1 {
			s.CurrentPhase = next
		}
	}

	// Verify S11 is truly final
	s.CurrentPhase = "S11"
	_, err := s.NextPhase()
	if err == nil {
		t.Error("expected error when advancing from S11, got none")
	}
	if err.Error() != "already at final phase S11" {
		t.Errorf("unexpected error message: %v", err)
	}
}
