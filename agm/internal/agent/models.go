package agent

import (
	"fmt"
	"os"
)

// ModelSpec maps a short alias to a full model identifier.
type ModelSpec struct {
	Alias       string
	FullName    string
	Description string
}

// HarnessModels defines known models per harness.
var HarnessModels = map[string][]ModelSpec{
	"claude-code": {
		{Alias: "opus", FullName: "claude-opus-4-6[1m]", Description: "Latest Opus, 1M context"},
		{Alias: "sonnet", FullName: "claude-sonnet-4-6[1m]", Description: "Latest Sonnet, 1M context"},
		{Alias: "haiku", FullName: "claude-haiku-4-5", Description: "Fast, 200k context"},
		{Alias: "opus-200k", FullName: "claude-opus-4-6", Description: "Opus with default 200k context"},
		{Alias: "sonnet-200k", FullName: "claude-sonnet-4-6", Description: "Sonnet with default 200k context"},
		{Alias: "opusplan", FullName: "opusplan", Description: "Opus for planning, Sonnet for execution"},
	},
	"gemini-cli": {
		{Alias: "3.1-pro", FullName: "gemini-3.1-pro-preview", Description: "Latest, advanced reasoning"},
		{Alias: "3-flash", FullName: "gemini-3-flash-preview", Description: "High performance, lower cost"},
		{Alias: "3.1-flash-lite", FullName: "gemini-3.1-flash-lite-preview", Description: "Fastest, cheapest"},
		{Alias: "2.5-pro", FullName: "gemini-2.5-pro", Description: "Stable, complex tasks"},
		{Alias: "2.5-flash", FullName: "gemini-2.5-flash", Description: "Stable, best price-performance"},
		{Alias: "2.5-flash-lite", FullName: "gemini-2.5-flash-lite", Description: "Budget multimodal"},
	},
	"codex-cli": {
		{Alias: "5.4", FullName: "gpt-5.4", Description: "Flagship frontier model"},
		{Alias: "5.4-mini", FullName: "gpt-5.4-mini", Description: "Fast, efficient"},
		{Alias: "5.3-codex", FullName: "gpt-5.3-codex", Description: "Industry-leading coding model"},
		{Alias: "5.3-codex-spark", FullName: "gpt-5.3-codex-spark", Description: "Research preview"},
	},
	// opencode-cli: aggregated from all other harnesses (built dynamically)
}

// CrossHarnessAliases maps abstract tier names to harness-specific aliases.
// When AGM_DEFAULT_MODEL=opus is set and a gemini-cli session is created,
// "opus" is not a native gemini-cli alias. This table maps it to the equivalent.
// Only tier names that differ across harnesses need entries.
var CrossHarnessAliases = map[string]map[string]string{
	"gemini-cli": {
		"opus":   "2.5-pro",   // highest-tier → gemini-2.5-pro
		"sonnet": "3.1-pro",   // mid-tier → gemini-3.1-pro-preview
		"haiku":  "2.5-flash", // fast-tier → gemini-2.5-flash
	},
	"codex-cli": {
		"opus":   "5.4",      // highest-tier → gpt-5.4
		"sonnet": "5.4",      // mid-tier → gpt-5.4 (no direct equivalent)
		"haiku":  "5.4-mini", // fast-tier → gpt-5.4-mini
	},
	"claude-code": {
		"2.5-pro":   "opus",   // gemini alias → claude equivalent
		"3.1-pro":   "sonnet", // gemini alias → claude equivalent
		"2.5-flash": "haiku",  // gemini alias → claude equivalent
		"5.4":       "opus",   // codex alias → claude equivalent
		"5.4-mini":  "haiku",  // codex alias → claude equivalent
	},
}

// HarnessDefaults defines the default model alias for each harness.
// Harnesses not listed here require interactive model selection.
//
// claude-code defaults to sonnet. Opus stays opt-in via --model=opus or
// AGM_DEFAULT_MODEL=opus; Opus costs ~5× Sonnet per token and should not be
// the silent default.
var HarnessDefaults = map[string]string{
	"claude-code": "sonnet",
	"gemini-cli":  "2.5-flash",
	"codex-cli":   "5.4",
	// opencode-cli intentionally omitted — requires interactive picker
}

// HarnessModeDefaults defines the default permission mode for each harness.
// Harnesses not listed here start with no explicit permission mode.
var HarnessModeDefaults = map[string]string{
	"claude-code": "plan",
}

// TestModelDefaults defines cheap/fast models used for --test sessions.
// These ensure predictable, low-cost test runs regardless of the caller's model.
var TestModelDefaults = map[string]string{
	"claude-code":  "haiku",
	"gemini-cli":   "2.5-flash-lite",
	"codex-cli":    "5.4-mini",
	"opencode-cli": "haiku", // opencode supports Claude models via providers
}

// TestModelForHarness returns the test-specific model for a harness.
// Returns the test default and true, or empty string and false if none defined.
func TestModelForHarness(harnessName string) (string, bool) {
	d, ok := TestModelDefaults[harnessName]
	return d, ok
}

// HarnessModelFlag defines the CLI flag each harness uses to receive the model.
var HarnessModelFlag = map[string]string{
	"claude-code": "--model",
	"gemini-cli":  "-m",
	"codex-cli":   "-m",
	// opencode-cli uses config/env var, not a CLI flag
}

// ValidateModel checks if a model alias is known for the given harness.
// Returns nil if known. Prints a warning (but does NOT error) if unknown,
// allowing forward compatibility with newly released models.
func ValidateModel(harnessName, modelAlias string) error {
	models := GetModelsForHarness(harnessName)
	for _, m := range models {
		if m.Alias == modelAlias || m.FullName == modelAlias {
			return nil
		}
	}
	// Unknown model: warn but allow
	fmt.Fprintf(os.Stderr, "Warning: model '%s' not in registry for harness '%s'. Passing through as-is.\n", modelAlias, harnessName)
	return nil
}

// ResolveModelFullName resolves an alias to a full model name.
// If the alias is not found natively, checks CrossHarnessAliases for a mapping
// from another harness's tier name (e.g., "opus" → "2.5-pro" for gemini-cli).
// If still not found, returns the input as-is (passthrough for unknown models).
func ResolveModelFullName(harnessName, aliasOrFull string) string {
	models, ok := HarnessModels[harnessName]
	if !ok {
		return aliasOrFull
	}
	for _, m := range models {
		if m.Alias == aliasOrFull {
			return m.FullName
		}
	}

	// Check cross-harness alias mapping (e.g., "opus" → "2.5-pro" for gemini-cli)
	if crossMap, ok := CrossHarnessAliases[harnessName]; ok {
		if mapped, ok := crossMap[aliasOrFull]; ok {
			fmt.Fprintf(os.Stderr, "Note: mapping cross-harness alias '%s' → '%s' for %s\n", aliasOrFull, mapped, harnessName)
			// Resolve the mapped alias to its full name
			for _, m := range models {
				if m.Alias == mapped {
					return m.FullName
				}
			}
			return mapped // mapped alias not in models (shouldn't happen)
		}
	}

	return aliasOrFull // passthrough
}

// GetModelsForHarness returns known models for a harness.
// For opencode-cli, returns aggregated models from all harnesses.
func GetModelsForHarness(harnessName string) []ModelSpec {
	if harnessName == "opencode-cli" {
		return AllModels()
	}
	return HarnessModels[harnessName]
}

// DefaultModelForHarness returns the default model alias and true,
// or empty string and false if no default exists (e.g., opencode-cli).
func DefaultModelForHarness(harnessName string) (string, bool) {
	d, ok := HarnessDefaults[harnessName]
	return d, ok
}

// DefaultModeForHarness returns the default permission mode and true,
// or empty string and false if no default exists.
func DefaultModeForHarness(harnessName string) (string, bool) {
	d, ok := HarnessModeDefaults[harnessName]
	return d, ok
}

// NeedsInteractivePicker returns true if the harness has no default model
// and requires the user to select one interactively.
func NeedsInteractivePicker(harnessName string) bool {
	_, hasDefault := HarnessDefaults[harnessName]
	return !hasDefault
}

// AllModels returns all known models from all harnesses, for use in the
// opencode-cli interactive picker.
func AllModels() []ModelSpec {
	var all []ModelSpec
	for _, models := range HarnessModels {
		all = append(all, models...)
	}
	return all
}

// ModelAliases returns just the alias strings for a harness, suitable for
// tab completion.
func ModelAliases(harnessName string) []string {
	models := GetModelsForHarness(harnessName)
	aliases := make([]string, 0, len(models))
	for _, m := range models {
		aliases = append(aliases, m.Alias)
	}
	return aliases
}
