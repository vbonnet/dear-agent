package gemini

import (
	"errors"
	"fmt"

	"github.com/vbonnet/ai-tools/claude-session-manager/internal/agent"
)

// Common errors
var (
	ErrProjectIDMissing     = errors.New("GCP_PROJECT_ID or GOOGLE_CLOUD_PROJECT environment variable not set")
	ErrSessionNotFound      = errors.New("session not found")
	ErrMaxRetriesExceeded   = errors.New("max retries exceeded for Vertex AI API call")
	ErrFormatNotSupported   = errors.New("export format not supported (only JSONL supported in V1)")
	ErrInvalidMessage       = errors.New("invalid message: content cannot be empty")
	ErrUnsupportedCommand   = errors.New("command not supported by gemini adapter")
)

// ParameterError represents an error with command parameters
type ParameterError struct {
	CommandType   agent.CommandType
	ParameterName string
	Issue         string
}

func (e *ParameterError) Error() string {
	return fmt.Sprintf("invalid parameter for command %s.%s: %s", e.CommandType, e.ParameterName, e.Issue)
}

// APIError represents an error from Vertex AI API with actionable suggestion
type APIError struct {
	Operation  string
	Err        error
	Suggestion string
}

func (e *APIError) Error() string {
	return fmt.Sprintf("%s failed: %v\n\nSuggestion: %s", e.Operation, e.Err, e.Suggestion)
}

func (e *APIError) Unwrap() error {
	return e.Err
}

func wrapAPIError(operation string, err error) error {
	return &APIError{
		Operation:  operation,
		Err:        err,
		Suggestion: "Check Vertex AI quota, network connection, and credentials",
	}
}

func wrapAuthError(err error) error {
	return &APIError{
		Operation:  "authentication",
		Err:        err,
		Suggestion: "Run 'gcloud auth application-default login' or set GOOGLE_APPLICATION_CREDENTIALS",
	}
}
