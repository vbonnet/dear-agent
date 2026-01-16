package gpt

import (
	"errors"
	"fmt"
)

var (
	// ErrAPIKeyNotSet is returned when OPENAI_API_KEY environment variable is not set.
	ErrAPIKeyNotSet = errors.New("OPENAI_API_KEY environment variable not set")

	// ErrSessionNotFound is returned when a session ID is not found in storage.
	ErrSessionNotFound = errors.New("session not found")

	// ErrInvalidSessionID is returned when a session ID is invalid or empty.
	ErrInvalidSessionID = errors.New("invalid session ID")

	// ErrInvalidFormat is returned when an unsupported conversation format is requested.
	ErrInvalidFormat = errors.New("unsupported conversation format")

	// ErrMaxRetriesExceeded is returned when maximum API retry attempts are exceeded.
	ErrMaxRetriesExceeded = errors.New("maximum retries exceeded")
)

// APIError wraps OpenAI API errors with additional context.
type APIError struct {
	Operation  string
	StatusCode int
	Message    string
	Err        error
}

// Error implements the error interface.
func (e *APIError) Error() string {
	return fmt.Sprintf("%s failed (HTTP %d): %s", e.Operation, e.StatusCode, e.Message)
}

// Unwrap returns the underlying error.
func (e *APIError) Unwrap() error {
	return e.Err
}
