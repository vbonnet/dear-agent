package session

import "testing"

// TestAstraContextWindow pins the context window to the value the provider
// catalog reports for gpt-6-astra (context_window 272000, max 872000). We wire
// the default rather than the max: the larger window is the long-context tier,
// which prices at $20/$75 per Mtok instead of $10/$50.
func TestAstraContextWindow(t *testing.T) {
	got, ok := piKnownDirectModelContextWindow("gpt-6-astra")
	if !ok {
		t.Fatal("gpt-6-astra missing from the direct-model context window table")
	}
	if got != 272000 {
		t.Errorf("gpt-6-astra context window = %d, want 272000", got)
	}
	route, ok := piKnownNativeModelContextWindow("openai/gpt-6-astra")
	if !ok || route != 272000 {
		t.Errorf("openai/gpt-6-astra context window = %d (ok=%v), want 272000", route, ok)
	}
}
