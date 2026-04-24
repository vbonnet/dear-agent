package status

import "time"

// Status represents the WAYFINDER-STATUS.md file structure
type Status struct {
	SchemaVersion string     `yaml:"schema_version"`
	SessionID     string     `yaml:"session_id"`
	ProjectPath   string     `yaml:"project_path"`
	StartedAt     time.Time  `yaml:"started_at"`
	EndedAt       *time.Time `yaml:"ended_at,omitempty"`
	Status        string     `yaml:"status"` // in_progress, completed, abandoned
	CurrentPhase  string     `yaml:"current_phase,omitempty"`
	Phases        []Phase    `yaml:"phases"`

	// Adaptive depth fields (schema v2.0)
	Depth          string            `yaml:"depth,omitempty"`
	DepthSource    string            `yaml:"depth_source,omitempty"`
	Classification *Classification   `yaml:"classification,omitempty"`
	Escalation     *Escalation       `yaml:"escalation,omitempty"`
	EstimatedTime  string            `yaml:"estimated_time,omitempty"`
	PhaseBudgets   map[string]string `yaml:"phase_budgets,omitempty"`
}

// Phase represents a single Wayfinder phase
type Phase struct {
	Name        string     `yaml:"name"`
	Status      string     `yaml:"status"` // pending, in_progress, completed, skipped
	StartedAt   *time.Time `yaml:"started_at,omitempty"`
	CompletedAt *time.Time `yaml:"completed_at,omitempty"`
	Outcome     string     `yaml:"outcome,omitempty"` // success, partial, skipped
}

// Classification represents auto-classification results
type Classification struct {
	PredictedDepth     string                 `yaml:"predicted_depth"`
	Confidence         string                 `yaml:"confidence"`
	Rationale          string                 `yaml:"rationale"`
	Signals            map[string]interface{} `yaml:"signals,omitempty"`
	EscalationTriggers []string               `yaml:"escalation_triggers,omitempty"`
}

// Escalation represents depth escalation events
type Escalation struct {
	OriginalDepth string            `yaml:"original_depth"`
	EscalatedTo   string            `yaml:"escalated_to"`
	TriggerPhase  string            `yaml:"trigger_phase"`
	TriggerReason string            `yaml:"trigger_reason"`
	Timestamp     string            `yaml:"timestamp"`
	Metadata      map[string]string `yaml:"metadata,omitempty"`
}

// Constants
const (
	SchemaVersion  = "2.0"
	StatusFilename = "WAYFINDER-STATUS.md"

	// Session status
	StatusInProgress = "in_progress"
	StatusCompleted  = "completed"
	StatusAbandoned  = "abandoned"

	// Phase status
	PhaseStatusPending    = "pending"
	PhaseStatusInProgress = "in_progress"
	PhaseStatusCompleted  = "completed"
	PhaseStatusSkipped    = "skipped"

	// Outcomes
	OutcomeSuccess = "success"
	OutcomePartial = "partial"
	OutcomeSkipped = "skipped"

	// Depth tiers
	DepthXS = "XS"
	DepthS  = "S"
	DepthM  = "M"
	DepthL  = "L"
	DepthXL = "XL"

	// Depth sources
	DepthSourceAutoClassified = "auto-classified"
	DepthSourceUserOverride   = "user-override"
	DepthSourceEscalated      = "escalated"

	// Default depth
	DefaultDepth = DepthS
)

// AllPhases returns the standard Wayfinder phase sequence
func AllPhases() []string {
	return []string{"D1", "D2", "D3", "D4", "S4", "S5", "S6", "S7", "S8", "S9", "S10", "S11"}
}
