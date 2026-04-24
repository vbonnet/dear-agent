package orchestrator

import (
	"fmt"
	"log"
	"time"

	"github.com/vbonnet/dear-agent/wayfinder/cmd/wayfinder-session/internal/lintcontext"
	"github.com/vbonnet/dear-agent/wayfinder/cmd/wayfinder-session/internal/status"
)

// BuildLoopState represents the current state in the BUILD loop
type BuildLoopState string

const (
	// BuildLoopStateIdle means no BUILD loop is active
	BuildLoopStateIdle BuildLoopState = "idle"

	// BuildLoopStateTestingPre means running tests before implementation (should FAIL)
	BuildLoopStateTestingPre BuildLoopState = "testing-pre"

	// BuildLoopStateCoding means writing implementation code
	BuildLoopStateCoding BuildLoopState = "coding"

	// BuildLoopStateTestingPost means running tests after implementation (should PASS)
	BuildLoopStateTestingPost BuildLoopState = "testing-post"

	// BuildLoopStateValidating means running validation checks
	BuildLoopStateValidating BuildLoopState = "validating"

	// BuildLoopStateTaskComplete means current task is complete, ready for next
	BuildLoopStateTaskComplete BuildLoopState = "task-complete"

	// BuildLoopStateIntegrationTesting means running integration tests (after all tasks)
	BuildLoopStateIntegrationTesting BuildLoopState = "integration-testing"

	// BuildLoopStateDeploying means deploying to target environment
	BuildLoopStateDeploying BuildLoopState = "deploying"

	// BuildLoopStateComplete means BUILD loop is complete
	BuildLoopStateComplete BuildLoopState = "complete"
)

// BuildLoopContext holds state for BUILD loop execution
type BuildLoopContext struct {
	CurrentState        BuildLoopState
	CurrentTaskID       string
	TotalTasks          int
	CompletedTasks      int
	BuildIterations     int
	LastTestResult      TestResult
	ValidationsPending  []string
	IntegrationComplete bool
	DeploymentComplete  bool
	// LintContext holds a concise summary of project lint rules,
	// populated by lintcontext.Summarize() when BUILD starts.
	LintContext string
}

// TestResult represents the result of running tests
type TestResult struct {
	Passed       bool
	TotalTests   int
	PassedTests  int
	FailedTests  int
	Timestamp    time.Time
	ErrorMessage string
}

// BuildLoopExecutor manages the BUILD loop state machine
type BuildLoopExecutor struct {
	orchestrator *PhaseOrchestratorV2
	context      *BuildLoopContext
	projectDir   string // optional: set via SetProjectDir for lint context injection
}

// NewBuildLoopExecutor creates a new BUILD loop executor
func NewBuildLoopExecutor(orchestrator *PhaseOrchestratorV2) *BuildLoopExecutor {
	return &BuildLoopExecutor{
		orchestrator: orchestrator,
		context: &BuildLoopContext{
			CurrentState: BuildLoopStateIdle,
		},
	}
}

// SetProjectDir sets the project directory for lint context injection.
// When set, StartBuildLoop will summarize lint rules and populate LintContext.
func (e *BuildLoopExecutor) SetProjectDir(dir string) {
	e.projectDir = dir
}

// GetLintContext returns the lint context summary, or empty string if not populated.
func (e *BuildLoopExecutor) GetLintContext() string {
	return e.context.LintContext
}

// StartBuildLoop initializes the BUILD loop for BUILD phase
func (e *BuildLoopExecutor) StartBuildLoop() error {
	// Verify we're in BUILD phase
	if e.orchestrator.GetCurrentPhase() != status.PhaseV2Build {
		return fmt.Errorf("BUILD loop can only start in BUILD phase, current phase: %s",
			e.orchestrator.GetCurrentPhase())
	}

	// Get tasks from roadmap
	tasks, err := e.getBuildTasks()
	if err != nil {
		return fmt.Errorf("failed to get BUILD tasks: %w", err)
	}

	if len(tasks) == 0 {
		return fmt.Errorf("no tasks defined for BUILD phase")
	}

	// Summarize lint rules if project directory is set
	var lintSummary string
	if e.projectDir != "" {
		summary, lintErr := lintcontext.Summarize(e.projectDir)
		if lintErr != nil {
			log.Printf("warning: failed to summarize lint config: %v", lintErr)
		} else {
			lintSummary = summary
		}
	}

	// Initialize context
	e.context = &BuildLoopContext{
		CurrentState:   BuildLoopStateTestingPre,
		CurrentTaskID:  tasks[0].ID,
		TotalTasks:     len(tasks),
		CompletedTasks: 0,
		LintContext:    lintSummary,
	}

	return nil
}

// AdvanceState advances the BUILD loop to the next state
func (e *BuildLoopExecutor) AdvanceState() (BuildLoopState, error) {
	switch e.context.CurrentState {
	case BuildLoopStateIdle:
		return BuildLoopStateIdle, fmt.Errorf("BUILD loop not started")

	case BuildLoopStateTestingPre:
		// After pre-testing (expecting FAIL), move to coding
		return e.transitionToState(BuildLoopStateCoding)

	case BuildLoopStateCoding:
		// After coding, run tests again (expecting PASS)
		return e.transitionToState(BuildLoopStateTestingPost)

	case BuildLoopStateTestingPost:
		// After post-testing, validate
		return e.transitionToState(BuildLoopStateValidating)

	case BuildLoopStateValidating:
		// After validation, task is complete
		return e.transitionToState(BuildLoopStateTaskComplete)

	case BuildLoopStateTaskComplete:
		// Move to next task or integration testing
		return e.advanceToNextTask()

	case BuildLoopStateIntegrationTesting:
		// After integration tests, deploy
		return e.transitionToState(BuildLoopStateDeploying)

	case BuildLoopStateDeploying:
		// After deployment, BUILD loop complete
		return e.transitionToState(BuildLoopStateComplete)

	case BuildLoopStateComplete:
		return BuildLoopStateComplete, fmt.Errorf("BUILD loop already complete")

	default:
		return BuildLoopStateIdle, fmt.Errorf("unknown state: %s", e.context.CurrentState)
	}
}

// RecordTestResult records the result of a test run
func (e *BuildLoopExecutor) RecordTestResult(result TestResult) error {
	e.context.LastTestResult = result

	// Validate TDD discipline
	if e.context.CurrentState == BuildLoopStateTestingPre {
		// Tests MUST fail before implementation
		if result.Passed {
			return fmt.Errorf("TDD violation: tests passed before implementation. " +
				"Tests should fail initially to validate they're testing the right thing")
		}
	}

	if e.context.CurrentState == BuildLoopStateTestingPost {
		// Tests MUST pass after implementation
		if !result.Passed {
			return fmt.Errorf("tests failed after implementation. " +
				"Fix failures before proceeding")
		}
	}

	return nil
}

// GetCurrentState returns the current BUILD loop state
func (e *BuildLoopExecutor) GetCurrentState() BuildLoopState {
	return e.context.CurrentState
}

// GetCurrentTask returns the current task being worked on
func (e *BuildLoopExecutor) GetCurrentTask() (string, error) {
	if e.context.CurrentTaskID == "" {
		return "", fmt.Errorf("no current task")
	}
	return e.context.CurrentTaskID, nil
}

// GetProgress returns BUILD loop progress
func (e *BuildLoopExecutor) GetProgress() (completed, total int) {
	return e.context.CompletedTasks, e.context.TotalTasks
}

// GetBuildIterations returns the number of build iterations
func (e *BuildLoopExecutor) GetBuildIterations() int {
	return e.context.BuildIterations
}

// MarkIntegrationTestsComplete marks integration tests as complete
func (e *BuildLoopExecutor) MarkIntegrationTestsComplete() error {
	if e.context.CurrentState != BuildLoopStateIntegrationTesting {
		return fmt.Errorf("not in integration testing state")
	}
	e.context.IntegrationComplete = true
	return nil
}

// MarkDeploymentComplete marks deployment as complete
func (e *BuildLoopExecutor) MarkDeploymentComplete() error {
	if e.context.CurrentState != BuildLoopStateDeploying {
		return fmt.Errorf("not in deploying state")
	}
	e.context.DeploymentComplete = true
	return nil
}

// transitionToState transitions to a new state
func (e *BuildLoopExecutor) transitionToState(newState BuildLoopState) (BuildLoopState, error) {
	e.context.CurrentState = newState
	e.context.BuildIterations++
	return newState, nil
}

// advanceToNextTask advances to the next task or integration testing
func (e *BuildLoopExecutor) advanceToNextTask() (BuildLoopState, error) {
	e.context.CompletedTasks++

	// Check if all tasks complete
	if e.context.CompletedTasks >= e.context.TotalTasks {
		// All tasks done, move to integration testing
		e.context.IntegrationComplete = false
		return e.transitionToState(BuildLoopStateIntegrationTesting)
	}

	// Get next task
	tasks, err := e.getBuildTasks()
	if err != nil {
		return BuildLoopStateIdle, fmt.Errorf("failed to get tasks: %w", err)
	}

	// Find next pending task
	nextTask, err := e.findNextTask(tasks)
	if err != nil {
		return BuildLoopStateIdle, err
	}

	e.context.CurrentTaskID = nextTask.ID
	return e.transitionToState(BuildLoopStateTestingPre)
}

// getBuildTasks retrieves tasks for BUILD phase from roadmap
func (e *BuildLoopExecutor) getBuildTasks() ([]status.Task, error) {
	st := e.orchestrator.status

	if st.Roadmap == nil {
		return nil, fmt.Errorf("no roadmap defined")
	}

	// Find BUILD phase in roadmap
	for _, phase := range st.Roadmap.Phases {
		if phase.ID == status.PhaseV2Build {
			return phase.Tasks, nil
		}
	}

	return nil, fmt.Errorf("BUILD phase not found in roadmap")
}

// findNextTask finds the next pending task that's ready to execute
func (e *BuildLoopExecutor) findNextTask(tasks []status.Task) (*status.Task, error) {
	// Build dependency map
	completed := make(map[string]bool)
	for _, task := range tasks {
		if task.Status == status.TaskStatusCompleted {
			completed[task.ID] = true
		}
	}

	// Find first pending task with all dependencies met
	for i := range tasks {
		task := &tasks[i]
		if task.Status == status.TaskStatusPending || task.Status == status.TaskStatusInProgress {
			// Check dependencies
			allDepsMet := true
			for _, depID := range task.DependsOn {
				if !completed[depID] {
					allDepsMet = false
					break
				}
			}

			if allDepsMet {
				return task, nil
			}
		}
	}

	return nil, fmt.Errorf("no ready tasks found")
}

// ValidateTDDCycle validates the TDD cycle was followed
func (e *BuildLoopExecutor) ValidateTDDCycle() error {
	if e.context.BuildIterations == 0 {
		return fmt.Errorf("no build iterations recorded")
	}

	// Check that we have test results
	if e.context.LastTestResult.Timestamp.IsZero() {
		return fmt.Errorf("no test results recorded")
	}

	return nil
}

// GetValidationRequirements returns validations needed for current state
func (e *BuildLoopExecutor) GetValidationRequirements() []string {
	switch e.context.CurrentState {
	case BuildLoopStateTestingPre:
		return []string{"Run tests (expect FAIL)"}
	case BuildLoopStateCoding:
		return []string{"Implement task requirements"}
	case BuildLoopStateTestingPost:
		return []string{"Run tests (expect PASS)"}
	case BuildLoopStateValidating:
		return []string{
			"Check code coverage",
			"Validate assertion density",
			"Run linter",
		}
	case BuildLoopStateIntegrationTesting:
		return []string{
			"Run integration test suite",
			"Validate system behavior",
		}
	case BuildLoopStateDeploying:
		return []string{
			"Deploy to target environment",
			"Run smoke tests",
			"Verify deployment health",
		}
	case BuildLoopStateIdle, BuildLoopStateTaskComplete, BuildLoopStateComplete:
		return []string{}
	}
	return nil
}

// IsBuildLoopComplete checks if BUILD loop is complete
func (e *BuildLoopExecutor) IsBuildLoopComplete() bool {
	return e.context.CurrentState == BuildLoopStateComplete
}

// ResetBuildLoop resets the BUILD loop state
func (e *BuildLoopExecutor) ResetBuildLoop() {
	e.context = &BuildLoopContext{
		CurrentState: BuildLoopStateIdle,
	}
}

// UpdatePhaseHistory updates the phase history with BUILD loop metrics
func (e *BuildLoopExecutor) UpdatePhaseHistory() error {
	st := e.orchestrator.status

	// Find BUILD phase in history
	for i := len(st.WaypointHistory) - 1; i >= 0; i-- {
		if st.WaypointHistory[i].Name == status.PhaseV2Build {
			// Update build iterations
			st.WaypointHistory[i].BuildIterations = e.context.BuildIterations

			// Update validation status
			if e.context.CurrentState == BuildLoopStateComplete {
				st.WaypointHistory[i].ValidationStatus = status.ValidationStatusPassed
				st.WaypointHistory[i].DeploymentStatus = status.DeploymentStatusDeployed
			}

			// Update build metrics if available
			if !e.context.LastTestResult.Timestamp.IsZero() {
				if st.WaypointHistory[i].BuildMetrics == nil {
					st.WaypointHistory[i].BuildMetrics = &status.BuildMetrics{}
				}
				st.WaypointHistory[i].BuildMetrics.TestsPassed = e.context.LastTestResult.PassedTests
				st.WaypointHistory[i].BuildMetrics.TestsFailed = e.context.LastTestResult.FailedTests
			}

			break
		}
	}

	return nil
}
