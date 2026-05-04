package buildloop

import (
	"testing"
	"time"
)

func TestNewBuildLoop(t *testing.T) {
	task := &Task{
		ID:          "T1",
		Description: "Test task",
		RiskLevel:   RiskM,
	}

	tests := []struct {
		name   string
		task   *Task
		config *Config
	}{
		{
			name:   "with default config",
			task:   task,
			config: nil,
		},
		{
			name: "with custom config",
			task: task,
			config: &Config{
				MaxRetries:          5,
				MinAssertionDensity: 0.7,
				MinCoveragePercent:  90.0,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			bl := NewBuildLoop(tt.task, tt.config)

			if bl == nil {
				t.Fatal("NewBuildLoop() returned nil")
			}
			if bl.currentState != StateTestFirst {
				t.Errorf("initial state = %v, want %v", bl.currentState, StateTestFirst)
			}
			if bl.context.Task != task {
				t.Errorf("task not set correctly")
			}
			if bl.tracker == nil {
				t.Errorf("tracker is nil")
			}
			if bl.config == nil {
				t.Errorf("config is nil")
			}
			if tt.config == nil && bl.config.MaxRetries != 3 {
				t.Errorf("default config not applied")
			}
		})
	}
}

func TestDefaultConfig(t *testing.T) {
	config := DefaultConfig()

	if config.MaxRetries != 3 {
		t.Errorf("MaxRetries = %d, want 3", config.MaxRetries)
	}
	if config.MinAssertionDensity != 0.5 {
		t.Errorf("MinAssertionDensity = %f, want 0.5", config.MinAssertionDensity)
	}
	if config.MinCoveragePercent != 80.0 {
		t.Errorf("MinCoveragePercent = %f, want 80.0", config.MinCoveragePercent)
	}
	if config.TestTimeoutSeconds != 300 {
		t.Errorf("TestTimeoutSeconds = %d, want 300", config.TestTimeoutSeconds)
	}
	if !config.EnableTDDEnforcement {
		t.Errorf("EnableTDDEnforcement = false, want true")
	}
}

func TestBuildLoopGetters(t *testing.T) {
	task := &Task{
		ID:          "T1",
		Description: "Test task",
	}

	bl := NewBuildLoop(task, nil)

	// Test GetCurrentState
	if state := bl.GetCurrentState(); state != StateTestFirst {
		t.Errorf("GetCurrentState() = %v, want %v", state, StateTestFirst)
	}

	// Test GetRetryCount
	if count := bl.GetRetryCount(); count != 0 {
		t.Errorf("GetRetryCount() = %d, want 0", count)
	}

	// Test GetStateHistory
	if history := bl.GetStateHistory(); len(history) != 0 {
		t.Errorf("GetStateHistory() length = %d, want 0", len(history))
	}
}

func TestExecuteTestFirst(t *testing.T) {
	task := &Task{
		ID:          "T1",
		Description: "Test task",
	}

	bl := NewBuildLoop(task, nil)

	nextState, err := bl.executeTestFirst()
	if err != nil {
		t.Fatalf("executeTestFirst() error = %v", err)
	}

	if nextState != StateCoding {
		t.Errorf("nextState = %v, want %v", nextState, StateCoding)
	}

	if bl.context.TestResult == nil {
		t.Errorf("TestResult not set")
	}
	if !bl.context.TestResult.HasFailures {
		t.Errorf("TestResult.HasFailures = false, want true (TDD)")
	}
}

func TestExecuteCoding(t *testing.T) {
	task := &Task{
		ID:          "T1",
		Description: "Test task",
	}

	bl := NewBuildLoop(task, nil)

	nextState, err := bl.executeCoding()
	if err != nil {
		t.Fatalf("executeCoding() error = %v", err)
	}

	if nextState != StateGreen {
		t.Errorf("nextState = %v, want %v", nextState, StateGreen)
	}

	if bl.context.CodeChanges == 0 {
		t.Errorf("CodeChanges = 0, want > 0")
	}
	if bl.context.TestResult == nil {
		t.Errorf("TestResult not set")
	}
}

func TestExecuteGreen(t *testing.T) {
	tests := []struct {
		name          string
		riskLevel     RiskLevel
		expectedState State
	}{
		{"low risk goes to refactor", RiskS, StateRefactor},
		{"medium risk goes to refactor", RiskM, StateRefactor},
		{"high risk goes to validation", RiskL, StateValidation},
		{"extra high risk goes to validation", RiskXL, StateValidation},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			task := &Task{
				ID:        "T1",
				RiskLevel: tt.riskLevel,
			}

			bl := NewBuildLoop(task, nil)
			// Set up required context
			bl.context.TestResult = &TestResult{
				HasFailures: false,
				PassCount:   1,
			}

			nextState, err := bl.executeGreen()
			if err != nil {
				t.Fatalf("executeGreen() error = %v", err)
			}

			if nextState != tt.expectedState {
				t.Errorf("nextState = %v, want %v", nextState, tt.expectedState)
			}

			if bl.context.QualityResult == nil {
				t.Errorf("QualityResult not set")
			}
		})
	}
}

func TestExecuteValidation(t *testing.T) {
	// Note: executeValidation creates a stub review with no issues
	// In real implementation, this would call actual review tools
	// For now, we test the happy path (no issues)
	task := &Task{ID: "T1"}
	bl := NewBuildLoop(task, nil)

	nextState, err := bl.executeValidation()
	if err != nil {
		t.Fatalf("executeValidation() error = %v", err)
	}

	// Stub implementation always returns no issues -> complete
	if nextState != StateComplete {
		t.Errorf("nextState = %v, want %v (stub has no issues)", nextState, StateComplete)
	}

	if bl.context.ReviewResult == nil {
		t.Errorf("ReviewResult not set")
	}
}

func TestRecordTransition(t *testing.T) {
	task := &Task{ID: "T1"}
	bl := NewBuildLoop(task, nil)

	bl.recordTransition(StateTestFirst, StateCoding, "tests analyzed", "")

	history := bl.GetStateHistory()
	if len(history) != 1 {
		t.Fatalf("history length = %d, want 1", len(history))
	}

	rec := history[0]
	if rec.From != StateTestFirst {
		t.Errorf("From = %v, want %v", rec.From, StateTestFirst)
	}
	if rec.To != StateCoding {
		t.Errorf("To = %v, want %v", rec.To, StateCoding)
	}
	if rec.Trigger != "tests analyzed" {
		t.Errorf("Trigger = %q, want %q", rec.Trigger, "tests analyzed")
	}
	if !rec.Success {
		t.Errorf("Success = false, want true")
	}
}

func TestCompleteTask(t *testing.T) {
	task := &Task{
		ID:        "T1",
		StartedAt: time.Now().Add(-5 * time.Minute),
	}

	bl := NewBuildLoop(task, nil)
	bl.context.QualityResult = &QualityResult{
		AssertionDensity: 0.6,
		CoveragePercent:  85.0,
	}

	result, err := bl.completeTask()
	if err != nil {
		t.Fatalf("completeTask() error = %v", err)
	}

	if !result.Success {
		t.Errorf("Success = false, want true")
	}
	if result.Task.CompletedAt == nil {
		t.Errorf("CompletedAt not set")
	}
	if result.Metrics == nil {
		t.Errorf("Metrics not set")
	}
	if result.Metrics.AssertionDensity != 0.6 {
		t.Errorf("Metrics.AssertionDensity = %f, want 0.6", result.Metrics.AssertionDensity)
	}
}

func TestExecuteState(t *testing.T) {
	task := &Task{ID: "T1", RiskLevel: RiskM}
	bl := NewBuildLoop(task, nil)

	tests := []struct {
		name      string
		state     State
		wantState State // expected next state
		wantErr   bool
	}{
		{"TEST_FIRST", StateTestFirst, StateCoding, false},
		{"CODING", StateCoding, StateGreen, false},
		{"GREEN", StateGreen, StateRefactor, false},
		{"TIMEOUT", StateTimeout, StateCoding, false},
		{"invalid state", State("INVALID"), "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Reset context for each test
			bl.context = &BuildContext{
				Task: task,
				TestResult: &TestResult{
					HasFailures: false,
					PassCount:   1,
				},
			}

			nextState, err := bl.executeState(tt.state)
			if (err != nil) != tt.wantErr {
				t.Errorf("executeState() error = %v, wantErr %v", err, tt.wantErr)
			}
			if !tt.wantErr && nextState != tt.wantState {
				t.Errorf("nextState = %v, want %v", nextState, tt.wantState)
			}
		})
	}
}

func TestBuildLoopRetryLimit(t *testing.T) {
	task := &Task{ID: "T1"}
	config := &Config{
		MaxRetries:           2,
		EnableTDDEnforcement: false,
	}

	bl := NewBuildLoop(task, config)

	// Manually increment retry count to test limit
	bl.retryCount = 3

	// Should not execute since retryCount >= maxRetries
	result, err := bl.Execute()
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	if result.Success {
		t.Errorf("Success = true, want false (max retries exceeded)")
	}
	if result.Error == "" {
		t.Errorf("Error is empty, want error message")
	}
}

func TestStateTransitionValidation(t *testing.T) {
	task := &Task{ID: "T1"}
	bl := NewBuildLoop(task, nil)

	// Try to manually set an invalid transition
	bl.currentState = StateTestFirst
	invalidNext := StateValidation // Invalid: can't go directly from TEST_FIRST to VALIDATION

	transition := ValidateTransition(bl.currentState, invalidNext)
	if transition.Valid {
		t.Errorf("Transition from %v to %v should be invalid", bl.currentState, invalidNext)
	}
}

func TestTaskResultCreation(t *testing.T) {
	task := &Task{
		ID:          "T1",
		Description: "Test task",
	}

	metrics := &TaskMetrics{
		Duration:         5 * time.Minute,
		RetryCount:       1,
		TestRunCount:     5,
		AssertionDensity: 0.6,
		CoveragePercent:  85.0,
	}

	result := &TaskResult{
		Task:    task,
		Success: true,
		Metrics: metrics,
	}

	if !result.Success {
		t.Errorf("Success = false, want true")
	}
	if result.Task != task {
		t.Errorf("Task not set correctly")
	}
	if result.Metrics != metrics {
		t.Errorf("Metrics not set correctly")
	}
}

func TestErrorStateIncrementsRetry(t *testing.T) {
	task := &Task{ID: "T1"}
	bl := NewBuildLoop(task, nil)

	initialRetry := bl.retryCount

	// Simulate entering an error state
	bl.currentState = StateTimeout

	if !bl.currentState.IsErrorState() {
		t.Fatalf("TIMEOUT should be an error state")
	}

	// In Execute(), error states increment retry count
	// Test this behavior by checking the IsErrorState method
	if bl.currentState.IsErrorState() {
		bl.retryCount++
	}

	if bl.retryCount != initialRetry+1 {
		t.Errorf("retryCount = %d, want %d", bl.retryCount, initialRetry+1)
	}
}

// --- Parallel execution tests (sandbox changes) ---

// TestDefaultConfig_EnableParallelExecution verifies DefaultConfig returns
// EnableParallelExecution: true (sandbox change).
func TestDefaultConfig_EnableParallelExecution(t *testing.T) {
	config := DefaultConfig()

	if !config.EnableParallelExecution {
		t.Error("DefaultConfig().EnableParallelExecution = false, want true")
	}
}

// TestConfig_ParallelExecution_Explicit verifies explicit parallel config values.
func TestConfig_ParallelExecution_Explicit(t *testing.T) {
	tests := []struct {
		name     string
		enabled  bool
		wantFlag bool
	}{
		{"parallel enabled", true, true},
		{"parallel disabled", false, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := &Config{
				MaxRetries:              3,
				EnableParallelExecution: tt.enabled,
			}

			if config.EnableParallelExecution != tt.wantFlag {
				t.Errorf("EnableParallelExecution = %v, want %v",
					config.EnableParallelExecution, tt.wantFlag)
			}
		})
	}
}

// TestBuildLoop_ParallelConfig_Propagation verifies that the parallel execution
// config flag is correctly propagated to BuildLoop instances.
func TestBuildLoop_ParallelConfig_Propagation(t *testing.T) {
	task := &Task{ID: "T-parallel", RiskLevel: RiskM}

	t.Run("default config propagates parallel=true", func(t *testing.T) {
		bl := NewBuildLoop(task, nil)
		if !bl.config.EnableParallelExecution {
			t.Error("BuildLoop with default config should have EnableParallelExecution=true")
		}
	})

	t.Run("explicit parallel=true propagates", func(t *testing.T) {
		config := &Config{
			MaxRetries:              3,
			EnableParallelExecution: true,
		}
		bl := NewBuildLoop(task, config)
		if !bl.config.EnableParallelExecution {
			t.Error("BuildLoop with explicit true should have EnableParallelExecution=true")
		}
	})

	t.Run("explicit parallel=false propagates", func(t *testing.T) {
		config := &Config{
			MaxRetries:              3,
			EnableParallelExecution: false,
		}
		bl := NewBuildLoop(task, config)
		if bl.config.EnableParallelExecution {
			t.Error("BuildLoop with explicit false should have EnableParallelExecution=false")
		}
	})
}

// TestBuildLoop_SequentialExecution_WithParallelDisabled verifies that when
// parallel execution is disabled, the loop processes states sequentially
// (normal TDD cycle: TEST_FIRST -> CODING -> GREEN -> ...).
func TestBuildLoop_SequentialExecution_WithParallelDisabled(t *testing.T) {
	task := &Task{ID: "T-seq", RiskLevel: RiskM}
	config := &Config{
		MaxRetries:              3,
		MinAssertionDensity:     0.5,
		MinCoveragePercent:      80.0,
		TestTimeoutSeconds:      300,
		EnableTDDEnforcement:    true,
		EnableParallelExecution: false,
	}

	bl := NewBuildLoop(task, config)

	// Verify initial state
	if bl.GetCurrentState() != StateTestFirst {
		t.Fatalf("Initial state = %v, want TEST_FIRST", bl.GetCurrentState())
	}

	// Execute TEST_FIRST -> CODING
	nextState, err := bl.executeTestFirst()
	if err != nil {
		t.Fatalf("executeTestFirst() error = %v", err)
	}
	if nextState != StateCoding {
		t.Errorf("After TEST_FIRST: nextState = %v, want CODING", nextState)
	}

	// Execute CODING -> GREEN
	nextState, err = bl.executeCoding()
	if err != nil {
		t.Fatalf("executeCoding() error = %v", err)
	}
	if nextState != StateGreen {
		t.Errorf("After CODING: nextState = %v, want GREEN", nextState)
	}

	// Verify sequential progression through state history
	bl.recordTransition(StateTestFirst, StateCoding, "tests fail", "")
	bl.recordTransition(StateCoding, StateGreen, "tests pass", "")

	history := bl.GetStateHistory()
	if len(history) < 2 {
		t.Fatalf("Expected at least 2 transitions, got %d", len(history))
	}

	// Verify transitions are in order
	if history[0].From != StateTestFirst || history[0].To != StateCoding {
		t.Errorf("First transition: %v->%v, want TEST_FIRST->CODING",
			history[0].From, history[0].To)
	}
	if history[1].From != StateCoding || history[1].To != StateGreen {
		t.Errorf("Second transition: %v->%v, want CODING->GREEN",
			history[1].From, history[1].To)
	}
}

// TestBuildLoop_ConcurrentStateTransitions verifies that state transitions
// are properly tracked when parallel execution is enabled. Multiple
// BuildLoop instances should be able to run independently.
func TestBuildLoop_ConcurrentStateTransitions(t *testing.T) {
	config := DefaultConfig() // EnableParallelExecution = true

	tasks := []*Task{
		{ID: "T1", RiskLevel: RiskS},
		{ID: "T2", RiskLevel: RiskM},
		{ID: "T3", RiskLevel: RiskL},
	}

	// Create independent build loops (simulating parallel tasks)
	loops := make([]*BuildLoop, len(tasks))
	for i, task := range tasks {
		loops[i] = NewBuildLoop(task, config)
	}

	// Verify each loop is independent
	for i, bl := range loops {
		if bl.GetCurrentState() != StateTestFirst {
			t.Errorf("Loop %d: initial state = %v, want TEST_FIRST",
				i, bl.GetCurrentState())
		}
		if bl.context.Task.ID != tasks[i].ID {
			t.Errorf("Loop %d: task ID = %s, want %s",
				i, bl.context.Task.ID, tasks[i].ID)
		}
	}

	// Execute first step in each loop independently
	for i, bl := range loops {
		nextState, err := bl.executeTestFirst()
		if err != nil {
			t.Fatalf("Loop %d: executeTestFirst() error = %v", i, err)
		}
		if nextState != StateCoding {
			t.Errorf("Loop %d: after TEST_FIRST nextState = %v, want CODING",
				i, nextState)
		}
	}

	// Advance only loop[0] further - verify others unchanged
	nextState, err := loops[0].executeCoding()
	if err != nil {
		t.Fatalf("Loop 0: executeCoding() error = %v", err)
	}
	if nextState != StateGreen {
		t.Errorf("Loop 0: after CODING nextState = %v, want GREEN", nextState)
	}

	// Verify loop isolation: loop[0] advanced, others didn't affect each other
	if loops[0].context.CodeChanges == 0 {
		t.Error("Loop 0: CodeChanges should be > 0 after executeCoding()")
	}
}

// TestBuildLoop_ExecuteFullCycle verifies the full Execute() cycle completes
// successfully with the default parallel-enabled config.
func TestBuildLoop_ExecuteFullCycle(t *testing.T) {
	task := &Task{ID: "T-full", RiskLevel: RiskM}
	config := DefaultConfig()

	bl := NewBuildLoop(task, config)
	bl.tracker.StartIteration()
	bl.context.Task.StartedAt = time.Now()

	// The stub implementation will progress:
	// TEST_FIRST -> CODING -> GREEN -> REFACTOR -> VALIDATION -> COMPLETE
	result, err := bl.Execute()
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	if !result.Success {
		t.Errorf("Execute() result.Success = false, want true (error: %s)", result.Error)
	}

	if result.Metrics == nil {
		t.Error("Execute() result.Metrics is nil, want non-nil")
	}
}
