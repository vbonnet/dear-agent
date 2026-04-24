package retrospective

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestAppendToS11(t *testing.T) {
	tmpDir := t.TempDir()

	// Create mock rewind event data
	data := &RewindEventData{
		FromPhase: "S7",
		ToPhase:   "S5",
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
			Deliverables: []string{"D1-problem.md", "S6-design.md"},
			PhaseState: PhaseContext{
				CurrentPhase:    "S7",
				CompletedPhases: []string{"W0", "D1", "S6"},
				SessionID:       "test-session-123",
			},
		},
	}

	// Append to S11
	err := AppendToS11(tmpDir, data)
	if err != nil {
		t.Fatalf("AppendToS11 failed: %v", err)
	}

	// Read S11 file
	s11Path := filepath.Join(tmpDir, S11Filename)
	content, err := os.ReadFile(s11Path)
	if err != nil {
		t.Fatalf("Failed to read S11 file: %v", err)
	}

	contentStr := string(content)

	// Validate markdown structure
	if !strings.Contains(contentStr, "## Rewind: S7 → S5 (magnitude 2)") {
		t.Errorf("Missing rewind header in S11")
	}
	if !strings.Contains(contentStr, "**Reason**: Design was overcomplicated") {
		t.Errorf("Missing reason in S11")
	}
	if !strings.Contains(contentStr, "**Learnings**: Simpler approach is better") {
		t.Errorf("Missing learnings in S11")
	}
	if !strings.Contains(contentStr, "main@abc123") {
		t.Errorf("Missing git context in S11")
	}
	if !strings.Contains(contentStr, "uncommitted: yes") {
		t.Errorf("Missing uncommitted changes flag in S11")
	}
}

func TestAppendToS11_Multiple(t *testing.T) {
	tmpDir := t.TempDir()

	// Create first rewind
	data1 := &RewindEventData{
		FromPhase: "S7",
		ToPhase:   "S6",
		Magnitude: 1,
		Timestamp: time.Now(),
		Reason:    "First rewind",
	}

	err := AppendToS11(tmpDir, data1)
	if err != nil {
		t.Fatalf("First append failed: %v", err)
	}

	// Create second rewind
	data2 := &RewindEventData{
		FromPhase: "S8",
		ToPhase:   "S5",
		Magnitude: 3,
		Timestamp: time.Now(),
		Reason:    "Second rewind",
	}

	err = AppendToS11(tmpDir, data2)
	if err != nil {
		t.Fatalf("Second append failed: %v", err)
	}

	// Read S11 file
	s11Path := filepath.Join(tmpDir, S11Filename)
	content, err := os.ReadFile(s11Path)
	if err != nil {
		t.Fatalf("Failed to read S11 file: %v", err)
	}

	contentStr := string(content)

	// Both rewinds should be present
	if !strings.Contains(contentStr, "First rewind") {
		t.Errorf("First rewind missing in S11")
	}
	if !strings.Contains(contentStr, "Second rewind") {
		t.Errorf("Second rewind missing in S11")
	}
}

func TestFormatRewindEntry(t *testing.T) {
	data := &RewindEventData{
		FromPhase: "D3",
		ToPhase:   "D1",
		Magnitude: 2,
		Timestamp: time.Date(2026, 1, 7, 12, 0, 0, 0, time.UTC),
		Reason:    "Test reason",
		Learnings: "Test learnings",
		Context: ContextSnapshot{
			Git: GitContext{
				Branch: "feature",
				Commit: "xyz789",
			},
			Deliverables: []string{"D1-problem.md"},
			PhaseState: PhaseContext{
				CompletedPhases: []string{"W0", "D1"},
			},
		},
	}

	entry := formatRewindEntry(data)

	// Validate markdown format
	if !strings.Contains(entry, "## Rewind: D3 → D1 (magnitude 2)") {
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
		FromPhase: "S5",
		ToPhase:   "S4",
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
