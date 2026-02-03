package web

import (
	"errors"
	"strings"
	"testing"
)

func TestExtractionError_Error(t *testing.T) {
	err := &ExtractionError{
		URL:       "https://example.com",
		Operation: "extract content",
		Err:       errors.New("test error"),
		Troubleshooting: []string{
			"Tip 1",
			"Tip 2",
		},
	}

	errMsg := err.Error()

	expectedParts := []string{
		"Failed to extract content",
		"URL: https://example.com",
		"Reason: test error",
		"Troubleshooting:",
		"- Tip 1",
		"- Tip 2",
	}

	for _, part := range expectedParts {
		if !strings.Contains(errMsg, part) {
			t.Errorf("Error() missing expected part: %q", part)
		}
	}
}

func TestExtractionError_Unwrap(t *testing.T) {
	innerErr := errors.New("inner error")
	err := &ExtractionError{
		URL:       "https://example.com",
		Operation: "test",
		Err:       innerErr,
	}

	if unwrapped := err.Unwrap(); unwrapped != innerErr {
		t.Errorf("Unwrap() = %v, want %v", unwrapped, innerErr)
	}
}

func TestNewExtractionFailedError(t *testing.T) {
	url := "https://example.com"
	reason := errors.New("test reason")

	err := NewExtractionFailedError(url, reason)

	if err.URL != url {
		t.Errorf("URL = %q, want %q", err.URL, url)
	}

	if err.Operation != "extract web content" {
		t.Errorf("Operation = %q, want %q", err.Operation, "extract web content")
	}

	if !errors.Is(err.Err, ErrExtractionFailed) {
		t.Errorf("Error should wrap ErrExtractionFailed")
	}

	if len(err.Troubleshooting) == 0 {
		t.Errorf("Troubleshooting tips should not be empty")
	}
}

func TestNewEmptyContentError(t *testing.T) {
	url := "https://example.com"
	err := NewEmptyContentError(url)

	if err.URL != url {
		t.Errorf("URL = %q, want %q", err.URL, url)
	}

	if !errors.Is(err.Err, ErrEmptyContent) {
		t.Errorf("Error should be ErrEmptyContent")
	}

	if len(err.Troubleshooting) == 0 {
		t.Errorf("Troubleshooting tips should not be empty")
	}
}

func TestNewInvalidURLError(t *testing.T) {
	url := "invalid-url"
	reason := errors.New("parse error")
	err := NewInvalidURLError(url, reason)

	if err.URL != url {
		t.Errorf("URL = %q, want %q", err.URL, url)
	}

	if !errors.Is(err.Err, ErrInvalidURL) {
		t.Errorf("Error should wrap ErrInvalidURL")
	}

	if len(err.Troubleshooting) == 0 {
		t.Errorf("Troubleshooting tips should not be empty")
	}
}

func TestNewPaywallError(t *testing.T) {
	url := "https://example.com"
	err := NewPaywallError(url)

	if err.URL != url {
		t.Errorf("URL = %q, want %q", err.URL, url)
	}

	if !errors.Is(err.Err, ErrPaywallDetected) {
		t.Errorf("Error should be ErrPaywallDetected")
	}

	if len(err.Troubleshooting) == 0 {
		t.Errorf("Troubleshooting tips should not be empty")
	}

	// Check for --input-file suggestion
	errMsg := err.Error()
	if !strings.Contains(errMsg, "--input-file") {
		t.Errorf("Error message should suggest --input-file")
	}
}

func TestNewJavaScriptHeavyError(t *testing.T) {
	url := "https://example.com"
	err := NewJavaScriptHeavyError(url)

	if err.URL != url {
		t.Errorf("URL = %q, want %q", err.URL, url)
	}

	if !errors.Is(err.Err, ErrJavaScriptHeavy) {
		t.Errorf("Error should be ErrJavaScriptHeavy")
	}

	if len(err.Troubleshooting) == 0 {
		t.Errorf("Troubleshooting tips should not be empty")
	}

	// Check for --input-file suggestion
	errMsg := err.Error()
	if !strings.Contains(errMsg, "--input-file") {
		t.Errorf("Error message should suggest --input-file")
	}
}
