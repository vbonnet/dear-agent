package agent

import "testing"

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
		{"claude-code", "sonnet", "claude-sonnet-4-6[1m]"},
		{"claude-code", "opus", "claude-opus-4-6[1m]"},
		{"claude-code", "haiku", "claude-haiku-4-5"},
		{"gemini-cli", "2.5-flash", "gemini-2.5-flash"},
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
	// No default
	model, ok = DefaultModelForHarness("opencode-cli")
	if ok {
		t.Errorf("opencode-cli should have no default, got (%q, %v)", model, ok)
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
	// opus alias should resolve to claude-opus-4-6[1m] (1M context by default)
	got := ResolveModelFullName("claude-code", "opus")
	if got != "claude-opus-4-6[1m]" {
		t.Errorf("ResolveModelFullName(claude-code, opus) = %q, want %q", got, "claude-opus-4-6[1m]")
	}
	// sonnet alias should also get 1M context
	got = ResolveModelFullName("claude-code", "sonnet")
	if got != "claude-sonnet-4-6[1m]" {
		t.Errorf("ResolveModelFullName(claude-code, sonnet) = %q, want %q", got, "claude-sonnet-4-6[1m]")
	}
	// opus-200k should resolve to non-1M variant
	got = ResolveModelFullName("claude-code", "opus-200k")
	if got != "claude-opus-4-6" {
		t.Errorf("ResolveModelFullName(claude-code, opus-200k) = %q, want %q", got, "claude-opus-4-6")
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
	if !NeedsInteractivePicker("opencode-cli") {
		t.Error("opencode-cli should need interactive picker")
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
		{"gemini-cli", "haiku", "gemini-2.5-flash"},
		// Claude aliases → Codex models
		{"codex-cli", "opus", "gpt-5.4"},
		{"codex-cli", "haiku", "gpt-5.4-mini"},
		// Gemini aliases → Claude models
		{"claude-code", "2.5-pro", "claude-opus-4-6[1m]"},
		{"claude-code", "2.5-flash", "claude-haiku-4-5"},
		// Native aliases still work (not affected)
		{"gemini-cli", "2.5-flash", "gemini-2.5-flash"},
		{"claude-code", "opus", "claude-opus-4-6[1m]"},
	}
	for _, tt := range tests {
		got := ResolveModelFullName(tt.harness, tt.input)
		if got != tt.expected {
			t.Errorf("ResolveModelFullName(%q, %q) = %q, want %q", tt.harness, tt.input, got, tt.expected)
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
	foundGemini := false
	for _, m := range models {
		if m.FullName == "claude-sonnet-4-6[1m]" {
			foundClaude = true
		}
		if m.FullName == "gemini-2.5-flash" {
			foundGemini = true
		}
	}
	if !foundClaude {
		t.Error("opencode-cli models should include claude models")
	}
	if !foundGemini {
		t.Error("opencode-cli models should include gemini models")
	}
}
