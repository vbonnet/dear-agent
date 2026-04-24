// Package config implements the 4-tier configuration system for Engram.
//
// Configuration is loaded and merged from four tiers, with each tier
// overriding settings from the previous tier:
//
//  1. Core (system defaults)
//  2. Company (organization-wide settings)
//  3. Team (team/project settings)
//  4. User (individual developer preferences)
//
// Configuration file locations:
//   - Core: internal/config/default.yaml (embedded in binary)
//   - Company: /etc/engram/config.yaml (optional)
//   - Team: .engram/config.yaml in project root (optional)
//   - User: ~/.engram/config.yaml (optional)
//
// The loader reads configuration files in order and merges them using
// a "last-wins" strategy, where values from higher tiers override lower tiers.
//
// Example usage:
//
//	loader := config.NewLoader()
//	cfg, err := loader.Load()
//	if err != nil {
//	    log.Fatal(err)
//	}
//
//	// Access configuration
//	engramPath := cfg.Platform.EngramPath
//	tokenBudget := cfg.Platform.TokenBudget
//
// See ADR-003 for detailed rationale and design decisions.
package config

import "github.com/vbonnet/dear-agent/engram/internal/harnesseffort"

// Config represents the merged configuration from all tiers
// (core, company, team, user). See ADR-003 for hierarchy details.
type Config struct {
	// Core platform settings
	Platform PlatformConfig `yaml:"platform"`

	// Plugin settings
	Plugins PluginConfig `yaml:"plugins"`

	// Telemetry settings
	Telemetry TelemetryConfig `yaml:"telemetry"`

	// Enforcement settings
	Enforcement EnforcementConfig `yaml:"enforcement"`

	// Wayfinder settings
	Wayfinder WayfinderConfig `yaml:"wayfinder"`

	// HarnessEffort settings (loaded by harnesseffort package; exposed here for
	// inspection via 'engram config show').
	HarnessEffort harnesseffort.HarnessEffortConfig `yaml:"harness_effort"`

	// VCS settings for memory version control
	VCS VCSConfig `yaml:"vcs"`

	// Workspace settings (detected at runtime)
	Workspace           string `yaml:"workspace,omitempty"`
	WorkspaceConfigPath string `yaml:"workspace_config,omitempty"`
}

// VCSConfig controls version control tracking of memory files.
// Memory changes are automatically committed to a git repository for
// auditability, reversibility, and remote backup.
type VCSConfig struct {
	// Enabled controls whether VCS tracking is active (default: true)
	Enabled bool `yaml:"enabled"`

	// RepoPath is the directory for the memory git repository (default: ~/.engram/memories/)
	RepoPath string `yaml:"repo_path"`

	// PushStrategy controls when changes are pushed: immediate|async|batched|manual (default: async)
	PushStrategy string `yaml:"push_strategy"`

	// BatchInterval is the push interval for batched strategy (default: 5m)
	BatchInterval string `yaml:"batch_interval"`

	// RemoteURL is the git remote URL (empty = no remote)
	RemoteURL string `yaml:"remote_url"`

	// RemoteName is the git remote name (default: origin)
	RemoteName string `yaml:"remote_name"`

	// Branch is the git branch name (default: main)
	Branch string `yaml:"branch"`

	// Validation controls pre-commit validation behavior
	Validation VCSValidationConfig `yaml:"validation"`

	// OptIn controls which optional artifacts are tracked
	OptIn VCSOptInConfig `yaml:"opt_in"`
}

// VCSValidationConfig controls pre-commit validation for memory files
type VCSValidationConfig struct {
	// RequireWhyFile enforces .ai.md -> .why.md pairing (default: true)
	RequireWhyFile bool `yaml:"require_why_file"`

	// LintOnCommit runs engram linting before commit (default: true)
	LintOnCommit bool `yaml:"lint_on_commit"`
}

// VCSOptInConfig controls which optional artifacts are VCS-tracked
type VCSOptInConfig struct {
	// ErrorMemory tracks error-memory.jsonl (default: false)
	ErrorMemory bool `yaml:"error_memory"`

	// EcphoryMetadata tracks retrieval counts and timestamps (default: false)
	EcphoryMetadata bool `yaml:"ecphory_metadata"`

	// Logs tracks telemetry and session logs (default: false)
	Logs bool `yaml:"logs"`
}

// PlatformConfig contains core platform settings
type PlatformConfig struct {
	// Agent type (claude-code, cursor, windsurf, aider, unknown)
	Agent string `yaml:"agent"`

	// Base directory for engrams
	EngramPath string `yaml:"engram_path"`

	// Token budget for ecphory retrieval
	TokenBudget int `yaml:"token_budget"`
}

// PluginConfig contains plugin-related settings
type PluginConfig struct {
	// Plugin search paths
	Paths []string `yaml:"paths"`

	// Disabled plugins
	Disabled []string `yaml:"disabled"`

	// Security settings for plugin execution
	Security SecurityConfig `yaml:"security"`
}

// SecurityConfig contains security settings for plugin execution
type SecurityConfig struct {
	// Resource limits for plugin processes
	ResourceLimits ResourceLimitsConfig `yaml:"resource_limits"`
}

// ResourceLimitsConfig contains resource limit settings for processes
type ResourceLimitsConfig struct {
	// MaxProcesses: soft limit for number of child processes (default: 100)
	MaxProcesses uint64 `yaml:"max_processes"`

	// MaxProcessesHard: hard limit for number of child processes (default: 200)
	MaxProcessesHard uint64 `yaml:"max_processes_hard"`

	// MaxFileDescriptors: limit for number of open files (default: 8192)
	MaxFileDescriptors uint64 `yaml:"max_file_descriptors"`

	// MaxMemory: limit for virtual memory in bytes (default: 4GB)
	MaxMemory uint64 `yaml:"max_memory"`
}

// TelemetryConfig contains telemetry settings
type TelemetryConfig struct {
	// Enable telemetry collection
	Enabled bool `yaml:"enabled"`

	// Enforce telemetry (company can prevent user disable)
	Enforce bool `yaml:"enforce"`

	// Storage mode: "local" or "remote"
	Storage string `yaml:"storage"`

	// Local JSONL storage path
	Path string `yaml:"path"`

	// Maximum file size in MB before rotation
	MaxSizeMB int `yaml:"max_size_mb"`

	// Retention period in days
	RetentionDays int `yaml:"retention_days"`

	// Remote sink configuration (optional)
	Sink *SinkConfig `yaml:"sink,omitempty"`

	// Keep local backup when using remote sink
	LocalBackup bool `yaml:"local_backup"`
}

// SinkConfig contains remote telemetry sink configuration
type SinkConfig struct {
	// Sink type: "http", "grpc", etc.
	Type string `yaml:"type"`

	// Endpoint URL
	URL string `yaml:"url"`

	// Authentication configuration
	Auth *AuthConfig `yaml:"auth,omitempty"`
}

// AuthConfig contains authentication settings for remote sink
type AuthConfig struct {
	// Auth type: "bearer", "basic", etc.
	Type string `yaml:"type"`

	// Environment variable containing auth token
	TokenEnv string `yaml:"token_env"`
}

// WayfinderConfig contains wayfinder workflow settings
type WayfinderConfig struct {
	// W0 (Project Framing) settings
	W0 W0Config `yaml:"w0"`
}

// W0Config contains W0 project framing settings
type W0Config struct {
	// Enable W0 project framing
	Enabled bool `yaml:"enabled"`

	// Enforce W0 (company can prevent user disable)
	Enforce bool `yaml:"enforce"`

	// Detection settings
	Detection W0DetectionConfig `yaml:"detection"`

	// Question settings
	Questions W0QuestionConfig `yaml:"questions"`

	// Synthesis settings
	Synthesis W0SynthesisConfig `yaml:"synthesis"`

	// Telemetry settings
	Telemetry W0TelemetryConfig `yaml:"telemetry"`
}

// W0DetectionConfig contains vagueness detection settings
type W0DetectionConfig struct {
	// Enable automatic vagueness detection
	Enabled bool `yaml:"enabled"`

	// Minimum word count (below this triggers vague signal)
	MinWordCount int `yaml:"min_word_count"`

	// Maximum word count to skip W0 (above this, skip if detailed)
	MaxSkipWordCount int `yaml:"max_skip_word_count"`

	// Vagueness threshold (0.0-1.0, confidence to trigger W0)
	VaguenessThreshold float64 `yaml:"vagueness_threshold"`
}

// W0QuestionConfig contains question flow settings
type W0QuestionConfig struct {
	// Maximum clarification rounds (hard limit)
	MaxRounds int `yaml:"max_rounds"`

	// Include examples in questions
	IncludeExamples bool `yaml:"include_examples"`

	// Include expandable help text
	IncludeHelpText bool `yaml:"include_help_text"`
}

// W0SynthesisConfig contains charter synthesis settings
type W0SynthesisConfig struct {
	// Synthesis method: "simple" or "few_shot_cot"
	Method string `yaml:"method"`

	// Synthesis timeout in seconds
	TimeoutSeconds int `yaml:"timeout_seconds"`

	// Retry on synthesis failure
	RetryOnFailure bool `yaml:"retry_on_failure"`
}

// W0TelemetryConfig contains W0-specific telemetry settings
type W0TelemetryConfig struct {
	// Enable W0 telemetry (respects global telemetry.enabled)
	Enabled bool `yaml:"enabled"`

	// Log detected technical terms
	LogTechnicalTerms bool `yaml:"log_technical_terms"`

	// Log response metadata (lengths, scores)
	LogResponseMetadata bool `yaml:"log_response_metadata"`
}
