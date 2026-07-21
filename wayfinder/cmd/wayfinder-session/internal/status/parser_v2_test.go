package status

import (
	"os"
	"path/filepath"
	"strings"
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
		SchemaVersion:   SchemaVersion,
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
		WaypointHistory: completedHistoryBefore(PhaseV2Build, time.Now().Add(-time.Hour).Truncate(time.Second)),
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

func TestParseV2RejectsMissingOrUnsupportedSchema(t *testing.T) {
	for _, tc := range []struct {
		name    string
		schema  string
		message string
	}{
		{name: "missing", message: "schema_version is required"},
		{name: "unsupported", schema: "schema_version: \"1.0\"\n", message: "unsupported schema_version"},
		{name: "unquoted numeric", schema: "schema_version: 2.0\n", message: "schema_version must be an actual string scalar"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), StatusFilename)
			content := "---\n" + tc.schema + "project_name: test\n---\n"
			if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
				t.Fatal(err)
			}
			_, err := ParseV2(path)
			if err == nil || !strings.Contains(err.Error(), tc.message) {
				t.Fatalf("ParseV2() error = %v, want %q", err, tc.message)
			}
		})
	}
}

func TestParseV2RejectsNonStringCanonicalScalars(t *testing.T) {
	valid, err := os.ReadFile("testdata/valid-v2.yaml")
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name    string
		old     string
		replace string
		want    string
	}{
		{
			name:    "top-level string",
			old:     `project_name: "User Authentication Service"`,
			replace: "project_name: 123",
			want:    "project_name must be an actual string scalar",
		},
		{
			name:    "nested string",
			old:     `title: "Break down implementation tasks"`,
			replace: "title: 123",
			want:    "roadmap.phases[0].tasks[0].title must be an actual string scalar",
		},
		{
			name:    "string sequence item",
			old:     `  - "security"`,
			replace: "  - 123",
			want:    "tags[0] must be an actual string scalar",
		},
		{
			name:    "null boolean",
			old:     `risk_level: "L"`,
			replace: "risk_level: \"L\"\nskip_roadmap: null",
			want:    "skip_roadmap must be an actual boolean scalar",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			content := strings.Replace(string(valid), tc.old, tc.replace, 1)
			if content == string(valid) {
				t.Fatalf("fixture does not contain %q", tc.old)
			}
			_, err := ParseV2Content([]byte(content))
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("ParseV2Content() error = %v, want %q", err, tc.want)
			}
		})
	}
}

func TestParseV2RejectsRecursiveYAMLAlias(t *testing.T) {
	content := `---
schema_version: "2.0"
project_name: "test"
project_type: "feature"
risk_level: "M"
current_waypoint: "CHARTER"
status: "planning"
created_at: "2026-02-15T10:00:00Z"
updated_at: "2026-02-15T10:00:00Z"
waypoint_history: []
roadmap: &roadmap
  phases:
    - *roadmap
---
`
	_, err := ParseV2Content([]byte(content))
	if err == nil || !strings.Contains(err.Error(), "YAML aliases are not allowed") {
		t.Fatalf("ParseV2Content() error = %v, want alias rejection", err)
	}
}

func TestParseV2RejectsInvalidCanonicalStatus(t *testing.T) {
	for _, tc := range []struct {
		name    string
		mutate  func(*StatusV2)
		message string
	}{
		{name: "missing required field", mutate: func(status *StatusV2) { status.ProjectName = "" }, message: "project_name is required"},
		{name: "invalid enum", mutate: func(status *StatusV2) { status.RiskLevel = "UNKNOWN" }, message: "invalid risk_level"},
		{name: "invalid optional build metadata", mutate: func(status *StatusV2) {
			status.UpdatePhase(PhaseV2Build, PhaseStatusV2InProgress, "")
			status.FindWaypointHistory(PhaseV2Build).ValidationStatus = "unknown"
		}, message: "invalid validation_status"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			status := NewStatusV2("test", ProjectTypeFeature, RiskLevelM)
			tc.mutate(status)
			path := filepath.Join(t.TempDir(), StatusFilename)
			if err := WriteV2(status, path); err != nil {
				t.Fatalf("WriteV2: %v", err)
			}
			_, err := ParseV2(path)
			if err == nil || !strings.Contains(err.Error(), tc.message) {
				t.Fatalf("ParseV2() error = %v, want %q", err, tc.message)
			}
		})
	}
}

func TestParseV2RejectsUnknownFields(t *testing.T) {
	status := NewStatusV2("test", ProjectTypeFeature, RiskLevelM)
	path := filepath.Join(t.TempDir(), StatusFilename)
	if err := WriteV2(status, path); err != nil {
		t.Fatalf("WriteV2: %v", err)
	}
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	content = []byte(strings.Replace(string(content), "beads:", "beeds:", 1))
	if !strings.Contains(string(content), "beeds:") {
		content = []byte(strings.Replace(string(content), "---\n", "---\nbeeds:\n  - ce-123\n", 1))
	}
	if err := os.WriteFile(path, content, 0o600); err != nil {
		t.Fatal(err)
	}

	_, err = ParseV2(path)
	if err == nil || !strings.Contains(err.Error(), "field beeds not found") {
		t.Fatalf("ParseV2() error = %v, want unknown-field rejection", err)
	}
}

func TestParseV2RejectsNonRFC3339Timestamps(t *testing.T) {
	valid, err := os.ReadFile("testdata/valid-v2.yaml")
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name    string
		old     string
		replace string
		want    string
	}{
		{name: "top-level date", old: `created_at: "2026-02-15T10:00:00Z"`, replace: "created_at: 2026-02-15", want: "created_at must be an RFC3339 timestamp"},
		{name: "nested date", old: `started_at: "2026-02-15T10:00:00Z"`, replace: "started_at: 2026-02-15", want: "started_at must be an RFC3339 timestamp"},
		{name: "invalid offset hour", old: `created_at: "2026-02-15T10:00:00Z"`, replace: `created_at: "2026-02-15T10:00:00+24:00"`, want: "created_at must be an RFC3339 timestamp"},
		{name: "invalid offset minute", old: `created_at: "2026-02-15T10:00:00Z"`, replace: `created_at: "2026-02-15T10:00:00+00:60"`, want: "created_at must be an RFC3339 timestamp"},
		{name: "comma fraction", old: `created_at: "2026-02-15T10:00:00Z"`, replace: `created_at: "2026-02-15T10:00:00,5Z"`, want: "created_at must be an RFC3339 timestamp"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			content := strings.Replace(string(valid), tc.old, tc.replace, 1)
			if content == string(valid) {
				t.Fatalf("fixture does not contain %q", tc.old)
			}
			_, err := ParseV2Content([]byte(content))
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("ParseV2Content() error = %v, want %q", err, tc.want)
			}
		})
	}
}

func TestParseV2AcceptsCanonicalHistoryAfterRewind(t *testing.T) {
	st := NewStatusV2("test", ProjectTypeFeature, RiskLevelM)
	now := time.Now()
	st.WaypointHistory = []WaypointHistory{{
		Name:        WaypointV2Charter,
		Status:      WaypointStatusV2Completed,
		StartedAt:   now.Add(-time.Hour),
		CompletedAt: &now,
	}}
	st.SetCurrentPhase(WaypointV2Problem)
	path := filepath.Join(t.TempDir(), StatusFilename)
	if err := WriteV2(st, path); err != nil {
		t.Fatalf("WriteV2: %v", err)
	}
	if _, err := ParseV2(path); err != nil {
		t.Fatalf("ParseV2 rejected rewind-produced canonical history: %v", err)
	}
}

func TestParseV2AcceptsStatusWrittenAtPhaseStart(t *testing.T) {
	for _, phase := range []string{PhaseV2Spec, PhaseV2Plan, PhaseV2Build} {
		t.Run(phase, func(t *testing.T) {
			st := NewStatusV2("test", ProjectTypeFeature, RiskLevelM)
			st.Status = StatusV2InProgress
			st.WaypointHistory = completedHistoryBefore(phase, time.Now())
			st.SetCurrentPhase(phase)
			st.UpdatePhase(phase, PhaseStatusV2InProgress, "")

			path := filepath.Join(t.TempDir(), StatusFilename)
			if err := WriteV2(st, path); err != nil {
				t.Fatalf("WriteV2: %v", err)
			}
			if _, err := ParseV2(path); err != nil {
				t.Fatalf("ParseV2 rejected start-phase output: %v", err)
			}
		})
	}
}

func completedHistoryBefore(target string, now time.Time) []WaypointHistory {
	var history []WaypointHistory
	for _, waypoint := range AllWaypointsV2Schema() {
		if waypoint == target {
			break
		}
		history = append(history, WaypointHistory{
			Name:        waypoint,
			Status:      WaypointStatusV2Completed,
			StartedAt:   now,
			CompletedAt: &now,
		})
	}
	return history
}

func TestNewStatusV2(t *testing.T) {
	status := NewStatusV2("Test Project", ProjectTypeResearch, RiskLevelXS)

	if status.SchemaVersion != SchemaVersion {
		t.Errorf("expected schema_version '%s', got '%s'", SchemaVersion, status.SchemaVersion)
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
		{
			name: "content after closing delimiter",
			content: `---
schema_version: "2.0"
project_name: "Test"
---
# retired Markdown status`,
			wantErr: true,
		},
		{
			name: "whitespace after closing delimiter",
			content: `---
schema_version: "2.0"
project_name: "Test"
---
   `,
			wantErr: false,
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
