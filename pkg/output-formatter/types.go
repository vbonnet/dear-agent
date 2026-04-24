package outputformatter

// StatusLevel represents the severity/status of a result
type StatusLevel string

const (
	StatusOK      StatusLevel = "ok"      // Success, no issues
	StatusSuccess StatusLevel = "success" // Explicitly successful (alias for ok)
	StatusInfo    StatusLevel = "info"    // Informational, not an issue
	StatusWarning StatusLevel = "warning" // Warning, needs attention
	StatusError   StatusLevel = "error"   // Error, critical issue
	StatusFailed  StatusLevel = "failed"  // Failed operation (alias for error)
	StatusUnknown StatusLevel = "unknown" // Unknown status
)

// Result represents a formattable result item
type Result interface {
	// Status returns the status level of this result
	Status() StatusLevel

	// Message returns the human-readable message
	Message() string

	// Category returns the category/group this result belongs to
	Category() string
}

// Summary contains aggregated counts of results by status
type Summary struct {
	Total    int // Total number of results
	Passed   int // Results with ok/success status
	Info     int // Results with info status
	Warnings int // Results with warning status
	Errors   int // Results with error/failed status
	Unknown  int // Results with unknown status
}

// IsHealthy returns true if there are no errors or warnings
func (s Summary) IsHealthy() bool {
	return s.Errors == 0 && s.Warnings == 0
}

// HasIssues returns true if there are any errors or warnings
func (s Summary) HasIssues() bool {
	return s.Errors > 0 || s.Warnings > 0
}

// ExitCode returns appropriate Unix exit code
// 0 = healthy, 1 = warnings, 2 = errors
func (s Summary) ExitCode() int {
	if s.Errors > 0 {
		return 2
	}
	if s.Warnings > 0 {
		return 1
	}
	return 0
}

// OverallStatus returns a string representing overall health
func (s Summary) OverallStatus() string {
	if s.Errors > 0 {
		return "Critical"
	}
	if s.Warnings > 0 {
		return "Degraded"
	}
	if s.Passed > 0 || s.Info > 0 {
		return "Healthy"
	}
	return "Unknown"
}
