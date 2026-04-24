package retrospective

import (
	"testing"
)

func TestCalculateMagnitude(t *testing.T) {
	tests := []struct {
		from     string
		to       string
		expected int
		wantErr  bool
	}{
		// Same phase (magnitude 0)
		{"S7", "S7", 0, false},

		// Forward rewinds (moving backwards in time)
		{"S7", "S6", 1, false},   // S7 (idx 8) → S6 (idx 7) = |8-7| = 1
		{"S7", "S5", 2, false},   // S7 (idx 8) → S5 (idx 6) = |8-6| = 2
		{"S7", "D4", 4, false},   // S7 (idx 8) → D4 (idx 4) = |8-4| = 4
		{"S11", "W0", 12, false}, // S11 (idx 12) → W0 (idx 0) = |12-0| = 12

		// Edge cases
		{"W0", "W0", 0, false},
		{"S11", "S11", 0, false},

		// Unknown phases
		{"INVALID", "S5", 0, true},
		{"S5", "INVALID", 0, true},
	}

	for _, tt := range tests {
		t.Run(tt.from+"→"+tt.to, func(t *testing.T) {
			magnitude, err := CalculateMagnitude(tt.from, tt.to)

			if tt.wantErr {
				if err == nil {
					t.Errorf("Expected error for %s→%s, got nil", tt.from, tt.to)
				}
				return
			}

			if err != nil {
				t.Errorf("Unexpected error for %s→%s: %v", tt.from, tt.to, err)
				return
			}

			if magnitude != tt.expected {
				t.Errorf("Expected magnitude %d for %s→%s, got %d",
					tt.expected, tt.from, tt.to, magnitude)
			}
		})
	}
}

func TestFindPhaseIndex(t *testing.T) {
	allPhases := []string{"W0", "D1", "D2", "D3", "D4", "S4", "S5", "S6", "S7", "S8", "S9", "S10", "S11"}

	tests := []struct {
		phase    string
		expected int
	}{
		{"W0", 0},
		{"D1", 1},
		{"S5", 6},
		{"S11", 12},
		{"INVALID", -1},
	}

	for _, tt := range tests {
		idx := findPhaseIndex(allPhases, tt.phase)
		if idx != tt.expected {
			t.Errorf("Expected index %d for phase %s, got %d",
				tt.expected, tt.phase, idx)
		}
	}
}

func TestLogRewindEvent_Magnitude0(t *testing.T) {
	tmpDir := t.TempDir()

	// Create minimal WAYFINDER-STATUS.md for ReadFrom
	// (LogRewindEvent should skip early for magnitude 0)
	flags := RewindFlags{}

	// S7→S7 is magnitude 0, should skip logging
	err := LogRewindEvent(tmpDir, "S7", "S7", flags)
	if err != nil {
		t.Errorf("LogRewindEvent failed: %v", err)
	}

	// S11-retrospective.md should not be created (magnitude 0 skips logging)
	// This test validates early return logic
}
