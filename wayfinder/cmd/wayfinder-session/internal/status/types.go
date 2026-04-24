package status

import (
	"fmt"
	"regexp"
	"strings"
	"time"
)

// Status represents the WAYFINDER-STATUS.md file structure
type Status struct {
	SchemaVersion  string     `yaml:"schema_version"`
	Version        string     `yaml:"version,omitempty"` // "v1" or "v2", defaults to v1 if missing
	SessionID      string     `yaml:"session_id"`
	ProjectPath    string     `yaml:"project_path"`
	StartedAt      time.Time  `yaml:"started_at"`
	EndedAt        *time.Time `yaml:"ended_at,omitempty"`
	Status         string     `yaml:"status"`                    // in_progress, completed, abandoned, obsolete, blocked
	LifecycleState string     `yaml:"lifecycle_state,omitempty"` // working, input-required, dependency-blocked, validating, completed, failed, canceled
	CurrentPhase   string     `yaml:"current_phase,omitempty"`
	SkipRoadmap    bool       `yaml:"skip_roadmap,omitempty"`  // v2 only: skip roadmap.* phases (opt-out flag)
	BlockedOn      string     `yaml:"blocked_on,omitempty"`    // Agent ID or task ID causing block
	ErrorMessage   string     `yaml:"error_message,omitempty"` // Error details when lifecycle_state=failed
	InputNeeded    string     `yaml:"input_needed,omitempty"`  // Description of needed input when lifecycle_state=input-required
	Phases         []Phase    `yaml:"phases"`
}

// Phase represents a single Wayfinder phase
type Phase struct {
	Name        string     `yaml:"name"`
	Status      string     `yaml:"status"` // pending, in_progress, completed, skipped
	StartedAt   *time.Time `yaml:"started_at,omitempty"`
	CompletedAt *time.Time `yaml:"completed_at,omitempty"`
	Outcome     string     `yaml:"outcome,omitempty"` // success, partial, skipped
}

// Constants
const (
	SchemaVersion  = "1.0"
	StatusFilename = "WAYFINDER-STATUS.md"

	// Wayfinder versions
	WayfinderV1 = "v1" // Legacy W0-W12 phases
	WayfinderV2 = "v2" // Dot-notation phases (discovery.problem, build.implement, etc.)

	// Session status
	StatusInProgress = "in_progress"
	StatusCompleted  = "completed"
	StatusAbandoned  = "abandoned"
	StatusObsolete   = "obsolete"
	StatusBlocked    = "blocked"

	// Phase status
	PhaseStatusPending    = "pending"
	PhaseStatusInProgress = "in_progress"
	PhaseStatusCompleted  = "completed"
	PhaseStatusSkipped    = "skipped"

	// Outcomes
	OutcomeSuccess = "success"
	OutcomePartial = "partial"
	OutcomeSkipped = "skipped"

	// Lifecycle states (A2A-compatible, 7-state model for observability)
	LifecycleWorking           = "working"            // Agent actively executing
	LifecycleInputRequired     = "input-required"     // Blocked on user input (AskUserQuestion)
	LifecycleDependencyBlocked = "dependency-blocked" // Waiting for another agent/task
	LifecycleValidating        = "validating"         // Running S9 validation or quality gates
	LifecycleCompleted         = "completed"          // Task successfully finished
	LifecycleFailed            = "failed"             // Error encountered, cannot proceed
	LifecycleCanceled          = "canceled"           // Task abandoned or superseded
)

// AllPhases returns the standard Wayfinder phase sequence based on version
// Defaults to v1 if version is empty (backward compatibility)
// Can be called with no arguments (AllPhases()) which defaults to v1
func AllPhases(version ...string) []string {
	v := WayfinderV1 // default
	if len(version) > 0 && version[0] != "" {
		v = version[0]
	}
	if v == WayfinderV2 {
		return AllPhasesV2()
	}
	return AllPhasesV1()
}

// AllPhasesV1 returns v1 phase sequence (W0-W12)
// Returns all 13 phases: W0 (Project Framing) + D1-D4 (Discovery) + S4-S11 (SDLC)
func AllPhasesV1() []string {
	return []string{"W0", "D1", "D2", "D3", "D4", "S4", "S5", "S6", "S7", "S8", "S9", "S10", "S11"}
}

// AllPhasesV2 returns v2 phase sequence (short names: W0, D1, D2, etc.)
// Note: S7 (roadmap.planning) is optional and can be skipped for small projects
func AllPhasesV2() []string {
	return AllPhasesV2Schema()
}

// V2 phase validation regex pattern
// Matches: V2 descriptive names (CHARTER, PROBLEM, RESEARCH, DESIGN, SPEC, PLAN, SETUP, BUILD, RETRO)
// Also matches legacy V1 short names (W0, D1, D2, D3, D4, S4-S11) for backward compat
var v2PhasePattern = regexp.MustCompile(`^([WDS]\d+|CHARTER|PROBLEM|RESEARCH|DESIGN|SPEC|PLAN|SETUP|BUILD|RETRO)$`)

// IsValidV2Phase returns true if the phase name is valid v2 format
func IsValidV2Phase(phase string) bool {
	return v2PhasePattern.MatchString(phase)
}

// PhaseToFileName converts a phase name to a filename
// Examples: discovery.problem → discovery-problem.md, definition → definition.md
func PhaseToFileName(phase string) string {
	filename := strings.ReplaceAll(phase, ".", "-")
	return filename + ".md"
}

// FileNameToPhase converts a filename to a phase name
// Examples: discovery-problem.md → discovery.problem, definition.md → definition
// Returns empty string if filename doesn't match phase pattern
func FileNameToPhase(filename string) string {
	// Must have .md extension
	if !strings.HasSuffix(filename, ".md") {
		return ""
	}

	// Remove .md extension
	name := strings.TrimSuffix(filename, ".md")

	// Check if it's a v2 hyphenated name (contains hyphen but not at boundaries)
	parts := strings.Split(name, "-")

	// Single word phase (definition, deploy, etc.)
	if len(parts) == 1 {
		return name
	}

	// Two-part phase (discovery-problem → discovery.problem)
	if len(parts) == 2 {
		return parts[0] + "." + parts[1]
	}

	// Three-part phase (design-tech-lead → design.tech-lead)
	if len(parts) == 3 {
		return parts[0] + "." + parts[1] + "-" + parts[2]
	}

	// Invalid format (too many parts)
	return ""
}

// GetVersion returns the wayfinder version, defaulting to v1 for backward compatibility
func (s *Status) GetVersion() string {
	if s.Version == "" {
		return WayfinderV1 // default to v1
	}
	return s.Version
}

// V1ToV2PhaseMap maps v1 phase names to v2 equivalents
var V1ToV2PhaseMap = map[string]string{
	"W0":  "", // Project Framing removed in v2
	"D1":  "discovery.problem",
	"D2":  "discovery.solutions",
	"D3":  "discovery.approach",
	"D4":  "discovery.requirements",
	"S4":  "definition",
	"S5":  "",                 // Research merged into discovery/design in v2
	"S6":  "design.tech-lead", // S6 split into 3 sub-phases in v2
	"S7":  "roadmap.planning", // Optional in v2
	"S8":  "build.implement",
	"S9":  "build.test",
	"S10": "deploy",
	"S11": "retrospective",
}

// V2ToV1PhaseMap maps v2 phase names to v1 equivalents (for backward compat reference)
var V2ToV1PhaseMap = map[string]string{
	"discovery.problem":      "D1",
	"discovery.solutions":    "D2",
	"discovery.approach":     "D3",
	"discovery.requirements": "D4",
	"definition":             "S4",
	"design.tech-lead":       "S6",
	"design.security":        "S6",
	"design.qa":              "S6",
	"roadmap.planning":       "S7",
	"roadmap.breakdown":      "S7",
	"roadmap.dependencies":   "S7",
	"build.implement":        "S8",
	"build.test":             "S9",
	"build.integrate":        "S9",
	"deploy":                 "S10",
	"retrospective":          "S11",
}

// Nesting-related errors
var (
	// ErrMaxDepthExceeded is returned when nesting depth exceeds MaxNestingDepth
	ErrMaxDepthExceeded = fmt.Errorf("maximum nesting depth exceeded (%d levels)", 10)
)
