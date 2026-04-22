// Package context provides context window management for LLM conversations.
//
// This package implements model-specific sweet spot detection, zone classification,
// and smart compaction recommendations based on academic benchmarks and production data.
package context

import (
	"time"
)

// ModelConfig contains context window configuration for a specific LLM model.
type ModelConfig struct {
	// ModelID is the unique identifier for the model (e.g., "claude-sonnet-4.5")
	ModelID string `yaml:"model_id"`

	// Provider is the model provider (e.g., "anthropic", "google", "openai")
	Provider string `yaml:"provider"`

	// MaxContextTokens is the maximum context window size in tokens
	MaxContextTokens int `yaml:"max_context_tokens"`

	// SweetSpotThreshold is the percentage where quality is maintained (0.0-1.0)
	SweetSpotThreshold float64 `yaml:"sweet_spot_threshold"`

	// WarningThreshold is the percentage where degradation begins (0.0-1.0)
	WarningThreshold float64 `yaml:"warning_threshold"`

	// DangerThreshold is the percentage where significant quality loss occurs (0.0-1.0)
	DangerThreshold float64 `yaml:"danger_threshold"`

	// CriticalThreshold is the percentage where performance degrades significantly (0.0-1.0)
	CriticalThreshold float64 `yaml:"critical_threshold"`

	// BenchmarkSources lists the academic/community benchmarks used for thresholds
	BenchmarkSources []string `yaml:"benchmark_sources"`

	// Notes contains additional information about the model configuration
	Notes string `yaml:"notes"`

	// Confidence indicates data quality: HIGH, MEDIUM-HIGH, MEDIUM, LOW
	Confidence string `yaml:"confidence"`

	// LastUpdated is when this configuration was last updated
	LastUpdated time.Time `yaml:"last_updated"`

	// Capabilities describes what the model can handle well
	Capabilities *ModelCapabilities `yaml:"capabilities,omitempty"`

	// ScaffoldingOverrides controls how the harness adapts for this model
	ScaffoldingOverrides *ScaffoldingOverrides `yaml:"scaffolding_overrides,omitempty"`
}

// ModelCapabilities describes a model's strengths and supported features.
type ModelCapabilities struct {
	// ContextStability indicates how well the model handles large contexts.
	// Values: "high", "medium", "low"
	ContextStability string `yaml:"context_stability"`

	// SelfEvalQuality indicates how reliably the model evaluates its own work.
	// Values: "high", "medium", "low"
	SelfEvalQuality string `yaml:"self_eval_quality"`

	// LongContext indicates whether the model supports 1M+ tokens.
	LongContext bool `yaml:"long_context"`
}

// ScaffoldingOverrides controls how the harness process adapts for a model.
type ScaffoldingOverrides struct {
	// SkipContextResets indicates the model doesn't need context resets between phases.
	SkipContextResets bool `yaml:"skip_context_resets"`

	// EvaluatorRequired indicates whether an evaluator step is mandatory.
	EvaluatorRequired bool `yaml:"evaluator_required"`

	// LiteProcessEligible indicates the model can use a lighter Wayfinder process.
	LiteProcessEligible bool `yaml:"lite_process_eligible"`
}

// Usage represents current context window usage for a session.
type Usage struct {
	// TotalTokens is the maximum context window size
	TotalTokens int

	// UsedTokens is the current token usage
	UsedTokens int

	// PercentageUsed is the usage percentage (0.0-100.0)
	PercentageUsed float64

	// LastUpdated is when this usage was last measured
	LastUpdated time.Time

	// Source indicates how usage was detected
	// Values: "claude-cli", "gemini-cli", "opencode", "codex", "heuristic"
	Source string

	// ModelID is the model being used (e.g., "claude-sonnet-4.5")
	ModelID string

	// SessionID is the CLI session identifier (optional)
	SessionID string
}

// Zone represents the current context usage zone.
type Zone string

const (
	// ZoneSafe indicates context usage below warning threshold (quality maintained)
	ZoneSafe Zone = "safe"

	// ZoneWarning indicates context usage between warning and danger (light degradation)
	ZoneWarning Zone = "warning"

	// ZoneDanger indicates context usage between danger and critical (significant loss)
	ZoneDanger Zone = "danger"

	// ZoneCritical indicates context usage above critical threshold (high utilization, performance degrades)
	ZoneCritical Zone = "critical"
)

// Thresholds contains absolute token counts for each zone boundary.
type Thresholds struct {
	// SweetSpotTokens is the token count at sweet spot threshold
	SweetSpotTokens int

	// SweetSpotPercentage is the percentage at sweet spot threshold (0.0-1.0)
	SweetSpotPercentage float64

	// WarningTokens is the token count at warning threshold
	WarningTokens int

	// WarningPercentage is the percentage at warning threshold (0.0-1.0)
	WarningPercentage float64

	// DangerTokens is the token count at danger threshold
	DangerTokens int

	// DangerPercentage is the percentage at danger threshold (0.0-1.0)
	DangerPercentage float64

	// CriticalTokens is the token count at critical threshold
	CriticalTokens int

	// CriticalPercentage is the percentage at critical threshold (0.0-1.0)
	CriticalPercentage float64

	// MaxTokens is the total context window size
	MaxTokens int
}

// PhaseState represents the current phase state for smart compaction decisions.
type PhaseState string

const (
	// PhaseStart indicates a phase just started (new context will be added)
	PhaseStart PhaseState = "start"

	// PhaseMiddle indicates mid-phase operation (normal threshold)
	PhaseMiddle PhaseState = "middle"

	// PhaseEnd indicates phase nearing completion (can push +5%)
	PhaseEnd PhaseState = "end"
)

// CLI represents the type of CLI being used.
type CLI string

const (
	// CLIClaude represents Claude Code CLI
	CLIClaude CLI = "claude"

	// CLIGemini represents Gemini CLI
	CLIGemini CLI = "gemini"

	// CLIOpenCode represents OpenCode CLI
	CLIOpenCode CLI = "opencode"

	// CLICodex represents Codex CLI
	CLICodex CLI = "codex"

	// CLIUnknown represents an unknown or unsupported CLI
	CLIUnknown CLI = "unknown"
)

// CheckResult contains the result of a context check operation.
type CheckResult struct {
	// Zone is the current context usage zone
	Zone Zone

	// Percentage is the usage percentage (0.0-100.0)
	Percentage float64

	// ShouldCompact indicates if compaction is recommended
	ShouldCompact bool

	// Thresholds contains the zone boundaries for this model
	Thresholds Thresholds

	// ModelID is the model being checked
	ModelID string

	// Source indicates how usage was detected
	Source string

	// Timestamp is when this check was performed
	Timestamp time.Time
}

// CompactionRecommendation provides details about why compaction is recommended.
type CompactionRecommendation struct {
	// Recommended is true if compaction should occur
	Recommended bool

	// Reason explains why compaction is recommended
	Reason string

	// Urgency indicates priority: "low", "medium", "high", "critical"
	Urgency string

	// EstimatedReduction is the expected percentage reduction (e.g., 15-25%)
	EstimatedReduction string
}
