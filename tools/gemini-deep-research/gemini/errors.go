package gemini

import (
	"errors"
	"fmt"
)

var (
	// ErrGeminiNotFound indicates that the gemini CLI tool is not installed
	ErrGeminiNotFound = errors.New("gemini CLI not found")

	// ErrInvalidJSON indicates that Gemini returned invalid JSON
	ErrInvalidJSON = errors.New("gemini returned invalid JSON")

	// ErrNoTopics indicates that Gemini returned an empty topics array
	ErrNoTopics = errors.New("no topics identified")

	// ErrAPIError indicates a Gemini API error occurred
	ErrAPIError = errors.New("gemini API error")

	// ErrInvalidTopicCount indicates an invalid number of topics was returned
	ErrInvalidTopicCount = errors.New("invalid topic count")
)

// AnalysisError represents a topic analysis error with context
type AnalysisError struct {
	Operation       string
	Err             error
	Troubleshooting []string
}

// Error returns the formatted error message
func (e *AnalysisError) Error() string {
	msg := fmt.Sprintf("Failed to %s\n", e.Operation)
	msg += fmt.Sprintf("  Reason: %v\n", e.Err)
	if len(e.Troubleshooting) > 0 {
		msg += "  Troubleshooting:\n"
		for _, tip := range e.Troubleshooting {
			msg += fmt.Sprintf("    - %s\n", tip)
		}
	}
	return msg
}

// Unwrap returns the underlying error
func (e *AnalysisError) Unwrap() error {
	return e.Err
}

// NewAnalysisError creates a new AnalysisError with troubleshooting tips
func NewAnalysisError(operation string, err error, tips []string) *AnalysisError {
	return &AnalysisError{
		Operation:       operation,
		Err:             err,
		Troubleshooting: tips,
	}
}

// NewGeminiNotFoundError creates an error indicating Gemini CLI is not installed
func NewGeminiNotFoundError() *AnalysisError {
	return &AnalysisError{
		Operation: "find gemini CLI",
		Err:       ErrGeminiNotFound,
		Troubleshooting: []string{
			"Install Gemini CLI from: https://ai.google.dev/gemini-api/docs/cli",
			"Ensure gemini is in your PATH",
			"Verify installation with: gemini --version",
		},
	}
}

// NewInvalidJSONError creates an error for invalid JSON responses
func NewInvalidJSONError(rawResponse string) *AnalysisError {
	return &AnalysisError{
		Operation: "parse Gemini JSON response",
		Err:       fmt.Errorf("%w: %s", ErrInvalidJSON, rawResponse),
		Troubleshooting: []string{
			"Check Gemini API status",
			"Verify API key is valid",
			"Try running the command again",
		},
	}
}

// NewNoTopicsError creates an error for empty topics array
func NewNoTopicsError() *AnalysisError {
	return &AnalysisError{
		Operation: "identify topics",
		Err:       ErrNoTopics,
		Troubleshooting: []string{
			"Ensure content has sufficient text (minimum 100 characters)",
			"Try a different custom prompt if provided",
			"Check if content is meaningful and technical",
		},
	}
}

// NewAPIError creates an error for Gemini API failures
func NewAPIError(apiMessage string) *AnalysisError {
	return &AnalysisError{
		Operation: "call Gemini API",
		Err:       fmt.Errorf("%w: %s", ErrAPIError, apiMessage),
		Troubleshooting: []string{
			"Check GEMINI_API_KEY environment variable is set",
			"Verify API key has necessary permissions",
			"Check API quota and rate limits",
			"View API status at: https://status.cloud.google.com",
		},
	}
}
