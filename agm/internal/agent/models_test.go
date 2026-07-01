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
		{"codex-cli", "5.6", "gpt-5.6"},
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
	if !ok || model != "2.5-flash" {
		t.Errorf("agy default: got (%q, %v), want (2.5-flash, true)", model, ok)
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
		// Claude aliases → AGY models (ce-7sh1: proper tier mapping)
		{"agy", "opus", "gemini-2.5-pro"},
		{"agy", "sonnet", "gemini-2.5-flash"},
		{"agy", "haiku", "gemini-2.0-flash-lite"},
		// Gemini aliases → Claude models
		{"claude-code", "2.5-pro", "claude-opus-4-8[1m]"},
		{"claude-code", "3.5-flash", "claude-haiku-4-5"},
		{"claude-code", "5.5", "claude-opus-4-8[1m]"},
		// Native aliases still work (not affected)
		{"gemini-cli", "3.5-flash", "gemini-3.5-flash"},
		{"claude-code", "opus", "claude-opus-4-8[1m]"},
		{"agy", "2.5-pro", "gemini-2.5-pro"},
		{"agy", "2.5-flash", "gemini-2.5-flash"},
	}
	for _, tt := range tests {
		got := ResolveModelFullName(tt.harness, tt.input)
		if got != tt.expected {
			t.Errorf("ResolveModelFullName(%q, %q) = %q, want %q", tt.harness, tt.input, got, tt.expected)
		}
	}
}

func TestCodexRegistryKeepsExplicitGPT56(t *testing.T) {
	if got := ResolveModelFullName("codex-cli", "5.6"); got != "gpt-5.6" {
		t.Fatalf("explicit codex 5.6 resolved to %q, want gpt-5.6", got)
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
		if m.FullName == "gemini-2.5-flash" {
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
