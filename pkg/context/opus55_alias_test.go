package context

import "testing"

// TestOpus55RegistryEntry guards a lookup that looks like it already works and
// does not. GetModel matches exactly, then retries with only case and
// underscore normalization, so the "claude-opus-5" row cannot be reached from
// "claude-opus-5-5" — and a miss is silent: GetModel falls back to the
// "default" row, handing the caller a plausible-looking 200K window instead of
// Opus 5.5's 1M and mis-reporting every context percentage and compaction
// threshold derived from it. The provider-qualified spelling that Pi and
// OpenRouter use needs its own row for the same reason. The registry is loaded
// from this package's own models.yaml so the test does not depend on a
// machine-local ~/.engram copy.
func TestOpus55RegistryEntry(t *testing.T) {
	registry, err := NewRegistry("models.yaml")
	if err != nil {
		t.Fatalf("load registry: %v", err)
	}

	canonical := registry.GetModel("claude-opus-5-5")
	if canonical == nil || canonical.MaxContextTokens != 1000000 {
		t.Fatalf("claude-opus-5-5 = %+v, want max_context_tokens 1000000", canonical)
	}

	fallback := registry.GetModel("definitely-not-a-registered-model")
	qualified := registry.GetModel("anthropic/claude-opus-5-5")
	if qualified == nil {
		t.Fatal("anthropic/claude-opus-5-5 resolved to nil")
	}
	if fallback != nil && qualified.ModelID == fallback.ModelID {
		t.Fatalf("anthropic/claude-opus-5-5 fell back to the default row (%q); the alias is missing", fallback.ModelID)
	}
	if qualified.MaxContextTokens != canonical.MaxContextTokens {
		t.Errorf("anthropic/claude-opus-5-5 window = %d, want the same as claude-opus-5-5 (%d)",
			qualified.MaxContextTokens, canonical.MaxContextTokens)
	}
}
