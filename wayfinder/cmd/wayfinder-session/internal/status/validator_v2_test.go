package status

import (
	"math"
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
				SchemaVersion:   SchemaVersion,
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
			name: "whitespace-only project name",
			status: &StatusV2{
				SchemaVersion:   SchemaVersion,
				ProjectName:     "  \t ",
				ProjectType:     ProjectTypeFeature,
				RiskLevel:       RiskLevelM,
				CurrentWaypoint: PhaseV2Charter,
				Status:          StatusV2Planning,
				CreatedAt:       time.Now(),
				UpdatedAt:       time.Now(),
			},
			wantErr: true,
			errMsg:  "project_name is required",
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
				SchemaVersion:   SchemaVersion,
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
				SchemaVersion:   SchemaVersion,
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
			name: "invalid lifecycle_state",
			status: &StatusV2{
				SchemaVersion:   SchemaVersion,
				ProjectName:     "Test",
				ProjectType:     ProjectTypeFeature,
				RiskLevel:       RiskLevelM,
				CurrentWaypoint: PhaseV2Charter,
				Status:          StatusV2Planning,
				LifecycleState:  "invalid",
				CreatedAt:       time.Now(),
				UpdatedAt:       time.Now(),
			},
			wantErr: true,
			errMsg:  "invalid lifecycle_state",
		},
		{
			name: "lifecycle_state conflicts with status",
			status: &StatusV2{
				SchemaVersion:   SchemaVersion,
				ProjectName:     "Test",
				ProjectType:     ProjectTypeFeature,
				RiskLevel:       RiskLevelM,
				CurrentWaypoint: PhaseV2Charter,
				Status:          StatusV2Planning,
				LifecycleState:  LifecycleWorking,
				CreatedAt:       time.Now(),
				UpdatedAt:       time.Now(),
			},
			wantErr: true,
			errMsg:  "requires status \"in-progress\"",
		},
		{
			name: "invalid current_phase",
			status: &StatusV2{
				SchemaVersion:   SchemaVersion,
				ProjectName:     "Test",
				ProjectType:     ProjectTypeFeature,
				RiskLevel:       RiskLevelM,
				CurrentWaypoint: "INVALID",
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
				SchemaVersion:   SchemaVersion,
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

func TestValidateV2RejectsIncompleteCompletedSession(t *testing.T) {
	now := time.Now()
	st := &StatusV2{
		SchemaVersion:   SchemaVersion,
		ProjectName:     "Test",
		ProjectType:     ProjectTypeFeature,
		RiskLevel:       RiskLevelM,
		CurrentWaypoint: WaypointV2Retro,
		Status:          StatusV2Completed,
		CompletionDate:  &now,
		CreatedAt:       now,
		UpdatedAt:       now,
		WaypointHistory: []WaypointHistory{{
			Name:        WaypointV2Charter,
			Status:      WaypointStatusV2Completed,
			StartedAt:   now,
			CompletedAt: &now,
		}},
	}

	err := ValidateV2(st)
	if err == nil || !strings.Contains(err.Error(), "required Wayfinder phases are incomplete") {
		t.Fatalf("ValidateV2() error = %v, want incomplete completion rejection", err)
	}
}

func TestValidateV2LifecycleMetadata(t *testing.T) {
	newStatus := func(state string) *StatusV2 {
		now := time.Now()
		return &StatusV2{
			SchemaVersion:   SchemaVersion,
			ProjectName:     "Test",
			ProjectType:     ProjectTypeFeature,
			RiskLevel:       RiskLevelM,
			CurrentWaypoint: WaypointV2Build,
			Status:          StatusV2Blocked,
			LifecycleState:  state,
			BlockedReason:   "generic reason",
			CreatedAt:       now,
			UpdatedAt:       now,
			WaypointHistory: completedHistoryBefore(WaypointV2Build, now),
		}
	}

	tests := []struct {
		name      string
		state     string
		configure func(*StatusV2)
		wantErr   string
	}{
		{name: "input-required missing input", state: LifecycleInputRequired, wantErr: "requires input_needed"},
		{name: "input-required with input", state: LifecycleInputRequired, configure: func(st *StatusV2) { st.InputNeeded = "choose a database" }},
		{name: "dependency-blocked missing dependency", state: LifecycleDependencyBlocked, wantErr: "requires blocked_on"},
		{name: "dependency-blocked with dependency", state: LifecycleDependencyBlocked, configure: func(st *StatusV2) { st.BlockedOn = "ce-123" }},
		{name: "failed missing error", state: LifecycleFailed, wantErr: "requires error_message"},
		{name: "failed with error", state: LifecycleFailed, configure: func(st *StatusV2) { st.ErrorMessage = "tests failed" }},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			st := newStatus(tt.state)
			if tt.configure != nil {
				tt.configure(st)
			}
			err := ValidateV2(st)
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("ValidateV2() error = %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("ValidateV2() error = %v, want %q", err, tt.wantErr)
			}
		})
	}
}

func TestValidateV2RejectsUnsafeOrDuplicateSkipPhases(t *testing.T) {
	newValid := func() *StatusV2 {
		now := time.Now()
		return &StatusV2{
			SchemaVersion:   SchemaVersion,
			ProjectName:     "test",
			ProjectType:     ProjectTypeFeature,
			RiskLevel:       RiskLevelM,
			CurrentWaypoint: WaypointV2Charter,
			Status:          StatusV2Planning,
			CreatedAt:       now,
			UpdatedAt:       now,
		}
	}
	for _, test := range []struct {
		name   string
		phases []string
		want   string
	}{
		{name: "build", phases: []string{WaypointV2Build}, want: "unsafe phase"},
		{name: "empty", phases: []string{""}, want: "unsafe phase"},
		{name: "duplicate", phases: []string{WaypointV2Design, WaypointV2Design}, want: "duplicate phase"},
	} {
		t.Run(test.name, func(t *testing.T) {
			st := newValid()
			st.SkipPhases = test.phases
			if err := ValidateV2(st); err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("ValidateV2() error = %v, want %q", err, test.want)
			}
		})
	}
}

func TestValidateV2RejectsActiveHistoryForConfiguredSkip(t *testing.T) {
	now := time.Now()
	st := &StatusV2{
		SchemaVersion:   SchemaVersion,
		ProjectName:     "test",
		ProjectType:     ProjectTypeFeature,
		RiskLevel:       RiskLevelM,
		CurrentWaypoint: WaypointV2Design,
		Status:          StatusV2InProgress,
		SkipPhases:      []string{WaypointV2Design},
		CreatedAt:       now,
		UpdatedAt:       now,
		WaypointHistory: append(completedHistoryBefore(WaypointV2Design, now), WaypointHistory{
			Name:      WaypointV2Design,
			Status:    WaypointStatusV2InProgress,
			StartedAt: now,
		}),
	}

	err := ValidateV2(st)
	if err == nil || !strings.Contains(err.Error(), "configured skipped waypoint 'DESIGN' cannot have active status") {
		t.Fatalf("ValidateV2() error = %v, want active configured-skip rejection", err)
	}
}

func TestValidateV2RejectsUnresolvedSkippedCurrentWaypoint(t *testing.T) {
	now := time.Now()
	st := &StatusV2{
		SchemaVersion:   SchemaVersion,
		ProjectName:     "test",
		ProjectType:     ProjectTypeFeature,
		RiskLevel:       RiskLevelM,
		CurrentWaypoint: WaypointV2Design,
		Status:          StatusV2InProgress,
		SkipPhases:      []string{WaypointV2Design},
		CreatedAt:       now,
		UpdatedAt:       now,
		WaypointHistory: completedHistoryBefore(WaypointV2Design, now),
	}

	err := ValidateV2(st)
	if err == nil || !strings.Contains(err.Error(), "current_waypoint 'DESIGN' is configured to be skipped") {
		t.Fatalf("ValidateV2() error = %v, want unresolved skipped-current rejection", err)
	}
}

func TestValidateV2RejectsDuplicateWaypointHistory(t *testing.T) {
	now := time.Now()
	st := &StatusV2{
		SchemaVersion:   SchemaVersion,
		ProjectName:     "test",
		ProjectType:     ProjectTypeFeature,
		RiskLevel:       RiskLevelM,
		CurrentWaypoint: WaypointV2Charter,
		Status:          StatusV2InProgress,
		CreatedAt:       now,
		UpdatedAt:       now,
		WaypointHistory: []WaypointHistory{
			{Name: WaypointV2Charter, Status: WaypointStatusV2Completed, StartedAt: now, CompletedAt: &now},
			{Name: WaypointV2Charter, Status: WaypointStatusV2InProgress, StartedAt: now},
		},
	}

	if err := ValidateV2(st); err == nil || !strings.Contains(err.Error(), "duplicate waypoint name 'CHARTER'") {
		t.Fatalf("ValidateV2() error = %v, want duplicate CHARTER rejection", err)
	}
}

func TestValidateV2RejectsInvalidWaypointOutcome(t *testing.T) {
	now := time.Now()
	invalidOutcome := "typo"
	st := &StatusV2{
		SchemaVersion:   SchemaVersion,
		ProjectName:     "test",
		ProjectType:     ProjectTypeFeature,
		RiskLevel:       RiskLevelM,
		CurrentWaypoint: WaypointV2Charter,
		Status:          StatusV2InProgress,
		CreatedAt:       now,
		UpdatedAt:       now,
		WaypointHistory: []WaypointHistory{
			{Name: WaypointV2Charter, Status: WaypointStatusV2Completed, StartedAt: now, CompletedAt: &now, Outcome: &invalidOutcome},
		},
	}

	if err := ValidateV2(st); err == nil || !strings.Contains(err.Error(), "invalid outcome 'typo'") {
		t.Fatalf("ValidateV2() error = %v, want invalid outcome rejection", err)
	}
}

func TestValidateV2RejectsSkippedMandatoryWaypoint(t *testing.T) {
	now := time.Now()
	st := &StatusV2{
		SchemaVersion:   SchemaVersion,
		ProjectName:     "test",
		ProjectType:     ProjectTypeFeature,
		RiskLevel:       RiskLevelM,
		CurrentWaypoint: WaypointV2Retro,
		Status:          StatusV2InProgress,
		CreatedAt:       now,
		UpdatedAt:       now,
		WaypointHistory: []WaypointHistory{
			{Name: WaypointV2Build, Status: WaypointStatusV2Skipped, StartedAt: now},
		},
	}

	if err := ValidateV2(st); err == nil || !strings.Contains(err.Error(), "mandatory waypoint 'BUILD' cannot be skipped") {
		t.Fatalf("ValidateV2() error = %v, want mandatory BUILD skip rejection", err)
	}
}

func TestValidateV2RejectsMissingMandatoryPredecessors(t *testing.T) {
	now := time.Now()
	st := &StatusV2{
		SchemaVersion:   SchemaVersion,
		ProjectName:     "test",
		ProjectType:     ProjectTypeFeature,
		RiskLevel:       RiskLevelM,
		CurrentWaypoint: WaypointV2Build,
		Status:          StatusV2InProgress,
		CreatedAt:       now,
		UpdatedAt:       now,
		WaypointHistory: []WaypointHistory{
			{Name: WaypointV2Build, Status: WaypointStatusV2Completed, StartedAt: now, CompletedAt: &now},
		},
	}

	if err := ValidateV2(st); err == nil || !strings.Contains(err.Error(), "predecessor 'CHARTER' must be completed before 'BUILD'") {
		t.Fatalf("ValidateV2() error = %v, want missing predecessor rejection", err)
	}
}

func TestValidateV2RejectsMissingHistoryForLaterWaypoint(t *testing.T) {
	now := time.Now()
	st := &StatusV2{
		SchemaVersion:   SchemaVersion,
		ProjectName:     "test",
		ProjectType:     ProjectTypeFeature,
		RiskLevel:       RiskLevelM,
		CurrentWaypoint: WaypointV2Build,
		Status:          StatusV2InProgress,
		CreatedAt:       now,
		UpdatedAt:       now,
		WaypointHistory: nil,
	}

	if err := ValidateV2(st); err == nil || !strings.Contains(err.Error(), "current_waypoint 'BUILD' requires completed predecessor 'CHARTER'") {
		t.Fatalf("ValidateV2() error = %v, want nil-history predecessor rejection", err)
	}
}

func TestValidateV2RejectsHistoryAheadOfCurrentWaypoint(t *testing.T) {
	now := time.Now()
	st := &StatusV2{
		SchemaVersion:   SchemaVersion,
		ProjectName:     "test",
		ProjectType:     ProjectTypeFeature,
		RiskLevel:       RiskLevelM,
		CurrentWaypoint: WaypointV2Charter,
		Status:          StatusV2InProgress,
		CreatedAt:       now,
		UpdatedAt:       now,
		WaypointHistory: []WaypointHistory{
			{Name: WaypointV2Charter, Status: WaypointStatusV2Completed, StartedAt: now, CompletedAt: &now},
			{Name: WaypointV2Problem, Status: WaypointStatusV2Completed, StartedAt: now, CompletedAt: &now},
		},
	}

	if err := ValidateV2(st); err == nil || !strings.Contains(err.Error(), "waypoint 'PROBLEM' cannot be ahead of current_waypoint 'CHARTER'") {
		t.Fatalf("ValidateV2() error = %v, want history/current consistency rejection", err)
	}
}

func TestValidateV2AllowsConfiguredSkippedPredecessors(t *testing.T) {
	now := time.Now()
	st := &StatusV2{
		SchemaVersion:   SchemaVersion,
		ProjectName:     "lite",
		ProjectType:     ProjectTypeFeature,
		RiskLevel:       RiskLevelS,
		CurrentWaypoint: WaypointV2Build,
		Status:          StatusV2InProgress,
		CreatedAt:       now,
		UpdatedAt:       now,
		SkipRoadmap:     true,
		SkipPhases:      []string{WaypointV2Design, WaypointV2Spec, WaypointV2Plan},
		WaypointHistory: completedHistoryBefore(WaypointV2Design, now),
	}
	st.WaypointHistory = append(st.WaypointHistory, WaypointHistory{
		Name:      WaypointV2Build,
		Status:    WaypointStatusV2InProgress,
		StartedAt: now,
	})

	if err := ValidateV2(st); err != nil {
		t.Fatalf("ValidateV2() rejected configured skips: %v", err)
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
				SchemaVersion:   SchemaVersion,
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
			name: "whitespace-only task ID",
			roadmap: &Roadmap{
				Phases: []RoadmapPhase{
					{
						ID:     PhaseV2Setup,
						Name:   "Planning",
						Status: PhaseStatusV2Completed,
						Tasks: []Task{
							{ID: "  \t ", Title: "Task", Status: TaskStatusCompleted},
						},
					},
				},
			},
			wantErr: true,
			errMsg:  "task has empty ID",
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
			name: "duplicate phase IDs",
			roadmap: &Roadmap{Phases: []RoadmapPhase{
				{ID: PhaseV2Setup, Status: PhaseStatusV2Completed},
				{ID: PhaseV2Setup, Status: PhaseStatusV2Pending},
			}},
			wantErr: true,
			errMsg:  "duplicate waypoint_id",
		},
		{
			name: "non-finite task effort",
			roadmap: &Roadmap{Phases: []RoadmapPhase{{
				ID:     PhaseV2Setup,
				Status: PhaseStatusV2Pending,
				Tasks: []Task{{
					ID:         "task-1",
					Status:     TaskStatusPending,
					EffortDays: math.NaN(),
				}},
			}}},
			wantErr: true,
			errMsg:  "effort_days must be finite",
		},
		{
			name: "negative task effort",
			roadmap: &Roadmap{Phases: []RoadmapPhase{{
				ID:     PhaseV2Setup,
				Status: PhaseStatusV2Pending,
				Tasks: []Task{{
					ID:         "task-1",
					Status:     TaskStatusPending,
					EffortDays: -1,
				}},
			}}},
			wantErr: true,
			errMsg:  "effort_days cannot be negative",
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
				SchemaVersion:   SchemaVersion,
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
		{
			name: "non-finite metric",
			metrics: &QualityMetrics{
				CoveragePercent: math.NaN(),
			},
			wantErr: true,
			errMsg:  "coverage_percent must be finite",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			status := &StatusV2{
				SchemaVersion:   SchemaVersion,
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

func TestValidateBuildMetricsRejectsNonFiniteValues(t *testing.T) {
	tests := []struct {
		name     string
		waypoint string
		metrics  BuildMetrics
		want     string
	}{
		{
			name:     "NaN coverage on BUILD",
			waypoint: PhaseV2Build,
			metrics:  BuildMetrics{CoveragePercent: math.NaN()},
			want:     "build_metrics.coverage_percent must be finite",
		},
		{
			name:     "infinite assertion density on BUILD",
			waypoint: PhaseV2Build,
			metrics:  BuildMetrics{AssertionDensity: math.Inf(1)},
			want:     "build_metrics.assertion_density must be finite",
		},
		{
			name:     "NaN coverage on non-BUILD history",
			waypoint: PhaseV2Charter,
			metrics:  BuildMetrics{CoveragePercent: math.NaN()},
			want:     "waypoint_history[0] (CHARTER): build_metrics.coverage_percent must be finite",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateWaypointMetadata(WaypointHistory{
				Name:         tt.waypoint,
				BuildMetrics: &tt.metrics,
			}, 0)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("validateWaypointMetadata() error = %v, want error containing %q", err, tt.want)
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
