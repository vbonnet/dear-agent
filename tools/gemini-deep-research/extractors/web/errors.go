package web

import (
	"errors"
	"fmt"
)

var (
	// ErrExtractionFailed indicates that content extraction failed
	ErrExtractionFailed = errors.New("content extraction failed")

	// ErrEmptyContent indicates that no content was extracted
	ErrEmptyContent = errors.New("extracted content is empty")

	// ErrInvalidURL indicates that the URL format is invalid
	ErrInvalidURL = errors.New("invalid URL format")

	// ErrPaywallDetected indicates that the content is behind a paywall
	ErrPaywallDetected = errors.New("content behind paywall or login required")

	// ErrJavaScriptHeavy indicates that the site requires JavaScript rendering
	ErrJavaScriptHeavy = errors.New("unable to extract content from JavaScript-heavy site")
)

// ExtractionError represents a web content extraction error with context
type ExtractionError struct {
	URL             string
	Operation       string
	Err             error
	Troubleshooting []string
}

// Error returns the formatted error message
func (e *ExtractionError) Error() string {
	msg := fmt.Sprintf("Failed to %s\n", e.Operation)
	if e.URL != "" {
		msg += fmt.Sprintf("  URL: %s\n", e.URL)
	}
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
func (e *ExtractionError) Unwrap() error {
	return e.Err
}

// NewExtractionFailedError creates an error for extraction failures
func NewExtractionFailedError(url string, reason error) *ExtractionError {
	return &ExtractionError{
		URL:       url,
		Operation: "extract web content",
		Err:       fmt.Errorf("%w: %v", ErrExtractionFailed, reason),
		Troubleshooting: []string{
			"Verify the URL is accessible",
			"Check if the content requires JavaScript rendering",
			"Try using --input-file to provide content manually",
		},
	}
}

// NewEmptyContentError creates an error for empty content
func NewEmptyContentError(url string) *ExtractionError {
	return &ExtractionError{
		URL:       url,
		Operation: "validate extracted content",
		Err:       ErrEmptyContent,
		Troubleshooting: []string{
			"Verify the URL contains readable content",
			"Check if the page requires authentication",
			"Ensure the page is not empty or placeholder",
		},
	}
}

// NewInvalidURLError creates an error for invalid URLs
func NewInvalidURLError(url string, reason error) *ExtractionError {
	return &ExtractionError{
		URL:       url,
		Operation: "parse URL",
		Err:       fmt.Errorf("%w: %v", ErrInvalidURL, reason),
		Troubleshooting: []string{
			"Verify the URL format is correct (http:// or https://)",
			"Check for typos in the URL",
			"Examples: https://huggingface.co/papers/2501.12345",
		},
	}
}

// NewPaywallError creates an error for paywall-protected content
func NewPaywallError(url string) *ExtractionError {
	return &ExtractionError{
		URL:       url,
		Operation: "extract article content",
		Err:       ErrPaywallDetected,
		Troubleshooting: []string{
			"Content is behind a paywall or requires login",
			"Use --input-file to provide content manually",
			"Copy the article text to a file and use --input-file <path>",
		},
	}
}

// NewJavaScriptHeavyError creates an error for JavaScript-heavy sites
func NewJavaScriptHeavyError(url string) *ExtractionError {
	return &ExtractionError{
		URL:       url,
		Operation: "extract article content",
		Err:       ErrJavaScriptHeavy,
		Troubleshooting: []string{
			"Site requires JavaScript rendering which is not supported",
			"Use --input-file to provide content manually",
			"Copy the article text to a file and use --input-file <path>",
			"Consider using a browser to save the page as HTML",
		},
	}
}
