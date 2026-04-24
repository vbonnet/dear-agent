package buildloop

import (
	"testing"
)

// TestStateMachineHappyPath tests the complete happy path through all states
func TestStateMachineHappyPath(t *testing.T) {
	task := &Task{
		ID:          "T1",
		Description: "Happy path task",
		RiskLevel:   RiskM, // Medium risk - no per-task review
	}

	config := &Config{
		MaxRetries:           3,
		EnableTDDEnforcement: false, // Disable for controlled test
	}

	bl := NewBuildLoop(task, config)

	// Expected state sequence for medium risk task:
	// TEST_FIRST -> CODING -> GREEN -> REFACTOR -> VALIDATION -> COMPLETE
	expectedStates := []State{
		StateTestFirst,
		StateCoding,
		StateGreen,
		StateRefactor,
		StateValidation,
		StateComplete,
	}

	result, err := bl.Execute()
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	if !result.Success {
		t.Errorf("Success = false, want true")
	}

	history := bl.GetStateHistory()
	if len(history) < len(expectedStates)-1 {
		t.Errorf("state transitions = %d, want >= %d", len(history), len(expectedStates)-1)
	}

	// Verify final state
	if bl.GetCurrentState() != StateComplete {
		t.Errorf("final state = %v, want %v", bl.GetCurrentState(), StateComplete)
	}
}

// TestStateMachineHighRiskTask tests high risk task requiring per-task review
func TestStateMachineHighRiskTask(t *testing.T) {
	task := &Task{
		ID:        "T2",
		RiskLevel: RiskL, // Large risk - requires per-task review
	}

	bl := NewBuildLoop(task, DefaultConfig())
	bl.config.EnableTDDEnforcement = false

	result, err := bl.Execute()
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	if !result.Success {
		t.Errorf("Success = false, want true")
	}

	// Verify VALIDATION state was visited
	found := false
	for _, rec := range bl.GetStateHistory() {
		if rec.To == StateValidation {
			found = true
			break
		}
	}

	if !found {
		t.Errorf("VALIDATION state not visited for high risk task")
	}
}

// TestStateMachineIterationLoop tests the TEST_FIRST <-> CODING loop
func TestStateMachineIterationLoop(t *testing.T) {
	// Simulate multiple iterations by manually controlling state
	states := []State{
		StateTestFirst,
		StateCoding,
		StateTestFirst, // Back to red
		StateCoding,
		StateGreen, // Finally green
	}

	currentState := StateTestFirst
	for i, expectedNext := range states[1:] {
		transition := ValidateTransition(currentState, expectedNext)
		if !transition.Valid {
			t.Errorf("iteration %d: transition %v -> %v invalid: %s",
				i, currentState, expectedNext, transition.Reason)
		}
		currentState = expectedNext
	}
}

// TestStateMachineErrorRecovery tests error state transitions
func TestStateMachineErrorRecovery(t *testing.T) {
	tests := []struct {
		name          string
		errorState    State
		recoveryState State
	}{
		{"timeout recovery", StateTimeout, StateCoding},
		{"review failed recovery", StateReviewFailed, StateCoding},
		{"integration failed recovery", StateIntegrateFail, StateTestFirst},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			transition := ValidateTransition(tt.errorState, tt.recoveryState)
			if !transition.Valid {
				t.Errorf("recovery transition %v -> %v should be valid: %s",
					tt.errorState, tt.recoveryState, transition.Reason)
			}
		})
	}
}

// TestStateMachineInvalidTransitions tests that invalid transitions are rejected
func TestStateMachineInvalidTransitions(t *testing.T) {
	invalidTransitions := []struct {
		from State
		to   State
	}{
		{StateTestFirst, StateGreen},    // Can't skip CODING
		{StateTestFirst, StateComplete}, // Can't skip entire loop
		{StateCoding, StateValidation},  // Must go through GREEN
		{StateGreen, StateTimeout},      // Can't transition to error from GREEN
		{StateComplete, StateCoding},    // Can't go back to CODING from COMPLETE
		{StateComplete, StateGreen},     // Can't go back to GREEN
		{StateRefactor, StateComplete},  // Must go through VALIDATION
	}

	for _, tt := range invalidTransitions {
		t.Run(tt.from.String()+"->"+tt.to.String(), func(t *testing.T) {
			transition := ValidateTransition(tt.from, tt.to)
			if transition.Valid {
				t.Errorf("transition %v -> %v should be invalid", tt.from, tt.to)
			}
		})
	}
}

// TestStateMachineTerminalState tests that COMPLETE is terminal
func TestStateMachineTerminalState(t *testing.T) {
	if !StateComplete.IsTerminal() {
		t.Errorf("COMPLETE should be terminal state")
	}

	// No other state should be terminal
	nonTerminalStates := []State{
		StateTestFirst, StateCoding, StateGreen, StateRefactor,
		StateValidation, StateDeploy, StateMonitoring,
		StateTimeout, StateReviewFailed, StateIntegrateFail,
	}

	for _, state := range nonTerminalStates {
		if state.IsTerminal() {
			t.Errorf("%v should not be terminal", state)
		}
	}
}

// TestStateMachineTDDEnforcement tests TDD violation detection logic
func TestStateMachineTDDEnforcement(t *testing.T) {
	task := &Task{ID: "T4"}
	config := &Config{
		EnableTDDEnforcement: true,
		MaxRetries:           3,
	}

	bl := NewBuildLoop(task, config)

	// Note: The stub implementation of executeTestFirst() always creates
	// a failing test result, which is correct for TDD. In real implementation,
	// if tests pass before code is written, it would return an error.
	// Here we test that the enforcement logic exists by checking the config.

	if !bl.config.EnableTDDEnforcement {
		t.Errorf("TDD enforcement should be enabled")
	}

	// Execute TEST_FIRST state - should succeed with stub (tests fail as expected)
	nextState, err := bl.executeTestFirst()
	if err != nil {
		t.Fatalf("executeTestFirst() error = %v", err)
	}

	if nextState != StateCoding {
		t.Errorf("nextState = %v, want CODING (tests failed as expected)", nextState)
	}
}

// TestStateMachineQualityGates tests quality gate enforcement
func TestStateMachineQualityGates(t *testing.T) {
	task := &Task{ID: "T5"}
	bl := NewBuildLoop(task, DefaultConfig())

	// Set up passing tests
	bl.context.TestResult = &TestResult{
		HasFailures: false,
		PassCount:   5,
	}

	// Execute GREEN state which runs quality gates
	nextState, err := bl.executeGreen()
	if err != nil {
		t.Fatalf("executeGreen() error = %v", err)
	}

	// Should have quality result
	if bl.context.QualityResult == nil {
		t.Errorf("QualityResult not set after GREEN state")
	}

	// Medium risk should go to REFACTOR
	if nextState != StateRefactor && nextState != StateValidation {
		t.Errorf("nextState = %v, want REFACTOR or VALIDATION", nextState)
	}
}

// TestStateMachineCompleteWorkflow tests complete workflow with all transitions
func TestStateMachineCompleteWorkflow(t *testing.T) {
	task := &Task{
		ID:          "T6",
		Description: "Complete workflow test",
		RiskLevel:   RiskXL, // Extra large - requires review
	}

	bl := NewBuildLoop(task, DefaultConfig())
	bl.config.EnableTDDEnforcement = false

	result, err := bl.Execute()
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	if !result.Success {
		t.Errorf("workflow failed: %s", result.Error)
	}

	// Verify metrics were collected
	if result.Metrics == nil {
		t.Errorf("Metrics not collected")
	}
	if result.Metrics.StateTransitions == 0 {
		t.Errorf("StateTransitions = 0, want > 0")
	}

	// Verify task was marked complete
	if result.Task.CompletedAt == nil {
		t.Errorf("CompletedAt not set")
	}
}

// TestStateMachineStateHistory tests state transition recording
func TestStateMachineStateHistory(t *testing.T) {
	task := &Task{ID: "T7", RiskLevel: RiskS}
	bl := NewBuildLoop(task, DefaultConfig())
	bl.config.EnableTDDEnforcement = false

	_, err := bl.Execute()
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	history := bl.GetStateHistory()
	if len(history) == 0 {
		t.Errorf("no state history recorded")
	}

	// Verify history entries have required fields
	for i, rec := range history {
		if !rec.From.IsValid() && rec.From != "" {
			t.Errorf("history[%d].From invalid: %v", i, rec.From)
		}
		if !rec.To.IsValid() {
			t.Errorf("history[%d].To invalid: %v", i, rec.To)
		}
		if rec.Timestamp.IsZero() {
			t.Errorf("history[%d].Timestamp not set", i)
		}
	}
}

// TestStateMachineRiskLevelRouting tests different paths based on risk level
func TestStateMachineRiskLevelRouting(t *testing.T) {
	tests := []struct {
		name         string
		riskLevel    RiskLevel
		shouldReview bool
	}{
		{"XS risk - no review", RiskXS, false},
		{"S risk - no review", RiskS, false},
		{"M risk - no review", RiskM, false},
		{"L risk - requires review", RiskL, true},
		{"XL risk - requires review", RiskXL, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			task := &Task{ID: "T8", RiskLevel: tt.riskLevel}
			bl := NewBuildLoop(task, DefaultConfig())
			bl.config.EnableTDDEnforcement = false

			result, err := bl.Execute()
			if err != nil {
				t.Fatalf("Execute() error = %v", err)
			}

			if !result.Success {
				t.Errorf("execution failed: %s", result.Error)
			}

			// Check if VALIDATION was visited
			visitedValidation := false
			for _, rec := range bl.GetStateHistory() {
				if rec.To == StateValidation {
					visitedValidation = true
					break
				}
			}

			if tt.shouldReview && !visitedValidation {
				t.Errorf("risk level %v should require VALIDATION", tt.riskLevel)
			}
		})
	}
}

// TestStateMachineMetricsCollection tests that metrics are properly collected
func TestStateMachineMetricsCollection(t *testing.T) {
	task := &Task{ID: "T9", RiskLevel: RiskM}
	bl := NewBuildLoop(task, DefaultConfig())
	bl.config.EnableTDDEnforcement = false

	result, err := bl.Execute()
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	if !result.Success {
		t.Errorf("execution failed")
	}

	metrics := result.Metrics
	if metrics == nil {
		t.Fatal("Metrics not collected")
	}

	// Verify metrics fields
	if metrics.Duration == 0 {
		t.Errorf("Duration = 0, want > 0")
	}
	if metrics.StateTransitions == 0 {
		t.Errorf("StateTransitions = 0, want > 0")
	}
	if metrics.AssertionDensity == 0 {
		t.Errorf("AssertionDensity = 0, want > 0")
	}
	if metrics.CoveragePercent == 0 {
		t.Errorf("CoveragePercent = 0, want > 0")
	}
}

// TestStateMachineIdempotence tests that state machine can be executed multiple times
func TestStateMachineIdempotence(t *testing.T) {
	task := &Task{ID: "T10", RiskLevel: RiskS}

	// First execution
	bl1 := NewBuildLoop(task, DefaultConfig())
	bl1.config.EnableTDDEnforcement = false
	result1, err1 := bl1.Execute()
	if err1 != nil {
		t.Fatalf("first execution error = %v", err1)
	}

	// Second execution (new instance)
	bl2 := NewBuildLoop(task, DefaultConfig())
	bl2.config.EnableTDDEnforcement = false
	result2, err2 := bl2.Execute()
	if err2 != nil {
		t.Fatalf("second execution error = %v", err2)
	}

	// Both should succeed
	if !result1.Success || !result2.Success {
		t.Errorf("execution results differ: result1=%v, result2=%v",
			result1.Success, result2.Success)
	}
}

// TestStateMachineAllStatesReachable tests that all states are reachable
func TestStateMachineAllStatesReachable(t *testing.T) {
	// Map to track which states are reachable
	reachable := make(map[State]bool)
	reachable[StateTestFirst] = true // Starting state

	// Iteratively find all reachable states
	changed := true
	for changed {
		changed = false
		for from, targets := range StateTransitions {
			if reachable[from] {
				for _, to := range targets {
					if !reachable[to] {
						reachable[to] = true
						changed = true
					}
				}
			}
		}
	}

	// All states should be reachable
	allStates := []State{
		StateTestFirst, StateCoding, StateGreen, StateRefactor,
		StateValidation, StateDeploy, StateMonitoring, StateComplete,
		StateTimeout, StateReviewFailed, StateIntegrateFail,
	}

	for _, state := range allStates {
		if !reachable[state] {
			t.Errorf("state %v is not reachable from TEST_FIRST", state)
		}
	}
}
