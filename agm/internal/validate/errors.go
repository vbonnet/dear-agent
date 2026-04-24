// Package validate provides validate functionality.
package validate

import (
	"fmt"
	"strings"
)

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

// FormatErrorWithGuidance formats an error with context and actionable suggestions.
// This helper improves error message quality by providing users with clear steps to fix issues.
//
// Example usage:
//
//	return FormatErrorWithGuidance(
//	    fmt.Errorf("lock held by PID %d", pid),
//	    "Another process is using this session",
//	    []string{
//	        "Wait for the process to complete",
//	        "Run `agm unlock` to force unlock",
//	    },
//	)
func FormatErrorWithGuidance(err error, context string, suggestions []string) error {
	var sb strings.Builder

	// Error prefix with icon
	sb.WriteString(fmt.Sprintf("❌ Error: %s\n", err))

	// Context section
	if context != "" {
		sb.WriteString(fmt.Sprintf("\nContext: %s\n", context))
	}

	// Suggestions section
	if len(suggestions) > 0 {
		sb.WriteString("\nTo fix:\n")
		for _, s := range suggestions {
			sb.WriteString(fmt.Sprintf("- %s\n", s))
		}
	}

	return fmt.Errorf("%s", sb.String())
}
