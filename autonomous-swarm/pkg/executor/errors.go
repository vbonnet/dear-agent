package executor

import "fmt"

// ErrorType classifies errors for handling strategy
type ErrorType int

const (
	// ErrorRecoverable indicates retriable errors (CSM timeout, parse error)
	ErrorRecoverable ErrorType = iota
	// ErrorEscalation indicates escalation required (max iterations, explicit signal)
	ErrorEscalation
	// ErrorFatal indicates immediate stop (file not found, invalid config)
	ErrorFatal
)

// ExecutionError wraps errors with classification and context
type ExecutionError struct {
	Type      ErrorType
	BeadID    string
	Iteration int
	Cause     error
	Message   string
}

// Error implements error interface
func (e *ExecutionError) Error() string {
	return fmt.Sprintf("[%s] bead=%s iteration=%d: %s: %v", e.typeString(), e.BeadID, e.Iteration, e.Message, e.Cause)
}

// Unwrap returns the underlying error
func (e *ExecutionError) Unwrap() error {
	return e.Cause
}

func (e *ExecutionError) typeString() string {
	switch e.Type {
	case ErrorRecoverable:
		return "RECOVERABLE"
	case ErrorEscalation:
		return "ESCALATION"
	case ErrorFatal:
		return "FATAL"
	default:
		return "UNKNOWN"
	}
}

// NewRecoverableError creates a recoverable error
func NewRecoverableError(beadID string, iteration int, message string, cause error) *ExecutionError {
	return &ExecutionError{
		Type:      ErrorRecoverable,
		BeadID:    beadID,
		Iteration: iteration,
		Message:   message,
		Cause:     cause,
	}
}

// NewEscalationError creates an escalation error
func NewEscalationError(beadID string, iteration int, message string, cause error) *ExecutionError {
	return &ExecutionError{
		Type:      ErrorEscalation,
		BeadID:    beadID,
		Iteration: iteration,
		Message:   message,
		Cause:     cause,
	}
}

// NewFatalError creates a fatal error
func NewFatalError(beadID string, iteration int, message string, cause error) *ExecutionError {
	return &ExecutionError{
		Type:      ErrorFatal,
		BeadID:    beadID,
		Iteration: iteration,
		Message:   message,
		Cause:     cause,
	}
}
