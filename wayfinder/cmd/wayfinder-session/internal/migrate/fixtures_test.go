package migrate

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/vbonnet/dear-agent/wayfinder/cmd/wayfinder-session/internal/status"
)

// TestWithRealV1File tests migration with a real V1 WAYFINDER-STATUS.md file
func TestWithRealV1File(t *testing.T) {
	// Create temp directory
	tmpDir := t.TempDir()

	// Write a realistic V1 file
	v1Content := `---
schema_version: "1.0"
session_id: test-session-123
project_path: /tmp/test/projects/test-project
started_at: 2026-02-01T10:00:00Z
status: in_progress
current_phase: S8
phases:
  - name: W0
    status: completed
    started_at: 2026-02-01T10:00:00Z
    completed_at: 2026-02-01T11:00:00Z
    outcome: success
  - name: D1
    status: completed
    started_at: 2026-02-01T11:00:00Z
    completed_at: 2026-02-01T13:00:00Z
    outcome: success
  - name: D2
    status: completed
    started_at: 2026-02-01T13:00:00Z
    completed_at: 2026-02-01T15:00:00Z
    outcome: success
  - name: D3
    status: completed
    started_at: 2026-02-01T15:00:00Z
    completed_at: 2026-02-01T17:00:00Z
    outcome: success
  - name: D4
    status: completed
    started_at: 2026-02-01T17:00:00Z
    completed_at: 2026-02-01T18:00:00Z
    outcome: success
  - name: S4
    status: completed
    started_at: 2026-02-01T18:00:00Z
    completed_at: 2026-02-01T19:00:00Z
    outcome: success
  - name: S6
    status: completed
    started_at: 2026-02-02T09:00:00Z
    completed_at: 2026-02-02T12:00:00Z
    outcome: success
  - name: S8
    status: in_progress
    started_at: 2026-02-03T10:00:00Z
---

# Wayfinder Session

Test project in progress.
`

	statusFile := filepath.Join(tmpDir, "WAYFINDER-STATUS.md")
	if err := os.WriteFile(statusFile, []byte(v1Content), 0644); err != nil {
		t.Fatalf("failed to write V1 file: %v", err)
	}

	// Read V1 status
	v1Status, err := status.ReadFrom(tmpDir)
	if err != nil {
		t.Fatalf("failed to read V1 status: %v", err)
	}

	// Convert to V2
	opts := &ConvertOptions{
		ProjectName: "Test Project",
		ProjectType: status.ProjectTypeFeature,
		RiskLevel:   status.RiskLevelM,
	}

	v2Status, err := ConvertV1ToV2(v1Status, opts)
	if err != nil {
		t.Fatalf("ConvertV1ToV2() error = %v", err)
	}

	// Validate conversion
	if v2Status.SchemaVersion != status.SchemaVersionV2 {
		t.Errorf("SchemaVersion = %q, want %q", v2Status.SchemaVersion, status.SchemaVersionV2)
	}

	if v2Status.CurrentWaypoint != status.PhaseV2Build {
		t.Errorf("CurrentPhase = %q, want %q", v2Status.CurrentWaypoint, status.PhaseV2Build)
	}

	// Should have merged S4 into D4
	foundD4 := false
	for _, phase := range v2Status.WaypointHistory {
		if phase.Name == status.PhaseV2Spec {
			foundD4 = true
			if phase.StakeholderApproved == nil {
				t.Error("D4 should have stakeholder_approved set from S4 merge")
			}
		}
	}

	if !foundD4 {
		t.Error("expected D4 phase in history")
	}
}

// TestMigrationWithCompletedProject tests migration of a completed project
func TestMigrationWithCompletedProject(t *testing.T) {
	endTime := time.Date(2026, 2, 10, 18, 0, 0, 0, time.UTC)

	v1 := &status.Status{
		SchemaVersion: "1.0",
		SessionID:     "completed-session",
		ProjectPath:   "/tmp/test/projects/completed",
		StartedAt:     time.Date(2026, 2, 1, 10, 0, 0, 0, time.UTC),
		EndedAt:       &endTime,
		Status:        status.StatusCompleted,
		CurrentPhase:  "S11",
		Phases:        createAllCompletedPhases(),
	}

	v2, err := ConvertV1ToV2(v1, nil)
	if err != nil {
		t.Fatalf("ConvertV1ToV2() error = %v", err)
	}

	// Verify completion status
	if v2.Status != status.StatusV2Completed {
		t.Errorf("Status = %q, want %q", v2.Status, status.StatusV2Completed)
	}

	if v2.CompletionDate == nil {
		t.Error("CompletionDate should be set for completed project")
	} else if !v2.CompletionDate.Equal(endTime) {
		t.Errorf("CompletionDate = %v, want %v", v2.CompletionDate, endTime)
	}

	if v2.CurrentWaypoint != status.PhaseV2Retro {
		t.Errorf("CurrentPhase = %q, want %q", v2.CurrentWaypoint, status.PhaseV2Retro)
	}

	// All phase history entries should be completed
	for _, phase := range v2.WaypointHistory {
		if phase.Status != status.PhaseStatusV2Completed {
			t.Errorf("phase %s status = %q, want %q",
				phase.Name, phase.Status, status.PhaseStatusV2Completed)
		}
		if phase.CompletedAt == nil {
			t.Errorf("phase %s should have completed_at set", phase.Name)
		}
	}
}

// TestMigrationWithBlockedProject tests migration of a blocked project
func TestMigrationWithBlockedProject(t *testing.T) {
	v1 := &status.Status{
		SchemaVersion: "1.0",
		SessionID:     "blocked-session",
		ProjectPath:   "/tmp/test/projects/blocked",
		StartedAt:     time.Now().Add(-24 * time.Hour),
		Status:        status.StatusBlocked,
		CurrentPhase:  "D3",
		Phases: []status.Phase{
			{Name: "W0", Status: status.PhaseStatusCompleted},
			{Name: "D1", Status: status.PhaseStatusCompleted},
			{Name: "D2", Status: status.PhaseStatusCompleted},
			{Name: "D3", Status: status.PhaseStatusInProgress},
		},
	}

	v2, err := ConvertV1ToV2(v1, nil)
	if err != nil {
		t.Fatalf("ConvertV1ToV2() error = %v", err)
	}

	if v2.Status != status.StatusV2Blocked {
		t.Errorf("Status = %q, want %q", v2.Status, status.StatusV2Blocked)
	}

	if v2.CurrentWaypoint != status.PhaseV2Design {
		t.Errorf("CurrentPhase = %q, want %q", v2.CurrentWaypoint, status.PhaseV2Design)
	}
}

// TestMigrationWithSkippedPhases tests migration with skipped phases
func TestMigrationWithSkippedPhases(t *testing.T) {
	v1 := &status.Status{
		SchemaVersion: "1.0",
		SessionID:     "skipped-session",
		ProjectPath:   "/tmp/test/projects/skipped",
		StartedAt:     time.Now().Add(-48 * time.Hour),
		Status:        status.StatusInProgress,
		CurrentPhase:  "S8",
		Phases: []status.Phase{
			{Name: "W0", Status: status.PhaseStatusCompleted},
			{Name: "D1", Status: status.PhaseStatusCompleted},
			{Name: "D2", Status: status.PhaseStatusSkipped}, // Skipped
			{Name: "D3", Status: status.PhaseStatusCompleted},
			{Name: "S8", Status: status.PhaseStatusInProgress},
		},
	}

	v2, err := ConvertV1ToV2(v1, nil)
	if err != nil {
		t.Fatalf("ConvertV1ToV2() error = %v", err)
	}

	// Find D2 in phase history
	foundD2 := false
	for _, phase := range v2.WaypointHistory {
		if phase.Name == status.PhaseV2Research {
			foundD2 = true
			if phase.Status != status.PhaseStatusV2Skipped {
				t.Errorf("D2 status = %q, want %q", phase.Status, status.PhaseStatusV2Skipped)
			}
		}
	}

	if !foundD2 {
		t.Error("expected D2 phase in history")
	}
}

// TestMigrationWithMinimalV1 tests migration with minimal V1 data
func TestMigrationWithMinimalV1(t *testing.T) {
	v1 := &status.Status{
		SchemaVersion: "1.0",
		SessionID:     "minimal-session",
		ProjectPath:   "/test",
		StartedAt:     time.Now(),
		Status:        status.StatusInProgress,
		CurrentPhase:  "W0",
		Phases: []status.Phase{
			{Name: "W0", Status: status.PhaseStatusInProgress},
		},
	}

	v2, err := ConvertV1ToV2(v1, nil)
	if err != nil {
		t.Fatalf("ConvertV1ToV2() error = %v", err)
	}

	// Should have minimal but valid V2 structure
	if v2.SchemaVersion != status.SchemaVersionV2 {
		t.Errorf("SchemaVersion = %q, want %q", v2.SchemaVersion, status.SchemaVersionV2)
	}

	if v2.ProjectName == "" {
		t.Error("ProjectName should be derived from path")
	}

	if len(v2.WaypointHistory) == 0 {
		t.Error("expected at least one phase in history")
	}
}

// Helper functions

func createAllCompletedPhases() []status.Phase {
	now := time.Now()
	baseTime := now.Add(-100 * time.Hour)

	allPhases := []string{"W0", "D1", "D2", "D3", "D4", "S4", "S5", "S6", "S7", "S8", "S9", "S10", "S11"}
	phases := make([]status.Phase, len(allPhases))

	for i, name := range allPhases {
		startTime := baseTime.Add(time.Duration(i*2) * time.Hour)
		endTime := startTime.Add(2 * time.Hour)

		phases[i] = status.Phase{
			Name:        name,
			Status:      status.PhaseStatusCompleted,
			StartedAt:   &startTime,
			CompletedAt: &endTime,
			Outcome:     status.OutcomeSuccess,
		}
	}

	return phases
}
