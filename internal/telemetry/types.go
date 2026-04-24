package telemetry

import "time"

// ViolationEvent represents a detected instruction violation
type ViolationEvent struct {
	ID              string          `json:"id" db:"id"`
	Timestamp       time.Time       `json:"timestamp" db:"timestamp"`
	InstructionType string          `json:"instruction_type" db:"instruction_type"`
	InstructionRule string          `json:"instruction_rule" db:"instruction_rule"`
	ViolationType   string          `json:"violation_type" db:"violation_type"`
	Confidence      Confidence      `json:"confidence" db:"confidence"`
	Agent           string          `json:"agent" db:"agent"`
	Context         string          `json:"context" db:"context"`
	DetectionMethod DetectionMethod `json:"detection_method" db:"detection_method"`
	ProjectPath     string          `json:"project_path,omitempty" db:"project_path"`
	Phase           string          `json:"phase,omitempty" db:"phase"`
}

// Confidence represents detection confidence level
type Confidence string

const (
	ConfidenceHigh   Confidence = "HIGH"
	ConfidenceMedium Confidence = "MEDIUM"
	ConfidenceLow    Confidence = "LOW"
)

// DetectionMethod represents how violation was detected
type DetectionMethod string

const (
	DetectionExternal     DetectionMethod = "external"
	DetectionSelfReported DetectionMethod = "self_reported"
)
