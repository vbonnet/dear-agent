package healthchecker

import "context"

// Status represents the severity level of a check result
type Status string

const (
	StatusOK      Status = "ok"      // Check passed successfully
	StatusInfo    Status = "info"    // Informational, not an issue
	StatusWarning Status = "warning" // Warning, needs attention
	StatusError   Status = "error"   // Error, critical issue
)

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
	return r.Status == StatusOK || r.Status == StatusInfo
}

// IsIssue returns true if the result indicates a problem
func (r Result) IsIssue() bool {
	return r.Status == StatusWarning || r.Status == StatusError
}

// IsCritical returns true if the result is an error
func (r Result) IsCritical() bool {
	return r.Status == StatusError
}
