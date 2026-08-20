package agent

import "testing"

// Every harness default is a bare tier alias carrying no vendor token, so
// name-only inference returns "" for all of them. A quota guardrail keyed on
// that answer treats the normal launch of every provider as unmetered and
// skips itself. That is the regression this guards.
func TestModelFamilyForHarnessModelResolvesEveryHarnessDefault(t *testing.T) {
	for harness, alias := range HarnessDefaults {
		family := ModelFamilyForHarnessModel(harness, alias)
		if family == "" {
			t.Errorf("harness %s default model %q resolved to no provider family;"+
				" a quota gate keyed on this skips the default spawn path", harness, alias)
			continue
		}
		if !IsSupportedModelFamily(family) {
			t.Errorf("harness %s default model %q resolved to unsupported family %q",
				harness, alias, family)
		}
	}
}

// The supervisor pins its own default outside HarnessDefaults, so it needs its
// own guard: "sonnet-200k" names no vendor either.
func TestModelFamilyForHarnessModelResolvesTheSupervisorDefault(t *testing.T) {
	if got := ModelFamilyForHarnessModel("claude-code", "sonnet-200k"); got != "anthropic" {
		t.Errorf("ModelFamilyForHarnessModel(claude-code, sonnet-200k) = %q, want anthropic", got)
	}
}

// Every alias a harness publishes bills to some provider. One that resolves to
// no family is a spawn that silently escapes quota accounting.
func TestModelFamilyForHarnessModelResolvesEveryRegisteredAlias(t *testing.T) {
	for _, harness := range append(ActiveHarnesses(), "gemini-cli") {
		for _, spec := range GetModelsForHarness(harness) {
			if got := ModelFamilyForHarnessModel(harness, spec.Alias); got == "" {
				t.Errorf("harness %s alias %q (%s) resolved to no provider family",
					harness, spec.Alias, spec.FullName)
			}
		}
	}
}

func TestModelFamilyForHarnessModelMapsAliasTables(t *testing.T) {
	tests := []struct {
		name    string
		harness string
		model   string
		want    string
	}{
		{"native alias", "claude-code", "sonnet", "anthropic"},
		{"native alias with context suffix", "claude-code", "sonnet-200k", "anthropic"},
		{"codex tier alias", "codex-cli", "5.5", "openai"},
		{"agy public label alias", "agy", "3.5-flash", "gemini"},
		{"pi provider-qualified alias", "pi-cli", "sonnet", "anthropic"},
		{"open-model alias", "opencode-cli", "glm-5.2", "glm"},
		{"cross-harness tier alias", "claude-code", "5.6-terra", "anthropic"},
		{"legacy alias", "agy", "gemini-2.5-flash", "gemini"},
		{"vendor-named full identifier", "claude-code", "claude-opus-4-8", "anthropic"},
		{"registered label naming no vendor", "claude-code", "opusplan", "anthropic"},
		{"case-insensitive alias", "claude-code", "Sonnet", "anthropic"},
		{"genuinely unknown model", "claude-code", "no-such-model", ""},
		{"unknown harness and model", "not-a-harness", "no-such-model", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ModelFamilyForHarnessModel(tt.harness, tt.model); got != tt.want {
				t.Errorf("ModelFamilyForHarnessModel(%q, %q) = %q, want %q",
					tt.harness, tt.model, got, tt.want)
			}
		})
	}
}
