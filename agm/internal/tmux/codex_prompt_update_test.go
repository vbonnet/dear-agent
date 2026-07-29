package tmux

import "testing"

// The update selector owns input before the composer renders, so AGM must
// recognize it exactly: missing it costs a full readiness timeout, while a
// false positive sends keystrokes into whatever actually owns the pane.
func TestContainsCodexUpdatePromptPattern(t *testing.T) {
	livePane := "  ✨ Update available! 0.145.0 -> 0.146.0\n" +
		"\n" +
		"  Release notes: https://github.com/openai/codex/releases/latest\n" +
		"\n" +
		"› 1. Update now (runs `brew upgrade --cask codex`)\n" +
		"  2. Skip\n" +
		"  3. Skip until next version\n" +
		"\n" +
		"  Press enter to continue"

	if !containsCodexUpdatePromptPattern(livePane) {
		t.Fatal("live Codex update selector was not detected")
	}

	for name, content := range map[string]string{
		"empty":            "",
		"post-skip banner": "✨ Update available! 0.145.0 -> 0.146.0\nRun brew upgrade --cask codex to update.",
		"transcript":       "the release notes mention Update now and Skip until next version",
		"composer":         "OpenAI Codex (v0.145.0)\nmodel: gpt-5.6 xhigh   /model to change\n›",
	} {
		if containsCodexUpdatePromptPattern(content) {
			t.Errorf("%s was misread as the update selector", name)
		}
	}
}
