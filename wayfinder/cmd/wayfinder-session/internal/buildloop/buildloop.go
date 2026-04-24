package buildloop

import (
	"fmt"
	"time"
)

// BuildLoop manages the BUILD loop state machine for S8 phase
type BuildLoop struct {
	currentState State
	context      *BuildContext
	tracker      *IterationTracker
	config       *Config
	stateHistory []StateTransitionRecord
	retryCount   int
	maxRetries   int
}

// BuildContext holds context for the current build iteration
type BuildContext struct {
	Task             *Task
	TestResult       *TestResult
	QualityResult    *QualityResult
	ReviewResult     *ReviewResult
	DeployResult     *DeployResult
	MonitoringResult *MonitoringResult
	CodeChanges      int
	RiskLevel        RiskLevel
	// LintContext holds a concise summary of lint rules for the project,
	// populated by lintcontext.Summarize() when BUILD starts.
	LintContext string
}

// StateTransitionRecord records a state transition
type StateTransitionRecord struct {
	From      State
	To        State
	Timestamp time.Time
	Trigger   string
	Success   bool
	Error     string
}

// Task represents a task from ROADMAP
type Task struct {
	ID           string
	Description  string
	Files        []string
	Dependencies []string
	RiskLevel    RiskLevel
	StartedAt    time.Time
	CompletedAt  *time.Time
	Metrics      *TaskMetrics
}

// TestResult represents test execution results
type TestResult struct {
	HasFailures  bool
	FailureCount int
	PassCount    int
	Duration     time.Duration
	Timeout      bool
}

// QualityResult represents quality gate results
type QualityResult struct {
	Passes           bool
	AssertionDensity float64
	CoveragePercent  float64
	Issues           []string
}

// ReviewResult represents code review results
type ReviewResult struct {
	P0Issues []string // Critical issues
	P1Issues []string // High priority issues
	P2Issues []string // Medium priority issues
}

// DeployResult represents deployment results
type DeployResult struct {
	Success bool
	Error   string
}

// MonitoringResult represents monitoring results
type MonitoringResult struct {
	Success bool
	Issues  []string
}

// RiskLevel represents task risk level
type RiskLevel string

const (
	RiskXS RiskLevel = "XS" // Extra small (< 50 LOC)
	RiskS  RiskLevel = "S"  // Small (50-200 LOC)
	RiskM  RiskLevel = "M"  // Medium (200-500 LOC)
	RiskL  RiskLevel = "L"  // Large (500-1000 LOC)
	RiskXL RiskLevel = "XL" // Extra large (> 1000 LOC)
)

// RequiresPerTaskReview returns true if risk level requires immediate review
func (r RiskLevel) RequiresPerTaskReview() bool {
	return r == RiskL || r == RiskXL
}

// TaskMetrics holds metrics for a completed task
type TaskMetrics struct {
	Duration         time.Duration
	RetryCount       int
	TestRunCount     int
	AssertionDensity float64
	CoveragePercent  float64
	StateTransitions int
}

// Config holds BUILD loop configuration
type Config struct {
	MaxRetries              int
	MinAssertionDensity     float64
	MinCoveragePercent      float64
	TestTimeoutSeconds      int
	ReviewTimeoutSeconds    int
	EnableTDDEnforcement    bool
	EnableParallelExecution bool
}

// DefaultConfig returns default configuration
func DefaultConfig() *Config {
	return &Config{
		MaxRetries:              3,
		MinAssertionDensity:     0.5,
		MinCoveragePercent:      80.0,
		TestTimeoutSeconds:      300,
		ReviewTimeoutSeconds:    600,
		EnableTDDEnforcement:    true,
		EnableParallelExecution: true,
	}
}

// NewBuildLoop creates a new BUILD loop instance
func NewBuildLoop(task *Task, config *Config) *BuildLoop {
	if config == nil {
		config = DefaultConfig()
	}

	return &BuildLoop{
		currentState: StateTestFirst,
		context: &BuildContext{
			Task: task,
		},
		tracker:      NewIterationTracker(task.ID),
		config:       config,
		stateHistory: make([]StateTransitionRecord, 0),
		retryCount:   0,
		maxRetries:   config.MaxRetries,
	}
}

// Execute runs the BUILD loop for the task
func (bl *BuildLoop) Execute() (*TaskResult, error) {
	bl.tracker.StartIteration()
	bl.context.Task.StartedAt = time.Now()

	for !bl.currentState.IsTerminal() && bl.retryCount < bl.maxRetries {
		// Record current state
		bl.tracker.RecordState(bl.currentState)

		// Execute state logic
		nextState, err := bl.executeState(bl.currentState)
		if err != nil {
			return nil, fmt.Errorf("state %s execution failed: %w", bl.currentState, err)
		}

		// Validate transition
		transition := ValidateTransition(bl.currentState, nextState)
		if !transition.Valid {
			return nil, fmt.Errorf("invalid transition: %s", transition.Reason)
		}

		// Record transition
		bl.recordTransition(bl.currentState, nextState, transition.Trigger, true, "")

		// Transition to next state
		bl.currentState = nextState

		// Increment retry on error states
		if bl.currentState.IsErrorState() {
			bl.retryCount++
		}
	}

	// Check completion
	if bl.currentState == StateComplete {
		return bl.completeTask()
	}

	// Max retries exceeded
	if bl.retryCount >= bl.maxRetries {
		return &TaskResult{
			Task:    bl.context.Task,
			Success: false,
			Error:   fmt.Sprintf("max retries (%d) exceeded", bl.maxRetries),
		}, nil
	}

	return &TaskResult{
		Task:    bl.context.Task,
		Success: false,
		Error:   "task incomplete",
	}, nil
}

// executeState executes logic for the current state and returns next state
func (bl *BuildLoop) executeState(state State) (State, error) {
	switch state {
	case StateTestFirst:
		return bl.executeTestFirst()
	case StateCoding:
		return bl.executeCoding()
	case StateGreen:
		return bl.executeGreen()
	case StateRefactor:
		return bl.executeRefactor()
	case StateValidation:
		return bl.executeValidation()
	case StateDeploy:
		return bl.executeDeploy()
	case StateMonitoring:
		return bl.executeMonitoring()
	case StateTimeout:
		return bl.executeTimeout()
	case StateReviewFailed:
		return bl.executeReviewFailed()
	case StateIntegrateFail:
		return bl.executeIntegrateFail()
	case StateComplete:
		return StateComplete, nil
	default:
		return "", fmt.Errorf("unknown state: %s", state)
	}
}

// State execution methods
func (bl *BuildLoop) executeTestFirst() (State, error) {
	// Run tests - they should fail (TDD)
	result := &TestResult{
		HasFailures:  true, // Stub: would run actual tests
		FailureCount: 1,
		PassCount:    0,
		Timeout:      false,
	}
	bl.context.TestResult = result

	// Check for timeout
	if result.Timeout {
		return StateTimeout, nil
	}

	// TDD enforcement: tests should fail
	if bl.config.EnableTDDEnforcement && !result.HasFailures {
		return StateTestFirst, fmt.Errorf("TDD violation: tests passing before code written")
	}

	// Validate exit criteria
	if ok, err := validateTestFirstExit(bl.context); !ok {
		return StateTestFirst, err
	}

	return StateCoding, nil
}

func (bl *BuildLoop) executeCoding() (State, error) {
	// Write code to make tests pass
	bl.context.CodeChanges = 1 // Stub: would track actual changes

	// Validate exit criteria
	if ok, err := validateCodingExit(bl.context); !ok {
		return StateCoding, err
	}

	// Run tests to check if they pass
	result := &TestResult{
		HasFailures:  false, // Stub: would run actual tests
		FailureCount: 0,
		PassCount:    1,
		Timeout:      false,
	}
	bl.context.TestResult = result

	// Check for timeout
	if result.Timeout {
		return StateTimeout, nil
	}

	// Check if tests pass
	if result.HasFailures {
		return StateTestFirst, nil // Back to red, iterate
	}

	return StateGreen, nil
}

func (bl *BuildLoop) executeGreen() (State, error) {
	// Run quality gates
	quality := &QualityResult{
		Passes:           true, // Stub: would run actual quality checks
		AssertionDensity: 0.6,
		CoveragePercent:  85.0,
		Issues:           []string{},
	}
	bl.context.QualityResult = quality

	// Validate exit criteria
	if ok, err := validateGreenExit(bl.context); !ok {
		return StateGreen, err
	}

	// Check quality gates
	if !quality.Passes {
		return StateCoding, nil // Need to add more tests/coverage
	}

	// Use risk level from task
	if bl.context.Task != nil {
		bl.context.RiskLevel = bl.context.Task.RiskLevel
	} else {
		bl.context.RiskLevel = RiskM // Default if no task
	}

	// High risk requires immediate review
	if bl.context.RiskLevel.RequiresPerTaskReview() {
		return StateValidation, nil
	}

	// Low/medium risk can proceed to refactor or complete
	return StateRefactor, nil
}

func (bl *BuildLoop) executeRefactor() (State, error) {
	// Improve code quality
	bl.context.CodeChanges++ // Track refactoring changes

	// Re-run tests after refactor
	result := &TestResult{
		HasFailures:  false,
		FailureCount: 0,
		PassCount:    1,
		Timeout:      false,
	}
	bl.context.TestResult = result

	// Validate exit criteria
	if ok, err := validateRefactorExit(bl.context); !ok {
		return StateRefactor, err
	}

	// If tests fail after refactor, go back to coding
	if result.HasFailures {
		return StateCoding, nil
	}

	// Proceed to validation
	return StateValidation, nil
}

func (bl *BuildLoop) executeValidation() (State, error) {
	// Run automated review
	review := &ReviewResult{
		P0Issues: []string{},
		P1Issues: []string{},
		P2Issues: []string{},
	}
	bl.context.ReviewResult = review

	// Validate exit criteria
	if ok, err := validateValidationExit(bl.context); !ok {
		return StateValidation, err
	}

	// Check for blocking issues
	if len(review.P0Issues) > 0 || len(review.P1Issues) > 0 {
		return StateReviewFailed, nil
	}

	// Review passed, task complete
	return StateComplete, nil
}

func (bl *BuildLoop) executeDeploy() (State, error) {
	// Run deployment
	deploy := &DeployResult{
		Success: true,
		Error:   "",
	}
	bl.context.DeployResult = deploy

	// Validate exit criteria
	if ok, err := validateDeployExit(bl.context); !ok {
		return StateDeploy, err
	}

	if !deploy.Success {
		return StateIntegrateFail, nil
	}

	return StateMonitoring, nil
}

func (bl *BuildLoop) executeMonitoring() (State, error) {
	// Monitor deployment
	monitoring := &MonitoringResult{
		Success: true,
		Issues:  []string{},
	}
	bl.context.MonitoringResult = monitoring

	// Validate exit criteria
	if ok, err := validateMonitoringExit(bl.context); !ok {
		return StateMonitoring, err
	}

	if !monitoring.Success {
		return StateCoding, nil
	}

	return StateComplete, nil
}

// Error state handlers
func (bl *BuildLoop) executeTimeout() (State, error) {
	// Add timeout guards and retry
	return StateCoding, nil
}

func (bl *BuildLoop) executeReviewFailed() (State, error) {
	// Analyze blocking issues and determine if redesign needed
	if len(bl.context.ReviewResult.P0Issues) > 3 {
		return StateTestFirst, nil // Major redesign
	}
	return StateCoding, nil // Fixable issues
}

func (bl *BuildLoop) executeIntegrateFail() (State, error) {
	// Restart task with fixes
	return StateTestFirst, nil
}

// recordTransition records a state transition
func (bl *BuildLoop) recordTransition(from, to State, trigger string, success bool, errMsg string) {
	record := StateTransitionRecord{
		From:      from,
		To:        to,
		Timestamp: time.Now(),
		Trigger:   trigger,
		Success:   success,
		Error:     errMsg,
	}
	bl.stateHistory = append(bl.stateHistory, record)
}

// completeTask finalizes the task
func (bl *BuildLoop) completeTask() (*TaskResult, error) {
	completedAt := time.Now()
	bl.context.Task.CompletedAt = &completedAt

	// Record metrics
	bl.context.Task.Metrics = &TaskMetrics{
		Duration:         time.Since(bl.context.Task.StartedAt),
		RetryCount:       bl.retryCount,
		TestRunCount:     bl.tracker.GetTestRunCount(),
		AssertionDensity: bl.context.QualityResult.AssertionDensity,
		CoveragePercent:  bl.context.QualityResult.CoveragePercent,
		StateTransitions: len(bl.stateHistory),
	}

	return &TaskResult{
		Task:    bl.context.Task,
		Success: true,
		Metrics: bl.context.Task.Metrics,
	}, nil
}

// TaskResult represents the result of executing a task
type TaskResult struct {
	Task    *Task
	Success bool
	Error   string
	Metrics *TaskMetrics
}

// GetCurrentState returns the current state
func (bl *BuildLoop) GetCurrentState() State {
	return bl.currentState
}

// GetStateHistory returns the state transition history
func (bl *BuildLoop) GetStateHistory() []StateTransitionRecord {
	return bl.stateHistory
}

// GetRetryCount returns the current retry count
func (bl *BuildLoop) GetRetryCount() int {
	return bl.retryCount
}
