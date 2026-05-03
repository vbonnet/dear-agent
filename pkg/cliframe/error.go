package cliframe

import (
	"encoding/json"
	"fmt"
	"strings"
)

// CLIError represents a structured CLI error with recovery hints
type CLIError struct {
	// Symbol is a machine-readable error identifier (e.g., "file_not_found")
	Symbol string `json:"symbol"`

	// Message is a human-readable description
	Message string `json:"message"`

	// Suggestions provides recovery steps
	Suggestions []string `json:"suggestions,omitempty"`

	// RelatedCommands lists commands that might help
	RelatedCommands []string `json:"related_commands,omitempty"`

	// ExitCode is the process exit code (64-77 range)
	ExitCode int `json:"exit_code"`

	// Cause is the underlying error (may be nil)
	Cause error `json:"-"`

	// Retryable indicates if the operation can be retried
	Retryable bool `json:"retryable"`

	// RetryAfter suggests delay before retry (seconds, 0 = immediate)
	RetryAfter int `json:"retry_after,omitempty"`

	// Fields contains arbitrary metadata for structured logging
	Fields map[string]interface{} `json:"fields,omitempty"`
}

// Error implements error interface
func (e *CLIError) Error() string {
	var sb strings.Builder

	// Symbol and message
	if e.Symbol != "" {
		fmt.Fprintf(&sb, "[%s] ", e.Symbol)
	}
	sb.WriteString(e.Message)

	// Suggestions
	if len(e.Suggestions) > 0 {
		sb.WriteString("\n\nSuggestions:")
		for i, suggestion := range e.Suggestions {
			fmt.Fprintf(&sb, "\n  %d. %s", i+1, suggestion)
		}
	}

	// Related commands
	if len(e.RelatedCommands) > 0 {
		sb.WriteString("\n\nRelated commands:")
		for _, cmd := range e.RelatedCommands {
			fmt.Fprintf(&sb, "\n  - %s", cmd)
		}
	}

	return sb.String()
}

// Unwrap implements errors.Unwrap
func (e *CLIError) Unwrap() error {
	return e.Cause
}

// JSON returns JSON representation (for --error-format json)
func (e *CLIError) JSON() ([]byte, error) {
	// Create sanitized copy for JSON output
	sanitized := &CLIError{
		Symbol:          e.Symbol,
		Message:         sanitizeMessage(e.Message),
		Suggestions:     e.Suggestions,
		RelatedCommands: e.RelatedCommands,
		ExitCode:        e.ExitCode,
		Retryable:       e.Retryable,
		RetryAfter:      e.RetryAfter,
		Fields:          e.Fields,
	}

	return json.MarshalIndent(sanitized, "", "  ")
}

// NewError creates a CLIError with required fields
func NewError(symbol, message string) *CLIError {
	return &CLIError{
		Symbol:          symbol,
		Message:         message,
		ExitCode:        ExitGeneralError,
		Suggestions:     []string{},
		RelatedCommands: []string{},
		Fields:          make(map[string]interface{}),
		Retryable:       false,
	}
}

// WithExitCode sets the exit code
func (e *CLIError) WithExitCode(code int) *CLIError {
	e.ExitCode = code
	return e
}

// WithCause sets the underlying cause
func (e *CLIError) WithCause(err error) *CLIError {
	e.Cause = err
	return e
}

// AddSuggestion adds a recovery suggestion
func (e *CLIError) AddSuggestion(s string) *CLIError {
	// Deduplicate suggestions
	for _, existing := range e.Suggestions {
		if existing == s {
			return e
		}
	}
	e.Suggestions = append(e.Suggestions, s)
	return e
}

// AddRelatedCommand adds a related command
func (e *CLIError) AddRelatedCommand(cmd string) *CLIError {
	// Deduplicate commands
	for _, existing := range e.RelatedCommands {
		if existing == cmd {
			return e
		}
	}
	e.RelatedCommands = append(e.RelatedCommands, cmd)
	return e
}

// MarkRetryable marks the error as retryable with optional delay
func (e *CLIError) MarkRetryable(retryAfter int) *CLIError {
	e.Retryable = true
	e.RetryAfter = retryAfter
	return e
}

// WithField adds arbitrary metadata
func (e *CLIError) WithField(key string, value interface{}) *CLIError {
	if e.Fields == nil {
		e.Fields = make(map[string]interface{})
	}
	e.Fields[key] = value
	return e
}

// Common error constructors

// ErrFileNotFound creates a file not found error
func ErrFileNotFound(path string) *CLIError {
	return NewError("file_not_found", fmt.Sprintf("File not found: %s", path)).
		WithExitCode(ExitNoInput).
		AddSuggestion(fmt.Sprintf("Check that the file path is correct: %s", path)).
		AddSuggestion("Use an absolute path or verify the current directory")
}

// ErrInvalidArgument creates an invalid argument error
func ErrInvalidArgument(arg, reason string) *CLIError {
	return NewError("invalid_argument", fmt.Sprintf("Invalid argument '%s': %s", arg, reason)).
		WithExitCode(ExitUsageError).
		AddSuggestion("Run with --help to see valid arguments").
		AddRelatedCommand("--help")
}

// ErrServiceUnavailable creates a service unavailable error (retryable)
func ErrServiceUnavailable(service string, retryAfter int) *CLIError {
	return NewError("service_unavailable", fmt.Sprintf("Service unavailable: %s", service)).
		WithExitCode(ExitServiceUnavailable).
		MarkRetryable(retryAfter).
		AddSuggestion(fmt.Sprintf("Wait %d seconds and try again", retryAfter)).
		AddSuggestion("Check service status or network connectivity")
}

// ErrPermissionDenied creates a permission denied error
func ErrPermissionDenied(resource string) *CLIError {
	return NewError("permission_denied", fmt.Sprintf("Permission denied: %s", resource)).
		WithExitCode(ExitPermissionDenied).
		AddSuggestion("Check file/directory permissions").
		AddSuggestion("Ensure you have the required access rights")
}

// ErrConfigMissing creates a config missing error
func ErrConfigMissing(path string) *CLIError {
	return NewError("config_missing", fmt.Sprintf("Configuration file not found: %s", path)).
		WithExitCode(ExitNoInput).
		AddSuggestion("Run init command to create default configuration").
		AddSuggestion(fmt.Sprintf("Create config file at: %s", path))
}

// sanitizeMessage removes sensitive information from error messages
func sanitizeMessage(msg string) string {
	// Patterns to redact
	patterns := []struct {
		keyword string
		replace string
	}{
		{"api_key", "[REDACTED]"},
		{"apikey", "[REDACTED]"},
		{"token", "[REDACTED]"},
		{"password", "[REDACTED]"},
		{"secret", "[REDACTED]"},
		{"bearer", "[REDACTED]"},
	}

	sanitized := msg
	lowerMsg := strings.ToLower(msg)

	for _, pattern := range patterns {
		if strings.Contains(lowerMsg, pattern.keyword) {
			// Find and redact the value after the keyword
			// This is a simple implementation - production would use regex
			idx := strings.Index(lowerMsg, pattern.keyword)
			if idx != -1 {
				// Redact next 20 characters after keyword (simple heuristic)
				start := idx
				end := start + len(pattern.keyword) + 20
				if end > len(sanitized) {
					end = len(sanitized)
				}
				sanitized = sanitized[:start] + pattern.keyword + "=" + pattern.replace + sanitized[end:]
			}
		}
	}

	// Sanitize absolute paths with usernames
	// Convert /home/username/... to ~/...
	if strings.Contains(sanitized, "/home/") {
		parts := strings.Split(sanitized, "/home/")
		if len(parts) > 1 {
			// Find username (next path component)
			pathParts := strings.SplitN(parts[1], "/", 2)
			if len(pathParts) > 1 {
				sanitized = strings.Replace(sanitized, "/home/"+pathParts[0], "~", 1)
			}
		}
	}

	return sanitized
}
