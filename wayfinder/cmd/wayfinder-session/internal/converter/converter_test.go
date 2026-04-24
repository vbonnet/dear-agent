package converter

import (
	"testing"
	"time"

	"github.com/vbonnet/dear-agent/wayfinder/cmd/wayfinder-session/internal/status"
)

func TestConvertV1ToV2_Success(t *testing.T) {
	now := time.Now()
	ended := now.Add(2 * time.Hour)

	v1 := &status.Status{
		SessionID:    "test-session-123",
		ProjectPath:  "/tmp/test/src/ws/oss/wf/test-project",
		StartedAt:    now,
		EndedAt:      &ended,
		Status:       status.StatusInProgress,
		CurrentPhase: "S8",
		Phases: []status.Phase{
			{
				Name:        "W0",
				Status:      status.PhaseStatusCompleted,
				StartedAt:   &now,
				CompletedAt: &ended,
				Outcome:     status.OutcomeSuccess,
			},
			{
				Name:      "D1",
				Status:    status.PhaseStatusInProgress,
				StartedAt: &now,
			},
		},
	}

	v2, err := ConvertV1ToV2(v1)
	if err != nil {
		t.Fatalf("ConvertV1ToV2() error = %v", err)
	}

	// Verify required fields
	if v2.SchemaVersion != status.SchemaVersionV2 {
		t.Errorf("SchemaVersion = %v, want %v", v2.SchemaVersion, status.SchemaVersionV2)
	}

	if v2.ProjectName != "test-project" {
		t.Errorf("ProjectName = %v, want %v", v2.ProjectName, "test-project")
	}

	if v2.CurrentWaypoint != status.PhaseV2Build {
		t.Errorf("CurrentPhase = %v, want %v", v2.CurrentWaypoint, status.PhaseV2Build)
	}

	if v2.Status != status.StatusV2InProgress {
		t.Errorf("Status = %v, want %v", v2.Status, status.StatusV2InProgress)
	}

	// Verify timestamps
	if !v2.CreatedAt.Equal(now) {
		t.Errorf("CreatedAt = %v, want %v", v2.CreatedAt, now)
	}

	if v2.CompletionDate == nil || !v2.CompletionDate.Equal(ended) {
		t.Errorf("CompletionDate = %v, want %v", v2.CompletionDate, ended)
	}

	// Verify phase history conversion
	if len(v2.WaypointHistory) != 2 {
		t.Fatalf("PhaseHistory length = %v, want 2", len(v2.WaypointHistory))
	}

	// First phase (W0 -> W0, unchanged per SPEC.md)
	if v2.WaypointHistory[0].Name != status.PhaseV2Charter {
		t.Errorf("PhaseHistory[0].Name = %v, want %v", v2.WaypointHistory[0].Name, status.PhaseV2Charter)
	}

	// Second phase (D1 -> D1)
	if v2.WaypointHistory[1].Name != status.PhaseV2Problem {
		t.Errorf("PhaseHistory[1].Name = %v, want %v", v2.WaypointHistory[1].Name, status.PhaseV2Problem)
	}
}

func TestConvertV1ToV2_NilInput(t *testing.T) {
	_, err := ConvertV1ToV2(nil)
	if err == nil {
		t.Error("ConvertV1ToV2(nil) expected error, got nil")
	}
}

func TestConvertV1ToV2_EmptyProject(t *testing.T) {
	now := time.Now()
	v1 := &status.Status{
		SessionID:    "test-123",
		ProjectPath:  "",
		StartedAt:    now,
		Status:       status.StatusInProgress,
		CurrentPhase: "D1",
		Phases:       []status.Phase{},
	}

	v2, err := ConvertV1ToV2(v1)
	if err != nil {
		t.Fatalf("ConvertV1ToV2() error = %v", err)
	}

	if v2.ProjectName != "Unknown Project" {
		t.Errorf("ProjectName = %v, want 'Unknown Project'", v2.ProjectName)
	}
}

func TestExtractProjectName(t *testing.T) {
	tests := []struct {
		name     string
		path     string
		expected string
	}{
		{
			name:     "unix path",
			path:     "/tmp/test/src/ws/oss/wf/my-project",
			expected: "my-project",
		},
		{
			name:     "windows path",
			path:     "C:\\Users\\user\\src\\ws\\oss\\wf\\my-project",
			expected: "my-project",
		},
		{
			name:     "simple name",
			path:     "my-project",
			expected: "my-project",
		},
		{
			name:     "empty path",
			path:     "",
			expected: "Unknown Project",
		},
		{
			name:     "trailing slash",
			path:     "/tmp/test/src/ws/oss/wf/my-project/",
			expected: "my-project",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractProjectName(tt.path)
			if result != tt.expected {
				t.Errorf("extractProjectName(%q) = %q, want %q", tt.path, result, tt.expected)
			}
		})
	}
}

func TestInferRiskLevel(t *testing.T) {
	tests := []struct {
		name         string
		phaseCount   int
		expectedRisk string
	}{
		{
			name:         "small project (3 phases)",
			phaseCount:   3,
			expectedRisk: status.RiskLevelS,
		},
		{
			name:         "medium project (6 phases)",
			phaseCount:   6,
			expectedRisk: status.RiskLevelM,
		},
		{
			name:         "large project (9 phases)",
			phaseCount:   9,
			expectedRisk: status.RiskLevelL,
		},
		{
			name:         "xl project (13 phases)",
			phaseCount:   13,
			expectedRisk: status.RiskLevelXL,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v1 := &status.Status{
				Phases: make([]status.Phase, tt.phaseCount),
			}
			result := inferRiskLevel(v1)
			if result != tt.expectedRisk {
				t.Errorf("inferRiskLevel() = %v, want %v", result, tt.expectedRisk)
			}
		})
	}
}

func TestConvertPhase(t *testing.T) {
	tests := []struct {
		v1Phase  string
		expected string
	}{
		{"W0", status.PhaseV2Charter}, // W0 → W0 (unchanged per SPEC.md)
		{"D1", status.PhaseV2Problem},
		{"D2", status.PhaseV2Research},
		{"D3", status.PhaseV2Design},
		{"D4", status.PhaseV2Spec},
		{"S4", status.PhaseV2Spec}, // S4 merged into D4
		{"S5", status.PhaseV2Plan}, // S5 merged into S6
		{"S6", status.PhaseV2Plan},
		{"S7", status.PhaseV2Setup},
		{"S8", status.PhaseV2Build},
		{"S9", status.PhaseV2Build},  // S9 merged into S8
		{"S10", status.PhaseV2Build}, // S10 merged into S8
		{"S11", status.PhaseV2Retro},
		{"", status.PhaseV2Charter}, // Empty maps to W0
	}

	for _, tt := range tests {
		t.Run(tt.v1Phase, func(t *testing.T) {
			result := convertPhase(tt.v1Phase)
			if result != tt.expected {
				t.Errorf("convertPhase(%q) = %q, want %q", tt.v1Phase, result, tt.expected)
			}
		})
	}
}

func TestConvertStatus(t *testing.T) {
	tests := []struct {
		v1Status string
		expected string
	}{
		{status.StatusInProgress, status.StatusV2InProgress},
		{status.StatusCompleted, status.StatusV2Completed},
		{status.StatusAbandoned, status.StatusV2Abandoned},
		{status.StatusBlocked, status.StatusV2Blocked},
		{"unknown", status.StatusV2Planning},
	}

	for _, tt := range tests {
		t.Run(tt.v1Status, func(t *testing.T) {
			result := convertStatus(tt.v1Status)
			if result != tt.expected {
				t.Errorf("convertStatus(%q) = %q, want %q", tt.v1Status, result, tt.expected)
			}
		})
	}
}

func TestConvertPhaseStatus(t *testing.T) {
	tests := []struct {
		v1PhaseStatus string
		expected      string
	}{
		{status.PhaseStatusPending, status.PhaseStatusV2Pending},
		{status.PhaseStatusInProgress, status.PhaseStatusV2InProgress},
		{status.PhaseStatusCompleted, status.PhaseStatusV2Completed},
		{status.PhaseStatusSkipped, status.PhaseStatusV2Skipped},
		{"unknown", status.PhaseStatusV2Pending},
	}

	for _, tt := range tests {
		t.Run(tt.v1PhaseStatus, func(t *testing.T) {
			result := convertPhaseStatus(tt.v1PhaseStatus)
			if result != tt.expected {
				t.Errorf("convertPhaseStatus(%q) = %q, want %q", tt.v1PhaseStatus, result, tt.expected)
			}
		})
	}
}

func TestConvertPhaseHistory(t *testing.T) {
	now := time.Now()
	ended := now.Add(time.Hour)

	v1Phases := []status.Phase{
		{
			Name:        "W0",
			Status:      status.PhaseStatusCompleted,
			StartedAt:   &now,
			CompletedAt: &ended,
			Outcome:     status.OutcomeSuccess,
		},
		{
			Name:      "D1",
			Status:    status.PhaseStatusInProgress,
			StartedAt: &now,
		},
		{
			Name:        "S5",
			Status:      status.PhaseStatusSkipped,
			StartedAt:   &now,
			CompletedAt: &ended,
			Outcome:     status.OutcomeSkipped,
		},
	}

	result := convertPhaseHistory(v1Phases)

	// Expect 3 phases: W0→W0, D1→D1, S5→S6 per SPEC.md
	expectedCount := 3
	if len(result) != expectedCount {
		t.Fatalf("convertPhaseHistory() returned %d phases, want %d", len(result), expectedCount)
	}

	// First phase: W0 -> W0 (unchanged per SPEC.md)
	if result[0].Name != status.PhaseV2Charter {
		t.Errorf("PhaseHistory[0].Name = %v, want %v", result[0].Name, status.PhaseV2Charter)
	}
	if result[0].Status != status.PhaseStatusV2Completed {
		t.Errorf("PhaseHistory[0].Status = %v, want %v", result[0].Status, status.PhaseStatusV2Completed)
	}

	// Second phase: D1 -> D1
	if result[1].Name != status.PhaseV2Problem {
		t.Errorf("PhaseHistory[1].Name = %v, want %v", result[1].Name, status.PhaseV2Problem)
	}
	if result[1].Status != status.PhaseStatusV2InProgress {
		t.Errorf("PhaseHistory[1].Status = %v, want %v", result[1].Status, status.PhaseStatusV2InProgress)
	}

	// Third phase: S5 -> S6 (merged into S6 per SPEC.md)
	if result[2].Name != status.PhaseV2Plan {
		t.Errorf("PhaseHistory[2].Name = %v, want %v", result[2].Name, status.PhaseV2Plan)
	}
	if result[2].Status != status.PhaseStatusV2Skipped {
		t.Errorf("PhaseHistory[2].Status = %v, want %v", result[2].Status, status.PhaseStatusV2Skipped)
	}
}

func TestConvertV1ToV2_BlockedStatus(t *testing.T) {
	now := time.Now()
	v1 := &status.Status{
		SessionID:    "test-123",
		ProjectPath:  "/test/project",
		StartedAt:    now,
		Status:       status.StatusBlocked,
		BlockedOn:    "agent-456",
		CurrentPhase: "S8",
		Phases:       []status.Phase{},
	}

	v2, err := ConvertV1ToV2(v1)
	if err != nil {
		t.Fatalf("ConvertV1ToV2() error = %v", err)
	}

	if v2.Status != status.StatusV2Blocked {
		t.Errorf("Status = %v, want %v", v2.Status, status.StatusV2Blocked)
	}

	expectedBlockedReason := "Blocked on: agent-456"
	if v2.BlockedReason != expectedBlockedReason {
		t.Errorf("BlockedReason = %v, want %v", v2.BlockedReason, expectedBlockedReason)
	}
}

func TestConvertV1ToV2_InitializesRoadmapAndMetrics(t *testing.T) {
	now := time.Now()
	v1 := &status.Status{
		SessionID:    "test-123",
		ProjectPath:  "/test/project",
		StartedAt:    now,
		Status:       status.StatusInProgress,
		CurrentPhase: "D1",
		Phases:       []status.Phase{},
	}

	v2, err := ConvertV1ToV2(v1)
	if err != nil {
		t.Fatalf("ConvertV1ToV2() error = %v", err)
	}

	// Verify roadmap is initialized
	if v2.Roadmap == nil {
		t.Error("Roadmap should be initialized")
	} else if v2.Roadmap.Phases == nil {
		t.Error("Roadmap.Phases should be initialized")
	}

	// Verify quality metrics are initialized
	if v2.QualityMetrics == nil {
		t.Error("QualityMetrics should be initialized")
	} else {
		if v2.QualityMetrics.CoverageTarget != 80.0 {
			t.Errorf("CoverageTarget = %v, want 80.0", v2.QualityMetrics.CoverageTarget)
		}
		if v2.QualityMetrics.AssertionDensityTarget != 3.0 {
			t.Errorf("AssertionDensityTarget = %v, want 3.0", v2.QualityMetrics.AssertionDensityTarget)
		}
	}
}
