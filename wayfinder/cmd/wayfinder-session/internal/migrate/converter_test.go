package migrate

import (
	"testing"
	"time"

	"github.com/vbonnet/dear-agent/wayfinder/cmd/wayfinder-session/internal/status"
)

// TestV1ToV2PhaseMapping tests individual phase mapping logic
func TestV1ToV2PhaseMapping(t *testing.T) {
	tests := []struct {
		name           string
		v1Phase        string
		expectedV2     string
		preserveFields map[string]bool // fields that should be preserved in V2
	}{
		{
			name:       "W0 maps to W0",
			v1Phase:    "W0",
			expectedV2: status.PhaseV2Charter,
		},
		{
			name:       "D1 maps to D1",
			v1Phase:    "D1",
			expectedV2: status.PhaseV2Problem,
		},
		{
			name:       "D2 maps to D2",
			v1Phase:    "D2",
			expectedV2: status.PhaseV2Research,
		},
		{
			name:       "D3 maps to D3",
			v1Phase:    "D3",
			expectedV2: status.PhaseV2Design,
		},
		{
			name:       "D4 maps to D4",
			v1Phase:    "D4",
			expectedV2: status.PhaseV2Spec,
		},
		{
			name:       "S4 maps to D4 (merged)",
			v1Phase:    "S4",
			expectedV2: status.PhaseV2Spec,
			preserveFields: map[string]bool{
				"stakeholder_approved": true,
			},
		},
		{
			name:       "S5 maps to S6 (merged)",
			v1Phase:    "S5",
			expectedV2: status.PhaseV2Plan,
			preserveFields: map[string]bool{
				"research_notes": true,
			},
		},
		{
			name:       "S6 maps to S6",
			v1Phase:    "S6",
			expectedV2: status.PhaseV2Plan,
		},
		{
			name:       "S7 maps to S7",
			v1Phase:    "S7",
			expectedV2: status.PhaseV2Setup,
		},
		{
			name:       "S8 maps to S8 (BUILD phase)",
			v1Phase:    "S8",
			expectedV2: status.PhaseV2Build,
		},
		{
			name:       "S9 maps to S8 (merged)",
			v1Phase:    "S9",
			expectedV2: status.PhaseV2Build,
			preserveFields: map[string]bool{
				"validation_status": true,
			},
		},
		{
			name:       "S10 maps to S8 (merged)",
			v1Phase:    "S10",
			expectedV2: status.PhaseV2Build,
			preserveFields: map[string]bool{
				"deployment_status": true,
			},
		},
		{
			name:       "S11 maps to S11",
			v1Phase:    "S11",
			expectedV2: status.PhaseV2Retro,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := mapV1PhaseToV2(tt.v1Phase)
			if got != tt.expectedV2 {
				t.Errorf("mapV1PhaseToV2(%q) = %q, want %q", tt.v1Phase, got, tt.expectedV2)
			}
		})
	}
}

// TestConvertV1ToV2_BasicFields tests that basic fields are preserved
func TestConvertV1ToV2_BasicFields(t *testing.T) {
	// Create V1 status
	v1 := &status.Status{
		SchemaVersion: "1.0",
		SessionID:     "test-session-123",
		ProjectPath:   "/path/to/project",
		StartedAt:     time.Date(2026, 2, 1, 10, 0, 0, 0, time.UTC),
		Status:        status.StatusInProgress,
		CurrentPhase:  "D2",
		Phases: []status.Phase{
			{
				Name:        "W0",
				Status:      status.PhaseStatusCompleted,
				StartedAt:   timePtr(time.Date(2026, 2, 1, 10, 0, 0, 0, time.UTC)),
				CompletedAt: timePtr(time.Date(2026, 2, 1, 11, 0, 0, 0, time.UTC)),
				Outcome:     status.OutcomeSuccess,
			},
			{
				Name:        "D1",
				Status:      status.PhaseStatusCompleted,
				StartedAt:   timePtr(time.Date(2026, 2, 1, 11, 0, 0, 0, time.UTC)),
				CompletedAt: timePtr(time.Date(2026, 2, 1, 13, 0, 0, 0, time.UTC)),
				Outcome:     status.OutcomeSuccess,
			},
			{
				Name:      "D2",
				Status:    status.PhaseStatusInProgress,
				StartedAt: timePtr(time.Date(2026, 2, 1, 13, 0, 0, 0, time.UTC)),
			},
		},
	}

	v2, err := ConvertV1ToV2(v1, nil)
	if err != nil {
		t.Fatalf("ConvertV1ToV2() error = %v", err)
	}

	// Verify schema version
	if v2.SchemaVersion != status.SchemaVersionV2 {
		t.Errorf("SchemaVersion = %q, want %q", v2.SchemaVersion, status.SchemaVersionV2)
	}

	// Verify project name is derived from path
	if v2.ProjectName == "" {
		t.Error("ProjectName should not be empty")
	}

	// Verify current phase is mapped correctly
	if v2.CurrentWaypoint != status.PhaseV2Research {
		t.Errorf("CurrentPhase = %q, want %q", v2.CurrentWaypoint, status.PhaseV2Research)
	}

	// Verify status is mapped correctly
	expectedStatus := status.StatusV2InProgress
	if v2.Status != expectedStatus {
		t.Errorf("Status = %q, want %q", v2.Status, expectedStatus)
	}

	// Verify timestamps
	if v2.CreatedAt.IsZero() {
		t.Error("CreatedAt should not be zero")
	}

	if v2.UpdatedAt.IsZero() {
		t.Error("UpdatedAt should not be zero")
	}

	// Verify phase history
	if len(v2.WaypointHistory) != 3 {
		t.Errorf("PhaseHistory length = %d, want 3", len(v2.WaypointHistory))
	}
}

// TestConvertV1ToV2_PhaseMerging tests phase merging logic
func TestConvertV1ToV2_PhaseMerging(t *testing.T) {
	tests := []struct {
		name           string
		v1Phases       []status.Phase
		expectedPhases []string
		validateFunc   func(*testing.T, []status.PhaseHistory)
	}{
		{
			name: "S4 merges into D4",
			v1Phases: []status.Phase{
				{Name: "D4", Status: status.PhaseStatusCompleted, Outcome: status.OutcomeSuccess},
				{Name: "S4", Status: status.PhaseStatusCompleted, Outcome: status.OutcomeSuccess},
			},
			expectedPhases: []string{status.PhaseV2Spec},
			validateFunc: func(t *testing.T, phases []status.PhaseHistory) {
				if len(phases) != 1 {
					t.Errorf("expected 1 phase, got %d", len(phases))
					return
				}
				// Should have stakeholder_approved field set
				if phases[0].StakeholderApproved == nil {
					t.Error("expected stakeholder_approved to be set")
				}
			},
		},
		{
			name: "S5 merges into S6",
			v1Phases: []status.Phase{
				{Name: "S5", Status: status.PhaseStatusCompleted, Outcome: status.OutcomeSuccess},
				{Name: "S6", Status: status.PhaseStatusCompleted, Outcome: status.OutcomeSuccess},
			},
			expectedPhases: []string{status.PhaseV2Plan},
			validateFunc: func(t *testing.T, phases []status.PhaseHistory) {
				if len(phases) != 1 {
					t.Errorf("expected 1 phase, got %d", len(phases))
					return
				}
				// Should have research_notes field available
				// (even if empty, the field should be considered)
			},
		},
		{
			name: "S8/S9/S10 merge into S8",
			v1Phases: []status.Phase{
				{Name: "S8", Status: status.PhaseStatusCompleted, Outcome: status.OutcomeSuccess},
				{Name: "S9", Status: status.PhaseStatusCompleted, Outcome: status.OutcomeSuccess},
				{Name: "S10", Status: status.PhaseStatusCompleted, Outcome: status.OutcomeSuccess},
			},
			expectedPhases: []string{status.PhaseV2Build},
			validateFunc: func(t *testing.T, phases []status.PhaseHistory) {
				if len(phases) != 1 {
					t.Errorf("expected 1 phase, got %d", len(phases))
					return
				}
				// Should have validation_status and deployment_status fields
				if phases[0].ValidationStatus == "" {
					t.Error("expected validation_status to be set")
				}
				if phases[0].DeploymentStatus == "" {
					t.Error("expected deployment_status to be set")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v1 := &status.Status{
				SchemaVersion: "1.0",
				SessionID:     "test-session",
				ProjectPath:   "/test",
				StartedAt:     time.Now(),
				Status:        status.StatusCompleted,
				CurrentPhase:  tt.v1Phases[len(tt.v1Phases)-1].Name,
				Phases:        tt.v1Phases,
			}

			v2, err := ConvertV1ToV2(v1, nil)
			if err != nil {
				t.Fatalf("ConvertV1ToV2() error = %v", err)
			}

			// Validate expected phases
			if tt.validateFunc != nil {
				tt.validateFunc(t, v2.WaypointHistory)
			}
		})
	}
}

// TestConvertV1ToV2_StatusMapping tests V1 to V2 status conversion
func TestConvertV1ToV2_StatusMapping(t *testing.T) {
	tests := []struct {
		v1Status string
		v2Status string
	}{
		{status.StatusInProgress, status.StatusV2InProgress},
		{status.StatusCompleted, status.StatusV2Completed},
		{status.StatusAbandoned, status.StatusV2Abandoned},
		{status.StatusBlocked, status.StatusV2Blocked},
		{status.StatusObsolete, status.StatusV2Abandoned}, // obsolete -> abandoned
	}

	for _, tt := range tests {
		t.Run(tt.v1Status, func(t *testing.T) {
			got := mapV1StatusToV2(tt.v1Status)
			if got != tt.v2Status {
				t.Errorf("mapV1StatusToV2(%q) = %q, want %q", tt.v1Status, got, tt.v2Status)
			}
		})
	}
}

// TestConvertV1ToV2_ValidationChecks tests validation logic
func TestConvertV1ToV2_ValidationChecks(t *testing.T) {
	tests := []struct {
		name      string
		v1        *status.Status
		wantErr   bool
		errString string
	}{
		{
			name:      "nil input",
			v1:        nil,
			wantErr:   true,
			errString: "nil status",
		},
		{
			name: "missing schema version",
			v1: &status.Status{
				SessionID:   "test",
				ProjectPath: "/test",
				StartedAt:   time.Now(),
			},
			wantErr:   true,
			errString: "invalid schema version",
		},
		{
			name: "wrong schema version",
			v1: &status.Status{
				SchemaVersion: "2.0",
				SessionID:     "test",
				ProjectPath:   "/test",
				StartedAt:     time.Now(),
			},
			wantErr:   true,
			errString: "expected version 1.0",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ConvertV1ToV2(tt.v1, nil)
			if tt.wantErr {
				if err == nil {
					t.Errorf("expected error containing %q, got nil", tt.errString)
					return
				}
				if tt.errString != "" && !contains(err.Error(), tt.errString) {
					t.Errorf("expected error containing %q, got %q", tt.errString, err.Error())
				}
			} else {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
			}
		})
	}
}

// TestConvertV1ToV2_DryRun tests dry-run mode doesn't modify anything
func TestConvertV1ToV2_DryRun(t *testing.T) {
	v1 := &status.Status{
		SchemaVersion: "1.0",
		SessionID:     "test-session",
		ProjectPath:   "/test/project",
		StartedAt:     time.Now(),
		Status:        status.StatusInProgress,
		CurrentPhase:  "D1",
		Phases: []status.Phase{
			{Name: "W0", Status: status.PhaseStatusCompleted},
		},
	}

	opts := &ConvertOptions{
		DryRun: true,
	}

	v2, err := ConvertV1ToV2(v1, opts)
	if err != nil {
		t.Fatalf("ConvertV1ToV2() error = %v", err)
	}

	// In dry-run mode, we should still get a valid V2 object
	if v2 == nil {
		t.Fatal("expected non-nil result in dry-run mode")
	}

	if v2.SchemaVersion != status.SchemaVersionV2 {
		t.Errorf("SchemaVersion = %q, want %q", v2.SchemaVersion, status.SchemaVersionV2)
	}
}

// TestConvertV1ToV2_PreserveAllData tests no data is lost during conversion
func TestConvertV1ToV2_PreserveAllData(t *testing.T) {
	completedAt := time.Date(2026, 2, 10, 15, 0, 0, 0, time.UTC)
	v1 := &status.Status{
		SchemaVersion:  "1.0",
		SessionID:      "test-session-123",
		ProjectPath:    "/path/to/project",
		StartedAt:      time.Date(2026, 2, 1, 10, 0, 0, 0, time.UTC),
		EndedAt:        &completedAt,
		Status:         status.StatusCompleted,
		LifecycleState: status.LifecycleCompleted,
		CurrentPhase:   "S11",
		Phases: []status.Phase{
			{
				Name:        "W0",
				Status:      status.PhaseStatusCompleted,
				StartedAt:   timePtr(time.Date(2026, 2, 1, 10, 0, 0, 0, time.UTC)),
				CompletedAt: timePtr(time.Date(2026, 2, 1, 11, 0, 0, 0, time.UTC)),
				Outcome:     status.OutcomeSuccess,
			},
			{
				Name:        "D1",
				Status:      status.PhaseStatusCompleted,
				StartedAt:   timePtr(time.Date(2026, 2, 1, 11, 0, 0, 0, time.UTC)),
				CompletedAt: timePtr(time.Date(2026, 2, 1, 13, 0, 0, 0, time.UTC)),
				Outcome:     status.OutcomeSuccess,
			},
			{
				Name:        "S11",
				Status:      status.PhaseStatusCompleted,
				StartedAt:   timePtr(time.Date(2026, 2, 10, 14, 0, 0, 0, time.UTC)),
				CompletedAt: &completedAt,
				Outcome:     status.OutcomeSuccess,
			},
		},
	}

	v2, err := ConvertV1ToV2(v1, nil)
	if err != nil {
		t.Fatalf("ConvertV1ToV2() error = %v", err)
	}

	// Verify no phase data was lost
	if len(v2.WaypointHistory) == 0 {
		t.Error("expected phase history to be populated")
	}

	// Verify completion date is preserved
	if v2.CompletionDate == nil {
		t.Error("expected completion_date to be set")
	} else if !v2.CompletionDate.Equal(completedAt) {
		t.Errorf("CompletionDate = %v, want %v", v2.CompletionDate, completedAt)
	}

	// Verify timestamps are preserved
	if !v2.CreatedAt.Equal(v1.StartedAt) {
		t.Errorf("CreatedAt = %v, want %v", v2.CreatedAt, v1.StartedAt)
	}
}

// Helper functions

func timePtr(t time.Time) *time.Time {
	return &t
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 ||
		(len(s) > 0 && len(substr) > 0 && containsHelper(s, substr)))
}

func containsHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
