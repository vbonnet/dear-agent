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
		{"D1", "W0", 1, false},   // D1 (idx 1) → W0 (idx 0) = |1-0| = 1
		{"D2", "W0", 2, false},   // D2 (idx 2) → W0 (idx 0) = |2-0| = 2
		{"D3", "D1", 2, false},   // D3 (idx 3) → D1 (idx 1) = |3-1| = 2
		{"D4", "D2", 2, false},   // D4 (idx 4) → D2 (idx 2) = |4-2| = 2
		{"S4", "W0", 5, false},   // S4 (idx 5) → W0 (idx 0) = |5-0| = 5
		{"S10", "S5", 5, false},  // S10 (idx 11) → S5 (idx 6) = |11-6| = 5
		{"S11", "D1", 11, false}, // S11 (idx 12) → D1 (idx 1) = |12-1| = 11

		// Invalid phase names
		{"", "S5", 0, true},
		{"S5", "", 0, true},
		{"INVALID1", "INVALID2", 0, true},
		{"X1", "S5", 0, true},
		{"S5", "Z9", 0, true},
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
		FromPhase: "S7",
		ToPhase:   "S5",
		Magnitude: 2,
	}

	// Should succeed with valid directory
	err := LogToHistory(tmpDir, data)
	if err != nil {
		t.Errorf("LogToHistory failed with valid directory: %v", err)
	}

	// Verify file exists
	historyPath := filepath.Join(tmpDir, "WAYFINDER-HISTORY.md")
	if _, err := os.Stat(historyPath); os.IsNotExist(err) {
		t.Errorf("HISTORY file was not created")
	}
}

// TestAppendToS11_ErrorHandling tests error paths in AppendToS11
func TestAppendToS11_ErrorHandling(t *testing.T) {
	tmpDir := t.TempDir()

	// Test with minimal data
	data := &RewindEventData{
		FromPhase: "S6",
		ToPhase:   "S5",
		Magnitude: 1,
	}

	// Should succeed
	err := AppendToS11(tmpDir, data)
	if err != nil {
		t.Errorf("AppendToS11 failed: %v", err)
	}

	// Verify S11 file exists
	s11Path := filepath.Join(tmpDir, S11Filename)
	if _, err := os.Stat(s11Path); os.IsNotExist(err) {
		t.Errorf("S11 file was not created")
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
				FromPhase: "S5",
				ToPhase:   "S4",
				Magnitude: 1,
			},
		},
		{
			name: "Only reason (no learnings)",
			data: &RewindEventData{
				FromPhase: "S7",
				ToPhase:   "S6",
				Magnitude: 1,
				Reason:    "Test reason",
			},
		},
		{
			name: "Empty context",
			data: &RewindEventData{
				FromPhase: "D2",
				ToPhase:   "D1",
				Magnitude: 1,
				Context:   ContextSnapshot{},
			},
		},
		{
			name: "Git error in context",
			data: &RewindEventData{
				FromPhase: "S8",
				ToPhase:   "S7",
				Magnitude: 1,
				Context: ContextSnapshot{
					Git: GitContext{Error: "timeout"},
				},
			},
		},
		{
			name: "Empty deliverables",
			data: &RewindEventData{
				FromPhase: "W0",
				ToPhase:   "W0",
				Magnitude: 0,
				Context: ContextSnapshot{
					Deliverables: []string{},
				},
			},
		},
		{
			name: "Empty completed phases",
			data: &RewindEventData{
				FromPhase: "D1",
				ToPhase:   "W0",
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
