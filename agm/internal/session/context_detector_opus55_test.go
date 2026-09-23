package session

import "testing"

// TestOpus55ContextWindow covers the two Pi lookup paths. Both switch on exact
// model strings, so "claude-opus-5-5" misses every arm and returns (0, false),
// which the caller reads as "unknown model" and replaces with a conservative
// default. Opus 5.5 carries the same 1M window as Opus 5, so the detector must
// say so rather than silently shrinking the budget it reports.
func TestOpus55ContextWindow(t *testing.T) {
	direct, ok := piKnownDirectModelContextWindow("claude-opus-5-5")
	if !ok {
		t.Fatal("piKnownDirectModelContextWindow(claude-opus-5-5) reported unknown")
	}
	if direct != extendedContextWindowTokens {
		t.Errorf("direct window = %d, want %d", direct, extendedContextWindowTokens)
	}

	routed, ok := piKnownOpenRouterModelContextWindow("anthropic/claude-opus-5-5")
	if !ok {
		t.Fatal("piKnownOpenRouterModelContextWindow(anthropic/claude-opus-5-5) reported unknown")
	}
	if routed != direct {
		t.Errorf("OpenRouter window = %d, want the same as direct (%d)", routed, direct)
	}

	// The prefix map already resolves this via "claude-opus-5"; assert the
	// value so a future narrowing of that prefix cannot silently halve it.
	if got := getModelContextWindow("claude-opus-5-5"); got != extendedContextWindowTokens {
		t.Errorf("getModelContextWindow(claude-opus-5-5) = %d, want %d", got, extendedContextWindowTokens)
	}
}
