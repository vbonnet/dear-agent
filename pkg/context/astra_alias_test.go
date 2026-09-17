package context

import "testing"

// TestAstraProviderQualifiedAlias guards a lookup that is easy to assume works
// and does not. GetModel matches exactly, then retries with only case and
// underscore normalization, so it cannot reach the unqualified "gpt-6-astra"
// row from the provider-qualified "openai/gpt-6-astra" spelling that Pi and
// the generated OpenCode config use. A miss is silent: GetModel falls back to
// the "default" row, so the caller gets a plausible-looking 200K window
// instead of Astra's 272K and mis-reports context percentages and compaction
// thresholds. The registry is loaded from this package's own models.yaml so
// the test does not depend on a machine-local ~/.engram copy.
func TestAstraProviderQualifiedAlias(t *testing.T) {
	registry, err := NewRegistry("models.yaml")
	if err != nil {
		t.Fatalf("load registry: %v", err)
	}

	canonical := registry.GetModel("gpt-6-astra")
	if canonical == nil || canonical.MaxContextTokens != 272000 {
		t.Fatalf("gpt-6-astra = %+v, want max_context_tokens 272000", canonical)
	}

	fallback := registry.GetModel("definitely-not-a-registered-model")
	qualified := registry.GetModel("openai/gpt-6-astra")
	if qualified == nil {
		t.Fatal("openai/gpt-6-astra resolved to nil")
	}
	if fallback != nil && qualified.ModelID == fallback.ModelID {
		t.Fatalf("openai/gpt-6-astra fell back to the default row (%q); the alias is missing", fallback.ModelID)
	}
	if qualified.MaxContextTokens != canonical.MaxContextTokens {
		t.Errorf("openai/gpt-6-astra window = %d, want the same as gpt-6-astra (%d)",
			qualified.MaxContextTokens, canonical.MaxContextTokens)
	}
}
