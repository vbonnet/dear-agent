package validate

import (
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/vbonnet/dear-agent/agm/internal/manifest"
	"github.com/vbonnet/dear-agent/agm/internal/ui"
)

// Report contains validation results for all tested sessions.
type Report struct {
	ValidatedAt   time.Time       `json:"validated_at"`
	TotalSessions int             `json:"total_sessions"`
	Resumable     int             `json:"resumable"`
	Failed        int             `json:"failed"`
	Unknown       int             `json:"unknown"`
	Sessions      []SessionResult `json:"sessions"`
}

// HasFailures returns true if any sessions failed validation.
func (r *Report) HasFailures() bool {
	return r.Failed > 0 || r.Unknown > 0
}

// SuccessRate returns the percentage of resumable sessions.
func (r *Report) SuccessRate() float64 {
	if r.TotalSessions == 0 {
		return 0.0
	}
	return float64(r.Resumable) / float64(r.TotalSessions) * 100
}

// SessionResult contains validation results for a single session.
//
// Thread-safety: SessionResult instances must not be shared across goroutines.
// The Manifest pointer may reference shared data; validation code must ensure
// manifests are not modified concurrently.
type SessionResult struct {
	Name     string             `json:"name"`
	UUID     string             `json:"uuid"`
	Path     string             `json:"path"`   // Session directory path
	Status   string             `json:"status"` // "resumable", "failed", "unknown"
	Issues   []Issue            `json:"issues,omitempty"`
	Fixed    bool               `json:"fixed,omitempty"`
	Manifest *manifest.Manifest `json:"-"` // Not serialized to JSON
}

// Issue represents a detected problem with session resumability.
type Issue struct {
	Type        IssueType `json:"type"`
	Message     string    `json:"message"`
	Fix         string    `json:"fix"` // Human-readable fix description
	AutoFixable bool      `json:"auto_fixable"`
}

// IssueType represents the category of resumability issue.
type IssueType string

// Recognized resumability issue type values.
const (
	IssueVersionMismatch   IssueType = "version_mismatch"
	IssueEmptySessionEnv   IssueType = "empty_session_env"
	IssueCompactedJSONL    IssueType = "compacted_jsonl"
	IssueJSONLMissing      IssueType = "jsonl_missing"
	IssueCwdMismatch       IssueType = "cwd_mismatch"
	IssueLockContention    IssueType = "lock_contention"
	IssuePermissions       IssueType = "permissions"        // File/directory access denied
	IssueCorruptedData     IssueType = "corrupted_data"     // Invalid YAML/JSONL
	IssueMissingDependency IssueType = "missing_dependency" // tmux, claude not available
	IssueEnvironment       IssueType = "environment"        // Shell, PATH issues
	IssueSessionConflict   IssueType = "session_conflict"   // UUID/name collision
	IssueUnknown           IssueType = "unknown"
)

// ValidIssueTypes returns all valid issue type constants.
func ValidIssueTypes() []IssueType {
	return []IssueType{
		IssueVersionMismatch,
		IssueEmptySessionEnv,
		IssueCompactedJSONL,
		IssueJSONLMissing,
		IssueCwdMismatch,
		IssueLockContention,
		IssuePermissions,
		IssueCorruptedData,
		IssueMissingDependency,
		IssueEnvironment,
		IssueSessionConflict,
		IssueUnknown,
	}
}

// IsValid returns true if the IssueType is a known constant.
func (it IssueType) IsValid() bool {
	for _, valid := range ValidIssueTypes() {
		if it == valid {
			return true
		}
	}
	return false
}

// String returns the string representation of the IssueType.
func (it IssueType) String() string {
	return string(it)
}

// Options configures validation behavior.
type Options struct {
	AutoFix           bool
	JSONOutput        bool
	TimeoutPerSession int // seconds per session test, 0 = no timeout (wait indefinitely)
}

// PrintText outputs the validation report in human-readable format with colors.
func PrintText(report *Report) {
	// Header
	fmt.Printf("\n%s\n", ui.Bold(fmt.Sprintf("Session Resumability Validation (%d sessions)", report.TotalSessions)))
	fmt.Printf("Validated at: %s\n\n", report.ValidatedAt.Format("2006-01-02 15:04:05"))

	// Session results
	for _, session := range report.Sessions {
		switch session.Status {
		case "resumable":
			fmt.Printf("  %s %s\n", ui.Green("✓"), ui.Bold(session.Name))
			if session.Path != "" {
				fmt.Printf("    %s\n", ui.Blue(session.Path))
			}

		case "failed":
			fmt.Printf("  %s %s\n", ui.Red("✗"), ui.Bold(session.Name))
			if session.Path != "" {
				fmt.Printf("    %s\n", ui.Blue(session.Path))
			}
			for i, issue := range session.Issues {
				fmt.Printf("    %s %s\n", ui.Yellow("Issue:"), issue.Message)
				if issue.Fix != "" {
					fmt.Printf("    %s %s\n", ui.Green("Fix:"), issue.Fix)
				}
				if issue.AutoFixable {
					fmt.Printf("    %s\n", ui.Green("(auto-fixable)"))
				}
				if i < len(session.Issues)-1 {
					fmt.Println()
				}
			}

		case "unknown":
			fmt.Printf("  %s %s\n", ui.Yellow("?"), ui.Bold(session.Name))
			if session.Path != "" {
				fmt.Printf("    %s\n", ui.Blue(session.Path))
			}
			for _, issue := range session.Issues {
				fmt.Printf("    %s %s\n", ui.Yellow("Issue:"), issue.Message)
				if issue.Fix != "" {
					fmt.Printf("    %s %s\n", ui.Green("Fix:"), issue.Fix)
				}
			}
		}
		fmt.Println()
	}

	// Summary section
	fmt.Printf("%s\n", ui.Bold("Summary:"))
	fmt.Printf("  Total sessions:    %d\n", report.TotalSessions)
	fmt.Printf("  %s %d (%.1f%%)\n",
		ui.Green("Resumable:"),
		report.Resumable,
		report.SuccessRate())
	fmt.Printf("  %s %d\n", ui.Red("Failed:"), report.Failed)
	if report.Unknown > 0 {
		fmt.Printf("  %s %d\n", ui.Yellow("Unknown:"), report.Unknown)
	}

	// Next steps
	fmt.Println()
	if report.HasFailures() {
		autoFixable := 0
		for _, session := range report.Sessions {
			if session.Status != "resumable" {
				for _, issue := range session.Issues {
					if issue.AutoFixable {
						autoFixable++
					}
				}
			}
		}

		if autoFixable > 0 {
			fmt.Printf("%s\n", ui.Bold("Next Steps:"))
			fmt.Printf("  Run %s to automatically fix %d issue(s)\n",
				ui.Green("agm admin doctor --validate --fix"),
				autoFixable)
		} else {
			fmt.Printf("%s\n", ui.Bold("Next Steps:"))
			fmt.Printf("  Review issues above and apply fixes manually\n")
		}
	} else {
		fmt.Printf("%s All sessions can resume successfully!\n", ui.Green("✓"))
	}
	fmt.Println()
}

// StandardReport represents the standardized health check JSON schema
type StandardReport struct {
	Timestamp      string          `json:"timestamp"`
	Tool           string          `json:"tool"`
	Command        string          `json:"command"`
	Status         string          `json:"status"` // "healthy", "degraded", "critical"
	Summary        StandardSummary `json:"summary"`
	Checks         []CheckResult   `json:"checks"`
	FixesAvailable int             `json:"fixes_available"`
}

// StandardSummary represents aggregated health check statistics
type StandardSummary struct {
	Total    int `json:"total"`
	Passed   int `json:"passed"`
	Warnings int `json:"warnings"`
	Failed   int `json:"failed"`
}

// CheckResult represents a single health check result
type CheckResult struct {
	Name     string `json:"name"`
	Category string `json:"category"` // "session"
	Status   string `json:"status"`   // "passed", "warning", "failed"
	Message  string `json:"message,omitempty"`
	Fix      string `json:"fix,omitempty"`
}

// ToStandardReport converts the validation report to standard schema
func (r *Report) ToStandardReport() *StandardReport {
	// Convert sessions to checks
	checks := make([]CheckResult, 0, len(r.Sessions))
	fixesAvailable := 0

	for _, session := range r.Sessions {
		check := CheckResult{
			Name:     session.Name,
			Category: "session",
		}

		switch session.Status {
		case "resumable":
			check.Status = "passed"
		case "failed":
			check.Status = "failed"
		case "unknown":
			check.Status = "warning"
		}

		// Add issues as message
		if len(session.Issues) > 0 {
			check.Message = session.Issues[0].Message
			check.Fix = session.Issues[0].Fix
			if session.Issues[0].AutoFixable {
				fixesAvailable++
			}
		}

		checks = append(checks, check)
	}

	// Determine overall status
	status := "healthy"
	if r.Failed > 0 {
		status = "critical"
	} else if r.Unknown > 0 {
		status = "degraded"
	}

	return &StandardReport{
		Timestamp: r.ValidatedAt.Format(time.RFC3339),
		Tool:      "agm",
		Command:   "doctor --validate",
		Status:    status,
		Summary: StandardSummary{
			Total:    r.TotalSessions,
			Passed:   r.Resumable,
			Warnings: r.Unknown,
			Failed:   r.Failed,
		},
		Checks:         checks,
		FixesAvailable: fixesAvailable,
	}
}

// PrintJSON outputs the validation report in standardized JSON format.
func PrintJSON(report *Report) error {
	standard := report.ToStandardReport()
	data, err := json.MarshalIndent(standard, "", "  ")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to marshal JSON: %v\n", err)
		return err
	}
	fmt.Println(string(data))
	return nil
}
