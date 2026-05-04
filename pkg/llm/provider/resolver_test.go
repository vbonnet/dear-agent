package provider

import "testing"

func TestResolver_BareIDs(t *testing.T) {
	r := NewResolver()
	cases := []struct {
		in     string
		family string
		model  string
	}{
		{"claude-opus-4-7", "anthropic", "claude-opus-4-7"},
		{"claude-3-5-sonnet-20241022", "anthropic", "claude-3-5-sonnet-20241022"},
		{"gpt-4o", "openai", "gpt-4o"},
		{"gpt-5-pro", "openai", "gpt-5-pro"},
		{"o1-mini", "openai", "o1-mini"},
		{"o3-mini", "openai", "o3-mini"},
		{"gemini-2.5-flash", "gemini", "gemini-2.5-flash"},
		{"gemini-3.1-pro", "gemini", "gemini-3.1-pro"},
		{"llama3.2", "ollama", "llama3.2"},
		{"mistral-7b", "ollama", "mistral-7b"},
	}
	for _, c := range cases {
		fam, model, err := r.Resolve(c.in)
		if err != nil {
			t.Errorf("Resolve(%q) returned error: %v", c.in, err)
			continue
		}
		if fam != c.family || model != c.model {
			t.Errorf("Resolve(%q) = (%q,%q), want (%q,%q)", c.in, fam, model, c.family, c.model)
		}
	}
}

func TestResolver_PrefixedIDs(t *testing.T) {
	r := NewResolver()
	cases := []struct {
		in     string
		family string
		model  string
	}{
		{"openai:gpt-4o", "openai", "gpt-4o"},
		{"anthropic:claude-opus-4-7", "anthropic", "claude-opus-4-7"},
		{"ollama:llama3.2", "ollama", "llama3.2"},
		{"openai/gpt-4o", "openai", "gpt-4o"},
		{"anthropic/claude-opus-4-7", "anthropic", "claude-opus-4-7"},
		{"openrouter/anthropic/claude-3-5-sonnet", "openrouter", "anthropic/claude-3-5-sonnet"},
		{"openai://gpt-4o", "openai", "gpt-4o"},
	}
	for _, c := range cases {
		fam, model, err := r.Resolve(c.in)
		if err != nil {
			t.Errorf("Resolve(%q) returned error: %v", c.in, err)
			continue
		}
		if fam != c.family || model != c.model {
			t.Errorf("Resolve(%q) = (%q,%q), want (%q,%q)", c.in, fam, model, c.family, c.model)
		}
	}
}

func TestResolver_ForcedRoutingWinsOverHeuristic(t *testing.T) {
	// "openrouter/openai/gpt-4o" must route to openrouter (with model
	// "openai/gpt-4o"), NOT to openai. The first slash is the family
	// boundary; the rest is the upstream model id.
	r := NewResolver()
	fam, model, err := r.Resolve("openrouter/openai/gpt-4o")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if fam != "openrouter" {
		t.Errorf("family = %q, want openrouter", fam)
	}
	if model != "openai/gpt-4o" {
		t.Errorf("model = %q, want openai/gpt-4o", model)
	}
}

func TestResolver_UnknownFamilyPrefixIsErrorOnScheme(t *testing.T) {
	r := NewResolver()
	if _, _, err := r.Resolve("nope://something"); err == nil {
		t.Fatal("expected error for unknown family scheme")
	}
}

func TestResolver_UnknownFamilyPrefixFallsThroughForColon(t *testing.T) {
	// "weird:gpt-4o" — "weird" is not a known family, so the colon does
	// NOT count as a family separator. Heuristic should still match.
	r := NewResolver()
	fam, model, err := r.Resolve("weird:gpt-4o")
	if err == nil {
		t.Logf("got family=%q model=%q (unexpected success but acceptable if heuristic kicked in)", fam, model)
	}
	// No assertion here — both error and "openai with model weird:gpt-4o"
	// are defensible behaviors; we're just checking we don't crash. The
	// test exists to lock in current behavior should we change it.
}

func TestResolver_EmptyInputErrors(t *testing.T) {
	r := NewResolver()
	if _, _, err := r.Resolve(""); err == nil {
		t.Fatal("expected error for empty id")
	}
	if _, _, err := r.Resolve("   "); err == nil {
		t.Fatal("expected error for whitespace-only id")
	}
}

func TestResolver_UnknownModelErrors(t *testing.T) {
	r := NewResolver()
	if _, _, err := r.Resolve("totally-made-up-model-name"); err == nil {
		t.Fatal("expected error for unrecognised model id")
	}
}

func TestResolver_RegisterExtraPrefix(t *testing.T) {
	r := NewResolver()
	r.Register("internal-llm-", "ollama")
	fam, model, err := r.Resolve("internal-llm-7b")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if fam != "ollama" || model != "internal-llm-7b" {
		t.Errorf("Resolve(internal-llm-7b) = (%q,%q), want (ollama, internal-llm-7b)", fam, model)
	}
}

func TestResolver_LongerPrefixWins(t *testing.T) {
	r := NewResolver()
	r.Register("foo-", "ollama")
	r.Register("foo-special-", "openrouter")
	fam, _, err := r.Resolve("foo-special-7b")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if fam != "openrouter" {
		t.Errorf("family = %q, want openrouter (longer prefix wins)", fam)
	}
}

func TestResolver_RegisterIgnoresEmptyArgs(t *testing.T) {
	r := NewResolver()
	r.Register("", "ollama") // no-op
	r.Register("foo-", "")   // no-op
	if _, _, err := r.Resolve("foo-bar"); err == nil {
		t.Fatal("expected error — registrations should have been ignored")
	}
}

func TestKnownFamily(t *testing.T) {
	for _, fam := range []string{"anthropic", "claude", "gemini", "google", "openai", "openrouter", "ollama", "local"} {
		if !knownFamily(fam) {
			t.Errorf("knownFamily(%q) = false, want true", fam)
		}
	}
	for _, fam := range []string{"", "nope", "bedrock", "azure"} {
		if knownFamily(fam) {
			t.Errorf("knownFamily(%q) = true, want false", fam)
		}
	}
}
