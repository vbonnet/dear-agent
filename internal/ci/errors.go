// Package ci provides types and interfaces for CI pipeline execution.
package ci

import "fmt"

// ErrorCode identifies specific CI error conditions.
type ErrorCode int

const (
	// ErrCodeExecutorNotFound means the CI tool binary is missing
	// Example: "act" binary not found in PATH
	ErrCodeExecutorNotFound ErrorCode = iota

	// ErrCodeWorkflowNotFound means the workflow file doesn't exist
	ErrCodeWorkflowNotFound

	// ErrCodeWorkflowInvalid means the workflow has syntax errors
	ErrCodeWorkflowInvalid

	// ErrCodePermissionDenied means insufficient permissions to execute
	ErrCodePermissionDenied

	// ErrCodeTimeout means the pipeline exceeded its timeout
	ErrCodeTimeout

	// ErrCodeInfrastructure means an infrastructure failure occurred
	// (not a pipeline failure - e.g., Docker daemon down, disk full)
	ErrCodeInfrastructure

	// ErrCodeInvalidRequest means the PipelineRequest is malformed
	ErrCodeInvalidRequest

	// ErrCodeSecretMissing means a required secret is not provided
	ErrCodeSecretMissing

	// ErrCodeEnvironmentMissing means a required environment dependency is missing
	// Example: Docker not installed for act executor
	ErrCodeEnvironmentMissing
)

// Error represents a structured CI error.
// Distinguishes between pipeline failures (steps that fail) and
// infrastructure failures (executor problems).
type Error struct {
	// Code identifies the specific error condition
	Code ErrorCode

	// Message is a human-readable error description
	Message string

	// Cause is the underlying error that triggered this error
	Cause error

	// Context provides additional debugging information
	// Examples:
	//   - "workflow_path": ".github/workflows/test.yml"
	//   - "executor": "act-native"
	//   - "missing_binary": "act"
	Context map[string]string
}

// Error implements the error interface.
func (e *Error) Error() string {
	if e.Cause != nil {
		return fmt.Sprintf("%s: %v", e.Message, e.Cause)
	}
	return e.Message
}

// Unwrap returns the underlying error for errors.Is and errors.As.
func (e *Error) Unwrap() error {
	return e.Cause
}

// IsRetriable returns true if the operation might succeed if retried.
// Used by orchestrators to decide whether to retry failed pipelines.
func (e *Error) IsRetriable() bool {
	switch e.Code {
	case ErrCodeTimeout:
		// Timeout might succeed with longer timeout or less load
		return true
	case ErrCodeInfrastructure:
		// Infrastructure issues might be transient
		return true
	case ErrCodeExecutorNotFound, ErrCodeWorkflowNotFound,
		ErrCodeWorkflowInvalid, ErrCodePermissionDenied,
		ErrCodeInvalidRequest, ErrCodeSecretMissing,
		ErrCodeEnvironmentMissing:
		// These require manual intervention
		return false
	default:
		return false
	}
}

// IsPipelineFailure returns false for infrastructure errors.
// Use this to distinguish between "pipeline failed" (steps returned non-zero)
// and "executor failed" (infrastructure problem).
//
// Pipeline failures should NOT return an error from Execute() - they should
// return PipelineResult with Success=false.
func (e *Error) IsPipelineFailure() bool {
	// All error codes represent infrastructure failures, not pipeline failures
	return false
}

// Convenience constructors

// NewError creates a new CI error with the given code and message.
func NewError(code ErrorCode, message string) *Error {
	return &Error{
		Code:    code,
		Message: message,
		Context: make(map[string]string),
	}
}

// WrapError wraps an underlying error with a CI error code and message.
func WrapError(code ErrorCode, message string, cause error) *Error {
	return &Error{
		Code:    code,
		Message: message,
		Cause:   cause,
		Context: make(map[string]string),
	}
}

// WithContext adds context key-value pairs to an error.
// Returns the modified error for method chaining.
func WithContext(err *Error, key, value string) *Error {
	if err.Context == nil {
		err.Context = make(map[string]string)
	}
	err.Context[key] = value
	return err
}

// Common error constructors for frequently used error types

// ErrExecutorNotFound creates an error for missing CI tool binary.
func ErrExecutorNotFound(binaryName string) *Error {
	return WithContext(
		NewError(ErrCodeExecutorNotFound,
			fmt.Sprintf("CI executor binary not found: %s", binaryName)),
		"missing_binary", binaryName,
	)
}

// ErrWorkflowNotFound creates an error for missing workflow file.
func ErrWorkflowNotFound(workflowPath string) *Error {
	return WithContext(
		NewError(ErrCodeWorkflowNotFound,
			fmt.Sprintf("workflow file not found: %s", workflowPath)),
		"workflow_path", workflowPath,
	)
}

// ErrWorkflowInvalid creates an error for invalid workflow syntax.
func ErrWorkflowInvalid(workflowPath string, reason error) *Error {
	return WithContext(
		WrapError(ErrCodeWorkflowInvalid,
			fmt.Sprintf("workflow file is invalid: %s", workflowPath),
			reason),
		"workflow_path", workflowPath,
	)
}

// ErrTimeout creates an error for pipeline timeout.
func ErrTimeout(duration string) *Error {
	return WithContext(
		NewError(ErrCodeTimeout,
			fmt.Sprintf("pipeline execution timed out after %s", duration)),
		"timeout", duration,
	)
}

// ErrSecretMissing creates an error for missing required secret.
func ErrSecretMissing(secretName string) *Error {
	return WithContext(
		NewError(ErrCodeSecretMissing,
			fmt.Sprintf("required secret not provided: %s", secretName)),
		"secret_name", secretName,
	)
}

// ErrEnvironmentMissing creates an error for missing environment dependency.
func ErrEnvironmentMissing(dependency string, reason string) *Error {
	err := WithContext(
		NewError(ErrCodeEnvironmentMissing,
			fmt.Sprintf("required environment dependency missing: %s", dependency)),
		"dependency", dependency,
	)
	if reason != "" {
		err.Context["reason"] = reason
	}
	return err
}
