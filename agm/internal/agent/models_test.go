package agent

import (
	"slices"
	"testing"
)

func TestValidateModel(t *testing.T) {
	// Known model should return nil
	if err := ValidateModel("claude-code", "sonnet"); err != nil {
		t.Errorf("expected nil for known model, got %v", err)
	}
	for _, model := range []string{"gpt-5.6", "gpt-5.5-codex"} {
		if err := ValidateModel("codex-cli", model); err != nil {
			t.Errorf("expected nil for mapped Codex model %q, got %v", model, err)
		}
	}
	// Unknown model should also return nil (warn but allow)
	if err := ValidateModel("claude-code", "unknown-model"); err != nil {
		t.Errorf("expected nil for unknown model (warn policy), got %v", err)
	}
}

func TestResolveModelFullName(t *testing.T) {
	tests := []struct {
		harness  string
		input    string
		expected string
	}{
		{"claude-code", "fable", "claude-fable-5"},
		{"claude-code", "sonnet", "claude-sonnet-4-6[1m]"},
		{"claude-code", "opus", "claude-opus-4-8[1m]"},
		{"claude-code", "haiku", "claude-haiku-4-5"},
		{"claude-code", "fable", "claude-fable-5"},
		{"gemini-cli", "3.5-flash", "gemini-3.5-flash"},
		{"agy", "3.5-flash", "Gemini 3.5 Flash (Medium)"},
		{"agy", "Gemini 3.5 Flash (Low)", "Gemini 3.5 Flash (Low)"},
		{"agy", "2.5-flash", "Gemini 3.5 Flash (Medium)"},
		{"agy", "gemini-2.5-flash", "Gemini 3.5 Flash (Medium)"},
		{"agy", "2.5-pro", "Gemini 3.1 Pro (High)"},
		{"agy", "gemini-2.5-pro", "Gemini 3.1 Pro (High)"},
		{"agy", "2.0-flash-lite", "Gemini 3.5 Flash (Low)"},
		{"agy", "gemini-2.0-flash-lite", "Gemini 3.5 Flash (Low)"},
		{"codex-cli", "5.6", "gpt-5.6-terra"},
		{"codex-cli", "gpt-5.6", "gpt-5.6-terra"},
		{"codex-cli", "gpt-5.5-codex", "gpt-5.5"},
		{"codex-cli", "5.5", "gpt-5.5"},
		{"codex-cli", "5.4", "gpt-5.4"},
		// Unknown alias passthrough
		{"claude-code", "future-model", "future-model"},
		// Unknown harness passthrough
		{"unknown-harness", "model", "model"},
	}
	for _, tt := range tests {
		got := ResolveModelFullName(tt.harness, tt.input)
		if got != tt.expected {
			t.Errorf("ResolveModelFullName(%q, %q) = %q, want %q", tt.harness, tt.input, got, tt.expected)
		}
	}
}

func TestNormalizeModelInputPreservesAgyPublicLabels(t *testing.T) {
	const publicLabel = "Gemini 3.5 Flash (Low)"
	if got := NormalizeModelInput("agy", publicLabel); got != publicLabel {
		t.Fatalf("NormalizeModelInput(agy, public label) = %q, want %q", got, publicLabel)
	}
	if got := NormalizeModelInput("agy", "3.5-FLASH-LOW"); got != "3.5-flash-low" {
		t.Fatalf("NormalizeModelInput(agy, alias) = %q, want 3.5-flash-low", got)
	}
}

func TestNormalizeModelInputCanonicalizesCrossHarnessAliases(t *testing.T) {
	for _, harness := range []string{"agy", "codex-cli"} {
		if got := NormalizeModelInput(harness, "OPUS"); got != "opus" {
			t.Errorf("NormalizeModelInput(%s, OPUS) = %q, want opus", harness, got)
		}
	}
}

func TestDefaultModelForHarness(t *testing.T) {
	// Has default — claude-code defaults to sonnet (opus is opt-in, it costs ~5× more).
	model, ok := DefaultModelForHarness("claude-code")
	if !ok || model != "sonnet" {
		t.Errorf("claude-code default: got (%q, %v), want (sonnet, true)", model, ok)
	}
	model, ok = DefaultModelForHarness("codex-cli")
	if !ok || model != "5.5" {
		t.Errorf("codex-cli default: got (%q, %v), want (5.5, true)", model, ok)
	}
	resolved := ResolveModelFullName("codex-cli", model)
	if resolved == "gpt-5.6" {
		t.Fatalf("codex-cli default resolved to unsupported ChatGPT-account model %q", resolved)
	}
	if resolved != "gpt-5.5" {
		t.Fatalf("codex-cli default resolved to %q, want gpt-5.5", resolved)
	}
	model, ok = DefaultModelForHarness("agy")
	if !ok || model != "3.5-flash" {
		t.Errorf("agy default: got (%q, %v), want (3.5-flash, true)", model, ok)
	}
	model, ok = DefaultModelForHarness("gemini-cli")
	if ok {
		t.Errorf("deprecated gemini-cli should not have an active default, got (%q, %v)", model, ok)
	}
	model, ok = DefaultModelForHarness("opencode-cli")
	if !ok || model != "glm-5.2" {
		t.Errorf("opencode-cli default: got (%q, %v), want (glm-5.2, true)", model, ok)
	}
}

func TestDefaultModeForHarness(t *testing.T) {
	// claude-code defaults to plan mode
	mode, ok := DefaultModeForHarness("claude-code")
	if !ok || mode != "plan" {
		t.Errorf("claude-code mode default: got (%q, %v), want (plan, true)", mode, ok)
	}
	// gemini-cli has no mode default
	mode, ok = DefaultModeForHarness("gemini-cli")
	if ok {
		t.Errorf("gemini-cli should have no mode default, got (%q, %v)", mode, ok)
	}
	// opencode-cli has no mode default
	mode, ok = DefaultModeForHarness("opencode-cli")
	if ok {
		t.Errorf("opencode-cli should have no mode default, got (%q, %v)", mode, ok)
	}
}

func TestResolveModelFullName_1MContext(t *testing.T) {
	// opus alias should resolve to claude-opus-4-8[1m] (1M context by default)
	got := ResolveModelFullName("claude-code", "opus")
	if got != "claude-opus-4-8[1m]" {
		t.Errorf("ResolveModelFullName(claude-code, opus) = %q, want %q", got, "claude-opus-4-8[1m]")
	}
	// sonnet alias should also get 1M context
	got = ResolveModelFullName("claude-code", "sonnet")
	if got != "claude-sonnet-4-6[1m]" {
		t.Errorf("ResolveModelFullName(claude-code, sonnet) = %q, want %q", got, "claude-sonnet-4-6[1m]")
	}
	// opus-200k should resolve to non-1M variant
	got = ResolveModelFullName("claude-code", "opus-200k")
	if got != "claude-opus-4-8" {
		t.Errorf("ResolveModelFullName(claude-code, opus-200k) = %q, want %q", got, "claude-opus-4-8")
	}
	// Default model alias should resolve correctly (default is sonnet).
	defaultModel, _ := DefaultModelForHarness("claude-code")
	resolved := ResolveModelFullName("claude-code", defaultModel)
	if resolved != "claude-sonnet-4-6[1m]" {
		t.Errorf("Default model resolves to %q, want claude-sonnet-4-6[1m]", resolved)
	}
}

func TestNeedsInteractivePicker(t *testing.T) {
	if NeedsInteractivePicker("claude-code") {
		t.Error("claude-code should not need interactive picker")
	}
	if NeedsInteractivePicker("opencode-cli") {
		t.Error("opencode-cli should not need interactive picker")
	}
}

func TestModelAliases(t *testing.T) {
	aliases := ModelAliases("claude-code")
	if len(aliases) == 0 {
		t.Error("expected non-empty aliases for claude-code")
	}
	found := false
	for _, a := range aliases {
		if a == "sonnet" {
			found = true
		}
	}
	if !found {
		t.Error("expected 'sonnet' in claude-code aliases")
	}
}

func TestResolveModelFullName_CrossHarness(t *testing.T) {
	tests := []struct {
		harness  string
		input    string
		expected string
	}{
		// Claude aliases → Gemini models
		{"gemini-cli", "opus", "gemini-2.5-pro"},
		{"gemini-cli", "sonnet", "gemini-3.1-pro-preview"},
		{"gemini-cli", "haiku", "gemini-3.5-flash"},
		// Claude aliases → Codex models
		{"codex-cli", "fable", "gpt-5.5"},
		{"codex-cli", "opus", "gpt-5.5"},
		{"codex-cli", "sonnet", "gpt-5.5"},
		{"codex-cli", "haiku", "gpt-5.4-mini"},
		// Claude aliases → current AGY catalog models
		{"agy", "fable", "Claude Opus 4.6 (Thinking)"},
		{"agy", "opus", "Claude Opus 4.6 (Thinking)"},
		{"agy", "sonnet", "Gemini 3.5 Flash (Medium)"},
		{"agy", "haiku", "Gemini 3.5 Flash (Low)"},
		// Gemini aliases → Claude models
		{"claude-code", "2.5-pro", "claude-opus-4-8[1m]"},
		{"claude-code", "3.5-flash", "claude-haiku-4-5"},
		{"claude-code", "5.5", "claude-opus-4-8[1m]"},
		{"claude-code", "5.6-sol", "claude-opus-4-8[1m]"},
		{"claude-code", "5.6-terra", "claude-sonnet-4-6[1m]"},
		{"claude-code", "5.6-luna", "claude-haiku-4-5"},
		// Native aliases still work (not affected)
		{"gemini-cli", "3.5-flash", "gemini-3.5-flash"},
		{"claude-code", "opus", "claude-opus-4-8[1m]"},
		{"agy", "3.1-pro-high", "Gemini 3.1 Pro (High)"},
		{"agy", "3.5-flash", "Gemini 3.5 Flash (Medium)"},
	}
	for _, tt := range tests {
		got := ResolveModelFullName(tt.harness, tt.input)
		if got != tt.expected {
			t.Errorf("ResolveModelFullName(%q, %q) = %q, want %q", tt.harness, tt.input, got, tt.expected)
		}
	}
}

func TestCodexResolves56Tiers(t *testing.T) {
	// "gpt-5.6" alone is unresolvable in codex CLI; the registry must map to the
	// explicit sol/terra/luna tier IDs, and bare "5.6" to the worker default.
	cases := map[string]string{
		"5.6":       "gpt-5.6-terra", // bare alias → worker default (terra), NOT frontier
		"5.6-sol":   "gpt-5.6-sol",
		"5.6-terra": "gpt-5.6-terra",
		"5.6-luna":  "gpt-5.6-luna",
	}
	for in, want := range cases {
		if got := ResolveModelFullName("codex-cli", in); got != want {
			t.Errorf("ResolveModelFullName(codex-cli, %q) = %q, want %q", in, got, want)
		}
	}
}

func TestGetModelsForHarness_OpenCode(t *testing.T) {
	models := GetModelsForHarness("opencode-cli")
	if len(models) == 0 {
		t.Error("opencode-cli should return aggregated models from all harnesses")
	}
	// Should have models from all harnesses
	foundClaude := false
	foundAgy := false
	foundDeprecatedGemini := false
	for _, m := range models {
		if m.FullName == "claude-sonnet-4-6[1m]" {
			foundClaude = true
		}
		if m.FullName == "Gemini 3.5 Flash (Medium)" {
			foundAgy = true
		}
		if m.FullName == "gemini-3.5-flash" {
			foundDeprecatedGemini = true
		}
	}
	if !foundClaude {
		t.Error("opencode-cli models should include claude models")
	}
	if !foundAgy {
		t.Error("opencode-cli models should include active AGY models")
	}
	if foundDeprecatedGemini {
		t.Error("opencode-cli models should not include deprecated gemini-cli-only models")
	}
}

func TestAgyModelCatalogMatchesPublicCLI(t *testing.T) {
	want := map[string]string{
		"3.5-flash":                  "Gemini 3.5 Flash (Medium)",
		"3.5-flash-medium":           "Gemini 3.5 Flash (Medium)",
		"3.5-flash-high":             "Gemini 3.5 Flash (High)",
		"3.5-flash-low":              "Gemini 3.5 Flash (Low)",
		"3.1-pro-low":                "Gemini 3.1 Pro (Low)",
		"3.1-pro-high":               "Gemini 3.1 Pro (High)",
		"claude-sonnet-4.6-thinking": "Claude Sonnet 4.6 (Thinking)",
		"claude-opus-4.6-thinking":   "Claude Opus 4.6 (Thinking)",
		"gpt-oss-120b-medium":        "GPT-OSS 120B (Medium)",
	}
	for _, model := range GetModelsForHarness("agy") {
		if fullName, ok := want[model.Alias]; ok {
			if model.FullName != fullName {
				t.Errorf("AGY alias %q resolves to %q, want %q", model.Alias, model.FullName, fullName)
			}
			delete(want, model.Alias)
		}
	}
	if len(want) != 0 {
		t.Fatalf("AGY model aliases missing from registry: %v", want)
	}
	if got := HarnessModelFlag["agy"]; got != "--model" {
		t.Fatalf("AGY model flag = %q, want --model", got)
	}
	if got, ok := TestModelForHarness("agy"); !ok || got != "3.5-flash-low" {
		t.Fatalf("AGY test default = (%q, %v), want (3.5-flash-low, true)", got, ok)
	}
}

func TestSupportedModelFamiliesPriorityOrder(t *testing.T) {
	want := []string{"anthropic", "openai", "gemini", "glm", "deepseek", "nemotron", "qwen"}
	got := ModelFamilyNames()
	if len(got) != len(want) {
		t.Fatalf("ModelFamilyNames length = %d, want %d: %v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("ModelFamilyNames()[%d] = %q, want %q (all: %v)", i, got[i], want[i], got)
		}
		if !IsSupportedModelFamily(got[i]) {
			t.Fatalf("family %q should be supported", got[i])
		}
	}
}

func TestDefaultModelForFamilyCoversParitySet(t *testing.T) {
	for _, family := range ModelFamilyNames() {
		model, ok := DefaultModelForFamily(family)
		if !ok {
			t.Fatalf("DefaultModelForFamily(%q) returned no model", family)
		}
		if err := ValidateModel("opencode-cli", model.FullName); err != nil {
			t.Fatalf("family %q default model %q should be syntactically safe: %v", family, model.FullName, err)
		}
	}
}

func TestOpenRouterProvidesRequestedOpenModelFamilies(t *testing.T) {
	want := []string{"glm", "deepseek", "nemotron", "qwen"}
	got := ModelFamiliesForHarness("openrouter")
	for _, family := range want {
		if !slices.Contains(got, family) {
			t.Fatalf("openrouter model aliases missing family %q; got %v", family, got)
		}
	}
}

// TestResolveModelFullName_NewModels2026_09 pins the two model launches wired
// in 2026-09: Gemini 3.8 Flash on AGY and Claude Fable 5.1 on Claude Code and
// Pi. AGY resolution must yield the exact public catalog label (AGP-20); the
// Anthropic surfaces must yield the API model id verified through
// GET /v1/models.
func TestResolveModelFullName_NewModels2026_09(t *testing.T) {
	tests := []struct {
		harness  string
		input    string
		expected string
	}{
		// Claude Fable 5.1: id verified live via the Anthropic Models API.
		{"claude-code", "fable5.1", "claude-fable-5-1"},
		{"pi-cli", "fable5.1", "anthropic/claude-fable-5-1"},
		// Fable 5 stays put: a point release must not silently repoint an
		// existing alias that callers already pin.
		{"claude-code", "fable", "claude-fable-5"},
		{"pi-cli", "fable", "anthropic/claude-fable-5"},

		// Gemini 3.8 Flash: labels verified live via `agy models`.
		{"agy", "3.8-flash", "Gemini 3.8 Flash (Medium)"},
		{"agy", "3.8-flash-medium", "Gemini 3.8 Flash (Medium)"},
		{"agy", "3.8-flash-high", "Gemini 3.8 Flash (High)"},
		{"agy", "3.8-flash-low", "Gemini 3.8 Flash (Low)"},
		// Exact public labels pass through unchanged (AGP-20).
		{"agy", "Gemini 3.8 Flash (High)", "Gemini 3.8 Flash (High)"},
		{"pi-cli", "gemini-flash-3.8", "google/gemini-3.8-flash"},
		// The incumbent Flash alias is unchanged.
		{"agy", "3.5-flash", "Gemini 3.5 Flash (Medium)"},
	}
	for _, tt := range tests {
		if got := ResolveModelFullName(tt.harness, tt.input); got != tt.expected {
			t.Errorf("ResolveModelFullName(%q, %q) = %q, want %q", tt.harness, tt.input, got, tt.expected)
		}
	}
}

// TestCrossHarnessAliases_Gemini38 keeps the new Gemini alias translatable to a
// Claude tier so a cross-harness spawn does not fall through as a literal.
func TestCrossHarnessAliases_Gemini38(t *testing.T) {
	if got := ResolveModelFullName("claude-code", "3.8-flash"); got != "claude-haiku-4-5" {
		t.Errorf("cross-harness 3.8-flash = %q, want claude-haiku-4-5", got)
	}
}
