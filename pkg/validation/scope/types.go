package scope

// PhaseID represents a Wayfinder phase identifier
type PhaseID string

const (
	PhaseProblem  PhaseID = "PROBLEM"
	PhaseResearch PhaseID = "RESEARCH"
	PhaseDesign   PhaseID = "DESIGN"
	PhaseSpec     PhaseID = "SPEC"
	PhasePlan     PhaseID = "PLAN"
	PhaseSetup    PhaseID = "SETUP"
	PhaseBuild    PhaseID = "BUILD"
	PhaseRetro    PhaseID = "RETRO"
)

// ViolationType represents the type of validation violation
type ViolationType string

const (
	ViolationAntiPattern       ViolationType = "anti-pattern"
	ViolationMissingSection    ViolationType = "missing-section"
	ViolationLength            ViolationType = "length"
	ViolationUnexpectedSection ViolationType = "unexpected-section"
)

// ViolationSeverity represents the severity level
type ViolationSeverity string

const (
	SeverityError   ViolationSeverity = "error"
	SeverityWarning ViolationSeverity = "warning"
)

// Violation represents a validation violation
type Violation struct {
	// Type of violation
	Type ViolationType `json:"type"`

	// Severity: error (blocks) or warning (logs)
	Severity ViolationSeverity `json:"severity"`

	// Section that triggered violation (if applicable)
	Section string `json:"section,omitempty"`

	// Line number in document (if applicable)
	Line int `json:"line,omitempty"`

	// Phase where this section belongs (for anti-patterns)
	BelongsIn PhaseID `json:"belongsIn,omitempty"`

	// Human-readable message
	Message string `json:"message"`

	// Suggestion for fixing
	Suggestion string `json:"suggestion,omitempty"`
}

// ValidationResult represents the result of validation
type ValidationResult struct {
	// Overall pass/fail (false if any errors)
	Passed bool `json:"passed"`

	// All violations (errors + warnings)
	Violations []Violation `json:"violations"`

	// Quick access to warnings
	Warnings []Violation `json:"warnings"`

	// Quick access to errors
	Errors []Violation `json:"errors"`

	// Actionable recommendations
	Recommendations []string `json:"recommendations"`

	// Metadata about the document
	Metadata ValidationMetadata `json:"metadata"`
}

// ValidationMetadata contains document metadata
type ValidationMetadata struct {
	PhaseID           PhaseID     `json:"phaseId"`
	WordCount         int         `json:"wordCount"`
	SectionCount      int         `json:"sectionCount"`
	ExpectedWordRange LengthRange `json:"expectedWordRange"`
}

// ValidationOptions contains validation configuration
type ValidationOptions struct {
	// Override validation (pass despite errors)
	Override bool `json:"override,omitempty"`

	// Custom fuzzy match threshold (default: 0.75)
	FuzzyThreshold float64 `json:"fuzzyThreshold,omitempty"`

	// Disable specific validation types
	Skip []string `json:"skip,omitempty"`
}

// Section represents a markdown section
type Section struct {
	// Heading text (without formatting)
	Heading string `json:"heading"`

	// Heading level (1-6 for h1-h6)
	Level int `json:"level"`

	// Line number in source document (1-indexed)
	StartLine int `json:"startLine"`

	// Original heading with formatting (for debugging)
	Raw string `json:"raw,omitempty"`
}

// AntiPattern represents a section that belongs in a different phase
type AntiPattern struct {
	// Section heading that triggers anti-pattern
	Section string

	// Which phase this section belongs in
	BelongsIn PhaseID

	// Severity: error (blocks) or warning (logs)
	Severity ViolationSeverity

	// Human-readable explanation of why this is an anti-pattern
	Explanation string

	// Alternative headings that also match this anti-pattern
	Aliases []string
}

// LengthRange represents word count range for phase documents
type LengthRange struct {
	// Minimum expected word count (warning if below)
	Min int `json:"min"`

	// Maximum expected word count (warning if 1.5x above)
	Max int `json:"max"`
}
