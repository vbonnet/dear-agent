package status

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestParseV2(t *testing.T) {
	tests := []struct {
		name    string
		file    string
		wantErr bool
	}{
		{
			name:    "valid V2 file",
			file:    "testdata/valid-v2.yaml",
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			status, err := ParseV2(tt.file)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseV2() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr {
				if status == nil {
					t.Error("ParseV2() returned nil status")
					return
				}
				// Validate basic fields
				if status.SchemaVersion != "2.0" {
					t.Errorf("expected schema_version '2.0', got '%s'", status.SchemaVersion)
				}
				if status.ProjectName == "" {
					t.Error("expected non-empty project_name")
				}
			}
		})
	}
}

func TestWriteV2(t *testing.T) {
	// Create temporary directory
	tmpDir := t.TempDir()

	// Create a test status
	status := NewStatusV2("Test Project", ProjectTypeFeature, RiskLevelM)
	status.Description = "Test description"
	status.Tags = []string{"test", "example"}

	// Write to file
	filePath := filepath.Join(tmpDir, "test-status.yaml")
	err := WriteV2(status, filePath)
	if err != nil {
		t.Fatalf("WriteV2() error = %v", err)
	}

	// Verify file exists
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		t.Error("WriteV2() did not create file")
	}

	// Read back and verify
	readStatus, err := ParseV2(filePath)
	if err != nil {
		t.Fatalf("ParseV2() error = %v", err)
	}

	if readStatus.ProjectName != status.ProjectName {
		t.Errorf("expected project_name '%s', got '%s'", status.ProjectName, readStatus.ProjectName)
	}
	if readStatus.ProjectType != status.ProjectType {
		t.Errorf("expected project_type '%s', got '%s'", status.ProjectType, readStatus.ProjectType)
	}
	if readStatus.RiskLevel != status.RiskLevel {
		t.Errorf("expected risk_level '%s', got '%s'", status.RiskLevel, readStatus.RiskLevel)
	}
}

func TestRoundTrip(t *testing.T) {
	// Create temporary directory
	tmpDir := t.TempDir()

	// Create a complex status with all fields
	status := &StatusV2{
		SchemaVersion:   SchemaVersionV2,
		ProjectName:     "Complex Test",
		ProjectType:     ProjectTypeFeature,
		RiskLevel:       RiskLevelL,
		CurrentWaypoint: PhaseV2Build,
		Status:          StatusV2InProgress,
		CreatedAt:       time.Now().Truncate(time.Second),
		UpdatedAt:       time.Now().Truncate(time.Second),
		Description:     "Complex test with all fields",
		Repository:      "https://github.com/test/repo",
		Branch:          "feature/test",
		Tags:            []string{"test", "complex"},
		Beads:           []string{"bead-1", "bead-2"},
		WaypointHistory: []PhaseHistory{
			{
				Name:      PhaseV2Charter,
				Status:    PhaseStatusV2Completed,
				StartedAt: time.Now().Add(-48 * time.Hour).Truncate(time.Second),
				CompletedAt: func() *time.Time {
					t := time.Now().Add(-47 * time.Hour).Truncate(time.Second)
					return &t
				}(),
			},
		},
		Roadmap: &Roadmap{
			Phases: []RoadmapPhase{
				{
					ID:     PhaseV2Setup,
					Name:   "Planning",
					Status: PhaseStatusV2Completed,
					Tasks: []Task{
						{
							ID:         "task-1",
							Title:      "Test task",
							EffortDays: 1.0,
							Status:     TaskStatusCompleted,
							DependsOn:  []string{},
						},
						{
							ID:         "task-2",
							Title:      "Second task",
							EffortDays: 2.0,
							Status:     TaskStatusInProgress,
							DependsOn:  []string{"task-1"},
						},
					},
				},
			},
		},
		QualityMetrics: &QualityMetrics{
			CoveragePercent:   85.5,
			CoverageTarget:    80.0,
			AssertionDensity:  3.5,
			MultiPersonaScore: 90.0,
			P0Issues:          0,
			P1Issues:          2,
		},
	}

	// Write
	filePath := filepath.Join(tmpDir, "roundtrip.yaml")
	if err := WriteV2(status, filePath); err != nil {
		t.Fatalf("WriteV2() error = %v", err)
	}

	// Read
	readStatus, err := ParseV2(filePath)
	if err != nil {
		t.Fatalf("ParseV2() error = %v", err)
	}

	// Compare key fields
	if readStatus.ProjectName != status.ProjectName {
		t.Errorf("project_name mismatch: want %s, got %s", status.ProjectName, readStatus.ProjectName)
	}
	if len(readStatus.Tags) != len(status.Tags) {
		t.Errorf("tags length mismatch: want %d, got %d", len(status.Tags), len(readStatus.Tags))
	}
	if len(readStatus.WaypointHistory) != len(status.WaypointHistory) {
		t.Errorf("phase_history length mismatch: want %d, got %d", len(status.WaypointHistory), len(readStatus.WaypointHistory))
	}
	if readStatus.Roadmap == nil || len(readStatus.Roadmap.Phases) != len(status.Roadmap.Phases) {
		t.Error("roadmap mismatch")
	}
	if readStatus.QualityMetrics == nil {
		t.Error("quality_metrics is nil")
	} else if readStatus.QualityMetrics.CoveragePercent != status.QualityMetrics.CoveragePercent {
		t.Errorf("coverage_percent mismatch: want %.2f, got %.2f", status.QualityMetrics.CoveragePercent, readStatus.QualityMetrics.CoveragePercent)
	}
}

func TestDetectSchemaVersion(t *testing.T) {
	tests := []struct {
		name    string
		file    string
		want    string
		wantErr bool
	}{
		{
			name:    "V2 file",
			file:    "testdata/valid-v2.yaml",
			want:    "2.0",
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			version, err := DetectSchemaVersion(tt.file)
			if (err != nil) != tt.wantErr {
				t.Errorf("DetectSchemaVersion() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if version != tt.want {
				t.Errorf("DetectSchemaVersion() = %v, want %v", version, tt.want)
			}
		})
	}
}

func TestNewStatusV2(t *testing.T) {
	status := NewStatusV2("Test Project", ProjectTypeResearch, RiskLevelXS)

	if status.SchemaVersion != SchemaVersionV2 {
		t.Errorf("expected schema_version '%s', got '%s'", SchemaVersionV2, status.SchemaVersion)
	}
	if status.ProjectName != "Test Project" {
		t.Errorf("expected project_name 'Test Project', got '%s'", status.ProjectName)
	}
	if status.ProjectType != ProjectTypeResearch {
		t.Errorf("expected project_type '%s', got '%s'", ProjectTypeResearch, status.ProjectType)
	}
	if status.RiskLevel != RiskLevelXS {
		t.Errorf("expected risk_level '%s', got '%s'", RiskLevelXS, status.RiskLevel)
	}
	if status.CurrentWaypoint != PhaseV2Charter {
		t.Errorf("expected current_phase '%s', got '%s'", PhaseV2Charter, status.CurrentWaypoint)
	}
	if status.Status != StatusV2Planning {
		t.Errorf("expected status '%s', got '%s'", StatusV2Planning, status.Status)
	}
	if status.CreatedAt.IsZero() {
		t.Error("expected non-zero created_at")
	}
	if status.UpdatedAt.IsZero() {
		t.Error("expected non-zero updated_at")
	}
	if status.Roadmap == nil {
		t.Error("expected non-nil roadmap")
	}
}

func TestExtractV2Frontmatter(t *testing.T) {
	tests := []struct {
		name    string
		content string
		wantErr bool
	}{
		{
			name: "valid frontmatter",
			content: `---
schema_version: "2.0"
project_name: "Test"
---`,
			wantErr: false,
		},
		{
			name: "missing opening",
			content: `schema_version: "2.0"
---`,
			wantErr: true,
		},
		{
			name: "missing closing",
			content: `---
schema_version: "2.0"`,
			wantErr: true,
		},
		{
			name:    "empty file",
			content: ``,
			wantErr: true,
		},
		{
			name: "empty frontmatter",
			content: `---
---`,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := extractV2Frontmatter(tt.content)
			if (err != nil) != tt.wantErr {
				t.Errorf("extractV2Frontmatter() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
