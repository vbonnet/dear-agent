package migrate

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/vbonnet/dear-agent/wayfinder/cmd/wayfinder-session/internal/status"
)

// TestMigrationIntegration tests end-to-end migration flow
func TestMigrationIntegration(t *testing.T) {
	// Create temporary directory
	tmpDir := t.TempDir()

	// Create a realistic V1 status file
	v1Status := createRealisticV1Status()

	// Write V1 status to file
	if err := v1Status.WriteTo(tmpDir); err != nil {
		t.Fatalf("failed to write V1 status: %v", err)
	}

	// Read it back to verify it was written correctly
	readV1, err := status.ReadFrom(tmpDir)
	if err != nil {
		t.Fatalf("failed to read V1 status: %v", err)
	}

	// Convert to V2
	opts := &ConvertOptions{
		ProjectName:       "Integration Test Project",
		ProjectType:       status.ProjectTypeFeature,
		RiskLevel:         status.RiskLevelM,
		PreserveSessionID: true,
	}

	v2Status, err := ConvertV1ToV2(readV1, opts)
	if err != nil {
		t.Fatalf("ConvertV1ToV2() error = %v", err)
	}

	// Validate V2 structure
	if v2Status.SchemaVersion != status.SchemaVersionV2 {
		t.Errorf("SchemaVersion = %q, want %q", v2Status.SchemaVersion, status.SchemaVersionV2)
	}

	if v2Status.ProjectName != opts.ProjectName {
		t.Errorf("ProjectName = %q, want %q", v2Status.ProjectName, opts.ProjectName)
	}

	if v2Status.ProjectType != opts.ProjectType {
		t.Errorf("ProjectType = %q, want %q", v2Status.ProjectType, opts.ProjectType)
	}

	if v2Status.RiskLevel != opts.RiskLevel {
		t.Errorf("RiskLevel = %q, want %q", v2Status.RiskLevel, opts.RiskLevel)
	}

	// Verify phase history was created
	if len(v2Status.WaypointHistory) == 0 {
		t.Error("expected phase history to be populated")
	}

	// Write V2 status to a different directory
	v2Dir := filepath.Join(tmpDir, "v2")
	if err := os.Mkdir(v2Dir, 0755); err != nil {
		t.Fatalf("failed to create v2 directory: %v", err)
	}

	if err := status.WriteV2ToDir(v2Status, v2Dir); err != nil {
		t.Fatalf("failed to write V2 status: %v", err)
	}

	// Read V2 status back and verify
	readV2, err := status.ParseV2FromDir(v2Dir)
	if err != nil {
		t.Fatalf("failed to read V2 status: %v", err)
	}

	// Verify V2 was round-tripped correctly
	if readV2.SchemaVersion != status.SchemaVersionV2 {
		t.Errorf("round-trip SchemaVersion = %q, want %q", readV2.SchemaVersion, status.SchemaVersionV2)
	}

	if readV2.ProjectName != v2Status.ProjectName {
		t.Errorf("round-trip ProjectName = %q, want %q", readV2.ProjectName, v2Status.ProjectName)
	}

	if len(readV2.WaypointHistory) != len(v2Status.WaypointHistory) {
		t.Errorf("round-trip PhaseHistory length = %d, want %d",
			len(readV2.WaypointHistory), len(v2Status.WaypointHistory))
	}
}

// TestMigrationWithAllPhases tests migration with all 13 V1 phases
func TestMigrationWithAllPhases(t *testing.T) {
	v1 := createCompleteV1Status()

	v2, err := ConvertV1ToV2(v1, nil)
	if err != nil {
		t.Fatalf("ConvertV1ToV2() error = %v", err)
	}

	// Verify all phases were converted
	// Should have 9 V2 phases (some V1 phases merge)
	expectedPhaseCount := 9
	if len(v2.WaypointHistory) != expectedPhaseCount {
		t.Errorf("PhaseHistory length = %d, want %d", len(v2.WaypointHistory), expectedPhaseCount)
	}

	// Verify phase names are correct
	expectedPhases := []string{
		status.PhaseV2Charter,
		status.PhaseV2Problem,
		status.PhaseV2Research,
		status.PhaseV2Design,
		status.PhaseV2Spec,
		status.PhaseV2Plan,
		status.PhaseV2Setup,
		status.PhaseV2Build,
		status.PhaseV2Retro,
	}

	for i, expectedPhase := range expectedPhases {
		if i >= len(v2.WaypointHistory) {
			t.Errorf("missing phase %s at index %d", expectedPhase, i)
			continue
		}
		if v2.WaypointHistory[i].Name != expectedPhase {
			t.Errorf("phase[%d] = %q, want %q", i, v2.WaypointHistory[i].Name, expectedPhase)
		}
	}

	// Verify merged phases have correct metadata
	for _, phase := range v2.WaypointHistory {
		if phase.Name == status.PhaseV2Spec {
			// Should have stakeholder data from merged S4
			if phase.StakeholderApproved == nil {
				t.Error("D4 should have stakeholder_approved set")
			}
		}

		if phase.Name == status.PhaseV2Plan {
			// Should have research notes from merged S5
			if phase.ResearchNotes == "" {
				t.Error("S6 should have research_notes set")
			}
		}

		if phase.Name == status.PhaseV2Build {
			// Should have validation and deployment status from merged S9/S10
			if phase.ValidationStatus == "" {
				t.Error("S8 should have validation_status set")
			}
			if phase.DeploymentStatus == "" {
				t.Error("S8 should have deployment_status set")
			}
		}
	}
}

// TestMigrationPreservesTimestamps verifies timestamps are preserved
func TestMigrationPreservesTimestamps(t *testing.T) {
	startTime := time.Date(2026, 2, 1, 10, 0, 0, 0, time.UTC)
	endTime := time.Date(2026, 2, 10, 18, 0, 0, 0, time.UTC)

	v1 := &status.Status{
		SchemaVersion: "1.0",
		SessionID:     "test-session",
		ProjectPath:   "/test/project",
		StartedAt:     startTime,
		EndedAt:       &endTime,
		Status:        status.StatusCompleted,
		CurrentPhase:  "S11",
		Phases: []status.Phase{
			{
				Name:        "W0",
				Status:      status.PhaseStatusCompleted,
				StartedAt:   &startTime,
				CompletedAt: timePtr(startTime.Add(1 * time.Hour)),
			},
		},
	}

	v2, err := ConvertV1ToV2(v1, nil)
	if err != nil {
		t.Fatalf("ConvertV1ToV2() error = %v", err)
	}

	// Verify created_at matches started_at
	if !v2.CreatedAt.Equal(startTime) {
		t.Errorf("CreatedAt = %v, want %v", v2.CreatedAt, startTime)
	}

	// Verify completion_date is preserved
	if v2.CompletionDate == nil {
		t.Error("CompletionDate should be set")
	} else if !v2.CompletionDate.Equal(endTime) {
		t.Errorf("CompletionDate = %v, want %v", v2.CompletionDate, endTime)
	}

	// Verify phase timestamps are preserved
	if len(v2.WaypointHistory) > 0 {
		phase := v2.WaypointHistory[0]
		if !phase.StartedAt.Equal(startTime) {
			t.Errorf("Phase StartedAt = %v, want %v", phase.StartedAt, startTime)
		}
		if phase.CompletedAt == nil {
			t.Error("Phase CompletedAt should be set")
		}
	}
}

// Helper functions

func createRealisticV1Status() *status.Status {
	now := time.Now()
	return &status.Status{
		SchemaVersion: "1.0",
		SessionID:     "realistic-session-123",
		ProjectPath:   "/tmp/test/projects/test-project",
		StartedAt:     now.Add(-48 * time.Hour),
		Status:        status.StatusInProgress,
		CurrentPhase:  "S8",
		Phases: []status.Phase{
			{
				Name:        "W0",
				Status:      status.PhaseStatusCompleted,
				StartedAt:   timePtr(now.Add(-48 * time.Hour)),
				CompletedAt: timePtr(now.Add(-46 * time.Hour)),
				Outcome:     status.OutcomeSuccess,
			},
			{
				Name:        "D1",
				Status:      status.PhaseStatusCompleted,
				StartedAt:   timePtr(now.Add(-46 * time.Hour)),
				CompletedAt: timePtr(now.Add(-44 * time.Hour)),
				Outcome:     status.OutcomeSuccess,
			},
			{
				Name:        "D2",
				Status:      status.PhaseStatusCompleted,
				StartedAt:   timePtr(now.Add(-44 * time.Hour)),
				CompletedAt: timePtr(now.Add(-42 * time.Hour)),
				Outcome:     status.OutcomeSuccess,
			},
			{
				Name:      "S8",
				Status:    status.PhaseStatusInProgress,
				StartedAt: timePtr(now.Add(-2 * time.Hour)),
			},
		},
	}
}

func createCompleteV1Status() *status.Status {
	now := time.Now()
	baseTime := now.Add(-100 * time.Hour)

	phases := []status.Phase{}
	allV1Phases := []string{"W0", "D1", "D2", "D3", "D4", "S4", "S5", "S6", "S7", "S8", "S9", "S10", "S11"}

	for i, phaseName := range allV1Phases {
		startTime := baseTime.Add(time.Duration(i*2) * time.Hour)
		endTime := startTime.Add(2 * time.Hour)

		phases = append(phases, status.Phase{
			Name:        phaseName,
			Status:      status.PhaseStatusCompleted,
			StartedAt:   &startTime,
			CompletedAt: &endTime,
			Outcome:     status.OutcomeSuccess,
		})
	}

	endTime := baseTime.Add(time.Duration(len(allV1Phases)*2) * time.Hour)

	return &status.Status{
		SchemaVersion: "1.0",
		SessionID:     "complete-session-123",
		ProjectPath:   "/tmp/test/projects/complete-project",
		StartedAt:     baseTime,
		EndedAt:       &endTime,
		Status:        status.StatusCompleted,
		CurrentPhase:  "S11",
		Phases:        phases,
	}
}
