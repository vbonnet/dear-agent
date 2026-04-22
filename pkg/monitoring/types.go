// Package monitoring provides sub-agent monitoring and validation infrastructure
package monitoring

import (
	"time"
)

// ValidationConfig defines thresholds for validation signals
type ValidationConfig struct {
	MinFileCount    int     // Minimum files created
	MinLineCount    int     // Minimum total lines of code
	MinCommitCount  int     // Minimum git commits
	MinTestRuns     int     // Minimum test executions
	MaxStubKeywords int     // Maximum TODO/FIXME/NotImplemented keywords
	PassThreshold   float64 // Minimum score to pass (0.0-1.0)
}

// DefaultValidationConfig provides balanced defaults
var DefaultValidationConfig = ValidationConfig{
	MinFileCount:    3,
	MinLineCount:    50,
	MinCommitCount:  2,
	MinTestRuns:     1,
	MaxStubKeywords: 3,
	PassThreshold:   0.6,
}

// SimpleTaskConfig for simple/trivial tasks
var SimpleTaskConfig = ValidationConfig{
	MinFileCount:    1,
	MinLineCount:    10,
	MinCommitCount:  1,
	MinTestRuns:     0,
	MaxStubKeywords: 5,
	PassThreshold:   0.5,
}

// ComplexTaskConfig for complex multi-component tasks
var ComplexTaskConfig = ValidationConfig{
	MinFileCount:    5,
	MinLineCount:    200,
	MinCommitCount:  5,
	MinTestRuns:     2,
	MaxStubKeywords: 0,
	PassThreshold:   0.7,
}

// ValidationResult contains validation outcome
type ValidationResult struct {
	Passed  bool               // Overall pass/fail
	Score   float64            // Aggregate score (0.0-1.0)
	Signals []ValidationSignal // Individual signal results
	Summary string             // Human-readable summary
}

// ValidationSignal represents one validation dimension
type ValidationSignal struct {
	Name     string      // Signal name (e.g., "git_commits")
	Value    interface{} // Actual value
	Expected interface{} // Expected threshold
	Weight   float64     // Weight in scoring (0.0-1.0)
	Passed   bool        // Whether this signal passed
	Message  string      // Explanation
}

// MonitorStats contains aggregated monitoring statistics
type MonitorStats struct {
	AgentID         string
	StartTime       time.Time
	Duration        time.Duration
	FilesCreated    int
	FilesModified   int
	CommitsDetected int
	TestRuns        int
	EventsTotal     int
}

// FileFilter is a function that returns true if a file should be filtered out
type FileFilter func(path string) bool

// Event types as constants
const (
	EventFileCreated  = "sub_agent.file.created"
	EventFileModified = "sub_agent.file.modified"
	EventFileDeleted  = "sub_agent.file.deleted"
	EventGitCommit    = "sub_agent.git.commit"
	EventTestStarted  = "sub_agent.test.started"
	EventTestPassed   = "sub_agent.test.passed"
	EventTestFailed   = "sub_agent.test.failed"
	EventAgentStarted = "sub_agent.started"
	EventAgentDone    = "sub_agent.completed"
)
