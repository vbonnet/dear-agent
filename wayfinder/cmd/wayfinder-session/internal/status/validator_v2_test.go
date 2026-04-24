package status

import (
	"strings"
	"testing"
	"time"
)

func TestValidateV2(t *testing.T) {
	tests := []struct {
		name    string
		status  *StatusV2
		wantErr bool
		errMsg  string
	}{
		{
			name:    "nil status",
			status:  nil,
			wantErr: true,
			errMsg:  "status is nil",
		},
		{
			name: "valid minimal status",
			status: &StatusV2{
				SchemaVersion:   SchemaVersionV2,
				ProjectName:     "Test",
				ProjectType:     ProjectTypeFeature,
				RiskLevel:       RiskLevelM,
				CurrentWaypoint: PhaseV2Charter,
				Status:          StatusV2Planning,
				CreatedAt:       time.Now(),
				UpdatedAt:       time.Now(),
			},
			wantErr: false,
		},
		{
			name: "missing schema_version",
			status: &StatusV2{
				ProjectName:     "Test",
				ProjectType:     ProjectTypeFeature,
				RiskLevel:       RiskLevelM,
				CurrentWaypoint: PhaseV2Charter,
				Status:          StatusV2Planning,
				CreatedAt:       time.Now(),
				UpdatedAt:       time.Now(),
			},
			wantErr: true,
			errMsg:  "schema_version",
		},
		{
			name: "invalid schema_version",
			status: &StatusV2{
				SchemaVersion:   "1.0",
				ProjectName:     "Test",
				ProjectType:     ProjectTypeFeature,
				RiskLevel:       RiskLevelM,
				CurrentWaypoint: PhaseV2Charter,
				Status:          StatusV2Planning,
				CreatedAt:       time.Now(),
				UpdatedAt:       time.Now(),
			},
			wantErr: true,
			errMsg:  "schema_version must be '2.0'",
		},
		{
			name: "invalid project_type",
			status: &StatusV2{
				SchemaVersion:   SchemaVersionV2,
				ProjectName:     "Test",
				ProjectType:     "invalid-type",
				RiskLevel:       RiskLevelM,
				CurrentWaypoint: PhaseV2Charter,
				Status:          StatusV2Planning,
				CreatedAt:       time.Now(),
				UpdatedAt:       time.Now(),
			},
			wantErr: true,
			errMsg:  "invalid project_type",
		},
		{
			name: "invalid risk_level",
			status: &StatusV2{
				SchemaVersion:   SchemaVersionV2,
				ProjectName:     "Test",
				ProjectType:     ProjectTypeFeature,
				RiskLevel:       "XXL",
				CurrentWaypoint: PhaseV2Charter,
				Status:          StatusV2Planning,
				CreatedAt:       time.Now(),
				UpdatedAt:       time.Now(),
			},
			wantErr: true,
			errMsg:  "invalid risk_level",
		},
		{
			name: "invalid current_phase",
			status: &StatusV2{
				SchemaVersion:   SchemaVersionV2,
				ProjectName:     "Test",
				ProjectType:     ProjectTypeFeature,
				RiskLevel:       RiskLevelM,
				CurrentWaypoint: "S5", // Merged phase
				Status:          StatusV2Planning,
				CreatedAt:       time.Now(),
				UpdatedAt:       time.Now(),
			},
			wantErr: true,
			errMsg:  "invalid current_waypoint",
		},
		{
			name: "completed without completion_date",
			status: &StatusV2{
				SchemaVersion:   SchemaVersionV2,
				ProjectName:     "Test",
				ProjectType:     ProjectTypeFeature,
				RiskLevel:       RiskLevelM,
				CurrentWaypoint: PhaseV2Retro,
				Status:          StatusV2Completed,
				CreatedAt:       time.Now(),
				UpdatedAt:       time.Now(),
			},
			wantErr: true,
			errMsg:  "completion_date is missing",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateV2(tt.status)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateV2() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantErr && tt.errMsg != "" {
				if !strings.Contains(err.Error(), tt.errMsg) {
					t.Errorf("ValidateV2() error = %v, want error containing %q", err, tt.errMsg)
				}
			}
		})
	}
}

func TestValidatePhaseHistory(t *testing.T) {
	tests := []struct {
		name    string
		history []PhaseHistory
		wantErr bool
		errMsg  string
	}{
		{
			name:    "nil history",
			history: nil,
			wantErr: false,
		},
		{
			name: "valid history",
			history: []PhaseHistory{
				{
					Name:      PhaseV2Charter,
					Status:    PhaseStatusV2Completed,
					StartedAt: time.Now(),
					CompletedAt: func() *time.Time {
						t := time.Now()
						return &t
					}(),
				},
			},
			wantErr: false,
		},
		{
			name: "invalid phase name",
			history: []PhaseHistory{
				{
					Name:      "INVALID",
					Status:    PhaseStatusV2Completed,
					StartedAt: time.Now(),
				},
			},
			wantErr: true,
			errMsg:  "invalid waypoint name",
		},
		{
			name: "legacy phase S4",
			history: []PhaseHistory{
				{
					Name:      "S4",
					Status:    PhaseStatusV2Completed,
					StartedAt: time.Now(),
				},
			},
			wantErr: true,
			errMsg:  "cannot use legacy waypoint 'S4'",
		},
		{
			name: "completed without completed_at",
			history: []PhaseHistory{
				{
					Name:      PhaseV2Charter,
					Status:    PhaseStatusV2Completed,
					StartedAt: time.Now(),
				},
			},
			wantErr: true,
			errMsg:  "must have completed_at",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			status := &StatusV2{
				SchemaVersion:   SchemaVersionV2,
				ProjectName:     "Test",
				ProjectType:     ProjectTypeFeature,
				RiskLevel:       RiskLevelM,
				CurrentWaypoint: PhaseV2Charter,
				Status:          StatusV2Planning,
				CreatedAt:       time.Now(),
				UpdatedAt:       time.Now(),
				WaypointHistory: tt.history,
			}

			err := validateWaypointHistory(status)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateWaypointHistory() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantErr && tt.errMsg != "" {
				if !strings.Contains(err.Error(), tt.errMsg) {
					t.Errorf("validateWaypointHistory() error = %v, want error containing %q", err, tt.errMsg)
				}
			}
		})
	}
}

func TestValidateRoadmap(t *testing.T) {
	tests := []struct {
		name    string
		roadmap *Roadmap
		wantErr bool
		errMsg  string
	}{
		{
			name:    "nil roadmap",
			roadmap: nil,
			wantErr: false,
		},
		{
			name: "valid roadmap",
			roadmap: &Roadmap{
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
							},
						},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "duplicate task IDs",
			roadmap: &Roadmap{
				Phases: []RoadmapPhase{
					{
						ID:     PhaseV2Setup,
						Name:   "Planning",
						Status: PhaseStatusV2Completed,
						Tasks: []Task{
							{ID: "task-1", Title: "Task 1", Status: TaskStatusCompleted},
							{ID: "task-1", Title: "Task 2", Status: TaskStatusCompleted},
						},
					},
				},
			},
			wantErr: true,
			errMsg:  "duplicate task_id",
		},
		{
			name: "invalid task dependency",
			roadmap: &Roadmap{
				Phases: []RoadmapPhase{
					{
						ID:     PhaseV2Setup,
						Name:   "Planning",
						Status: PhaseStatusV2Completed,
						Tasks: []Task{
							{
								ID:        "task-1",
								Title:     "Task 1",
								Status:    TaskStatusCompleted,
								DependsOn: []string{"non-existent"},
							},
						},
					},
				},
			},
			wantErr: true,
			errMsg:  "depends_on references non-existent task",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			status := &StatusV2{
				SchemaVersion:   SchemaVersionV2,
				ProjectName:     "Test",
				ProjectType:     ProjectTypeFeature,
				RiskLevel:       RiskLevelM,
				CurrentWaypoint: PhaseV2Charter,
				Status:          StatusV2Planning,
				CreatedAt:       time.Now(),
				UpdatedAt:       time.Now(),
				Roadmap:         tt.roadmap,
			}

			err := validateRoadmap(status)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateRoadmap() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantErr && tt.errMsg != "" {
				if !strings.Contains(err.Error(), tt.errMsg) {
					t.Errorf("validateRoadmap() error = %v, want error containing %q", err, tt.errMsg)
				}
			}
		})
	}
}

func TestDetectCyclicDependencies(t *testing.T) {
	tests := []struct {
		name    string
		tasks   []Task
		wantErr bool
		errMsg  string
	}{
		{
			name:    "no tasks",
			tasks:   []Task{},
			wantErr: false,
		},
		{
			name: "no dependencies",
			tasks: []Task{
				{ID: "task-1", Status: TaskStatusCompleted},
				{ID: "task-2", Status: TaskStatusCompleted},
			},
			wantErr: false,
		},
		{
			name: "linear dependencies",
			tasks: []Task{
				{ID: "task-1", Status: TaskStatusCompleted, DependsOn: []string{}},
				{ID: "task-2", Status: TaskStatusCompleted, DependsOn: []string{"task-1"}},
				{ID: "task-3", Status: TaskStatusCompleted, DependsOn: []string{"task-2"}},
			},
			wantErr: false,
		},
		{
			name: "simple cycle",
			tasks: []Task{
				{ID: "task-1", Status: TaskStatusCompleted, DependsOn: []string{"task-2"}},
				{ID: "task-2", Status: TaskStatusCompleted, DependsOn: []string{"task-1"}},
			},
			wantErr: true,
			errMsg:  "cyclic dependency detected",
		},
		{
			name: "complex cycle",
			tasks: []Task{
				{ID: "task-1", Status: TaskStatusCompleted, DependsOn: []string{"task-2"}},
				{ID: "task-2", Status: TaskStatusCompleted, DependsOn: []string{"task-3"}},
				{ID: "task-3", Status: TaskStatusCompleted, DependsOn: []string{"task-1"}},
			},
			wantErr: true,
			errMsg:  "cyclic dependency detected",
		},
		{
			name: "self-dependency",
			tasks: []Task{
				{ID: "task-1", Status: TaskStatusCompleted, DependsOn: []string{"task-1"}},
			},
			wantErr: true,
			errMsg:  "cyclic dependency detected",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := detectCyclicDependencies(tt.tasks)
			if (err != nil) != tt.wantErr {
				t.Errorf("detectCyclicDependencies() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantErr && tt.errMsg != "" {
				if !strings.Contains(err.Error(), tt.errMsg) {
					t.Errorf("detectCyclicDependencies() error = %v, want error containing %q", err, tt.errMsg)
				}
			}
		})
	}
}

func TestValidateQualityMetrics(t *testing.T) {
	tests := []struct {
		name    string
		metrics *QualityMetrics
		wantErr bool
		errMsg  string
	}{
		{
			name:    "nil metrics",
			metrics: nil,
			wantErr: false,
		},
		{
			name: "valid metrics",
			metrics: &QualityMetrics{
				CoveragePercent:   85.5,
				CoverageTarget:    80.0,
				AssertionDensity:  3.5,
				MultiPersonaScore: 90.0,
				P0Issues:          0,
				P1Issues:          2,
			},
			wantErr: false,
		},
		{
			name: "coverage out of range",
			metrics: &QualityMetrics{
				CoveragePercent: 150.0,
			},
			wantErr: true,
			errMsg:  "coverage_percent must be 0-100",
		},
		{
			name: "negative issues",
			metrics: &QualityMetrics{
				P0Issues: -1,
			},
			wantErr: true,
			errMsg:  "p0_issues cannot be negative",
		},
		{
			name: "score out of range",
			metrics: &QualityMetrics{
				SecurityScore: 110.0,
			},
			wantErr: true,
			errMsg:  "security_score must be 0-100",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			status := &StatusV2{
				SchemaVersion:   SchemaVersionV2,
				ProjectName:     "Test",
				ProjectType:     ProjectTypeFeature,
				RiskLevel:       RiskLevelM,
				CurrentWaypoint: PhaseV2Charter,
				Status:          StatusV2Planning,
				CreatedAt:       time.Now(),
				UpdatedAt:       time.Now(),
				QualityMetrics:  tt.metrics,
			}

			err := validateQualityMetrics(status)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateQualityMetrics() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantErr && tt.errMsg != "" {
				if !strings.Contains(err.Error(), tt.errMsg) {
					t.Errorf("validateQualityMetrics() error = %v, want error containing %q", err, tt.errMsg)
				}
			}
		})
	}
}

func TestValidateV2WithRealExample(t *testing.T) {
	// Parse the valid example file
	status, err := ParseV2("testdata/valid-v2.yaml")
	if err != nil {
		t.Fatalf("ParseV2() error = %v", err)
	}

	// Validate it
	err = ValidateV2(status)
	if err != nil {
		t.Errorf("ValidateV2() error = %v, expected valid file to pass validation", err)
	}
}
