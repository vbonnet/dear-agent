package migrate

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/vbonnet/dear-agent/wayfinder/cmd/wayfinder-session/internal/status"
)

func TestMigrator_ConvertV1ToV2(t *testing.T) {
	now := time.Now()
	endTime := now.Add(24 * time.Hour)

	tests := []struct {
		name            string
		v1Status        *status.Status
		wantPhase       string
		wantStatus      string
		wantRiskLevel   string
		wantProjectType string
	}{
		{
			name: "basic conversion",
			v1Status: &status.Status{
				SchemaVersion: "1.0",
				SessionID:     "test-session",
				ProjectPath:   "/tmp/test/test-project",
				StartedAt:     now,
				Status:        status.StatusInProgress,
				CurrentPhase:  "D3",
				Phases: []status.Phase{
					{Name: "W0", Status: status.PhaseStatusCompleted},
					{Name: "D1", Status: status.PhaseStatusCompleted},
					{Name: "D2", Status: status.PhaseStatusCompleted},
					{Name: "D3", Status: status.PhaseStatusInProgress},
				},
			},
			wantPhase:       status.PhaseV2Design,
			wantStatus:      status.StatusV2InProgress,
			wantRiskLevel:   status.RiskLevelS,
			wantProjectType: status.ProjectTypeFeature,
		},
		{
			name: "completed project",
			v1Status: &status.Status{
				SchemaVersion: "1.0",
				SessionID:     "test-session",
				ProjectPath:   "/tmp/test/completed-project",
				StartedAt:     now,
				EndedAt:       &endTime,
				Status:        status.StatusCompleted,
				CurrentPhase:  "S11",
				Phases:        createAllV1Phases(status.PhaseStatusCompleted),
			},
			wantPhase:       status.PhaseV2Retro,
			wantStatus:      status.StatusV2Completed,
			wantRiskLevel:   status.RiskLevelXL,
			wantProjectType: status.ProjectTypeFeature,
		},
		{
			name: "S4 phase maps to D4",
			v1Status: &status.Status{
				SchemaVersion: "1.0",
				SessionID:     "test-session",
				ProjectPath:   "/tmp/test/s4-project",
				StartedAt:     now,
				Status:        status.StatusInProgress,
				CurrentPhase:  "S4",
				Phases: []status.Phase{
					{Name: "S4", Status: status.PhaseStatusInProgress},
				},
			},
			wantPhase:       status.PhaseV2Spec,
			wantStatus:      status.StatusV2InProgress,
			wantRiskLevel:   status.RiskLevelXS,
			wantProjectType: status.ProjectTypeFeature,
		},
		{
			name: "S5 phase maps to S6",
			v1Status: &status.Status{
				SchemaVersion: "1.0",
				SessionID:     "test-session",
				ProjectPath:   "/tmp/test/s5-project",
				StartedAt:     now,
				Status:        status.StatusInProgress,
				CurrentPhase:  "S5",
				Phases: []status.Phase{
					{Name: "S5", Status: status.PhaseStatusInProgress},
				},
			},
			wantPhase:       status.PhaseV2Plan,
			wantStatus:      status.StatusV2InProgress,
			wantRiskLevel:   status.RiskLevelXS,
			wantProjectType: status.ProjectTypeFeature,
		},
		{
			name: "S8/S9/S10 phases map to S8",
			v1Status: &status.Status{
				SchemaVersion: "1.0",
				SessionID:     "test-session",
				ProjectPath:   "/tmp/test/build-project",
				StartedAt:     now,
				Status:        status.StatusInProgress,
				CurrentPhase:  "S9",
				Phases: []status.Phase{
					{Name: "S8", Status: status.PhaseStatusCompleted},
					{Name: "S9", Status: status.PhaseStatusInProgress},
				},
			},
			wantPhase:       status.PhaseV2Build,
			wantStatus:      status.StatusV2InProgress,
			wantRiskLevel:   status.RiskLevelXS,
			wantProjectType: status.ProjectTypeFeature,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := NewMigrator("/tmp/test")
			v2, err := m.convertV1ToV2(tt.v1Status)
			if err != nil {
				t.Fatalf("convertV1ToV2() error = %v", err)
			}

			if v2.CurrentWaypoint != tt.wantPhase {
				t.Errorf("CurrentPhase = %s, want %s", v2.CurrentWaypoint, tt.wantPhase)
			}

			if v2.Status != tt.wantStatus {
				t.Errorf("Status = %s, want %s", v2.Status, tt.wantStatus)
			}

			if v2.RiskLevel != tt.wantRiskLevel {
				t.Errorf("RiskLevel = %s, want %s", v2.RiskLevel, tt.wantRiskLevel)
			}

			if v2.ProjectType != tt.wantProjectType {
				t.Errorf("ProjectType = %s, want %s", v2.ProjectType, tt.wantProjectType)
			}

			// Verify schema version
			if v2.SchemaVersion != status.SchemaVersionV2 {
				t.Errorf("SchemaVersion = %s, want %s", v2.SchemaVersion, status.SchemaVersionV2)
			}

			// Verify timestamps
			if v2.CreatedAt != tt.v1Status.StartedAt {
				t.Errorf("CreatedAt not preserved from V1")
			}

			// Verify completion date
			if tt.v1Status.EndedAt != nil && v2.CompletionDate == nil {
				t.Error("CompletionDate should be set when V1 EndedAt is set")
			}
		})
	}
}

func TestMigrator_ConvertPhaseHistory(t *testing.T) {
	now := time.Now()
	completed := now.Add(-1 * time.Hour)

	tests := []struct {
		name       string
		v1Phases   []status.Phase
		wantPhases int
		checkPhase func(*testing.T, []status.WaypointHistory)
	}{
		{
			name: "convert basic phases",
			v1Phases: []status.Phase{
				{Name: "W0", Status: status.PhaseStatusCompleted, StartedAt: &now, CompletedAt: &completed},
				{Name: "D1", Status: status.PhaseStatusCompleted, StartedAt: &now},
				{Name: "D2", Status: status.PhaseStatusInProgress, StartedAt: &now},
			},
			wantPhases: 3,
			checkPhase: func(t *testing.T, history []status.WaypointHistory) {
				if history[0].Name != status.PhaseV2Charter {
					t.Errorf("First phase should be W0, got %s", history[0].Name)
				}
			},
		},
		{
			name: "D4 phase includes stakeholder metadata",
			v1Phases: []status.Phase{
				{Name: "D4", Status: status.PhaseStatusCompleted, StartedAt: &now},
			},
			wantPhases: 1,
			checkPhase: func(t *testing.T, history []status.WaypointHistory) {
				if history[0].StakeholderApproved == nil {
					t.Error("D4 phase should have StakeholderApproved set")
				}
				if !*history[0].StakeholderApproved {
					t.Error("Completed D4 should have StakeholderApproved = true")
				}
			},
		},
		{
			name: "S4 maps to D4 with stakeholder data",
			v1Phases: []status.Phase{
				{Name: "S4", Status: status.PhaseStatusCompleted, StartedAt: &now},
			},
			wantPhases: 1,
			checkPhase: func(t *testing.T, history []status.WaypointHistory) {
				if history[0].Name != status.PhaseV2Spec {
					t.Errorf("S4 should map to D4, got %s", history[0].Name)
				}
				if history[0].StakeholderApproved == nil {
					t.Error("Migrated S4 should have StakeholderApproved set")
				}
			},
		},
		{
			name: "S6 phase includes research metadata",
			v1Phases: []status.Phase{
				{Name: "S5", Status: status.PhaseStatusCompleted, StartedAt: &now},
			},
			wantPhases: 1,
			checkPhase: func(t *testing.T, history []status.WaypointHistory) {
				if history[0].Name != status.PhaseV2Plan {
					t.Errorf("S5 should map to S6, got %s", history[0].Name)
				}
				if history[0].ResearchNotes == "" {
					t.Error("S6 phase should have ResearchNotes set")
				}
			},
		},
		{
			name: "S8 phase includes build metadata",
			v1Phases: []status.Phase{
				{Name: "S8", Status: status.PhaseStatusCompleted, StartedAt: &now},
			},
			wantPhases: 1,
			checkPhase: func(t *testing.T, history []status.WaypointHistory) {
				if history[0].Name != status.PhaseV2Build {
					t.Errorf("S8 should map to S8, got %s", history[0].Name)
				}
				if history[0].BuildIterations == 0 {
					t.Error("S8 phase should have BuildIterations > 0")
				}
				if history[0].ValidationStatus == "" {
					t.Error("S8 phase should have ValidationStatus set")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := NewMigrator("/tmp/test")
			history := m.convertPhaseHistory(tt.v1Phases)

			if len(history) != tt.wantPhases {
				t.Errorf("convertPhaseHistory() returned %d phases, want %d",
					len(history), tt.wantPhases)
			}

			if tt.checkPhase != nil {
				tt.checkPhase(t, history)
			}
		})
	}
}

func TestMigrator_MapV1PhaseToV2(t *testing.T) {
	tests := []struct {
		v1Phase string
		want    string
	}{
		{"W0", status.PhaseV2Charter},
		{"D1", status.PhaseV2Problem},
		{"D2", status.PhaseV2Research},
		{"D3", status.PhaseV2Design},
		{"D4", status.PhaseV2Spec},
		{"S4", status.PhaseV2Spec},  // Merged
		{"S5", status.PhaseV2Plan},  // Merged
		{"S6", status.PhaseV2Setup}, // Renamed
		{"S7", status.PhaseV2Setup},
		{"S8", status.PhaseV2Build},
		{"S9", status.PhaseV2Build},  // Merged
		{"S10", status.PhaseV2Build}, // Merged
		{"S11", status.PhaseV2Retro},
		{"INVALID", ""},
	}

	m := NewMigrator("/tmp/test")
	for _, tt := range tests {
		t.Run(tt.v1Phase, func(t *testing.T) {
			got := m.mapV1PhaseToV2(tt.v1Phase)
			if got != tt.want {
				t.Errorf("mapV1PhaseToV2(%s) = %s, want %s", tt.v1Phase, got, tt.want)
			}
		})
	}
}

func TestMigrator_MapV1StatusToV2(t *testing.T) {
	tests := []struct {
		v1Status string
		want     string
	}{
		{status.StatusInProgress, status.StatusV2InProgress},
		{status.StatusCompleted, status.StatusV2Completed},
		{status.StatusAbandoned, status.StatusV2Abandoned},
		{status.StatusBlocked, status.StatusV2Blocked},
		{status.StatusObsolete, status.StatusV2Abandoned},
		{"unknown", status.StatusV2Planning},
	}

	m := NewMigrator("/tmp/test")
	for _, tt := range tests {
		t.Run(tt.v1Status, func(t *testing.T) {
			got := m.mapV1StatusToV2(tt.v1Status)
			if got != tt.want {
				t.Errorf("mapV1StatusToV2(%s) = %s, want %s", tt.v1Status, got, tt.want)
			}
		})
	}
}

func TestMigrator_CalculateRiskLevel(t *testing.T) {
	tests := []struct {
		name            string
		completedPhases int
		want            string
	}{
		{"very small project", 1, status.RiskLevelXS},
		{"small project", 3, status.RiskLevelS},
		{"medium project", 5, status.RiskLevelM},
		{"large project", 7, status.RiskLevelL},
		{"very large project", 10, status.RiskLevelXL},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v1 := &status.Status{
				Phases: make([]status.Phase, tt.completedPhases),
			}
			for i := range v1.Phases {
				v1.Phases[i].Status = status.PhaseStatusCompleted
			}

			m := NewMigrator("/tmp/test")
			got := m.calculateRiskLevel(v1)
			if got != tt.want {
				t.Errorf("calculateRiskLevel() = %s, want %s", got, tt.want)
			}
		})
	}
}

func TestMigrator_CreateInitialRoadmap(t *testing.T) {
	m := NewMigrator("/tmp/test")

	tests := []struct {
		name         string
		currentPhase string
		wantPhases   int
		checkRoadmap func(*testing.T, []status.RoadmapPhase)
	}{
		{
			name:         "roadmap for W0",
			currentPhase: status.PhaseV2Charter,
			wantPhases:   9,
			checkRoadmap: func(t *testing.T, phases []status.RoadmapPhase) {
				if phases[0].Status != status.PhaseStatusV2InProgress {
					t.Error("First phase (W0) should be in progress")
				}
				if phases[1].Status != status.PhaseStatusV2Pending {
					t.Error("Second phase (D1) should be pending")
				}
			},
		},
		{
			name:         "roadmap for D3",
			currentPhase: status.PhaseV2Design,
			wantPhases:   9,
			checkRoadmap: func(t *testing.T, phases []status.RoadmapPhase) {
				// W0, D1, D2 should be completed
				for i := 0; i < 3; i++ {
					if phases[i].Status != status.PhaseStatusV2Completed {
						t.Errorf("Phase %s should be completed", phases[i].ID)
					}
				}
				// D3 should be in progress
				if phases[3].Status != status.PhaseStatusV2InProgress {
					t.Error("D3 should be in progress")
				}
				// Rest should be pending
				for i := 4; i < len(phases); i++ {
					if phases[i].Status != status.PhaseStatusV2Pending {
						t.Errorf("Phase %s should be pending", phases[i].ID)
					}
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			roadmap := m.createInitialRoadmap(tt.currentPhase)

			if len(roadmap) != tt.wantPhases {
				t.Errorf("createInitialRoadmap() returned %d phases, want %d",
					len(roadmap), tt.wantPhases)
			}

			if tt.checkRoadmap != nil {
				tt.checkRoadmap(t, roadmap)
			}
		})
	}
}

func TestMigrator_Migrate_Integration(t *testing.T) {
	// Create temp directory with V1 project
	tmpDir := t.TempDir()

	// Create V1 WAYFINDER-STATUS.md
	v1Content := `---
schema_version: "1.0"
session_id: "test-session"
project_path: "` + tmpDir + `"
started_at: 2026-02-20T10:00:00Z
status: in_progress
current_phase: D3
phases:
  - name: W0
    status: completed
    started_at: 2026-02-20T10:00:00Z
    completed_at: 2026-02-20T11:00:00Z
  - name: D1
    status: completed
    started_at: 2026-02-20T11:00:00Z
    completed_at: 2026-02-20T12:00:00Z
  - name: D2
    status: completed
    started_at: 2026-02-20T12:00:00Z
    completed_at: 2026-02-20T13:00:00Z
  - name: D3
    status: in_progress
    started_at: 2026-02-20T13:00:00Z
---
# Wayfinder Status
`

	statusPath := filepath.Join(tmpDir, status.StatusFilename)
	if err := os.WriteFile(statusPath, []byte(v1Content), 0644); err != nil {
		t.Fatalf("failed to create V1 status: %v", err)
	}

	// Create some phase files
	files := map[string]string{
		"S4-approval.md": "Stakeholder approved",
		"S5-research.md": "Research findings",
	}

	for filename, content := range files {
		path := filepath.Join(tmpDir, filename)
		if err := os.WriteFile(path, []byte(content), 0644); err != nil {
			t.Fatalf("failed to create file: %v", err)
		}
	}

	// Run migration
	m := NewMigrator(tmpDir)
	v2Status, err := m.Migrate()
	if err != nil {
		t.Fatalf("Migrate() error = %v", err)
	}

	// Verify V2 status
	if v2Status.SchemaVersion != status.SchemaVersionV2 {
		t.Errorf("SchemaVersion = %s, want %s", v2Status.SchemaVersion, status.SchemaVersionV2)
	}

	if v2Status.CurrentWaypoint != status.PhaseV2Design {
		t.Errorf("CurrentPhase = %s, want %s", v2Status.CurrentWaypoint, status.PhaseV2Design)
	}

	// Verify V2 file was written
	v2Content, err := os.ReadFile(statusPath)
	if err != nil {
		t.Fatalf("V2 status file should exist: %v", err)
	}

	if !strings.Contains(string(v2Content), "schema_version: \"2.0\"") {
		t.Error("V2 file should contain schema_version 2.0")
	}

	// Verify V1 backup exists
	backupPath := filepath.Join(tmpDir, ".wayfinder-v1-backup", status.StatusFilename)
	if _, err := os.Stat(backupPath); os.IsNotExist(err) {
		t.Error("V1 backup should exist")
	}

	// Verify files were migrated
	d4Path := filepath.Join(tmpDir, "D4-requirements.md")
	if _, err := os.Stat(d4Path); os.IsNotExist(err) {
		t.Error("D4-requirements.md should exist after migration")
	}

	s6Path := filepath.Join(tmpDir, "S6-design.md")
	if _, err := os.Stat(s6Path); os.IsNotExist(err) {
		t.Error("S6-design.md should exist after migration")
	}
}

func TestMigrator_DryRun(t *testing.T) {
	tmpDir := t.TempDir()

	// Create V1 status
	v1Content := `---
schema_version: "1.0"
session_id: "test-session"
project_path: "` + tmpDir + `"
started_at: 2026-02-20T10:00:00Z
status: in_progress
current_phase: D1
phases:
  - name: W0
    status: completed
---
# Status
`

	statusPath := filepath.Join(tmpDir, status.StatusFilename)
	if err := os.WriteFile(statusPath, []byte(v1Content), 0644); err != nil {
		t.Fatalf("failed to create V1 status: %v", err)
	}

	// Run migration in dry-run mode
	m := NewMigrator(tmpDir)
	m.SetDryRun(true)
	_, err := m.Migrate()
	if err != nil {
		t.Fatalf("Migrate() error = %v", err)
	}

	// Verify V1 file still exists and unchanged
	content, err := os.ReadFile(statusPath)
	if err != nil {
		t.Fatalf("V1 status should still exist: %v", err)
	}

	if !strings.Contains(string(content), "schema_version: \"1.0\"") {
		t.Error("V1 file should be unchanged in dry-run mode")
	}

	// Verify no backup was created
	backupPath := filepath.Join(tmpDir, ".wayfinder-v1-backup", status.StatusFilename)
	if _, err := os.Stat(backupPath); err == nil {
		t.Error("No backup should be created in dry-run mode")
	}
}

func TestNewMigrator(t *testing.T) {
	projectDir := "/tmp/test-project"
	m := NewMigrator(projectDir)

	if m.projectDir != projectDir {
		t.Errorf("NewMigrator() projectDir = %s, want %s", m.projectDir, projectDir)
	}

	if m.fileMigrator == nil {
		t.Error("NewMigrator() should initialize fileMigrator")
	}

	if m.dryRun {
		t.Error("NewMigrator() should default to dryRun=false")
	}
}

// Helper functions

func createAllV1Phases(phaseStatus string) []status.Phase {
	allPhases := []string{"W0", "D1", "D2", "D3", "D4", "S4", "S5", "S6", "S7", "S8", "S9", "S10", "S11"}
	phases := make([]status.Phase, len(allPhases))

	for i, name := range allPhases {
		phases[i] = status.Phase{
			Name:   name,
			Status: phaseStatus,
		}
	}

	return phases
}
