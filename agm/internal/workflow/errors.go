// Package workflow provides workflow functionality.
package workflow

import "fmt"

// ErrWorkflowNotFound indicates the requested workflow doesn't exist.
type ErrWorkflowNotFound struct {
	Name string
}

func (e ErrWorkflowNotFound) Error() string {
	return fmt.Sprintf("workflow '%s' not found", e.Name)
}

// ErrWorkflowNotSupported indicates the workflow doesn't support the specified harness.
type ErrWorkflowNotSupported struct {
	WorkflowName string
	HarnessName  string
}

func (e ErrWorkflowNotSupported) Error() string {
	return fmt.Sprintf("workflow '%s' not supported by harness '%s'",
		e.WorkflowName, e.HarnessName)
}

// ErrWorkflowExecutionFailed indicates workflow execution failed.
type ErrWorkflowExecutionFailed struct {
	WorkflowName string
	Reason       string
}

func (e ErrWorkflowExecutionFailed) Error() string {
	return fmt.Sprintf("workflow '%s' execution failed: %s",
		e.WorkflowName, e.Reason)
}

// ErrInvalidWorkflowContext indicates the workflow context is invalid.
type ErrInvalidWorkflowContext struct {
	Field  string
	Reason string
}

func (e ErrInvalidWorkflowContext) Error() string {
	return fmt.Sprintf("invalid workflow context: %s - %s", e.Field, e.Reason)
}
