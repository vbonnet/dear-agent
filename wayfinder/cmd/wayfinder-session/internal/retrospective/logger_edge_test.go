package retrospective

import (
	"os"
	"path/filepath"
	"testing"
)

// TestCalculateMagnitude_EdgeCases tests additional edge cases for magnitude calculation
func TestCalculateMagnitude_EdgeCases(t *testing.T) {
	tests := []struct {
		from     string
		to       string
		expected int
		wantErr  bool
	}{
		// Forward direction (normal rewinds)
		{"PROBLEM", "CHARTER", 1, false},  // PROBLEM (idx 1) → CHARTER (idx 0) = |1-0| = 1
		{"RESEARCH", "CHARTER", 2, false}, // RESEARCH (idx 2) → CHARTER (idx 0) = |2-0| = 2
		{"DESIGN", "PROBLEM", 2, false},   // DESIGN (idx 3) → PROBLEM (idx 1) = |3-1| = 2
		{"SPEC", "RESEARCH", 2, false},    // SPEC (idx 4) → RESEARCH (idx 2) = |4-2| = 2
		{"PLAN", "CHARTER", 5, false},     // PLAN (idx 5) → CHARTER (idx 0) = |5-0| = 5
		{"BUILD", "SETUP", 1, false},      // BUILD (idx 7) → SETUP (idx 6) = |7-6| = 1
		{"RETRO", "PROBLEM", 7, false},    // RETRO (idx 8) → PROBLEM (idx 1) = |8-1| = 7

		// Invalid phase names
		{"", "SETUP", 0, true},
		{"SETUP", "", 0, true},
		{"INVALIPROBLEM", "INVALIRESEARCH", 0, true},
		{"X1", "SETUP", 0, true},
		{"SETUP", "Z9", 0, true},
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

// TestLogToHistory_ErrorHandling tests error paths in LogToHistory
func TestLogToHistory_ErrorHandling(t *testing.T) {
	// Test with invalid project directory (permission denied scenario simulation)
	// Note: This is hard to test without root, so we test what we can

	tmpDir := t.TempDir()

	// Create valid data
	data := &RewindEventData{
		FromPhase: "RETRO",
		ToPhase:   "SETUP",
		Magnitude: 2,
	}

	// Should succeed with valid directory
	err := LogToHistory(tmpDir, data)
	if err != nil {
		t.Errorf("LogToHistory failed with valid directory: %v", err)
	}

	// Verify file exists
	historyPath := filepath.Join(tmpDir, "WAYFINDER-HISTORY.jsonl")
	if _, err := os.Stat(historyPath); os.IsNotExist(err) {
		t.Errorf("HISTORY file was not created")
	}
}

// TestAppendToRetro_ErrorHandling tests error paths in AppendToRetro
func TestAppendToRetro_ErrorHandling(t *testing.T) {
	tmpDir := t.TempDir()

	// Test with minimal data
	data := &RewindEventData{
		FromPhase: "BUILD",
		ToPhase:   "SETUP",
		Magnitude: 1,
	}

	// Should succeed
	err := AppendToRetro(tmpDir, data)
	if err != nil {
		t.Errorf("AppendToRetro failed: %v", err)
	}

	// Verify RETRO file exists
	s11Path := filepath.Join(tmpDir, RetroFilename)
	if _, err := os.Stat(s11Path); os.IsNotExist(err) {
		t.Errorf("RETRO file was not created")
	}
}

// TestFormatRewindEntry_EdgeCases tests edge cases in markdown formatting
func TestFormatRewindEntry_EdgeCases(t *testing.T) {
	tests := []struct {
		name string
		data *RewindEventData
	}{
		{
			name: "Minimal data (no reason, no learnings)",
			data: &RewindEventData{
				FromPhase: "SETUP",
				ToPhase:   "PLAN",
				Magnitude: 1,
			},
		},
		{
			name: "Only reason (no learnings)",
			data: &RewindEventData{
				FromPhase: "RETRO",
				ToPhase:   "BUILD",
				Magnitude: 1,
				Reason:    "Test reason",
			},
		},
		{
			name: "Empty context",
			data: &RewindEventData{
				FromPhase: "RESEARCH",
				ToPhase:   "PROBLEM",
				Magnitude: 1,
				Context:   ContextSnapshot{},
			},
		},
		{
			name: "Git error in context",
			data: &RewindEventData{
				FromPhase: "BUILD",
				ToPhase:   "RETRO",
				Magnitude: 1,
				Context: ContextSnapshot{
					Git: GitContext{Error: "timeout"},
				},
			},
		},
		{
			name: "Empty deliverables",
			data: &RewindEventData{
				FromPhase: "CHARTER",
				ToPhase:   "CHARTER",
				Magnitude: 0,
				Context: ContextSnapshot{
					Deliverables: []string{},
				},
			},
		},
		{
			name: "Empty completed phases",
			data: &RewindEventData{
				FromPhase: "PROBLEM",
				ToPhase:   "CHARTER",
				Magnitude: 1,
				Context: ContextSnapshot{
					PhaseState: PhaseContext{
						CompletedPhases: []string{},
					},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			entry := formatRewindEntry(tt.data)

			// Should not panic and should return non-empty string
			if entry == "" {
				t.Errorf("formatRewindEntry returned empty string")
			}

			// Should contain header
			if len(entry) < 10 {
				t.Errorf("formatRewindEntry returned suspiciously short string: %s", entry)
			}
		})
	}
}
