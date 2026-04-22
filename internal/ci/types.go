package ci

import "time"

// EventType identifies the GitHub event that triggered the pipeline.
type EventType string

const (
	// EventPullRequest is triggered by pull_request events
	EventPullRequest EventType = "pull_request"

	// EventPush is triggered by push events
	EventPush EventType = "push"

	// EventWorkflowDispatch is triggered by manual workflow_dispatch
	EventWorkflowDispatch EventType = "workflow_dispatch"

	// EventSchedule is triggered by cron schedule
	EventSchedule EventType = "schedule"
)

// PipelineRequest contains all parameters needed to execute a CI pipeline.
type PipelineRequest struct {
	// EventType identifies what triggered this pipeline run.
	// Determines which workflow triggers to match.
	EventType EventType

	// WorkflowPath is the absolute path to the workflow file.
	// Examples:
	//   - ".github/workflows/test.yml"
	//   - ".github/workflows/lint.yml"
	WorkflowPath string

	// WorkingDir is the repository root where the workflow executes.
	// All workflow steps run relative to this directory.
	WorkingDir string

	// Secrets are injected into the workflow environment.
	// Key: SECRET_NAME, Value: secret value
	//
	// Example: {"ANTHROPIC_API_KEY": "sk-ant-..."}
	Secrets map[string]string

	// Vars are workflow variables (non-sensitive configuration).
	// Key: VAR_NAME, Value: variable value
	//
	// Example: {"NODE_VERSION": "18", "GO_VERSION": "1.21"}
	Vars map[string]string

	// EventPayload is the GitHub event payload (JSON).
	// Used by workflows that reference github.event.* context.
	//
	// Can be empty for simple workflows.
	EventPayload string

	// Timeout is the maximum duration for pipeline execution.
	// Zero means no timeout (use context cancellation instead).
	Timeout time.Duration

	// OutputCallback receives streaming output from pipeline execution.
	// Called once per output chunk (stdout/stderr).
	//
	// If nil, executor buffers all output in PipelineResult.Output.
	//
	// Thread-safety: Callback may be called concurrently from multiple
	// goroutines if multiple steps run in parallel.
	OutputCallback func(event PipelineEvent)
}

// PipelineResult represents the outcome of a pipeline execution.
type PipelineResult struct {
	// Success is true if all pipeline steps passed.
	// False if any step failed (exit code != 0).
	Success bool

	// ExitCode is the final exit code of the pipeline.
	// 0 = success, non-zero = failure
	ExitCode int

	// Output contains the complete stdout/stderr from the pipeline.
	// Only populated if PipelineRequest.OutputCallback is nil.
	//
	// Format: Plain text with ANSI color codes preserved.
	Output string

	// Steps contains detailed results for each workflow step.
	// Useful for identifying which step failed and why.
	Steps []StepResult

	// Duration is the total execution time.
	Duration time.Duration

	// StartedAt is when execution began.
	StartedAt time.Time

	// FinishedAt is when execution completed.
	FinishedAt time.Time

	// ExecutorName identifies which executor ran this pipeline.
	// Matches PipelineExecutor.Name()
	ExecutorName string
}

// StepResult represents the outcome of a single workflow step.
type StepResult struct {
	// Name is the step name from the workflow file.
	// Example: "Run tests", "Lint code"
	Name string

	// Status is the step outcome.
	Status StepStatus

	// ExitCode is the step's exit code (if applicable).
	// Only set for "run" steps, not for action steps.
	ExitCode int

	// Output is the stdout/stderr from this step.
	Output string

	// Duration is how long this step took.
	Duration time.Duration

	// StartedAt is when this step began.
	StartedAt time.Time

	// FinishedAt is when this step completed.
	FinishedAt time.Time
}

// StepStatus represents the outcome of a workflow step.
type StepStatus string

const (
	// StepStatusPending means the step hasn't started yet
	StepStatusPending StepStatus = "pending"

	// StepStatusRunning means the step is currently executing
	StepStatusRunning StepStatus = "running"

	// StepStatusSuccess means the step completed with exit code 0
	StepStatusSuccess StepStatus = "success"

	// StepStatusFailure means the step failed (exit code != 0)
	StepStatusFailure StepStatus = "failure"

	// StepStatusSkipped means the step was skipped (conditional)
	StepStatusSkipped StepStatus = "skipped"

	// StepStatusCancelled means the step was cancelled (context cancellation)
	StepStatusCancelled StepStatus = "cancelled"
)

// PipelineEvent represents a streaming output event during pipeline execution.
// Sent to PipelineRequest.OutputCallback for real-time monitoring.
type PipelineEvent struct {
	// Type identifies what kind of event this is.
	Type EventKind

	// StepName is the workflow step that generated this event.
	// Empty for pipeline-level events.
	StepName string

	// Output is the output chunk (stdout/stderr).
	// Only set for EventKindOutput.
	Output string

	// Timestamp is when this event occurred.
	Timestamp time.Time

	// Metadata contains additional context about the event.
	// Examples:
	//   - "exit_code": "1"
	//   - "duration_ms": "1234"
	Metadata map[string]string
}

// EventKind identifies the type of pipeline event.
type EventKind string

const (
	// EventKindStart is sent when pipeline execution begins
	EventKindStart EventKind = "start"

	// EventKindStepStart is sent when a step begins
	EventKindStepStart EventKind = "step_start"

	// EventKindOutput is sent for stdout/stderr chunks
	EventKindOutput EventKind = "output"

	// EventKindStepEnd is sent when a step completes
	EventKindStepEnd EventKind = "step_end"

	// EventKindEnd is sent when pipeline execution completes
	EventKindEnd EventKind = "end"

	// EventKindError is sent for executor errors (not step failures)
	EventKindError EventKind = "error"
)
