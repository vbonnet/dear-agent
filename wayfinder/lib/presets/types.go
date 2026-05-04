package presets

// Preset represents a complete wayfinder preset configuration
type Preset struct {
	Version        string           `yaml:"version" validate:"required,eq=1.0"`
	Name           string           `yaml:"name" validate:"required,max=64"`
	Description    string           `yaml:"description" validate:"required,min=10,max=200"`
	Extends        string           `yaml:"extends,omitempty"`
	TestCoverage   TestCoverage     `yaml:"test_coverage" validate:"required"`
	SpecAlignment  SpecAlignment    `yaml:"spec_alignment" validate:"required"`
	PhaseGates     PhaseGates       `yaml:"phase_gates" validate:"required"`
	Retrospective  Retrospective    `yaml:"retrospective" validate:"required"`
	EconomicTuning EconomicTuning   `yaml:"economic_tuning" validate:"required"`
	Overrides      *PresetOverrides `yaml:"overrides,omitempty"`
}

// TestCoverage configuration
type TestCoverage struct {
	MinimumPercentage    int  `yaml:"minimum_percentage" validate:"min=0,max=100"`
	MinimumTestCount     int  `yaml:"minimum_test_count" validate:"min=0"`
	EnforceCreativeTests bool `yaml:"enforce_creative_tests"`
}

// SpecAlignment configuration
type SpecAlignment struct {
	AllowDrift                  bool   `yaml:"allow_drift"`
	CheckpointAuditorStrictness string `yaml:"checkpoint_auditor_strictness" validate:"oneof=none basic strict maximum"`
	FreezeDuringBuild           bool   `yaml:"freeze_during_build"`
}

// PhaseGates configuration
type PhaseGates struct {
	S8BuildVerification bool   `yaml:"s8_build_verification"`
	S9ValidationDepth   string `yaml:"s9_validation_depth" validate:"oneof=minimal standard comprehensive"`
	S9HaltOnMinorIssues bool   `yaml:"s9_halt_on_minor_issues"`
	DeployGate          string `yaml:"deploy_gate" validate:"oneof=none advisory blocking"`
}

// Retrospective configuration
type Retrospective struct {
	Mandatory           bool `yaml:"mandatory"`
	StructuredLearnings bool `yaml:"structured_learnings"`
}

// EconomicTuning configuration (future Layer 2)
type EconomicTuning struct {
	ReputationMultiplier float64 `yaml:"reputation_multiplier" validate:"gt=0"`
	TokenCostMultiplier  float64 `yaml:"token_cost_multiplier" validate:"gt=0"`
	PenaltySeverity      string  `yaml:"penalty_severity" validate:"oneof=none low medium high"`
}

// PresetOverrides for inheritance (recursive structure matching Preset)
type PresetOverrides struct {
	TestCoverage   *TestCoverageOverrides   `yaml:"test_coverage,omitempty"`
	SpecAlignment  *SpecAlignmentOverrides  `yaml:"spec_alignment,omitempty"`
	PhaseGates     *PhaseGatesOverrides     `yaml:"phase_gates,omitempty"`
	Retrospective  *RetrospectiveOverrides  `yaml:"retrospective,omitempty"`
	EconomicTuning *EconomicTuningOverrides `yaml:"economic_tuning,omitempty"`
}

// TestCoverageOverrides overrides preset test-coverage thresholds; nil fields keep defaults.
type TestCoverageOverrides struct {
	MinimumPercentage    *int  `yaml:"minimum_percentage,omitempty"`
	MinimumTestCount     *int  `yaml:"minimum_test_count,omitempty"`
	EnforceCreativeTests *bool `yaml:"enforce_creative_tests,omitempty"`
}

// SpecAlignmentOverrides overrides spec-alignment policy; nil fields keep preset defaults.
type SpecAlignmentOverrides struct {
	AllowDrift                  *bool   `yaml:"allow_drift,omitempty"`
	CheckpointAuditorStrictness *string `yaml:"checkpoint_auditor_strictness,omitempty"`
	FreezeDuringBuild           *bool   `yaml:"freeze_during_build,omitempty"`
}

// PhaseGatesOverrides overrides phase-gate enforcement settings.
type PhaseGatesOverrides struct {
	S8BuildVerification *bool   `yaml:"s8_build_verification,omitempty"`
	S9ValidationDepth   *string `yaml:"s9_validation_depth,omitempty"`
	S9HaltOnMinorIssues *bool   `yaml:"s9_halt_on_minor_issues,omitempty"`
	DeployGate          *string `yaml:"deploy_gate,omitempty"`
}

// RetrospectiveOverrides overrides retrospective requirements for a phase.
type RetrospectiveOverrides struct {
	Mandatory           *bool `yaml:"mandatory,omitempty"`
	StructuredLearnings *bool `yaml:"structured_learnings,omitempty"`
}

// EconomicTuningOverrides overrides reputation/cost-economy parameters.
type EconomicTuningOverrides struct {
	ReputationMultiplier *float64 `yaml:"reputation_multiplier,omitempty"`
	TokenCostMultiplier  *float64 `yaml:"token_cost_multiplier,omitempty"`
	PenaltySeverity      *string  `yaml:"penalty_severity,omitempty"`
}
