package orchestrator

import (
	"testing"
	"time"

	"github.com/vbonnet/dear-agent/wayfinder/cmd/wayfinder-session/internal/status"
)

// TestNewBuildLoopExecutor tests BUILD loop executor creation
func TestNewBuildLoopExecutor(t *testing.T) {
	st := createTestStatusV2()
	orch := NewPhaseOrchestratorV2(st)
	executor := NewBuildLoopExecutor(orch)

	if executor == nil {
		t.Fatal("expected executor, got nil")
	}

	if executor.context.CurrentState != BuildLoopStateIdle {
		t.Errorf("expected idle state, got %s", executor.context.CurrentState)
	}
}

// TestStartBuildLoop tests BUILD loop initialization
func TestStartBuildLoop(t *testing.T) {
	st := createTestStatusV2()
	st.CurrentWaypoint = status.PhaseV2Build
	st.Roadmap = &status.Roadmap{
		Phases: []status.RoadmapPhase{
			{
				ID:   status.PhaseV2Build,
				Name: "BUILD Loop",
				Tasks: []status.Task{
					{ID: "task-1", Title: "Task 1", Status: status.TaskStatusPending},
					{ID: "task-2", Title: "Task 2", Status: status.TaskStatusPending},
				},
			},
		},
	}

	orch := NewPhaseOrchestratorV2(st)
	executor := NewBuildLoopExecutor(orch)

	err := executor.StartBuildLoop()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if executor.context.CurrentState != BuildLoopStateTestingPre {
		t.Errorf("expected testing-pre state, got %s", executor.context.CurrentState)
	}

	if executor.context.TotalTasks != 2 {
		t.Errorf("expected 2 tasks, got %d", executor.context.TotalTasks)
	}

	if executor.context.CurrentTaskID != "task-1" {
		t.Errorf("expected task-1, got %s", executor.context.CurrentTaskID)
	}
}

// TestStartBuildLoop_WrongPhase tests error when not in S8
func TestStartBuildLoop_WrongPhase(t *testing.T) {
	st := createTestStatusV2()
	st.CurrentWaypoint = status.PhaseV2Spec // Wrong phase

	orch := NewPhaseOrchestratorV2(st)
	executor := NewBuildLoopExecutor(orch)

	err := executor.StartBuildLoop()
	if err == nil {
		t.Error("expected error for wrong phase, got nil")
	}
}

// TestStartBuildLoop_NoTasks tests error when no tasks defined
func TestStartBuildLoop_NoTasks(t *testing.T) {
	st := createTestStatusV2()
	st.CurrentWaypoint = status.PhaseV2Build
	st.Roadmap = &status.Roadmap{
		Phases: []status.RoadmapPhase{
			{
				ID:    status.PhaseV2Build,
				Name:  "BUILD Loop",
				Tasks: []status.Task{}, // No tasks
			},
		},
	}

	orch := NewPhaseOrchestratorV2(st)
	executor := NewBuildLoopExecutor(orch)

	err := executor.StartBuildLoop()
	if err == nil {
		t.Error("expected error for no tasks, got nil")
	}
}

// TestBuildLoopStateTransitions tests state machine transitions
func TestBuildLoopStateTransitions(t *testing.T) {
	st := createTestStatusV2()
	st.CurrentWaypoint = status.PhaseV2Build
	st.Roadmap = &status.Roadmap{
		Phases: []status.RoadmapPhase{
			{
				ID:   status.PhaseV2Build,
				Name: "BUILD Loop",
				Tasks: []status.Task{
					{ID: "task-1", Title: "Task 1", Status: status.TaskStatusPending},
				},
			},
		},
	}

	orch := NewPhaseOrchestratorV2(st)
	executor := NewBuildLoopExecutor(orch)

	// Start BUILD loop
	if err := executor.StartBuildLoop(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	expectedTransitions := []BuildLoopState{
		BuildLoopStateCoding,             // After testing-pre
		BuildLoopStateTestingPost,        // After coding
		BuildLoopStateValidating,         // After testing-post
		BuildLoopStateTaskComplete,       // After validating
		BuildLoopStateIntegrationTesting, // After task complete (no more tasks)
		BuildLoopStateDeploying,          // After integration testing
		BuildLoopStateComplete,           // After deploying
	}

	for i, expected := range expectedTransitions {
		// Record test result if needed
		if executor.context.CurrentState == BuildLoopStateTestingPre {
			// Pre-test should fail
			executor.RecordTestResult(TestResult{
				Passed:      false,
				TotalTests:  10,
				FailedTests: 5,
				Timestamp:   time.Now(),
			})
		}
		if executor.context.CurrentState == BuildLoopStateTestingPost {
			// Post-test should pass
			executor.RecordTestResult(TestResult{
				Passed:      true,
				TotalTests:  10,
				PassedTests: 10,
				Timestamp:   time.Now(),
			})
		}
		if executor.context.CurrentState == BuildLoopStateIntegrationTesting {
			executor.MarkIntegrationTestsComplete()
		}
		if executor.context.CurrentState == BuildLoopStateDeploying {
			executor.MarkDeploymentComplete()
		}

		newState, err := executor.AdvanceState()
		if err != nil {
			t.Fatalf("step %d: unexpected error: %v", i, err)
		}

		if newState != expected {
			t.Errorf("step %d: expected state %s, got %s", i, expected, newState)
		}
	}

	// Verify final state
	if !executor.IsBuildLoopComplete() {
		t.Error("BUILD loop should be complete")
	}
}

// TestRecordTestResult_TDDViolation tests TDD discipline enforcement
func TestRecordTestResult_TDDViolation(t *testing.T) {
	st := createTestStatusV2()
	st.CurrentWaypoint = status.PhaseV2Build
	st.Roadmap = &status.Roadmap{
		Phases: []status.RoadmapPhase{
			{
				ID:   status.PhaseV2Build,
				Name: "BUILD Loop",
				Tasks: []status.Task{
					{ID: "task-1", Title: "Task 1", Status: status.TaskStatusPending},
				},
			},
		},
	}

	orch := NewPhaseOrchestratorV2(st)
	executor := NewBuildLoopExecutor(orch)

	executor.StartBuildLoop()

	// In testing-pre state, tests MUST fail
	err := executor.RecordTestResult(TestResult{
		Passed:      true, // VIOLATION: tests passed before implementation
		TotalTests:  10,
		PassedTests: 10,
		Timestamp:   time.Now(),
	})

	if err == nil {
		t.Error("expected TDD violation error, got nil")
	}

	if !contains(err.Error(), "TDD violation") {
		t.Errorf("expected TDD violation in error, got: %v", err)
	}
}

// TestRecordTestResult_PostTestFailure tests post-implementation test failure
func TestRecordTestResult_PostTestFailure(t *testing.T) {
	st := createTestStatusV2()
	st.CurrentWaypoint = status.PhaseV2Build
	st.Roadmap = &status.Roadmap{
		Phases: []status.RoadmapPhase{
			{
				ID:   status.PhaseV2Build,
				Name: "BUILD Loop",
				Tasks: []status.Task{
					{ID: "task-1", Title: "Task 1", Status: status.TaskStatusPending},
				},
			},
		},
	}

	orch := NewPhaseOrchestratorV2(st)
	executor := NewBuildLoopExecutor(orch)

	executor.StartBuildLoop()

	// Advance to testing-post
	executor.RecordTestResult(TestResult{Passed: false, Timestamp: time.Now()})
	executor.AdvanceState() // to coding
	executor.AdvanceState() // to testing-post

	// In testing-post state, tests MUST pass
	err := executor.RecordTestResult(TestResult{
		Passed:      false, // VIOLATION: tests failed after implementation
		TotalTests:  10,
		FailedTests: 3,
		Timestamp:   time.Now(),
	})

	if err == nil {
		t.Error("expected test failure error, got nil")
	}

	if !contains(err.Error(), "tests failed after implementation") {
		t.Errorf("expected test failure in error, got: %v", err)
	}
}

// TestBuildLoopWithMultipleTasks tests iteration through multiple tasks
func TestBuildLoopWithMultipleTasks(t *testing.T) {
	st := createTestStatusV2()
	st.CurrentWaypoint = status.PhaseV2Build
	st.Roadmap = &status.Roadmap{
		Phases: []status.RoadmapPhase{
			{
				ID:   status.PhaseV2Build,
				Name: "BUILD Loop",
				Tasks: []status.Task{
					{ID: "task-1", Title: "Task 1", Status: status.TaskStatusPending},
					{ID: "task-2", Title: "Task 2", Status: status.TaskStatusPending},
					{ID: "task-3", Title: "Task 3", Status: status.TaskStatusPending},
				},
			},
		},
	}

	orch := NewPhaseOrchestratorV2(st)
	executor := NewBuildLoopExecutor(orch)

	executor.StartBuildLoop()

	tasksCompleted := 0

	for !executor.IsBuildLoopComplete() {
		currentState := executor.GetCurrentState()

		switch currentState {
		case BuildLoopStateTestingPre:
			executor.RecordTestResult(TestResult{Passed: false, Timestamp: time.Now()})
		case BuildLoopStateTestingPost:
			executor.RecordTestResult(TestResult{Passed: true, TotalTests: 10, PassedTests: 10, Timestamp: time.Now()})
		case BuildLoopStateTaskComplete:
			tasksCompleted++
		case BuildLoopStateIntegrationTesting:
			executor.MarkIntegrationTestsComplete()
		case BuildLoopStateDeploying:
			executor.MarkDeploymentComplete()
		}

		_, err := executor.AdvanceState()
		if err != nil {
			t.Fatalf("unexpected error at state %s: %v", currentState, err)
		}
	}

	if tasksCompleted != 3 {
		t.Errorf("expected 3 tasks completed, got %d", tasksCompleted)
	}
}

// TestBuildLoopTaskDependencies tests task dependency handling
func TestBuildLoopTaskDependencies(t *testing.T) {
	st := createTestStatusV2()
	st.CurrentWaypoint = status.PhaseV2Build
	st.Roadmap = &status.Roadmap{
		Phases: []status.RoadmapPhase{
			{
				ID:   status.PhaseV2Build,
				Name: "BUILD Loop",
				Tasks: []status.Task{
					{ID: "task-1", Title: "Task 1", Status: status.TaskStatusPending, DependsOn: []string{}},
					{ID: "task-2", Title: "Task 2", Status: status.TaskStatusPending, DependsOn: []string{"task-1"}},
					{ID: "task-3", Title: "Task 3", Status: status.TaskStatusPending, DependsOn: []string{"task-1"}},
				},
			},
		},
	}

	orch := NewPhaseOrchestratorV2(st)
	executor := NewBuildLoopExecutor(orch)

	executor.StartBuildLoop()

	// First task should be task-1
	currentTask, _ := executor.GetCurrentTask()
	if currentTask != "task-1" {
		t.Errorf("expected task-1 first, got %s", currentTask)
	}

	// Complete task-1
	for executor.GetCurrentState() != BuildLoopStateTaskComplete {
		switch executor.GetCurrentState() {
		case BuildLoopStateTestingPre:
			executor.RecordTestResult(TestResult{Passed: false, Timestamp: time.Now()})
		case BuildLoopStateTestingPost:
			executor.RecordTestResult(TestResult{Passed: true, TotalTests: 10, PassedTests: 10, Timestamp: time.Now()})
		}
		executor.AdvanceState()
	}

	// Mark task-1 as completed in roadmap
	st.Roadmap.Phases[0].Tasks[0].Status = status.TaskStatusCompleted

	// Advance to next task
	executor.AdvanceState()

	// Next task should be task-2 or task-3 (both depend on task-1, either is valid)
	currentTask, _ = executor.GetCurrentTask()
	if currentTask != "task-2" && currentTask != "task-3" {
		t.Errorf("expected task-2 or task-3, got %s", currentTask)
	}
}

// TestGetProgress tests progress tracking
func TestGetProgress(t *testing.T) {
	st := createTestStatusV2()
	st.CurrentWaypoint = status.PhaseV2Build
	st.Roadmap = &status.Roadmap{
		Phases: []status.RoadmapPhase{
			{
				ID:   status.PhaseV2Build,
				Name: "BUILD Loop",
				Tasks: []status.Task{
					{ID: "task-1", Title: "Task 1", Status: status.TaskStatusPending},
					{ID: "task-2", Title: "Task 2", Status: status.TaskStatusPending},
					{ID: "task-3", Title: "Task 3", Status: status.TaskStatusPending},
				},
			},
		},
	}

	orch := NewPhaseOrchestratorV2(st)
	executor := NewBuildLoopExecutor(orch)

	executor.StartBuildLoop()

	completed, total := executor.GetProgress()
	if completed != 0 || total != 3 {
		t.Errorf("expected 0/3, got %d/%d", completed, total)
	}

	// Complete first task
	for executor.GetCurrentState() != BuildLoopStateTaskComplete {
		switch executor.GetCurrentState() {
		case BuildLoopStateTestingPre:
			executor.RecordTestResult(TestResult{Passed: false, Timestamp: time.Now()})
		case BuildLoopStateTestingPost:
			executor.RecordTestResult(TestResult{Passed: true, TotalTests: 10, PassedTests: 10, Timestamp: time.Now()})
		}
		executor.AdvanceState()
	}

	executor.AdvanceState() // Advance to next task

	completed, total = executor.GetProgress()
	if completed != 1 || total != 3 {
		t.Errorf("expected 1/3, got %d/%d", completed, total)
	}
}

// TestUpdatePhaseHistory tests phase history updates
func TestUpdatePhaseHistory(t *testing.T) {
	st := createTestStatusV2()
	st.CurrentWaypoint = status.PhaseV2Build
	addCompletedPhase(st, status.PhaseV2Build)
	st.Roadmap = &status.Roadmap{
		Phases: []status.RoadmapPhase{
			{
				ID:   status.PhaseV2Build,
				Name: "BUILD Loop",
				Tasks: []status.Task{
					{ID: "task-1", Title: "Task 1", Status: status.TaskStatusPending},
				},
			},
		},
	}

	orch := NewPhaseOrchestratorV2(st)
	executor := NewBuildLoopExecutor(orch)

	executor.StartBuildLoop()

	// Run through BUILD loop
	executor.RecordTestResult(TestResult{Passed: false, Timestamp: time.Now()})
	executor.AdvanceState()
	executor.AdvanceState()
	executor.RecordTestResult(TestResult{Passed: true, TotalTests: 15, PassedTests: 15, FailedTests: 0, Timestamp: time.Now()})
	executor.context.BuildIterations = 5

	// Update phase history
	err := executor.UpdatePhaseHistory()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Check S8 phase history updated
	var s8Entry *status.WaypointHistory
	for i := len(st.WaypointHistory) - 1; i >= 0; i-- {
		if st.WaypointHistory[i].Name == status.PhaseV2Build {
			s8Entry = &st.WaypointHistory[i]
			break
		}
	}

	if s8Entry == nil {
		t.Fatal("S8 entry not found in history")
	}

	if s8Entry.BuildIterations != 5 {
		t.Errorf("expected 5 iterations, got %d", s8Entry.BuildIterations)
	}

	if s8Entry.BuildMetrics == nil {
		t.Fatal("build metrics not set")
	}

	if s8Entry.BuildMetrics.TestsPassed != 15 {
		t.Errorf("expected 15 tests passed, got %d", s8Entry.BuildMetrics.TestsPassed)
	}
}

// TestGetValidationRequirements tests validation requirements by state
func TestGetValidationRequirements(t *testing.T) {
	st := createTestStatusV2()
	orch := NewPhaseOrchestratorV2(st)
	executor := NewBuildLoopExecutor(orch)

	testCases := []struct {
		state                BuildLoopState
		expectedRequirements int
	}{
		{BuildLoopStateTestingPre, 1},
		{BuildLoopStateCoding, 1},
		{BuildLoopStateTestingPost, 1},
		{BuildLoopStateValidating, 3},
		{BuildLoopStateIntegrationTesting, 2},
		{BuildLoopStateDeploying, 3},
		{BuildLoopStateIdle, 0},
	}

	for _, tc := range testCases {
		executor.context.CurrentState = tc.state
		requirements := executor.GetValidationRequirements()

		if len(requirements) != tc.expectedRequirements {
			t.Errorf("state %s: expected %d requirements, got %d",
				tc.state, tc.expectedRequirements, len(requirements))
		}
	}
}

// TestResetBuildLoop tests BUILD loop reset
func TestResetBuildLoop(t *testing.T) {
	st := createTestStatusV2()
	st.CurrentWaypoint = status.PhaseV2Build
	st.Roadmap = &status.Roadmap{
		Phases: []status.RoadmapPhase{
			{
				ID:   status.PhaseV2Build,
				Name: "BUILD Loop",
				Tasks: []status.Task{
					{ID: "task-1", Title: "Task 1", Status: status.TaskStatusPending},
				},
			},
		},
	}

	orch := NewPhaseOrchestratorV2(st)
	executor := NewBuildLoopExecutor(orch)

	executor.StartBuildLoop()
	executor.context.BuildIterations = 10
	executor.context.CompletedTasks = 5

	executor.ResetBuildLoop()

	if executor.context.CurrentState != BuildLoopStateIdle {
		t.Errorf("expected idle state after reset, got %s", executor.context.CurrentState)
	}

	if executor.context.BuildIterations != 0 {
		t.Errorf("expected 0 iterations after reset, got %d", executor.context.BuildIterations)
	}

	if executor.context.CompletedTasks != 0 {
		t.Errorf("expected 0 completed tasks after reset, got %d", executor.context.CompletedTasks)
	}
}
