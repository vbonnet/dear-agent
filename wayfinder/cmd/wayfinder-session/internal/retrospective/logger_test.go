package retrospective

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/vbonnet/dear-agent/wayfinder/cmd/wayfinder-session/internal/history"
)

func TestCalculateMagnitude(t *testing.T) {
	tests := []struct {
		from     string
		to       string
		expected int
		wantErr  bool
	}{
		// Same phase (magnitude 0)
		{"RETRO", "RETRO", 0, false},

		// Forward rewinds (moving backwards in time)
		{"RETRO", "BUILD", 1, false},   // RETRO (idx 8) → BUILD (idx 7) = |8-7| = 1
		{"RETRO", "SETUP", 2, false},   // RETRO (idx 8) → SETUP (idx 6) = |8-6| = 2
		{"RETRO", "SPEC", 4, false},    // RETRO (idx 8) → SPEC (idx 4) = |8-4| = 4
		{"RETRO", "CHARTER", 8, false}, // RETRO (idx 8) → CHARTER (idx 0) = |8-0| = 8

		// Edge cases
		{"CHARTER", "CHARTER", 0, false},
		{"RETRO", "RETRO", 0, false},

		// Unknown phases
		{"INVALID", "SETUP", 0, true},
		{"SETUP", "INVALID", 0, true},
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
	allPhases := []string{"CHARTER", "PROBLEM", "RESEARCH", "DESIGN", "SPEC", "PLAN", "SETUP", "BUILD", "RETRO"}

	tests := []struct {
		phase    string
		expected int
	}{
		{"CHARTER", 0},
		{"PROBLEM", 1},
		{"SETUP", 6},
		{"RETRO", 8},
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
	legacyPath := filepath.Join(tmpDir, history.LegacyHistoryFilename)
	if err := os.WriteFile(legacyPath, []byte("{\"event\":\"seed\"}\n"), 0o600); err != nil {
		t.Fatalf("seed legacy history: %v", err)
	}

	// Create minimal WAYFINDER-STATUS.md for ReadFrom
	// (LogRewindEvent should skip early for magnitude 0)
	flags := RewindFlags{}

	// RETRO→RETRO is magnitude 0, should skip logging
	err := LogRewindEvent(tmpDir, "RETRO", "RETRO", flags)
	if err != nil {
		t.Errorf("LogRewindEvent failed: %v", err)
	}

	// RETRO-retrospective.md should not be created (magnitude 0 skips logging)
	// This test validates early return logic
	if _, err := os.Stat(legacyPath); err != nil {
		t.Fatalf("no-op rewind migrated the legacy history file: %v", err)
	}
	if _, err := os.Stat(filepath.Join(tmpDir, history.HistoryFilename)); !os.IsNotExist(err) {
		t.Fatalf("no-op rewind created %s (stat err: %v)", history.HistoryFilename, err)
	}
}
