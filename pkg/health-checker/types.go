package healthchecker

import (
	"context"
	"errors"
	"fmt"
)

// Status represents the severity level of a check result
type Status string

// ErrInvalidStatus identifies a health result with an undeclared status.
var ErrInvalidStatus = errors.New("invalid health status")

// Health-check Status severity values.
const (
	StatusOK      Status = "ok"      // Check passed successfully
	StatusInfo    Status = "info"    // Informational, not an issue
	StatusWarning Status = "warning" // Warning, needs attention
	StatusError   Status = "error"   // Error, critical issue
)

// Valid reports whether the status is a declared health-check status.
func (s Status) Valid() bool {
	switch s {
	case StatusOK, StatusInfo, StatusWarning, StatusError:
		return true
	default:
		return false
	}
}

// Check represents a single health check
type Check interface {
	// Name returns the check identifier (e.g., "workspace_exists")
	Name() string

	// Category returns the check category (e.g., "core", "dependency")
	Category() string

	// Run executes the check and returns a result
	Run(ctx context.Context) Result
}

// Result represents the outcome of a health check
type Result struct {
	Name     string // Check identifier
	Category string // Check category
	Status   Status // ok, warning, error, info
	Message  string // Human-readable message (empty if ok)
	Fixable  bool   // Can this be auto-fixed?
	Fix      *Fix   // Fix information (if fixable)
}

// Validate reports whether the result has a declared status.
func (r Result) Validate() error {
	if r.Status.Valid() {
		return nil
	}

	return fmt.Errorf("%w: %q", ErrInvalidStatus, r.Status)
}

func normalizeResult(result Result) Result {
	err := result.Validate()
	if err == nil {
		return result
	}

	result.Status = StatusError
	if result.Message == "" {
		result.Message = err.Error()
	} else {
		result.Message += "; " + err.Error()
	}
	result.Fixable = false
	result.Fix = nil
	return result
}

// Fix represents an auto-fix operation
type Fix struct {
	Name        string                          // Human-readable name (e.g., "Create missing directory")
	Description string                          // What it does (e.g., "Creates ~/.engram/logs directory")
	Command     string                          // CLI command if applicable (e.g., "mkdir -p ~/.engram/logs")
	Apply       func(ctx context.Context) error // Function that performs the fix
	Reversible  bool                            // Can the fix be undone?
}

// IsHealthy returns true if the result indicates success
func (r Result) IsHealthy() bool {
	status := normalizeResult(r).Status
	return status == StatusOK || status == StatusInfo
}

// IsIssue returns true if the result indicates a problem
func (r Result) IsIssue() bool {
	status := normalizeResult(r).Status
	return status == StatusWarning || status == StatusError
}

// IsCritical returns true if the result is an error
func (r Result) IsCritical() bool {
	return normalizeResult(r).Status == StatusError
}
