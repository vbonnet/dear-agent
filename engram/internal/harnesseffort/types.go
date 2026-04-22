// Package harnesseffort provides configuration loading and config generation
// for effort-tier settings across AI harnesses (Codex CLI, OpenCode, Gemini CLI).
package harnesseffort

// HarnessEffortConfig is the top-level configuration structure for harness effort tiers.
// It is loaded from a canonical YAML defaults file and merged with optional company/user overrides.
type HarnessEffortConfig struct {
	ModelAliases       map[string]string         `yaml:"model_aliases"`
	SubagentPreference string                    `yaml:"subagent_preference"`
	TaskTypes          map[string]TaskTypeConfig `yaml:"task_types"`
	Tiers              map[string]TierConfig     `yaml:"tiers"`
}

// TaskTypeConfig defines harness preference ordering for a task type.
type TaskTypeConfig struct {
	HarnessOrder []string `yaml:"harness_order"`
}

// TierConfig defines per-provider model and effort settings for an effort tier.
type TierConfig struct {
	Description string                    `yaml:"description"`
	Providers   map[string]ProviderConfig `yaml:"providers"`
}

// ProviderConfig defines model and effort settings for a specific provider within a tier.
type ProviderConfig struct {
	Model           string `yaml:"model"`
	Effort          string `yaml:"effort,omitempty"`
	ReasoningEffort string `yaml:"reasoning_effort,omitempty"`
}

// GenerateOpts controls the behaviour of the Generate function.
type GenerateOpts struct {
	// DryRun prints what would be written without modifying files.
	DryRun bool
	// Provider limits generation to a single provider (empty = all providers).
	Provider string
	// Harness limits generation to a single harness (empty = all harnesses).
	// Valid values: "codex", "opencode", "gemini".
	Harness string
	// OpenCodeDir is the directory in which opencode.json is written (default ".").
	OpenCodeDir string
}

// OutputFile represents a file that the generator wants to write.
type OutputFile struct {
	Path    string
	Content []byte
}
