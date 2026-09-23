package session

import (
	"os"
	"testing"
)

// TestPiModelContextWindow_NewModels2026_09 covers the two models wired in
// 2026-09. Fable 5.1's 1M input limit is from the Anthropic Models API
// (max_input_tokens); Gemini 3.8 Flash's 1,048,576 is Google's published
// input token limit, matching the rest of the Gemini 3.x Flash line.
func TestPiModelContextWindow_NewModels2026_09(t *testing.T) {
	t.Setenv("PI_CODING_AGENT_DIR", t.TempDir())
	tests := map[string]int{
		"anthropic/claude-fable-5-1":            1000000,
		"openrouter/anthropic/claude-fable-5-1": 1000000,
		"google/gemini-3.8-flash":               1048576,
		"openrouter/google/gemini-3.8-flash":    1048576,
	}
	for model, want := range tests {
		if got := piModelContextWindow(model, os.Getenv("PI_CODING_AGENT_DIR")); got != want {
			t.Errorf("piModelContextWindow(%q) = %d, want %d", model, got, want)
		}
	}
}
