package research

import (
	"errors"
	"fmt"
	"strings"
)

var (
	// ErrAPIKeyNotSet indicates that the GEMINI_API_KEY environment variable is not set
	ErrAPIKeyNotSet = errors.New("GEMINI_API_KEY not set")

	// ErrInteractionNotFound indicates that the interaction ID was not found
	ErrInteractionNotFound = errors.New("interaction not found")

	// ErrEmptyOutput indicates that the research completed but output is empty
	ErrEmptyOutput = errors.New("research completed but output is empty")

	// ErrMalformedResponse indicates that the API response is malformed
	ErrMalformedResponse = errors.New("malformed API response")

	// ErrTimeout indicates that the research exceeded the timeout
	ErrTimeout = errors.New("research timeout exceeded")

	// ErrAuthFailed indicates that authentication failed
	ErrAuthFailed = errors.New("authentication failed")

	// ErrRateLimitExceeded indicates that the API rate limit was exceeded
	ErrRateLimitExceeded = errors.New("API rate limit exceeded")
)

// ResearchError represents a Deep Research API error with context
type ResearchError struct {
	Operation       string
	InteractionID   string
	Err             error
	Troubleshooting []string
}

// Error returns the formatted error message
func (e *ResearchError) Error() string {
	var b strings.Builder
	fmt.Fprintf(&b, "Failed to %s\n", e.Operation)
	if e.InteractionID != "" {
		fmt.Fprintf(&b, "  Interaction ID: %s\n", e.InteractionID)
	}
	fmt.Fprintf(&b, "  Reason: %v\n", e.Err)
	if len(e.Troubleshooting) > 0 {
		b.WriteString("  Troubleshooting:\n")
		for _, tip := range e.Troubleshooting {
			fmt.Fprintf(&b, "    - %s\n", tip)
		}
	}
	return b.String()
}

// Unwrap returns the underlying error
func (e *ResearchError) Unwrap() error {
	return e.Err
}

// NewAPIKeyNotSetError creates an error for missing API key
func NewAPIKeyNotSetError() *ResearchError {
	return &ResearchError{
		Operation: "authenticate with Deep Research API",
		Err:       ErrAPIKeyNotSet,
		Troubleshooting: []string{
			"Set GEMINI_API_KEY environment variable",
			"Get API key from: https://aistudio.google.com/apikey",
			"Or use --project flag with GCP project ID",
		},
	}
}

// NewInteractionNotFoundError creates an error for missing interaction
func NewInteractionNotFoundError(interactionID string) *ResearchError {
	return &ResearchError{
		Operation:     "poll interaction status",
		InteractionID: interactionID,
		Err:           ErrInteractionNotFound,
		Troubleshooting: []string{
			"Verify the interaction ID is correct",
			"Check if the interaction was created successfully",
			"The interaction may have expired (check API logs)",
		},
	}
}

// NewEmptyOutputError creates an error for empty research output
func NewEmptyOutputError(interactionID string) *ResearchError {
	return &ResearchError{
		Operation:     "extract research output",
		InteractionID: interactionID,
		Err:           ErrEmptyOutput,
		Troubleshooting: []string{
			"Check the Deep Research API status",
			"Verify the interaction completed successfully",
			"Try running the research again",
		},
	}
}

// NewMalformedResponseError creates an error for malformed API responses
func NewMalformedResponseError(interactionID string, rawResponse string) *ResearchError {
	return &ResearchError{
		Operation:     "parse API response",
		InteractionID: interactionID,
		Err:           fmt.Errorf("%w: %s", ErrMalformedResponse, rawResponse),
		Troubleshooting: []string{
			"Check the Deep Research API version",
			"Verify the API contract hasn't changed",
			"Report this issue if it persists",
		},
	}
}

// NewTimeoutError creates an error for research timeout
func NewTimeoutError(interactionID string, timeoutMinutes int) *ResearchError {
	return &ResearchError{
		Operation:     "complete research",
		InteractionID: interactionID,
		Err:           fmt.Errorf("%w after %d minutes", ErrTimeout, timeoutMinutes),
		Troubleshooting: []string{
			fmt.Sprintf("Research exceeded %d minute timeout", timeoutMinutes),
			"Use --timeout flag to increase timeout duration",
			"Manually check interaction status later using interaction ID",
			"Consider simplifying the research topics",
		},
	}
}

// NewAuthError creates an error for authentication failures
func NewAuthError(apiMessage string) *ResearchError {
	return &ResearchError{
		Operation: "authenticate with Deep Research API",
		Err:       fmt.Errorf("%w: %s", ErrAuthFailed, apiMessage),
		Troubleshooting: []string{
			"Check GEMINI_API_KEY is valid",
			"Verify API key has necessary permissions",
			"Generate new API key from: https://aistudio.google.com/apikey",
			"Ensure project ID is correct if using --project flag",
		},
	}
}

// NewRateLimitError creates an error for rate limit exceeded
func NewRateLimitError(retryAfter string) *ResearchError {
	msg := "API rate limit exceeded"
	if retryAfter != "" {
		msg = fmt.Sprintf("%s (retry after: %s)", msg, retryAfter)
	}

	return &ResearchError{
		Operation: "call Deep Research API",
		Err:       fmt.Errorf("%w: %s", ErrRateLimitExceeded, msg),
		Troubleshooting: []string{
			"Wait before retrying the request",
			"Check API quota and rate limits",
			"Consider reducing request frequency",
		},
	}
}
