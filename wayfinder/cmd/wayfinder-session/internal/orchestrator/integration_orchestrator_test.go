package orchestrator

import (
	"testing"
	"time"

	"github.com/vbonnet/dear-agent/wayfinder/cmd/wayfinder-session/internal/status"
)

// TestIntegration_CompleteWorkflow tests end-to-end W0→S11 workflow
func TestIntegration_CompleteWorkflow(t *testing.T) {
	// Create a complete project
	st := &status.StatusV2{
		SchemaVersion:   status.SchemaVersionV2,
		ProjectName:     "integration-test-project",
		ProjectType:     status.ProjectTypeFeature,
		RiskLevel:       status.RiskLevelM,
		CurrentWaypoint: status.PhaseV2Charter,
		Status:          status.StatusV2InProgress,
		CreatedAt:       time.Now(),
		UpdatedAt:       time.Now(),
		WaypointHistory: []status.WaypointHistory{},
	}

	orch := NewPhaseOrchestratorV2(st)

	// Define complete workflow with deliverables
	workflow := []struct {
		phase        string
		deliverables []string
		setupFunc    func()
	}{
		{
			phase:        status.PhaseV2Charter,
			deliverables: []string{"W0-intake.md"},
			setupFunc:    func() {},
		},
		{
			phase:        status.PhaseV2Problem,
			deliverables: []string{"D1-discovery.md"},
			setupFunc:    func() {},
		},
		{
			phase:        status.PhaseV2Research,
			deliverables: []string{"D2-investigation.md"},
			setupFunc:    func() {},
		},
		{
			phase:        status.PhaseV2Design,
			deliverables: []string{"D3-architecture.md"},
			setupFunc:    func() {},
		},
		{
			phase:        status.PhaseV2Spec,
			deliverables: []string{"D4-requirements.md", "TESTS.outline"},
			setupFunc: func() {
				// Mark stakeholder approval
				for i := len(st.WaypointHistory) - 1; i >= 0; i-- {
					if st.WaypointHistory[i].Name == status.PhaseV2Spec {
						approved := true
						st.WaypointHistory[i].StakeholderApproved = &approved
						break
					}
				}
			},
		},
		{
			phase:        status.PhaseV2Plan,
			deliverables: []string{"S6-design.md", "TESTS.feature"},
			setupFunc: func() {
				// Mark TESTS.feature created
				for i := len(st.WaypointHistory) - 1; i >= 0; i-- {
					if st.WaypointHistory[i].Name == status.PhaseV2Plan {
						created := true
						st.WaypointHistory[i].TestsFeatureCreated = &created
						break
					}
				}
			},
		},
		{
			phase:        status.PhaseV2Setup,
			deliverables: []string{"S7-plan.md"},
			setupFunc: func() {
				// Add roadmap with tasks
				st.Roadmap = &status.Roadmap{
					Phases: []status.RoadmapPhase{
						{
							ID:   status.PhaseV2Build,
							Name: "BUILD Loop",
							Tasks: []status.Task{
								{
									ID:     "task-1",
									Title:  "Implement core feature",
									Status: status.TaskStatusCompleted,
								},
								{
									ID:     "task-2",
									Title:  "Add tests",
									Status: status.TaskStatusCompleted,
								},
							},
						},
					},
				}
			},
		},
		{
			phase:        status.PhaseV2Build,
			deliverables: []string{"S8-build.md"},
			setupFunc: func() {
				// Mark validation and deployment complete
				for i := len(st.WaypointHistory) - 1; i >= 0; i-- {
					if st.WaypointHistory[i].Name == status.PhaseV2Build {
						st.WaypointHistory[i].ValidationStatus = status.ValidationStatusPassed
						st.WaypointHistory[i].DeploymentStatus = status.DeploymentStatusDeployed
						st.WaypointHistory[i].BuildIterations = 3
						break
					}
				}
			},
		},
		{
			phase:        status.PhaseV2Retro,
			deliverables: []string{"S11-retrospective.md"},
			setupFunc: func() {
				st.Status = status.StatusV2Completed
				now := time.Now()
				st.CompletionDate = &now
			},
		},
	}

	// Execute workflow
	for i, step := range workflow {
		// Validate current phase
		if st.CurrentWaypoint != step.phase {
			t.Fatalf("step %d: expected phase %s, got %s", i, step.phase, st.CurrentWaypoint)
		}

		// Add deliverables
		markPhaseDeliverables(st, step.phase, step.deliverables)

		// Run setup function
		step.setupFunc()

		// Try to advance (should fail on last phase)
		if i < len(workflow)-1 {
			nextPhase, err := orch.AdvancePhase()
			if err != nil {
				t.Fatalf("step %d: failed to advance from %s: %v", i, step.phase, err)
			}

			expectedNext := workflow[i+1].phase
			if nextPhase != expectedNext {
				t.Errorf("step %d: expected next phase %s, got %s", i, expectedNext, nextPhase)
			}
		} else {
			// Last phase, should not advance
			_, err := orch.AdvancePhase()
			if err == nil {
				t.Error("expected error advancing beyond S11")
			}
		}
	}

	// Validate final state
	if st.CurrentWaypoint != status.PhaseV2Retro {
		t.Errorf("expected final phase S11, got %s", st.CurrentWaypoint)
	}

	if st.Status != status.StatusV2Completed {
		t.Errorf("expected completed status, got %s", st.Status)
	}

	if len(st.WaypointHistory) != len(workflow) {
		t.Errorf("expected %d history entries, got %d", len(workflow), len(st.WaypointHistory))
	}
}

// TestIntegration_WorkflowWithRewind tests workflow with rewind
func TestIntegration_WorkflowWithRewind(t *testing.T) {
	st := createTestStatusV2()
	st.CurrentWaypoint = status.PhaseV2Charter
	addCompletedPhase(st, status.PhaseV2Charter)

	orch := NewPhaseOrchestratorV2(st)

	// Manually progress through phases to S7
	// W0 already in history, advance to D1
	markPhaseDeliverables(st, status.PhaseV2Charter, []string{"W0-intake.md"})
	orch.AdvancePhase() // W0 -> D1

	// D1 -> D2
	markPhaseDeliverables(st, status.PhaseV2Problem, []string{"D1-discovery.md"})
	orch.AdvancePhase()

	// D2 -> D3
	markPhaseDeliverables(st, status.PhaseV2Research, []string{"D2-investigation.md"})
	orch.AdvancePhase()

	// D3 -> D4
	markPhaseDeliverables(st, status.PhaseV2Design, []string{"D3-architecture.md"})
	orch.AdvancePhase()

	// D4 -> S6
	markPhaseDeliverables(st, status.PhaseV2Spec, []string{"D4-requirements.md", "TESTS.outline"})
	orch.AdvancePhase()

	// S6 -> S7
	markPhaseDeliverables(st, status.PhaseV2Plan, []string{"S6-design.md", "TESTS.feature"})
	created := true
	for i := len(st.WaypointHistory) - 1; i >= 0; i-- {
		if st.WaypointHistory[i].Name == status.PhaseV2Plan {
			st.WaypointHistory[i].TestsFeatureCreated = &created
			break
		}
	}
	orch.AdvancePhase()

	// At S7 now
	markPhaseDeliverables(st, status.PhaseV2Setup, []string{"S7-plan.md"})
	st.Roadmap = &status.Roadmap{
		Phases: []status.RoadmapPhase{
			{
				ID:    status.PhaseV2Build,
				Tasks: []status.Task{{ID: "task-1", Status: status.TaskStatusCompleted}},
			},
		},
	}

	// Verify at S7
	if st.CurrentWaypoint != status.PhaseV2Setup {
		t.Fatalf("expected S7, got %s", st.CurrentWaypoint)
	}

	historyBeforeRewind := len(st.WaypointHistory)

	// Rewind to S6
	err := orch.RewindPhase(status.PhaseV2Plan, "Need to revise design")
	if err != nil {
		t.Fatalf("rewind failed: %v", err)
	}

	// Verify rewind
	if st.CurrentWaypoint != status.PhaseV2Plan {
		t.Errorf("expected S6 after rewind, got %s", st.CurrentWaypoint)
	}

	if len(st.WaypointHistory) != historyBeforeRewind+1 {
		t.Errorf("expected history to grow by 1, got %d", len(st.WaypointHistory)-historyBeforeRewind)
	}

	// Update design and advance again
	markPhaseDeliverables(st, status.PhaseV2Plan, []string{"S6-design.md", "TESTS.feature"})
	testsCreated := true
	for i := len(st.WaypointHistory) - 1; i >= 0; i-- {
		if st.WaypointHistory[i].Name == status.PhaseV2Plan {
			st.WaypointHistory[i].TestsFeatureCreated = &testsCreated
			break
		}
	}

	nextPhase, err := orch.AdvancePhase()
	if err != nil {
		t.Fatalf("failed to advance after rewind: %v", err)
	}

	if nextPhase != status.PhaseV2Setup {
		t.Errorf("expected S7 after rewind+advance, got %s", nextPhase)
	}
}

// TestIntegration_BuildLoopWithOrchestrator tests BUILD loop integration
func TestIntegration_BuildLoopWithOrchestrator(t *testing.T) {
	st := createTestStatusV2()
	st.CurrentWaypoint = status.PhaseV2Build
	addCompletedPhase(st, status.PhaseV2Build)

	// Set up roadmap with multiple tasks
	st.Roadmap = &status.Roadmap{
		Phases: []status.RoadmapPhase{
			{
				ID:   status.PhaseV2Build,
				Name: "BUILD Loop",
				Tasks: []status.Task{
					{ID: "task-1", Title: "Task 1", Status: status.TaskStatusPending},
					{ID: "task-2", Title: "Task 2", Status: status.TaskStatusPending, DependsOn: []string{"task-1"}},
				},
			},
		},
	}

	orch := NewPhaseOrchestratorV2(st)
	executor := NewBuildLoopExecutor(orch)

	// Start BUILD loop
	err := executor.StartBuildLoop()
	if err != nil {
		t.Fatalf("failed to start BUILD loop: %v", err)
	}

	// Execute BUILD loop for first task
	for executor.GetCurrentState() != BuildLoopStateTaskComplete {
		state := executor.GetCurrentState()

		switch state {
		case BuildLoopStateTestingPre:
			executor.RecordTestResult(TestResult{
				Passed:      false,
				TotalTests:  5,
				FailedTests: 5,
				Timestamp:   time.Now(),
			})
		case BuildLoopStateTestingPost:
			executor.RecordTestResult(TestResult{
				Passed:      true,
				TotalTests:  5,
				PassedTests: 5,
				Timestamp:   time.Now(),
			})
		}

		executor.AdvanceState()
	}

	// Mark task-1 complete in roadmap
	st.Roadmap.Phases[0].Tasks[0].Status = status.TaskStatusCompleted

	// Continue to task-2
	executor.AdvanceState()

	currentTask, _ := executor.GetCurrentTask()
	if currentTask != "task-2" {
		t.Errorf("expected task-2, got %s", currentTask)
	}

	// Complete task-2
	for executor.GetCurrentState() != BuildLoopStateTaskComplete {
		state := executor.GetCurrentState()

		switch state {
		case BuildLoopStateTestingPre:
			executor.RecordTestResult(TestResult{Passed: false, Timestamp: time.Now()})
		case BuildLoopStateTestingPost:
			executor.RecordTestResult(TestResult{Passed: true, TotalTests: 8, PassedTests: 8, Timestamp: time.Now()})
		}

		executor.AdvanceState()
	}

	// Advance to integration testing
	executor.AdvanceState()

	if executor.GetCurrentState() != BuildLoopStateIntegrationTesting {
		t.Errorf("expected integration testing, got %s", executor.GetCurrentState())
	}

	// Complete integration and deployment
	executor.MarkIntegrationTestsComplete()
	executor.AdvanceState()

	executor.MarkDeploymentComplete()
	executor.AdvanceState()

	// Verify BUILD loop complete
	if !executor.IsBuildLoopComplete() {
		t.Error("BUILD loop should be complete")
	}

	// Update phase history
	executor.UpdatePhaseHistory()

	// Mark all tasks as completed in roadmap
	st.Roadmap.Phases[0].Tasks[0].Status = status.TaskStatusCompleted
	st.Roadmap.Phases[0].Tasks[1].Status = status.TaskStatusCompleted

	// Mark S8 deliverables and complete
	markPhaseDeliverables(st, status.PhaseV2Build, []string{"S8-build.md"})

	// Verify can advance to S11
	nextPhase, err := orch.AdvancePhase()
	if err != nil {
		t.Fatalf("failed to advance to S11: %v", err)
	}

	if nextPhase != status.PhaseV2Retro {
		t.Errorf("expected S11, got %s", nextPhase)
	}
}

// TestIntegration_ValidationFailures tests validation failure scenarios
func TestIntegration_ValidationFailures(t *testing.T) {
	testCases := []struct {
		name      string
		phase     string
		setupFunc func(*status.StatusV2)
		expectErr bool
	}{
		{
			name:  "S6 to S7 without TESTS.feature",
			phase: status.PhaseV2Plan,
			setupFunc: func(st *status.StatusV2) {
				markPhaseDeliverables(st, status.PhaseV2Plan, []string{"S6-design.md"})
				// Don't set TestsFeatureCreated
			},
			expectErr: true,
		},
		{
			name:  "S7 to S8 without roadmap tasks",
			phase: status.PhaseV2Setup,
			setupFunc: func(st *status.StatusV2) {
				markPhaseDeliverables(st, status.PhaseV2Setup, []string{"S7-plan.md"})
				// Don't add roadmap
			},
			expectErr: true,
		},
		{
			name:  "S8 to S11 without deployment",
			phase: status.PhaseV2Build,
			setupFunc: func(st *status.StatusV2) {
				markPhaseDeliverables(st, status.PhaseV2Build, []string{"S8-build.md"})
				st.Roadmap = &status.Roadmap{
					Phases: []status.RoadmapPhase{
						{ID: status.PhaseV2Build, Tasks: []status.Task{{ID: "t1", Status: status.TaskStatusCompleted}}},
					},
				}
				// Set validation but not deployment
				for i := len(st.WaypointHistory) - 1; i >= 0; i-- {
					if st.WaypointHistory[i].Name == status.PhaseV2Build {
						st.WaypointHistory[i].ValidationStatus = status.ValidationStatusPassed
						break
					}
				}
			},
			expectErr: true,
		},
		{
			name:  "S8 to S11 with P0 issues",
			phase: status.PhaseV2Build,
			setupFunc: func(st *status.StatusV2) {
				markPhaseDeliverables(st, status.PhaseV2Build, []string{"S8-build.md"})
				st.Roadmap = &status.Roadmap{
					Phases: []status.RoadmapPhase{
						{ID: status.PhaseV2Build, Tasks: []status.Task{{ID: "t1", Status: status.TaskStatusCompleted}}},
					},
				}
				for i := len(st.WaypointHistory) - 1; i >= 0; i-- {
					if st.WaypointHistory[i].Name == status.PhaseV2Build {
						st.WaypointHistory[i].ValidationStatus = status.ValidationStatusPassed
						st.WaypointHistory[i].DeploymentStatus = status.DeploymentStatusDeployed
						break
					}
				}
				// Add P0 issues
				st.QualityMetrics = &status.QualityMetrics{
					P0Issues: 2,
				}
			},
			expectErr: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			st := createTestStatusV2()
			st.CurrentWaypoint = tc.phase
			addCompletedPhase(st, tc.phase)

			tc.setupFunc(st)

			orch := NewPhaseOrchestratorV2(st)

			_, err := orch.AdvancePhase()

			if tc.expectErr && err == nil {
				t.Error("expected error, got nil")
			}
			if !tc.expectErr && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

// TestIntegration_PhaseHistory tests phase history tracking
func TestIntegration_PhaseHistory(t *testing.T) {
	st := createTestStatusV2()
	st.CurrentWaypoint = status.PhaseV2Charter
	addCompletedPhase(st, status.PhaseV2Charter)

	orch := NewPhaseOrchestratorV2(st)

	// Advance through several phases: W0 -> D1 -> D2 -> D3
	markPhaseDeliverables(st, status.PhaseV2Charter, []string{"W0-intake.md"})
	orch.AdvancePhase() // W0 -> D1

	markPhaseDeliverables(st, status.PhaseV2Problem, []string{"D1-discovery.md"})
	orch.AdvancePhase() // D1 -> D2

	markPhaseDeliverables(st, status.PhaseV2Research, []string{"D2-investigation.md"})
	orch.AdvancePhase() // D2 -> D3

	markPhaseDeliverables(st, status.PhaseV2Design, []string{"D3-architecture.md"})

	// Check history
	history := orch.GetPhaseHistory()

	// After advancing through phases, history should include:
	// - Initial W0 entry (completed)
	// - D1 entry (completed)
	// - D2 entry (completed)
	// - D3 entry (in-progress, current)
	expectedCount := 4 // W0, D1, D2, D3
	if len(history) != expectedCount {
		t.Errorf("expected %d history entries, got %d", expectedCount, len(history))
	}

	// Verify W0, D1, D2 are completed
	completedPhases := []string{status.PhaseV2Charter, status.PhaseV2Problem, status.PhaseV2Research}
	for _, phase := range completedPhases {
		found := false
		for _, entry := range history {
			if entry.Name == phase && entry.Status == status.PhaseStatusV2Completed {
				found = true
				if entry.CompletedAt == nil {
					t.Errorf("phase %s: expected completion time", phase)
				}
				break
			}
		}
		if !found {
			t.Errorf("phase %s not found as completed in history", phase)
		}
	}

	// Verify D3 is in progress (current phase)
	found := false
	for _, entry := range history {
		if entry.Name == status.PhaseV2Design && entry.Status == status.PhaseStatusV2InProgress {
			found = true
			break
		}
	}
	if !found {
		t.Error("D3 not found as in-progress in history")
	}
}
