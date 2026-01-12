package validate

import "fmt"

// ValidationError represents an error during session validation.
type ValidationError struct {
	Session string
	Phase   string // "test", "classify", "fix"
	Cause   error
}

func (e *ValidationError) Error() string {
	return fmt.Sprintf("validation failed for session %s during %s: %v", e.Session, e.Phase, e.Cause)
}

func (e *ValidationError) Unwrap() error {
	return e.Cause
}

// FixError represents an error during fix application.
type FixError struct {
	Session     string
	IssueType   IssueType
	Description string
	Cause       error
}

func (e *FixError) Error() string {
	return fmt.Sprintf("fix failed for %s (%s): %s: %v", e.Session, e.IssueType, e.Description, e.Cause)
}

func (e *FixError) Unwrap() error {
	return e.Cause
}
