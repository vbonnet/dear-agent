package retrolint

import (
	"time"
)

// GuardType represents the classification of a deterministic verification guard.
type GuardType string

// Guard classifications for deterministic verification.
const (
	GuardTypeTest     GuardType = "test"
	GuardTypeFile     GuardType = "file"
	GuardTypeLaunchd  GuardType = "launchd"
	GuardTypeHook     GuardType = "hook"
	GuardTypeWorkflow GuardType = "workflow"
	GuardTypeLint     GuardType = "lint"
	GuardTypeDeferred GuardType = "deferred"
)

// Guard represents a single machine-verifiable guard or tracked deferral.
type Guard struct {
	Type   GuardType `json:"type" yaml:"type"`
	Path   string    `json:"path,omitempty" yaml:"path,omitempty"`
	Label  string    `json:"label,omitempty" yaml:"label,omitempty"`
	Bead   string    `json:"bead,omitempty" yaml:"bead,omitempty"`
	Reason string    `json:"reason,omitempty" yaml:"reason,omitempty"`
	Detail string    `json:"detail,omitempty" yaml:"detail,omitempty"`
	Valid  bool      `json:"valid"`
	Error  string    `json:"error,omitempty"`
}

// Status represents the evaluation outcome for a retrospective or suite.
type Status string

// Evaluation status outcomes.
const (
	StatusPass    Status = "PASS"
	StatusFail    Status = "FAIL"
	StatusWaived  Status = "WAIVED"
	StatusAbsent  Status = "ABSENT"
	StatusPresent Status = "PRESENT"
)

// Retrospective represents a parsed retrospective document.
type Retrospective struct {
	Path       string     `json:"path"`
	Date       string     `json:"date,omitempty"`
	ParsedDate *time.Time `json:"-"`
	Guards     []Guard    `json:"guards"`
	RawYAML    string     `json:"raw_yaml,omitempty"`
}

// RetroResult captures the evaluation result for a single retrospective file.
type RetroResult struct {
	Path   string   `json:"path"`
	Status Status   `json:"status"`
	Waived bool     `json:"waived"`
	Date   string   `json:"date,omitempty"`
	Guards []Guard  `json:"guards"`
	Errors []string `json:"errors,omitempty"`
}

// Report captures the aggregated result of a retro-lint run.
type Report struct {
	Status        Status        `json:"status"`
	Evaluated     int           `json:"evaluated"`
	Passed        int           `json:"passed"`
	Failed        int           `json:"failed"`
	Waived        int           `json:"waived"`
	Results       []RetroResult `json:"results"`
	RatchetErrors []string      `json:"ratchet_errors,omitempty"`
}

// BaselineEntry represents one waived retrospective in the grandfathered baseline store.
type BaselineEntry struct {
	Path   string `json:"path"`
	Reason string `json:"reason,omitempty"`
	Added  string `json:"added,omitempty"`
	Status string `json:"status,omitempty"`
}
