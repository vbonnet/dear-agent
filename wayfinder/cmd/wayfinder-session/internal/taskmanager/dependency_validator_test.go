package taskmanager

import (
	"testing"

	"github.com/vbonnet/dear-agent/wayfinder/cmd/wayfinder-session/internal/status"
)

func TestDependencyValidator_ValidateTask(t *testing.T) {
	tests := []struct {
		name      string
		setup     func() *status.StatusV2
		task      *status.Task
		wantErr   bool
		errString string
	}{
		{
			name: "valid task with no dependencies",
			setup: func() *status.StatusV2 {
				return status.NewStatusV2("Test", status.ProjectTypeFeature, status.RiskLevelM)
			},
			task: &status.Task{
				ID:    "S8-1",
				Title: "Task 1",
			},
			wantErr: false,
		},
		{
			name: "valid task with existing dependency",
			setup: func() *status.StatusV2 {
				st := status.NewStatusV2("Test", status.ProjectTypeFeature, status.RiskLevelM)
				st.Roadmap = &status.Roadmap{
					Phases: []status.RoadmapPhase{
						{
							ID:   "S8",
							Name: "BUILD Loop",
							Tasks: []status.Task{
								{ID: "S8-1", Title: "Task 1"},
							},
						},
					},
				}
				return st
			},
			task: &status.Task{
				ID:        "S8-2",
				Title:     "Task 2",
				DependsOn: []string{"S8-1"},
			},
			wantErr: false,
		},
		{
			name: "invalid dependency - task not found",
			setup: func() *status.StatusV2 {
				return status.NewStatusV2("Test", status.ProjectTypeFeature, status.RiskLevelM)
			},
			task: &status.Task{
				ID:        "S8-1",
				Title:     "Task 1",
				DependsOn: []string{"INVALID"},
			},
			wantErr:   true,
			errString: "dependency task not found",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			st := tt.setup()
			validator := NewDependencyValidator(st)

			err := validator.ValidateTask(tt.task)

			if tt.wantErr {
				if err == nil {
					t.Errorf("expected error containing %q, got nil", tt.errString)
					return
				}
				if tt.errString != "" && !contains(err.Error(), tt.errString) {
					t.Errorf("expected error containing %q, got %q", tt.errString, err.Error())
				}
				return
			}

			if err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

func TestDependencyValidator_DetectCycles(t *testing.T) {
	tests := []struct {
		name      string
		setup     func() *status.StatusV2
		wantErr   bool
		errString string
	}{
		{
			name: "no cycles - linear dependency",
			setup: func() *status.StatusV2 {
				st := status.NewStatusV2("Test", status.ProjectTypeFeature, status.RiskLevelM)
				st.Roadmap = &status.Roadmap{
					Phases: []status.RoadmapPhase{
						{
							ID:   "S8",
							Name: "BUILD Loop",
							Tasks: []status.Task{
								{ID: "S8-1", Title: "Task 1", DependsOn: []string{}},
								{ID: "S8-2", Title: "Task 2", DependsOn: []string{"S8-1"}},
								{ID: "S8-3", Title: "Task 3", DependsOn: []string{"S8-2"}},
							},
						},
					},
				}
				return st
			},
			wantErr: false,
		},
		{
			name: "no cycles - multiple dependencies",
			setup: func() *status.StatusV2 {
				st := status.NewStatusV2("Test", status.ProjectTypeFeature, status.RiskLevelM)
				st.Roadmap = &status.Roadmap{
					Phases: []status.RoadmapPhase{
						{
							ID:   "S8",
							Name: "BUILD Loop",
							Tasks: []status.Task{
								{ID: "S8-1", Title: "Task 1", DependsOn: []string{}},
								{ID: "S8-2", Title: "Task 2", DependsOn: []string{}},
								{ID: "S8-3", Title: "Task 3", DependsOn: []string{"S8-1", "S8-2"}},
							},
						},
					},
				}
				return st
			},
			wantErr: false,
		},
		{
			name: "cycle - self dependency",
			setup: func() *status.StatusV2 {
				st := status.NewStatusV2("Test", status.ProjectTypeFeature, status.RiskLevelM)
				st.Roadmap = &status.Roadmap{
					Phases: []status.RoadmapPhase{
						{
							ID:   "S8",
							Name: "BUILD Loop",
							Tasks: []status.Task{
								{ID: "S8-1", Title: "Task 1", DependsOn: []string{"S8-1"}},
							},
						},
					},
				}
				return st
			},
			wantErr:   true,
			errString: "circular dependency",
		},
		{
			name: "cycle - two tasks",
			setup: func() *status.StatusV2 {
				st := status.NewStatusV2("Test", status.ProjectTypeFeature, status.RiskLevelM)
				st.Roadmap = &status.Roadmap{
					Phases: []status.RoadmapPhase{
						{
							ID:   "S8",
							Name: "BUILD Loop",
							Tasks: []status.Task{
								{ID: "S8-1", Title: "Task 1", DependsOn: []string{"S8-2"}},
								{ID: "S8-2", Title: "Task 2", DependsOn: []string{"S8-1"}},
							},
						},
					},
				}
				return st
			},
			wantErr:   true,
			errString: "circular dependency",
		},
		{
			name: "cycle - three tasks",
			setup: func() *status.StatusV2 {
				st := status.NewStatusV2("Test", status.ProjectTypeFeature, status.RiskLevelM)
				st.Roadmap = &status.Roadmap{
					Phases: []status.RoadmapPhase{
						{
							ID:   "S8",
							Name: "BUILD Loop",
							Tasks: []status.Task{
								{ID: "S8-1", Title: "Task 1", DependsOn: []string{"S8-3"}},
								{ID: "S8-2", Title: "Task 2", DependsOn: []string{"S8-1"}},
								{ID: "S8-3", Title: "Task 3", DependsOn: []string{"S8-2"}},
							},
						},
					},
				}
				return st
			},
			wantErr:   true,
			errString: "circular dependency",
		},
		{
			name: "complex graph no cycle",
			setup: func() *status.StatusV2 {
				st := status.NewStatusV2("Test", status.ProjectTypeFeature, status.RiskLevelM)
				st.Roadmap = &status.Roadmap{
					Phases: []status.RoadmapPhase{
						{
							ID:   "S8",
							Name: "BUILD Loop",
							Tasks: []status.Task{
								{ID: "S8-1", Title: "Task 1", DependsOn: []string{}},
								{ID: "S8-2", Title: "Task 2", DependsOn: []string{"S8-1"}},
								{ID: "S8-3", Title: "Task 3", DependsOn: []string{"S8-1"}},
								{ID: "S8-4", Title: "Task 4", DependsOn: []string{"S8-2", "S8-3"}},
							},
						},
					},
				}
				return st
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			st := tt.setup()
			validator := NewDependencyValidator(st)

			err := validator.detectCycles()

			if tt.wantErr {
				if err == nil {
					t.Errorf("expected error containing %q, got nil", tt.errString)
					return
				}
				if tt.errString != "" && !contains(err.Error(), tt.errString) {
					t.Errorf("expected error containing %q, got %q", tt.errString, err.Error())
				}
				return
			}

			if err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

func TestDependencyValidator_GetDependencyChain(t *testing.T) {
	st := status.NewStatusV2("Test", status.ProjectTypeFeature, status.RiskLevelM)
	st.Roadmap = &status.Roadmap{
		Phases: []status.RoadmapPhase{
			{
				ID:   "S8",
				Name: "BUILD Loop",
				Tasks: []status.Task{
					{ID: "S8-1", Title: "Task 1", DependsOn: []string{}},
					{ID: "S8-2", Title: "Task 2", DependsOn: []string{"S8-1"}},
					{ID: "S8-3", Title: "Task 3", DependsOn: []string{"S8-2"}},
					{ID: "S8-4", Title: "Task 4", DependsOn: []string{"S8-1", "S8-3"}},
				},
			},
		},
	}

	validator := NewDependencyValidator(st)

	tests := []struct {
		name      string
		taskID    string
		wantChain []string
		wantErr   bool
	}{
		{
			name:      "task with no dependencies",
			taskID:    "S8-1",
			wantChain: []string{},
			wantErr:   false,
		},
		{
			name:      "task with one dependency",
			taskID:    "S8-2",
			wantChain: []string{"S8-1"},
			wantErr:   false,
		},
		{
			name:      "task with transitive dependencies",
			taskID:    "S8-3",
			wantChain: []string{"S8-2", "S8-1"},
			wantErr:   false,
		},
		{
			name:      "task with multiple dependencies",
			taskID:    "S8-4",
			wantChain: []string{"S8-1", "S8-3", "S8-2", "S8-1"}, // S8-1 appears twice (direct + transitive)
			wantErr:   false,
		},
		{
			name:    "non-existent task",
			taskID:  "INVALID",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			chain, err := validator.GetDependencyChain(tt.taskID)

			if tt.wantErr {
				if err == nil {
					t.Error("expected error, got nil")
				}
				return
			}

			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}

			if len(chain) != len(tt.wantChain) {
				t.Errorf("expected chain length %d, got %d", len(tt.wantChain), len(chain))
				return
			}

			// Convert to sets for comparison (order may vary)
			chainSet := make(map[string]bool)
			for _, id := range chain {
				chainSet[id] = true
			}

			for _, id := range tt.wantChain {
				if !chainSet[id] {
					t.Errorf("expected %s in chain, but not found", id)
				}
			}
		})
	}
}

func TestDependencyValidator_GetBlockedBy(t *testing.T) {
	st := status.NewStatusV2("Test", status.ProjectTypeFeature, status.RiskLevelM)
	st.Roadmap = &status.Roadmap{
		Phases: []status.RoadmapPhase{
			{
				ID:   "S8",
				Name: "BUILD Loop",
				Tasks: []status.Task{
					{ID: "S8-1", Title: "Task 1", DependsOn: []string{}},
					{ID: "S8-2", Title: "Task 2", DependsOn: []string{"S8-1"}},
					{ID: "S8-3", Title: "Task 3", DependsOn: []string{"S8-1", "S8-2"}},
				},
			},
		},
	}

	validator := NewDependencyValidator(st)

	tests := []struct {
		name       string
		taskID     string
		wantBlocks []string
	}{
		{
			name:       "task with no dependencies",
			taskID:     "S8-1",
			wantBlocks: []string{},
		},
		{
			name:       "task with one dependency",
			taskID:     "S8-2",
			wantBlocks: []string{"S8-1"},
		},
		{
			name:       "task with multiple dependencies",
			taskID:     "S8-3",
			wantBlocks: []string{"S8-1", "S8-2"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			blockedBy := validator.GetBlockedBy(tt.taskID)

			if len(blockedBy) != len(tt.wantBlocks) {
				t.Errorf("expected %d blockers, got %d", len(tt.wantBlocks), len(blockedBy))
				return
			}

			blockedBySet := make(map[string]bool)
			for _, id := range blockedBy {
				blockedBySet[id] = true
			}

			for _, id := range tt.wantBlocks {
				if !blockedBySet[id] {
					t.Errorf("expected %s in blockedBy, but not found", id)
				}
			}
		})
	}
}

func TestDependencyValidator_GetBlocks(t *testing.T) {
	st := status.NewStatusV2("Test", status.ProjectTypeFeature, status.RiskLevelM)
	st.Roadmap = &status.Roadmap{
		Phases: []status.RoadmapPhase{
			{
				ID:   "S8",
				Name: "BUILD Loop",
				Tasks: []status.Task{
					{ID: "S8-1", Title: "Task 1", DependsOn: []string{}},
					{ID: "S8-2", Title: "Task 2", DependsOn: []string{"S8-1"}},
					{ID: "S8-3", Title: "Task 3", DependsOn: []string{"S8-1"}},
					{ID: "S8-4", Title: "Task 4", DependsOn: []string{"S8-2"}},
				},
			},
		},
	}

	validator := NewDependencyValidator(st)

	tests := []struct {
		name       string
		taskID     string
		wantBlocks []string
	}{
		{
			name:       "task that blocks multiple",
			taskID:     "S8-1",
			wantBlocks: []string{"S8-2", "S8-3"},
		},
		{
			name:       "task that blocks one",
			taskID:     "S8-2",
			wantBlocks: []string{"S8-4"},
		},
		{
			name:       "task that blocks none",
			taskID:     "S8-3",
			wantBlocks: []string{},
		},
		{
			name:       "task that blocks none",
			taskID:     "S8-4",
			wantBlocks: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			blocks := validator.GetBlocks(tt.taskID)

			if len(blocks) != len(tt.wantBlocks) {
				t.Errorf("expected %d blocks, got %d", len(tt.wantBlocks), len(blocks))
				return
			}

			blocksSet := make(map[string]bool)
			for _, id := range blocks {
				blocksSet[id] = true
			}

			for _, id := range tt.wantBlocks {
				if !blocksSet[id] {
					t.Errorf("expected %s in blocks, but not found", id)
				}
			}
		})
	}
}

func TestDependencyValidator_ValidateAll(t *testing.T) {
	tests := []struct {
		name      string
		setup     func() *status.StatusV2
		wantErr   bool
		errString string
	}{
		{
			name: "all valid",
			setup: func() *status.StatusV2 {
				st := status.NewStatusV2("Test", status.ProjectTypeFeature, status.RiskLevelM)
				st.Roadmap = &status.Roadmap{
					Phases: []status.RoadmapPhase{
						{
							ID:   "S8",
							Name: "BUILD Loop",
							Tasks: []status.Task{
								{ID: "S8-1", Title: "Task 1", DependsOn: []string{}},
								{ID: "S8-2", Title: "Task 2", DependsOn: []string{"S8-1"}},
							},
						},
					},
				}
				return st
			},
			wantErr: false,
		},
		{
			name: "invalid dependency",
			setup: func() *status.StatusV2 {
				st := status.NewStatusV2("Test", status.ProjectTypeFeature, status.RiskLevelM)
				st.Roadmap = &status.Roadmap{
					Phases: []status.RoadmapPhase{
						{
							ID:   "S8",
							Name: "BUILD Loop",
							Tasks: []status.Task{
								{ID: "S8-1", Title: "Task 1", DependsOn: []string{"INVALID"}},
							},
						},
					},
				}
				return st
			},
			wantErr:   true,
			errString: "invalid dependency",
		},
		{
			name: "cycle detected",
			setup: func() *status.StatusV2 {
				st := status.NewStatusV2("Test", status.ProjectTypeFeature, status.RiskLevelM)
				st.Roadmap = &status.Roadmap{
					Phases: []status.RoadmapPhase{
						{
							ID:   "S8",
							Name: "BUILD Loop",
							Tasks: []status.Task{
								{ID: "S8-1", Title: "Task 1", DependsOn: []string{"S8-2"}},
								{ID: "S8-2", Title: "Task 2", DependsOn: []string{"S8-1"}},
							},
						},
					},
				}
				return st
			},
			wantErr:   true,
			errString: "circular dependency",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			st := tt.setup()
			validator := NewDependencyValidator(st)

			err := validator.ValidateAll()

			if tt.wantErr {
				if err == nil {
					t.Errorf("expected error containing %q, got nil", tt.errString)
					return
				}
				if tt.errString != "" && !contains(err.Error(), tt.errString) {
					t.Errorf("expected error containing %q, got %q", tt.errString, err.Error())
				}
				return
			}

			if err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}
