package cli

import (
	"fmt"
	"strings"
)

// EngramError represents a structured CLI error with context and suggestions
type EngramError struct {
	Symbol          string   // Error symbol (✗, ❌)
	Message         string   // Main error message
	Cause           error    // Underlying error (optional)
	Suggestions     []string // Actionable suggestions
	RelatedCommands []string // Related commands to try
}

// Error implements error interface
func (e *EngramError) Error() string {
	var b strings.Builder

	// Main error
	fmt.Fprintf(&b, "%s %s\n", e.Symbol, e.Message)

	// Cause (if present)
	if e.Cause != nil {
		fmt.Fprintf(&b, "\nCause: %v\n", e.Cause)
	}

	// Suggestions
	if len(e.Suggestions) > 0 {
		fmt.Fprintf(&b, "\nTry:\n")
		for _, suggestion := range e.Suggestions {
			fmt.Fprintf(&b, "  - %s\n", suggestion)
		}
	}

	// Related commands
	if len(e.RelatedCommands) > 0 {
		fmt.Fprintf(&b, "\nRelated commands:\n")
		for _, cmd := range e.RelatedCommands {
			fmt.Fprintf(&b, "  %s\n", cmd)
		}
	}

	return b.String()
}

// Unwrap enables error unwrapping for errors.Is/As
func (e *EngramError) Unwrap() error {
	return e.Cause
}

// ConfigNotFoundError creates error for missing configuration
func ConfigNotFoundError(path string, cause error) error {
	return &EngramError{
		Symbol:  "✗",
		Message: fmt.Sprintf("Configuration file not found: %s", path),
		Cause:   cause,
		Suggestions: []string{
			"Run 'engram doctor --fix' to create default configuration",
			fmt.Sprintf("Check if file exists: ls -la %s", path),
			"Set ENGRAM_HOME environment variable to custom location",
		},
		RelatedCommands: []string{
			"engram doctor     - Diagnose configuration issues",
		},
	}
}

// EngramNotFoundError creates error for missing engram file
func EngramNotFoundError(query string, searchPath string) error {
	return &EngramError{
		Symbol:  "✗",
		Message: fmt.Sprintf("No engrams found matching: %s", query),
		Cause:   nil,
		Suggestions: []string{
			"Check spelling of search query",
			"Try removing filters (--tag, --type)",
			"Run 'engram index rebuild' to refresh index",
			fmt.Sprintf("Verify engram path exists: ls -la %s", searchPath),
		},
		RelatedCommands: []string{
			"engram index rebuild    - Rebuild engram index",
			"engram retrieve --help  - See all search options",
		},
	}
}

// PluginLoadError creates error for plugin loading failures
func PluginLoadError(pluginName string, cause error) error {
	return &EngramError{
		Symbol:  "✗",
		Message: fmt.Sprintf("Failed to load plugin: %s", pluginName),
		Cause:   cause,
		Suggestions: []string{
			"Check plugin manifest (package.json) is valid",
			"Verify plugin directory structure is correct",
			"Run 'engram doctor' to diagnose plugin issues",
			fmt.Sprintf("Disable plugin temporarily: engram plugin disable %s", pluginName),
		},
		RelatedCommands: []string{
			"engram plugin list      - List all plugins",
			"engram doctor           - Run health checks",
		},
	}
}

// PermissionDeniedError creates error for file permission issues
func PermissionDeniedError(path string, operation string) error {
	return &EngramError{
		Symbol:  "✗",
		Message: fmt.Sprintf("Permission denied: cannot %s %s", operation, path),
		Cause:   nil,
		Suggestions: []string{
			fmt.Sprintf("Check file permissions: ls -la %s", path),
			"Ensure you have read/write access to the directory",
			"Try running with appropriate permissions",
		},
		RelatedCommands: []string{
			"engram doctor           - Check workspace permissions",
		},
	}
}

// InvalidInputError creates error for invalid user input
func InvalidInputError(field string, value string, expected string) error {
	return &EngramError{
		Symbol:  "✗",
		Message: fmt.Sprintf("Invalid %s: %s", field, value),
		Cause:   nil,
		Suggestions: []string{
			fmt.Sprintf("Expected: %s", expected),
			"Check command usage with --help flag",
		},
		RelatedCommands: []string{},
	}
}
