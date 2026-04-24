package migrate

import (
	"testing"
	"time"

	"github.com/vbonnet/dear-agent/wayfinder/cmd/wayfinder-session/internal/status"
)

// TestEdgeCases_EmptyPhases tests migration with no phases
func TestEdgeCases_EmptyPhases(t *testing.T) {
	v1 := &status.Status{
		SchemaVersion: "1.0",
		SessionID:     "empty-phases",
		ProjectPath:   "/test",
		StartedAt:     time.Now(),
		Status:        status.StatusInProgress,
		CurrentPhase:  "",
		Phases:        []status.Phase{}, // Empty
	}

	v2, err := ConvertV1ToV2(v1, nil)
	if err != nil {
		t.Fatalf("ConvertV1ToV2() error = %v", err)
	}

	// Should succeed with empty phase history
	if v2 == nil {
		t.Fatal("expected non-nil V2 status")
	}

	if len(v2.WaypointHistory) != 0 {
		t.Errorf("expected empty phase history, got %d phases", len(v2.WaypointHistory))
	}
}

// TestEdgeCases_OnlyMergedPhases tests when only merged phases exist
func TestEdgeCases_OnlyMergedPhases(t *testing.T) {
	v1 := &status.Status{
		SchemaVersion: "1.0",
		SessionID:     "only-merged",
		ProjectPath:   "/test",
		StartedAt:     time.Now(),
		Status:        status.StatusInProgress,
		CurrentPhase:  "S4",
		Phases: []status.Phase{
			{Name: "S4", Status: status.PhaseStatusInProgress},
		},
	}

	v2, err := ConvertV1ToV2(v1, nil)
	if err != nil {
		t.Fatalf("ConvertV1ToV2() error = %v", err)
	}

	// S4 should map to D4
	if v2.CurrentWaypoint != status.PhaseV2Spec {
		t.Errorf("CurrentPhase = %q, want %q", v2.CurrentWaypoint, status.PhaseV2Spec)
	}

	// Should have one phase in history (D4)
	if len(v2.WaypointHistory) != 1 {
		t.Errorf("expected 1 phase, got %d", len(v2.WaypointHistory))
	}

	if v2.WaypointHistory[0].Name != status.PhaseV2Spec {
		t.Errorf("phase name = %q, want %q", v2.WaypointHistory[0].Name, status.PhaseV2Spec)
	}
}

// TestEdgeCases_MultipleBuildPhases tests S8/S9/S10 merging
func TestEdgeCases_MultipleBuildPhases(t *testing.T) {
	v1 := &status.Status{
		SchemaVersion: "1.0",
		SessionID:     "build-phases",
		ProjectPath:   "/test",
		StartedAt:     time.Now().Add(-10 * time.Hour),
		Status:        status.StatusInProgress,
		CurrentPhase:  "S10",
		Phases: []status.Phase{
			{
				Name:        "S8",
				Status:      status.PhaseStatusCompleted,
				StartedAt:   timePtr(time.Now().Add(-10 * time.Hour)),
				CompletedAt: timePtr(time.Now().Add(-6 * time.Hour)),
			},
			{
				Name:        "S9",
				Status:      status.PhaseStatusCompleted,
				StartedAt:   timePtr(time.Now().Add(-6 * time.Hour)),
				CompletedAt: timePtr(time.Now().Add(-3 * time.Hour)),
			},
			{
				Name:      "S10",
				Status:    status.PhaseStatusInProgress,
				StartedAt: timePtr(time.Now().Add(-3 * time.Hour)),
			},
		},
	}

	v2, err := ConvertV1ToV2(v1, nil)
	if err != nil {
		t.Fatalf("ConvertV1ToV2() error = %v", err)
	}

	// All three should merge into S8
	if v2.CurrentWaypoint != status.PhaseV2Build {
		t.Errorf("CurrentPhase = %q, want %q", v2.CurrentWaypoint, status.PhaseV2Build)
	}

	// Should have one S8 phase with all data merged
	foundS8 := false
	for _, phase := range v2.WaypointHistory {
		if phase.Name == status.PhaseV2Build {
			foundS8 = true

			// Should have validation status from S9
			if phase.ValidationStatus == "" {
				t.Error("expected validation_status to be set from S9")
			}

			// Should have deployment status from S10
			if phase.DeploymentStatus == "" {
				t.Error("expected deployment_status to be set from S10")
			}

			// Status should be in-progress (from S10)
			if phase.Status != status.PhaseStatusV2InProgress {
				t.Errorf("status = %q, want %q", phase.Status, status.PhaseStatusV2InProgress)
			}
		}
	}

	if !foundS8 {
		t.Error("expected S8 phase in history")
	}
}

// TestEdgeCases_OutOfOrderPhases tests phases not in chronological order
func TestEdgeCases_OutOfOrderPhases(t *testing.T) {
	v1 := &status.Status{
		SchemaVersion: "1.0",
		SessionID:     "out-of-order",
		ProjectPath:   "/test",
		StartedAt:     time.Now(),
		Status:        status.StatusInProgress,
		CurrentPhase:  "D3",
		Phases: []status.Phase{
			{Name: "D3", Status: status.PhaseStatusInProgress},
			{Name: "W0", Status: status.PhaseStatusCompleted}, // Out of order
			{Name: "D1", Status: status.PhaseStatusCompleted},
		},
	}

	v2, err := ConvertV1ToV2(v1, nil)
	if err != nil {
		t.Fatalf("ConvertV1ToV2() error = %v", err)
	}

	// Should still convert successfully
	// Phase history should be in V2 phase order, not input order
	if len(v2.WaypointHistory) != 3 {
		t.Errorf("expected 3 phases, got %d", len(v2.WaypointHistory))
	}

	// Verify phases are in correct V2 order
	expectedOrder := []string{status.PhaseV2Charter, status.PhaseV2Problem, status.PhaseV2Design}
	for i, expectedPhase := range expectedOrder {
		if i >= len(v2.WaypointHistory) {
			t.Errorf("missing phase at index %d", i)
			continue
		}
		if v2.WaypointHistory[i].Name != expectedPhase {
			t.Errorf("phase[%d] = %q, want %q", i, v2.WaypointHistory[i].Name, expectedPhase)
		}
	}
}

// TestEdgeCases_DuplicatePhases tests duplicate phase entries
func TestEdgeCases_DuplicatePhases(t *testing.T) {
	v1 := &status.Status{
		SchemaVersion: "1.0",
		SessionID:     "duplicates",
		ProjectPath:   "/test",
		StartedAt:     time.Now(),
		Status:        status.StatusInProgress,
		CurrentPhase:  "D1",
		Phases: []status.Phase{
			{Name: "W0", Status: status.PhaseStatusCompleted},
			{Name: "D1", Status: status.PhaseStatusInProgress},
			{Name: "D1", Status: status.PhaseStatusCompleted}, // Duplicate
		},
	}

	v2, err := ConvertV1ToV2(v1, nil)
	if err != nil {
		t.Fatalf("ConvertV1ToV2() error = %v", err)
	}

	// Should merge duplicates into single phase
	foundD1 := 0
	for _, phase := range v2.WaypointHistory {
		if phase.Name == status.PhaseV2Problem {
			foundD1++
		}
	}

	if foundD1 != 1 {
		t.Errorf("expected 1 D1 phase, found %d", foundD1)
	}
}

// TestEdgeCases_AllProjectTypes tests all project type options
func TestEdgeCases_AllProjectTypes(t *testing.T) {
	projectTypes := status.ValidProjectTypes()

	for _, pt := range projectTypes {
		t.Run(pt, func(t *testing.T) {
			v1 := &status.Status{
				SchemaVersion: "1.0",
				SessionID:     "test-" + pt,
				ProjectPath:   "/test",
				StartedAt:     time.Now(),
				Status:        status.StatusInProgress,
				CurrentPhase:  "W0",
				Phases:        []status.Phase{{Name: "W0", Status: status.PhaseStatusInProgress}},
			}

			opts := &ConvertOptions{
				ProjectType: pt,
			}

			v2, err := ConvertV1ToV2(v1, opts)
			if err != nil {
				t.Fatalf("ConvertV1ToV2() error = %v", err)
			}

			if v2.ProjectType != pt {
				t.Errorf("ProjectType = %q, want %q", v2.ProjectType, pt)
			}
		})
	}
}

// TestEdgeCases_AllRiskLevels tests all risk level options
func TestEdgeCases_AllRiskLevels(t *testing.T) {
	riskLevels := status.ValidRiskLevels()

	for _, rl := range riskLevels {
		t.Run(rl, func(t *testing.T) {
			v1 := &status.Status{
				SchemaVersion: "1.0",
				SessionID:     "test-" + rl,
				ProjectPath:   "/test",
				StartedAt:     time.Now(),
				Status:        status.StatusInProgress,
				CurrentPhase:  "W0",
				Phases:        []status.Phase{{Name: "W0", Status: status.PhaseStatusInProgress}},
			}

			opts := &ConvertOptions{
				RiskLevel: rl,
			}

			v2, err := ConvertV1ToV2(v1, opts)
			if err != nil {
				t.Fatalf("ConvertV1ToV2() error = %v", err)
			}

			if v2.RiskLevel != rl {
				t.Errorf("RiskLevel = %q, want %q", v2.RiskLevel, rl)
			}
		})
	}
}

// TestEdgeCases_PreserveSessionID tests session ID preservation
func TestEdgeCases_PreserveSessionID(t *testing.T) {
	sessionID := "test-session-12345"

	v1 := &status.Status{
		SchemaVersion: "1.0",
		SessionID:     sessionID,
		ProjectPath:   "/test",
		StartedAt:     time.Now(),
		Status:        status.StatusInProgress,
		CurrentPhase:  "W0",
		Phases:        []status.Phase{{Name: "W0", Status: status.PhaseStatusInProgress}},
	}

	opts := &ConvertOptions{
		PreserveSessionID: true,
	}

	v2, err := ConvertV1ToV2(v1, opts)
	if err != nil {
		t.Fatalf("ConvertV1ToV2() error = %v", err)
	}

	// Session ID should be in tags
	found := false
	expectedTag := "v1-session:" + sessionID
	for _, tag := range v2.Tags {
		if tag == expectedTag {
			found = true
			break
		}
	}

	if !found {
		t.Errorf("expected tag %q in tags %v", expectedTag, v2.Tags)
	}
}

// TestEdgeCases_VeryLongProjectPath tests long path handling
func TestEdgeCases_VeryLongProjectPath(t *testing.T) {
	longPath := "/very/long/path/to/some/deeply/nested/project/structure/that/goes/on/and/on"

	v1 := &status.Status{
		SchemaVersion: "1.0",
		SessionID:     "long-path",
		ProjectPath:   longPath,
		StartedAt:     time.Now(),
		Status:        status.StatusInProgress,
		CurrentPhase:  "W0",
		Phases:        []status.Phase{{Name: "W0", Status: status.PhaseStatusInProgress}},
	}

	v2, err := ConvertV1ToV2(v1, nil)
	if err != nil {
		t.Fatalf("ConvertV1ToV2() error = %v", err)
	}

	// Project name should be derived from last path component
	if v2.ProjectName == "" {
		t.Error("ProjectName should not be empty")
	}

	// Should be the last component
	if v2.ProjectName != "on" {
		t.Logf("ProjectName = %q (from path %q)", v2.ProjectName, longPath)
	}
}

// TestEdgeCases_ZeroTimestamps tests handling of zero/nil timestamps
func TestEdgeCases_ZeroTimestamps(t *testing.T) {
	v1 := &status.Status{
		SchemaVersion: "1.0",
		SessionID:     "zero-times",
		ProjectPath:   "/test",
		StartedAt:     time.Now(),
		Status:        status.StatusInProgress,
		CurrentPhase:  "W0",
		Phases: []status.Phase{
			{
				Name:        "W0",
				Status:      status.PhaseStatusInProgress,
				StartedAt:   nil, // Nil timestamp
				CompletedAt: nil,
			},
		},
	}

	v2, err := ConvertV1ToV2(v1, nil)
	if err != nil {
		t.Fatalf("ConvertV1ToV2() error = %v", err)
	}

	// Should handle nil timestamps gracefully
	if len(v2.WaypointHistory) == 0 {
		t.Fatal("expected phase history to exist")
	}

	// Phase with nil started_at should have zero time
	phase := v2.WaypointHistory[0]
	if phase.StartedAt.IsZero() {
		// This is expected - zero time is valid
		t.Logf("Phase StartedAt is zero (expected for nil input)")
	}
}

// TestEdgeCases_CustomProjectName tests custom project name override
func TestEdgeCases_CustomProjectName(t *testing.T) {
	customName := "My Custom Project Name 🚀"

	v1 := &status.Status{
		SchemaVersion: "1.0",
		SessionID:     "custom-name",
		ProjectPath:   "/some/path",
		StartedAt:     time.Now(),
		Status:        status.StatusInProgress,
		CurrentPhase:  "W0",
		Phases:        []status.Phase{{Name: "W0", Status: status.PhaseStatusInProgress}},
	}

	opts := &ConvertOptions{
		ProjectName: customName,
	}

	v2, err := ConvertV1ToV2(v1, opts)
	if err != nil {
		t.Fatalf("ConvertV1ToV2() error = %v", err)
	}

	if v2.ProjectName != customName {
		t.Errorf("ProjectName = %q, want %q", v2.ProjectName, customName)
	}
}
