package agent

import (
	"slices"
	"testing"
)

func TestPiModelRegistry(t *testing.T) {
	t.Parallel()
	if got := ResolveModelFullName("pi-cli", "sonnet"); got != "anthropic/claude-sonnet-4-6" {
		t.Fatalf("sonnet resolved to %q", got)
	}
	if got := ResolveModelFullName("pi-cli", "opus"); got != "anthropic/claude-opus-4-8" {
		t.Fatalf("opus resolved to %q", got)
	}
	if got := ResolveModelFullName("pi-cli", "fable"); got != "anthropic/claude-fable-5" {
		t.Fatalf("fable resolved to %q", got)
	}
	if got := ResolveModelFullName("pi-cli", "haiku"); got != "anthropic/claude-haiku-4-5" {
		t.Fatalf("haiku resolved to %q", got)
	}
	if got := ResolveModelFullName("pi-cli", "google/gemini-3.5-flash"); got != "google/gemini-3.5-flash" {
		t.Fatalf("provider-qualified model resolved to %q", got)
	}
	if got, ok := DefaultModelForHarness("pi-cli"); !ok || got != "sonnet" {
		t.Fatalf("default = %q, %v", got, ok)
	}
	if got, ok := TestModelForHarness("pi-cli"); !ok || got != "gemini-flash-lite" {
		t.Fatalf("test model = %q, %v", got, ok)
	}
	for _, family := range ModelFamilyNames() {
		if !slices.Contains(ModelFamiliesForHarness("pi-cli"), family) {
			t.Fatalf("Pi model families omit %q: %v", family, ModelFamiliesForHarness("pi-cli"))
		}
	}
}

func TestOpenCodeAggregatedModelAliasesAreDeterministicAndProviderQualified(t *testing.T) {
	t.Parallel()
	want := map[string]string{
		"glm-5.2":     "openrouter/z-ai/glm-5.2",
		"deepseek-v4": "openrouter/deepseek/deepseek-v4-pro",
		"nemotron":    "openrouter/nvidia/nemotron-3-ultra-550b-a55b",
		"qwen":        "openrouter/qwen/qwen3.6-max-preview",
	}
	for range 100 {
		for alias, fullName := range want {
			if got := ResolveModelFullName("opencode-cli", alias); got != fullName {
				t.Fatalf("OpenCode %s resolved to %q, want %q", alias, got, fullName)
			}
		}
	}
}
