package retrospective

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestAppendToRetro(t *testing.T) {
	tmpDir := t.TempDir()

	// Create mock rewind event data
	data := &RewindEventData{
		FromPhase: "RETRO",
		ToPhase:   "SETUP",
		Magnitude: 2,
		Timestamp: time.Date(2026, 1, 7, 12, 0, 0, 0, time.UTC),
		Prompted:  true,
		Reason:    "Design was overcomplicated",
		Learnings: "Simpler approach is better",
		Context: ContextSnapshot{
			Git: GitContext{
				Branch:             "main",
				Commit:             "abc123",
				UncommittedChanges: true,
			},
			Deliverables: []string{"PROBLEM-problem.md", "BUILD-design.md"},
			PhaseState: PhaseContext{
				CurrentPhase:    "RETRO",
				CompletedPhases: []string{"CHARTER", "PROBLEM", "BUILD"},
				SessionID:       "test-session-123",
			},
		},
	}

	// Append to RETRO
	err := AppendToRetro(tmpDir, data)
	if err != nil {
		t.Fatalf("AppendToRetro failed: %v", err)
	}

	// Read RETRO file
	s11Path := filepath.Join(tmpDir, RetroFilename)
	content, err := os.ReadFile(s11Path)
	if err != nil {
		t.Fatalf("Failed to read RETRO file: %v", err)
	}

	contentStr := string(content)

	// Validate markdown structure
	if !strings.Contains(contentStr, "## Rewind: RETRO → SETUP (magnitude 2)") {
		t.Errorf("Missing rewind header in RETRO")
	}
	if !strings.Contains(contentStr, "**Reason**: Design was overcomplicated") {
		t.Errorf("Missing reason in RETRO")
	}
	if !strings.Contains(contentStr, "**Learnings**: Simpler approach is better") {
		t.Errorf("Missing learnings in RETRO")
	}
	if !strings.Contains(contentStr, "main@abc123") {
		t.Errorf("Missing git context in RETRO")
	}
	if !strings.Contains(contentStr, "uncommitted: yes") {
		t.Errorf("Missing uncommitted changes flag in RETRO")
	}
}

func TestAppendToRetro_Multiple(t *testing.T) {
	tmpDir := t.TempDir()

	// Create first rewind
	data1 := &RewindEventData{
		FromPhase: "RETRO",
		ToPhase:   "BUILD",
		Magnitude: 1,
		Timestamp: time.Now(),
		Reason:    "First rewind",
	}

	err := AppendToRetro(tmpDir, data1)
	if err != nil {
		t.Fatalf("First append failed: %v", err)
	}

	// Create second rewind
	data2 := &RewindEventData{
		FromPhase: "BUILD",
		ToPhase:   "SETUP",
		Magnitude: 3,
		Timestamp: time.Now(),
		Reason:    "Second rewind",
	}

	err = AppendToRetro(tmpDir, data2)
	if err != nil {
		t.Fatalf("Second append failed: %v", err)
	}

	// Read RETRO file
	s11Path := filepath.Join(tmpDir, RetroFilename)
	content, err := os.ReadFile(s11Path)
	if err != nil {
		t.Fatalf("Failed to read RETRO file: %v", err)
	}

	contentStr := string(content)

	// Both rewinds should be present
	if !strings.Contains(contentStr, "First rewind") {
		t.Errorf("First rewind missing in RETRO")
	}
	if !strings.Contains(contentStr, "Second rewind") {
		t.Errorf("Second rewind missing in RETRO")
	}
}

func TestFormatRewindEntry(t *testing.T) {
	data := &RewindEventData{
		FromPhase: "DESIGN",
		ToPhase:   "PROBLEM",
		Magnitude: 2,
		Timestamp: time.Date(2026, 1, 7, 12, 0, 0, 0, time.UTC),
		Reason:    "Test reason",
		Learnings: "Test learnings",
		Context: ContextSnapshot{
			Git: GitContext{
				Branch: "feature",
				Commit: "xyz789",
			},
			Deliverables: []string{"PROBLEM-problem.md"},
			PhaseState: PhaseContext{
				CompletedPhases: []string{"CHARTER", "PROBLEM"},
			},
		},
	}

	entry := formatRewindEntry(data)

	// Validate markdown format
	if !strings.Contains(entry, "## Rewind: DESIGN → PROBLEM (magnitude 2)") {
		t.Errorf("Missing header")
	}
	if !strings.Contains(entry, "2026-01-07T12:00:00Z") {
		t.Errorf("Missing timestamp")
	}
	if !strings.Contains(entry, "**Reason**: Test reason") {
		t.Errorf("Missing reason")
	}
	if !strings.Contains(entry, "**Learnings**: Test learnings") {
		t.Errorf("Missing learnings")
	}
	if !strings.Contains(entry, "feature@xyz789") {
		t.Errorf("Missing git context")
	}
}

func TestFormatRewindEntry_GitError(t *testing.T) {
	data := &RewindEventData{
		FromPhase: "SETUP",
		ToPhase:   "PLAN",
		Magnitude: 1,
		Timestamp: time.Now(),
		Reason:    "Test",
		Context: ContextSnapshot{
			Git: GitContext{
				Error: "timeout",
			},
		},
	}

	entry := formatRewindEntry(data)

	// Should handle git error gracefully
	if !strings.Contains(entry, "Git: _(error: timeout)_") {
		t.Errorf("Git error not formatted correctly")
	}
}
