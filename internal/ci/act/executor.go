// Package act provides a PipelineExecutor implementation using nektos/act.
//
// nektos/act runs GitHub Actions workflows locally in Docker containers,
// providing high fidelity with actual GitHub Actions behavior.
//
// Key features:
//   - Runs workflows defined in .github/workflows/*.yml
//   - Simulates GitHub Actions environment
//   - Supports secrets, variables, and artifacts
//   - Requires Docker daemon (rootless or rootful)
//
// Usage:
//
//	executor := act.NewActExecutor()
//	result, err := executor.Execute(ctx, req)
package act

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/vbonnet/dear-agent/internal/ci"
)

// ActExecutor implements PipelineExecutor using nektos/act.
type ActExecutor struct {
	// actPath is the path to the act binary (default: "act")
	actPath string

	// defaultArtifactPath is where act stores workflow artifacts
	defaultArtifactPath string
}

// NewActExecutor creates a new act-based executor with default configuration.
func NewActExecutor() *ActExecutor {
	return &ActExecutor{
		actPath:             "act",
		defaultArtifactPath: "/tmp/act-artifacts",
	}
}

// NewActExecutorWithPath creates an executor with custom act binary path.
func NewActExecutorWithPath(actPath, artifactPath string) *ActExecutor {
	return &ActExecutor{
		actPath:             actPath,
		defaultArtifactPath: artifactPath,
	}
}

// Name returns the executor identifier.
func (e *ActExecutor) Name() string {
	return "act-native"
}

// Execute runs a GitHub Actions workflow using nektos/act.
//
// The method:
//  1. Validates the act binary exists
//  2. Builds the act command with appropriate flags
//  3. Handles secrets by writing them to a temporary file
//  4. Executes the workflow and captures output
//  5. Returns a PipelineResult with timing and exit code
//
// Important: A failed pipeline (exit != 0) is NOT an error - it's reflected
// in PipelineResult.Success = false. Only infrastructure failures return errors.
func (e *ActExecutor) Execute(ctx context.Context, req ci.PipelineRequest) (*ci.PipelineResult, error) {
	startTime := time.Now()

	// Build the command arguments
	args, cleanup, err := e.buildCommand(req)
	if err != nil {
		return nil, err
	}
	defer cleanup()

	// Create the command
	cmd := exec.CommandContext(ctx, e.actPath, args...)
	cmd.Dir = req.WorkingDir

	// Capture output
	var outputBuilder strings.Builder
	if req.OutputCallback != nil {
		// Stream output through callback
		cmd.Stdout = &callbackWriter{
			callback: req.OutputCallback,
			stepName: "", // act outputs are pipeline-level, not per-step
		}
		cmd.Stderr = &callbackWriter{
			callback: req.OutputCallback,
			stepName: "",
		}
	} else {
		// Buffer output
		cmd.Stdout = &outputBuilder
		cmd.Stderr = &outputBuilder
	}

	// Send start event if callback is configured
	if req.OutputCallback != nil {
		req.OutputCallback(ci.PipelineEvent{
			Type:      ci.EventKindStart,
			Timestamp: startTime,
		})
	}

	// Execute the workflow
	execErr := cmd.Run()
	finishTime := time.Now()

	// Determine exit code and success
	exitCode := 0
	success := true

	if execErr != nil {
		// Check if it's a context cancellation/timeout
		if ctx.Err() != nil {
			if ctx.Err() == context.DeadlineExceeded {
				return nil, ci.ErrTimeout(req.Timeout.String())
			}
			// Context was cancelled
			return nil, ci.WrapError(
				ci.ErrCodeInfrastructure,
				"pipeline execution cancelled",
				ctx.Err(),
			)
		}

		// Check if it's an exit error (pipeline failure, not infrastructure failure)
		exitErr := &exec.ExitError{}
		if errors.As(execErr, &exitErr) {
			exitCode = exitErr.ExitCode()
			success = false
			// Don't return error - pipeline failure is not an infrastructure error
		} else {
			// Infrastructure error (e.g., binary not found, permission denied)
			return nil, e.wrapInfrastructureError(execErr)
		}
	}

	// Build result
	result := &ci.PipelineResult{
		Success:      success,
		ExitCode:     exitCode,
		Output:       outputBuilder.String(),
		Steps:        []ci.StepResult{}, // act doesn't provide per-step details easily
		Duration:     finishTime.Sub(startTime),
		StartedAt:    startTime,
		FinishedAt:   finishTime,
		ExecutorName: e.Name(),
	}

	// Send end event if callback is configured
	if req.OutputCallback != nil {
		metadata := map[string]string{
			"exit_code":   fmt.Sprintf("%d", exitCode),
			"duration_ms": fmt.Sprintf("%d", result.Duration.Milliseconds()),
		}
		req.OutputCallback(ci.PipelineEvent{
			Type:      ci.EventKindEnd,
			Timestamp: finishTime,
			Metadata:  metadata,
		})
	}

	return result, nil
}

// Validate checks if the workflow configuration is valid and all dependencies are available.
func (e *ActExecutor) Validate(ctx context.Context, req ci.PipelineRequest) error {
	// Check if act binary exists
	actPath, err := exec.LookPath(e.actPath)
	if err != nil {
		return ci.ErrExecutorNotFound(e.actPath)
	}

	// Check if workflow file exists (convert to absolute path if needed)
	workflowPath := req.WorkflowPath
	if !filepath.IsAbs(workflowPath) {
		workflowPath = filepath.Join(req.WorkingDir, workflowPath)
	}

	if _, err := os.Stat(workflowPath); os.IsNotExist(err) {
		return ci.ErrWorkflowNotFound(req.WorkflowPath)
	}

	// Check if Docker is available (act requires Docker)
	if err := e.validateDockerAvailable(ctx); err != nil {
		return err
	}

	// Verify act binary is executable
	if err := exec.CommandContext(ctx, actPath, "--version").Run(); err != nil {
		return ci.WrapError(
			ci.ErrCodeExecutorNotFound,
			"act binary is not executable",
			err,
		)
	}

	return nil
}

// buildCommand constructs the act command arguments and returns a cleanup function.
func (e *ActExecutor) buildCommand(req ci.PipelineRequest) (args []string, cleanup func(), err error) {
	cleanup = func() {} // Default no-op cleanup

	// Start with the event type (e.g., "pull_request", "push")
	args = []string{string(req.EventType)}

	// Add artifact server path (prevents upload failures)
	artifactPath := e.defaultArtifactPath
	args = append(args, "--artifact-server-path", artifactPath)

	// Add workflow path if specified
	if req.WorkflowPath != "" {
		args = append(args, "--workflows", req.WorkflowPath)
	}

	// Handle secrets - write to temporary file
	if len(req.Secrets) > 0 {
		secretFile, err := e.writeSecretFile(req.Secrets)
		if err != nil {
			return nil, cleanup, ci.WrapError(
				ci.ErrCodeInfrastructure,
				"failed to create secrets file",
				err,
			)
		}
		args = append(args, "--secret-file", secretFile)

		// Update cleanup to remove secret file
		cleanup = func() {
			_ = os.Remove(secretFile)
		}
	}

	// Add variables as --var flags
	for key, value := range req.Vars {
		args = append(args, "--var", fmt.Sprintf("%s=%s", key, value))
	}

	// Add event payload if provided
	if req.EventPayload != "" {
		payloadFile, err := e.writeEventPayload(req.EventPayload)
		if err != nil {
			cleanup() // Clean up secrets file if already created
			return nil, func() {}, ci.WrapError(
				ci.ErrCodeInfrastructure,
				"failed to create event payload file",
				err,
			)
		}

		args = append(args, "--eventpath", payloadFile)

		// Update cleanup to remove both files
		oldCleanup := cleanup
		cleanup = func() {
			oldCleanup()
			_ = os.Remove(payloadFile)
		}
	}

	return args, cleanup, nil
}

// writeSecretFile creates a temporary file with secrets in KEY=VALUE format.
func (e *ActExecutor) writeSecretFile(secrets map[string]string) (string, error) {
	// Create temp file
	tmpFile, err := os.CreateTemp("", "act-secrets-*.env")
	if err != nil {
		return "", err
	}
	defer tmpFile.Close()

	// Write secrets in KEY=VALUE format
	for key, value := range secrets {
		if _, err := fmt.Fprintf(tmpFile, "%s=%s\n", key, value); err != nil {
			os.Remove(tmpFile.Name())
			return "", err
		}
	}

	return tmpFile.Name(), nil
}

// writeEventPayload creates a temporary file with the event payload JSON.
func (e *ActExecutor) writeEventPayload(payload string) (string, error) {
	tmpFile, err := os.CreateTemp("", "act-event-*.json")
	if err != nil {
		return "", err
	}
	defer tmpFile.Close()

	if _, err := tmpFile.WriteString(payload); err != nil {
		os.Remove(tmpFile.Name())
		return "", err
	}

	return tmpFile.Name(), nil
}

// validateDockerAvailable checks if Docker daemon is accessible.
func (e *ActExecutor) validateDockerAvailable(ctx context.Context) error {
	// Try to run docker version command
	cmd := exec.CommandContext(ctx, "docker", "version")
	if err := cmd.Run(); err != nil {
		return ci.ErrEnvironmentMissing(
			"docker",
			"Docker daemon not available. nektos/act requires Docker to run workflows.",
		)
	}
	return nil
}

// wrapInfrastructureError converts exec errors to appropriate CI errors.
func (e *ActExecutor) wrapInfrastructureError(err error) error {
	errMsg := err.Error()

	// Check for common error patterns
	if strings.Contains(errMsg, "executable file not found") ||
		strings.Contains(errMsg, "no such file or directory") {
		return ci.ErrExecutorNotFound(e.actPath)
	}

	if strings.Contains(errMsg, "permission denied") {
		return ci.WrapError(
			ci.ErrCodePermissionDenied,
			"permission denied executing act",
			err,
		)
	}

	// Generic infrastructure error
	return ci.WrapError(
		ci.ErrCodeInfrastructure,
		"act execution failed",
		err,
	)
}

// callbackWriter wraps a callback function to implement io.Writer interface.
type callbackWriter struct {
	callback func(event ci.PipelineEvent)
	stepName string
}

func (w *callbackWriter) Write(p []byte) (n int, err error) {
	if w.callback != nil {
		w.callback(ci.PipelineEvent{
			Type:      ci.EventKindOutput,
			StepName:  w.stepName,
			Output:    string(p),
			Timestamp: time.Now(),
		})
	}
	return len(p), nil
}

// WorkflowResult represents the outcome of a single workflow execution.
type WorkflowResult struct {
	// WorkflowPath is the path to the workflow file that was executed
	WorkflowPath string

	// Result is the pipeline execution result (nil if not executed)
	Result *ci.PipelineResult

	// Error is any infrastructure error that occurred (nil if successful)
	Error error

	// Skipped indicates if this workflow was skipped due to dependencies
	Skipped bool

	// SkipReason explains why the workflow was skipped
	SkipReason string
}

// MultiWorkflowResult aggregates results from multiple workflow executions.
type MultiWorkflowResult struct {
	// Results maps workflow paths to their execution results
	Results map[string]*WorkflowResult

	// OverallSuccess is true if all required workflows passed
	OverallSuccess bool

	// TotalDuration is the total time spent executing workflows
	TotalDuration time.Duration

	// StartedAt is when execution began
	StartedAt time.Time

	// FinishedAt is when all executions completed
	FinishedAt time.Time
}

// ExecuteWorkflows runs multiple workflows sequentially or in parallel.
//
// Behavior:
//   - Sequential (default): Runs workflows in order, stops on first failure
//   - Parallel: Runs all workflows concurrently, waits for all to complete
//
// Returns:
//   - MultiWorkflowResult with individual and aggregate results
//   - Error only for infrastructure failures (not workflow failures)
func (e *ActExecutor) ExecuteWorkflows(
	ctx context.Context,
	req ci.PipelineRequest,
	workflows []string,
	parallel bool,
) (*MultiWorkflowResult, error) {
	if parallel {
		return e.executeWorkflowsParallel(ctx, req, workflows)
	}
	return e.executeWorkflowsSequential(ctx, req, workflows)
}

// ExecuteWorkflowsWithDependencies runs workflows respecting dependency order.
//
// This method:
//  1. Topologically sorts workflows based on dependencies
//  2. Runs independent workflows in parallel
//  3. Waits for dependencies before starting dependent workflows
//  4. Stops execution if a dependency fails
//
// Parameters:
//   - dependencies: map of workflow -> list of workflows it depends on
//
// Example:
//
//	dependencies := map[string][]string{
//	    "deploy.yml": {"test.yml", "lint.yml"},
//	    "e2e.yml": {"deploy.yml"},
//	}
func (e *ActExecutor) ExecuteWorkflowsWithDependencies(
	ctx context.Context,
	req ci.PipelineRequest,
	workflows []string,
	dependencies map[string][]string,
) (*MultiWorkflowResult, error) {
	startTime := time.Now()

	// Build execution plan (topological sort)
	plan, err := buildExecutionPlan(workflows, dependencies)
	if err != nil {
		return nil, ci.WrapError(
			ci.ErrCodeInvalidRequest,
			"failed to build workflow execution plan",
			err,
		)
	}

	results := make(map[string]*WorkflowResult)
	var resultsMu sync.Mutex

	// Execute in phases
	for _, phase := range plan {
		// Check if any dependencies failed
		if err := checkDependenciesPassed(phase, dependencies, results); err != nil {
			// Skip remaining workflows
			for _, workflow := range phase {
				resultsMu.Lock()
				results[workflow] = &WorkflowResult{
					WorkflowPath: workflow,
					Skipped:      true,
					SkipReason:   err.Error(),
				}
				resultsMu.Unlock()
			}
			continue
		}

		// Run this phase (parallel within phase)
		var wg sync.WaitGroup
		for _, workflow := range phase {
			wg.Add(1)
			go func(w string) {
				defer wg.Done()

				// Create workflow-specific request
				workflowReq := req
				workflowReq.WorkflowPath = w

				result, execErr := e.Execute(ctx, workflowReq)

				resultsMu.Lock()
				results[w] = &WorkflowResult{
					WorkflowPath: w,
					Result:       result,
					Error:        execErr,
				}
				resultsMu.Unlock()
			}(workflow)
		}
		wg.Wait()

		// Check for context cancellation
		if ctx.Err() != nil {
			break
		}
	}

	// Aggregate results
	finishTime := time.Now()
	multiResult := buildMultiWorkflowResult(results, startTime, finishTime)

	// Check for context cancellation
	if ctx.Err() != nil {
		return multiResult, ctx.Err()
	}

	return multiResult, nil
}

// executeWorkflowsSequential runs workflows one at a time in order.
func (e *ActExecutor) executeWorkflowsSequential(
	ctx context.Context,
	req ci.PipelineRequest,
	workflows []string,
) (*MultiWorkflowResult, error) {
	startTime := time.Now()
	results := make(map[string]*WorkflowResult)

	for _, workflow := range workflows {
		// Check for context cancellation
		if ctx.Err() != nil {
			// Mark remaining workflows as skipped
			for i, w := range workflows {
				if _, exists := results[w]; !exists && i > 0 {
					results[w] = &WorkflowResult{
						WorkflowPath: w,
						Skipped:      true,
						SkipReason:   "execution cancelled",
					}
				}
			}
			break
		}

		// Execute workflow
		workflowReq := req
		workflowReq.WorkflowPath = workflow

		result, err := e.Execute(ctx, workflowReq)
		results[workflow] = &WorkflowResult{
			WorkflowPath: workflow,
			Result:       result,
			Error:        err,
		}

		// Stop on first failure (infrastructure or pipeline)
		if err != nil || (result != nil && !result.Success) {
			// Mark remaining workflows as skipped
			for _, w := range workflows {
				if _, exists := results[w]; !exists {
					results[w] = &WorkflowResult{
						WorkflowPath: w,
						Skipped:      true,
						SkipReason:   fmt.Sprintf("previous workflow failed: %s", workflow),
					}
				}
			}
			break
		}
	}

	finishTime := time.Now()
	return buildMultiWorkflowResult(results, startTime, finishTime), nil
}

// executeWorkflowsParallel runs all workflows concurrently.
func (e *ActExecutor) executeWorkflowsParallel(
	ctx context.Context,
	req ci.PipelineRequest,
	workflows []string,
) (*MultiWorkflowResult, error) {
	startTime := time.Now()
	results := make(map[string]*WorkflowResult)
	var resultsMu sync.Mutex
	var wg sync.WaitGroup

	for _, workflow := range workflows {
		wg.Add(1)
		go func(w string) {
			defer wg.Done()

			// Execute workflow
			workflowReq := req
			workflowReq.WorkflowPath = w

			result, err := e.Execute(ctx, workflowReq)

			resultsMu.Lock()
			results[w] = &WorkflowResult{
				WorkflowPath: w,
				Result:       result,
				Error:        err,
			}
			resultsMu.Unlock()
		}(workflow)
	}

	wg.Wait()
	finishTime := time.Now()
	return buildMultiWorkflowResult(results, startTime, finishTime), nil
}

// Helper functions

// buildExecutionPlan creates a topologically sorted execution plan.
// Returns phases where workflows in each phase can run in parallel.
func buildExecutionPlan(workflows []string, dependencies map[string][]string) ([][]string, error) {
	// Build in-degree map
	inDegree := make(map[string]int)
	graph := make(map[string][]string)

	// Initialize
	for _, w := range workflows {
		inDegree[w] = 0
		graph[w] = []string{}
	}

	// Build graph
	for _, w := range workflows {
		deps := dependencies[w]
		inDegree[w] = len(deps)
		for _, dep := range deps {
			graph[dep] = append(graph[dep], w)
		}
	}

	// Topological sort using Kahn's algorithm
	var plan [][]string
	remaining := make(map[string]bool)
	for _, w := range workflows {
		remaining[w] = true
	}

	for len(remaining) > 0 {
		// Find all workflows with no dependencies
		var phase []string
		for w := range remaining {
			if inDegree[w] == 0 {
				phase = append(phase, w)
			}
		}

		if len(phase) == 0 {
			// Circular dependency
			return nil, fmt.Errorf("circular dependency detected in workflows")
		}

		plan = append(plan, phase)

		// Remove this phase and update in-degrees
		for _, w := range phase {
			delete(remaining, w)
			for _, dependent := range graph[w] {
				inDegree[dependent]--
			}
		}
	}

	return plan, nil
}

// checkDependenciesPassed verifies all dependencies for workflows in a phase passed.
func checkDependenciesPassed(
	phase []string,
	dependencies map[string][]string,
	results map[string]*WorkflowResult,
) error {
	for _, workflow := range phase {
		deps := dependencies[workflow]
		for _, dep := range deps {
			result, exists := results[dep]
			if !exists {
				return fmt.Errorf("dependency %s not executed", dep)
			}
			if result.Error != nil {
				return fmt.Errorf("dependency %s failed with error: %w", dep, result.Error)
			}
			if result.Result != nil && !result.Result.Success {
				return fmt.Errorf("dependency %s failed (exit %d)", dep, result.Result.ExitCode)
			}
			if result.Skipped {
				return fmt.Errorf("dependency %s was skipped", dep)
			}
		}
	}
	return nil
}

// buildMultiWorkflowResult aggregates individual workflow results.
func buildMultiWorkflowResult(
	results map[string]*WorkflowResult,
	startTime, finishTime time.Time,
) *MultiWorkflowResult {
	overallSuccess := true

	for _, result := range results {
		// Infrastructure error = failure
		if result.Error != nil {
			overallSuccess = false
			break
		}
		// Pipeline failure = failure
		if result.Result != nil && !result.Result.Success {
			overallSuccess = false
			break
		}
		// Skipped workflow = check if it was required
		// (caller should determine if skipped workflows matter)
	}

	return &MultiWorkflowResult{
		Results:        results,
		OverallSuccess: overallSuccess,
		TotalDuration:  finishTime.Sub(startTime),
		StartedAt:      startTime,
		FinishedAt:     finishTime,
	}
}
