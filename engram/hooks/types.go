package hooks

import "time"

// HookEvent represents the event type that triggers a hook
type HookEvent string

const (
	// HookEventSessionCompletion is triggered when a session completes
	HookEventSessionCompletion HookEvent = "session-completion"
	// HookEventPhaseCompletion is triggered when a wayfinder phase completes
	HookEventPhaseCompletion HookEvent = "phase-completion"
	// HookEventPreCommit is triggered before a git commit
	HookEventPreCommit HookEvent = "pre-commit"
)

// HookType represents the type of hook executable
type HookType string

const (
	// HookTypeBinary represents a binary executable
	HookTypeBinary HookType = "binary"
	// HookTypeSkill represents an engram skill
	HookTypeSkill HookType = "skill"
	// HookTypeScript represents a shell script
	HookTypeScript HookType = "script"
)

// Hook represents a verification hook configuration
type Hook struct {
	Name        string    `toml:"name"`
	Event       HookEvent `toml:"event"`
	Priority    int       `toml:"priority"` // Higher = runs first
	Type        HookType  `toml:"type"`
	Command     string    `toml:"command"`
	Args        []string  `toml:"args"`
	Timeout     int       `toml:"timeout"`      // seconds
	CommandHash string    `toml:"command_hash"` // SHA-256 hash for integrity verification
}

// VerificationStatus represents the result status of a verification
type VerificationStatus string

const (
	// VerificationStatusPass indicates all checks passed
	VerificationStatusPass VerificationStatus = "pass"
	// VerificationStatusFail indicates critical failures
	VerificationStatusFail VerificationStatus = "fail"
	// VerificationStatusWarning indicates non-critical issues
	VerificationStatusWarning VerificationStatus = "warning"
)

// VerificationResult represents the output from a single hook execution
type VerificationResult struct {
	HookName   string             `json:"hook_name"`
	Status     VerificationStatus `json:"status"`
	Violations []Violation        `json:"violations"`
	Duration   time.Duration      `json:"duration_ms"`
	ExitCode   int                `json:"exit_code"`
}

// Violation represents a single verification failure or warning
type Violation struct {
	Severity   string   `json:"severity"`   // high|medium|low
	Message    string   `json:"message"`    // Human-readable error
	Files      []string `json:"files"`      // Affected files
	Suggestion string   `json:"suggestion"` // Remediation guidance
}

// AggregatedReport combines results from all hooks
type AggregatedReport struct {
	Timestamp time.Time            `json:"timestamp"`
	Event     HookEvent            `json:"event"`
	Results   []VerificationResult `json:"results"`
	Warnings  []HookWarning        `json:"warnings"`
	Summary   Summary              `json:"summary"`
}

// HookWarning represents a warning about hook execution
type HookWarning struct {
	Hook    string `json:"hook"`
	Message string `json:"message"`
}

// Summary provides aggregated statistics
type Summary struct {
	TotalHooks      int `json:"total_hooks"`
	PassedHooks     int `json:"passed_hooks"`
	FailedHooks     int `json:"failed_hooks"`
	WarningHooks    int `json:"warning_hooks"`
	TotalViolations int `json:"total_violations"`
	ExitCode        int `json:"exit_code"` // 0=pass, 1=fail, 2=warnings
}
