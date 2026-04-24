package validator

import "fmt"

// ValidationError represents a pre-validation failure with user-facing guidance
type ValidationError struct {
	Phase  string
	Reason string
	Fix    string // User-facing guidance on how to fix
}

// Error implements the error interface
func (e *ValidationError) Error() string {
	if e.Fix != "" {
		return fmt.Sprintf("cannot %s: %s. Fix: %s", e.Phase, e.Reason, e.Fix)
	}
	return fmt.Sprintf("cannot %s: %s", e.Phase, e.Reason)
}

// NewValidationError creates a new ValidationError
func NewValidationError(phase, reason, fix string) *ValidationError {
	return &ValidationError{
		Phase:  phase,
		Reason: reason,
		Fix:    fix,
	}
}
