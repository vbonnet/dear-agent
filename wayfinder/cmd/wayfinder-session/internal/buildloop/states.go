package buildloop

import "fmt"

// State represents a BUILD loop state
type State string

// BUILD loop states
const (
	// Core states
	StateTestFirst  State = "TEST_FIRST" // Red phase - test fails as expected
	StateCoding     State = "CODING"     // Write minimal code to pass
	StateGreen      State = "GREEN"      // Tests pass
	StateRefactor   State = "REFACTOR"   // Improve code quality
	StateValidation State = "VALIDATION" // Multi-persona review
	StateDeploy     State = "DEPLOY"     // Integration testing
	StateMonitoring State = "MONITORING" // Observe in production/staging
	StateComplete   State = "COMPLETE"   // Task done

	// Error/recovery states
	StateTimeout       State = "TIMEOUT"        // Test timeout
	StateReviewFailed  State = "REVIEW_FAILED"  // Review blocking issues
	StateIntegrateFail State = "INTEGRATE_FAIL" // Integration test failure
)

// Valid states set for validation
var validStates = map[State]bool{
	StateTestFirst:     true,
	StateCoding:        true,
	StateGreen:         true,
	StateRefactor:      true,
	StateValidation:    true,
	StateDeploy:        true,
	StateMonitoring:    true,
	StateComplete:      true,
	StateTimeout:       true,
	StateReviewFailed:  true,
	StateIntegrateFail: true,
}

// IsValid checks if a state is valid
func (s State) IsValid() bool {
	return validStates[s]
}

// IsErrorState checks if this is an error/recovery state
func (s State) IsErrorState() bool {
	return s == StateTimeout || s == StateReviewFailed || s == StateIntegrateFail
}

// IsTerminal checks if this state ends the loop
func (s State) IsTerminal() bool {
	return s == StateComplete
}

// String returns the string representation
func (s State) String() string {
	return string(s)
}

// Transition represents a state transition with metadata
type Transition struct {
	From    State
	To      State
	Trigger string // What triggered this transition
	Valid   bool   // Whether this transition is allowed
	Reason  string // Validation failure reason if !Valid
}

// StateTransitions defines valid transitions between states
var StateTransitions = map[State][]State{
	StateTestFirst: {
		StateCoding,    // Failures analyzed, ready to code
		StateTimeout,   // Test execution timeout
		StateTestFirst, // Wrong scope, restart
	},
	StateCoding: {
		StateGreen,     // Tests pass
		StateTestFirst, // Tests still fail, iterate
		StateTimeout,   // Test run timeout
	},
	StateGreen: {
		StateRefactor,   // Quality gates pass, improve code
		StateValidation, // Risk level requires review
		StateComplete,   // Low risk, quality gates pass
		StateCoding,     // Quality gates fail
	},
	StateRefactor: {
		StateValidation, // Ready for review
		StateGreen,      // Re-run tests after refactor
		StateCoding,     // Refactor broke tests
	},
	StateValidation: {
		StateComplete,     // No blocking issues
		StateReviewFailed, // P0/P1 issues found
		StateCoding,       // Fixable issues identified
	},
	StateDeploy: {
		StateMonitoring,    // Deployment successful
		StateReviewFailed,  // Batch review blocks
		StateIntegrateFail, // Deployment validation fails
	},
	StateMonitoring: {
		StateComplete, // Monitoring successful
		StateCoding,   // Issues found, need fixes
	},
	StateComplete: {
		StateTestFirst, // Next task
		StateDeploy,    // All tasks done, begin integration
	},
	// Error state transitions
	StateTimeout: {
		StateCoding,    // Fix timeout issues
		StateTestFirst, // Restart with different approach
	},
	StateReviewFailed: {
		StateCoding,    // Implement fixes
		StateTestFirst, // Redesign needed
	},
	StateIntegrateFail: {
		StateTestFirst, // Rework specific task
		StateDeploy,    // Retry integration tests
	},
}

// ValidateTransition checks if a state transition is allowed
func ValidateTransition(from, to State) Transition {
	t := Transition{
		From:  from,
		To:    to,
		Valid: false,
	}

	// Validate states exist
	if !from.IsValid() {
		t.Reason = fmt.Sprintf("invalid source state: %s", from)
		return t
	}
	if !to.IsValid() {
		t.Reason = fmt.Sprintf("invalid target state: %s", to)
		return t
	}

	// Check if transition is allowed
	allowedTargets, exists := StateTransitions[from]
	if !exists {
		t.Reason = fmt.Sprintf("no transitions defined for state: %s", from)
		return t
	}

	for _, allowed := range allowedTargets {
		if allowed == to {
			t.Valid = true
			return t
		}
	}

	t.Reason = fmt.Sprintf("transition from %s to %s not allowed", from, to)
	return t
}

// ExitCriteria defines conditions to exit a state
type ExitCriteria struct {
	State       State
	Description string
	Validator   func(ctx *BuildContext) (bool, error)
}

// GetExitCriteria returns the exit criteria for a given state
func GetExitCriteria(state State) ExitCriteria {
	criteria := map[State]ExitCriteria{
		StateTestFirst: {
			State:       StateTestFirst,
			Description: "Test failures match expected task scope and ready to write code",
			Validator:   validateTestFirstExit,
		},
		StateCoding: {
			State:       StateCoding,
			Description: "Code changes committed and ready for test validation",
			Validator:   validateCodingExit,
		},
		StateGreen: {
			State:       StateGreen,
			Description: "Tests green and quality gates checked",
			Validator:   validateGreenExit,
		},
		StateRefactor: {
			State:       StateRefactor,
			Description: "Code quality improved and tests still pass",
			Validator:   validateRefactorExit,
		},
		StateValidation: {
			State:       StateValidation,
			Description: "Review completed with P0/P1 issue count",
			Validator:   validateValidationExit,
		},
		StateDeploy: {
			State:       StateDeploy,
			Description: "Deployment successful or failed",
			Validator:   validateDeployExit,
		},
		StateMonitoring: {
			State:       StateMonitoring,
			Description: "Monitoring complete, no issues detected",
			Validator:   validateMonitoringExit,
		},
		StateComplete: {
			State:       StateComplete,
			Description: "Task marked complete with metrics recorded",
			Validator:   validateCompleteExit,
		},
	}

	if c, exists := criteria[state]; exists {
		return c
	}

	// Default criteria for error states
	return ExitCriteria{
		State:       state,
		Description: "Recovery action completed",
		Validator: func(ctx *BuildContext) (bool, error) {
			return true, nil
		},
	}
}

// Exit criteria validators (stub implementations)
func validateTestFirstExit(ctx *BuildContext) (bool, error) {
	// Check test failures are task-relevant and understood
	if ctx.TestResult == nil {
		return false, fmt.Errorf("no test results available")
	}
	if !ctx.TestResult.HasFailures {
		return false, fmt.Errorf("tests should fail in TEST_FIRST phase (TDD violation)")
	}
	return true, nil
}

func validateCodingExit(ctx *BuildContext) (bool, error) {
	// Check code changes exist
	if ctx.CodeChanges == 0 {
		return false, fmt.Errorf("no code changes made")
	}
	return true, nil
}

func validateGreenExit(ctx *BuildContext) (bool, error) {
	// Check tests pass and quality gates evaluated
	if ctx.TestResult == nil || ctx.TestResult.HasFailures {
		return false, fmt.Errorf("tests must pass in GREEN state")
	}
	if ctx.QualityResult == nil {
		return false, fmt.Errorf("quality gates not evaluated")
	}
	return true, nil
}

func validateRefactorExit(ctx *BuildContext) (bool, error) {
	// Check refactoring complete and tests still pass
	if ctx.TestResult == nil || ctx.TestResult.HasFailures {
		return false, fmt.Errorf("tests must pass after refactor")
	}
	return true, nil
}

func validateValidationExit(ctx *BuildContext) (bool, error) {
	// Check review completed
	if ctx.ReviewResult == nil {
		return false, fmt.Errorf("review not completed")
	}
	return true, nil
}

func validateDeployExit(ctx *BuildContext) (bool, error) {
	// Check deployment executed
	if ctx.DeployResult == nil {
		return false, fmt.Errorf("deployment not executed")
	}
	return true, nil
}

func validateMonitoringExit(ctx *BuildContext) (bool, error) {
	// Check monitoring complete
	if ctx.MonitoringResult == nil {
		return false, fmt.Errorf("monitoring not completed")
	}
	return true, nil
}

func validateCompleteExit(ctx *BuildContext) (bool, error) {
	// Check task metrics recorded
	if ctx.Task == nil {
		return false, fmt.Errorf("no task context")
	}
	return true, nil
}
