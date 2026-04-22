package validate

import (
	"errors"
	"testing"
)

func TestValidationError(t *testing.T) {
	cause := errors.New("underlying error")
	err := &ValidationError{
		Session: "test-session",
		Phase:   "test",
		Cause:   cause,
	}

	expected := "validation failed for session test-session during test: underlying error"
	if err.Error() != expected {
		t.Errorf("Error() = %s, want %s", err.Error(), expected)
	}

	// Test Unwrap
	if !errors.Is(err, cause) {
		t.Error("Unwrap() did not return original cause")
	}
}

func TestFixError(t *testing.T) {
	cause := errors.New("file not found")
	err := &FixError{
		Session:     "test-session",
		IssueType:   IssueVersionMismatch,
		Description: "failed to read version file",
		Cause:       cause,
	}

	expected := "fix failed for test-session (version_mismatch): failed to read version file: file not found"
	if err.Error() != expected {
		t.Errorf("Error() = %s, want %s", err.Error(), expected)
	}

	// Test Unwrap
	if !errors.Is(err, cause) {
		t.Error("Unwrap() did not return original cause")
	}
}

func TestValidationError_ErrorsIs(t *testing.T) {
	cause := errors.New("specific error")
	err := &ValidationError{
		Session: "test",
		Phase:   "test",
		Cause:   cause,
	}

	// Should be able to use errors.Is
	if !errors.Is(err, cause) {
		t.Error("errors.Is should find wrapped error")
	}
}

func TestFixError_ErrorsIs(t *testing.T) {
	cause := errors.New("specific error")
	err := &FixError{
		Session:     "test",
		IssueType:   IssueUnknown,
		Description: "test",
		Cause:       cause,
	}

	// Should be able to use errors.Is
	if !errors.Is(err, cause) {
		t.Error("errors.Is should find wrapped error")
	}
}
