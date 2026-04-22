// Package detectors implements instruction violation detection
package detectors

import (
	"context"

	"github.com/vbonnet/dear-agent/internal/telemetry"
)

// Detector analyzes content for instruction violations
type Detector interface {
	// Name returns unique identifier for this detector
	Name() string

	// Detect analyzes input and returns violations found
	Detect(ctx context.Context, input DetectorInput) ([]telemetry.ViolationEvent, error)

	// SupportedInstructionTypes returns types this detector handles
	SupportedInstructionTypes() []string
}

// DetectorInput encapsulates content to analyze
type DetectorInput struct {
	Content  string            // Bash command, markdown content, etc.
	Rules    []InstructionRule // Rules to check against
	Metadata map[string]string // Context: project_path, phase, agent
}

// InstructionRule defines a violation pattern
type InstructionRule struct {
	ID         string               // Unique rule identifier (e.g., "never_use_cd_and")
	Pattern    string               // Regex or exact phrase
	Confidence telemetry.Confidence // HIGH, MEDIUM, LOW
	Message    string               // Human-readable description
}
