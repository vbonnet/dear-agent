package ci

import "context"

// PipelineExecutor defines the interface for all CI pipeline implementations.
// Implementations must be safe for concurrent use.
//
// The executor is responsible for:
//   - Validating pipeline configuration before execution
//   - Running pipeline steps in the appropriate environment
//   - Collecting and streaming execution results
//   - Handling timeouts and cancellations gracefully
//
// Example implementations: ActExecutor (nektos/act), BashExecutor (shell scripts)
type PipelineExecutor interface {
	// Execute runs a CI pipeline and returns the result.
	// The executor should respect context cancellation and timeout.
	//
	// For streaming output, use PipelineRequest.OutputCallback if provided.
	// Otherwise, buffer output in PipelineResult.Output.
	//
	// Returns:
	//   - PipelineResult with success/failure status and step details
	//   - Error for infrastructure failures (not pipeline failures)
	//
	// Note: A failed pipeline step is NOT an error - it's reflected in
	// PipelineResult.Success = false. Only return error for executor issues
	// (missing binary, permission denied, etc.)
	Execute(ctx context.Context, req PipelineRequest) (*PipelineResult, error)

	// Validate checks if a pipeline configuration is valid without executing it.
	// This includes:
	//   - Workflow file syntax validation
	//   - Required tools/dependencies check
	//   - Secret/variable reference validation
	//
	// Returns nil if valid, or a descriptive error if invalid.
	Validate(ctx context.Context, req PipelineRequest) error

	// Name returns a human-readable identifier for this executor.
	// Examples: "act-native", "act-docker", "bash-script"
	//
	// Used for logging, metrics, and debugging.
	Name() string
}
