package tmux

import (
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/vbonnet/dear-agent/agm/internal/debug"
)

// CodexPromptPatterns indicate the Codex CLI TUI has finished booting and is
// showing its composer (i.e. it is ready for input).
//
// Captured empirically from a live `codex` pane (codex-cli v0.141.0, see
// DESIGN-agm-codex-harness §4.3). Once the composer renders, Codex draws a
// bordered input box whose header reads "OpenAI Codex (vX.Y.Z)" with a
// "/model to change" status line and a "›" (U+203A) input cursor:
//
//	╭───────────────────────────────────────╮
//	│ >_ OpenAI Codex (v0.141.0)            │
//	│ model:     gpt-5.5 xhigh   /model to change │
//	│ directory: /private/tmp/codex-probe   │
//	╰───────────────────────────────────────╯
//	› Write tests for @filename
//
// None of these primitives appear at a bash prompt or in the first-run trust
// dialog (which has no box), so they are reliable "the TUI is up" signals
// rather than coincidental matches against shell output. "OpenAI Codex" is the
// most specific and stable marker; the rounded box corners are kept as
// defensive fallbacks in case the header wording changes. If Codex changes its
// composer chrome, refine this list against a live `codex` pane.
//
// Note: the rounded box-drawing corners are shared with Gemini's UI. That
// overlap is harmless — both are genuine TUI-ready signals — but Codex gets its
// own pattern set (and its own WaitForCodexPrompt) so the detector is explicit
// about which harness it is matching and can grow Codex-specific signals.
//
// The bare "›" input cursor is deliberately NOT a pattern here: it also marks
// the highlighted option of the trust dialog ("› 1. Yes, continue"), so keying
// on it would risk a false "ready" before the trust prompt is answered.
var CodexPromptPatterns = []string{
	"OpenAI Codex",     // composer box header — present once the TUI renders
	"/model to change", // composer status-line hint
	"╭",                // top border of the Codex input box
	"╰",                // bottom border of the Codex input box
}

// CodexTrustPromptPatterns are substrings that indicate Codex is showing a
// first-run trust / onboarding consent prompt for the working directory,
// analogous to Claude's "Do you trust the files in this folder?" dialog.
//
// Captured empirically from a fresh-directory `codex` launch (v0.141.0):
//
//	> You are in /private/tmp/codex-probe
//	  Do you trust the contents of this directory? Working with untrusted
//	  contents comes with higher risk of prompt injection. ...
//	› 1. Yes, continue
//	  2. No, quit
//	  Press enter to continue
//
// The default highlighted option is "1. Yes, continue", so pressing Enter
// accepts it (verified against a live pane). Multiple phrasings are matched so
// detection survives minor wording changes across Codex versions.
var CodexTrustPromptPatterns = []string{
	"Do you trust the contents of this directory",
	"trust the contents of this directory",
}

// containsCodexPromptPattern reports whether content contains any Codex
// composer-ready signal.
func containsCodexPromptPattern(content string) bool {
	trimmed := strings.TrimSpace(content)
	if trimmed == "" {
		return false
	}
	for _, pattern := range CodexPromptPatterns {
		if strings.Contains(trimmed, pattern) {
			return true
		}
	}
	return false
}

// containsCodexTrustPromptPattern reports whether content contains a Codex
// first-run trust / onboarding consent prompt.
func containsCodexTrustPromptPattern(content string) bool {
	trimmed := strings.TrimSpace(content)
	if trimmed == "" {
		return false
	}
	for _, pattern := range CodexTrustPromptPatterns {
		if strings.Contains(trimmed, pattern) {
			return true
		}
	}
	return false
}

// WaitForCodexPrompt polls the pane until the Codex TUI shows its composer
// (ready for input), returning nil on success or a timeout error.
//
// It mirrors WaitForPromptSimple's capture-pane polling approach (control mode
// has proven unreliable here) but keys on Codex-specific composer signals and
// transparently auto-accepts a first-run trust/onboarding prompt if one appears
// — the equivalent of WaitForClaudePrompt's trust handling, folded into the
// readiness wait so prompt delivery never races a consent dialog. Without it,
// the trust prompt blocks the composer from rendering: the readiness wait would
// time out and the initial prompt would be typed into the trust selector
// instead of the composer.
//
// Trust auto-accept is best-effort: a failed keystroke just means we keep
// polling until the composer appears or the deadline elapses.
func WaitForCodexPrompt(sessionName string, timeout time.Duration) error {
	debug.Log("\n🔍 Starting Codex prompt detection for session: %s", sessionName)

	deadline := time.Now().Add(timeout)
	checkCount := 0
	trustAccepted := false

	for time.Now().Before(deadline) {
		checkCount++

		// Capture the recent pane tail (10 lines into scrollback through the
		// visible region) so a trust dialog and the composer that follows are
		// both observable across consecutive checks.
		output, err := exec.Command("tmux", "-S", GetSocketPath(), "capture-pane", "-t", sessionName, "-p", "-S", "-10").Output()
		if err != nil {
			// Session might not exist yet or not be accessible.
			time.Sleep(500 * time.Millisecond)
			continue
		}

		content := string(output)

		// A first-run trust prompt must be answered before the composer renders.
		// Auto-accept the default ("1. Yes, continue") with Enter, then keep
		// polling for the composer. Check this BEFORE the ready patterns: the
		// trust dialog has no input box, so it can never be mistaken for ready,
		// and answering it is what lets the box appear.
		if !trustAccepted && containsCodexTrustPromptPattern(content) {
			debug.Log("🛡️  Codex trust prompt detected (check #%d) — auto-answering with Enter", checkCount)
			if err := SendKeys(sessionName, "Enter"); err != nil {
				debug.Log("⚠️  Failed to answer Codex trust prompt: %v", err)
				// Don't give up; a later poll may still succeed.
			} else {
				trustAccepted = true
			}
			time.Sleep(1 * time.Second)
			continue
		}

		lines := strings.Split(content, "\n")
		for i, line := range lines {
			if containsCodexPromptPattern(line) {
				debug.Log("✓ Codex prompt detected in line %d (check #%d): %q", i, checkCount, strings.TrimSpace(line))
				// Found the composer — wait a beat to ensure it's stable.
				time.Sleep(500 * time.Millisecond)
				return nil
			}
		}

		if checkCount%10 == 0 {
			debug.Log("⏳ Still waiting for Codex prompt... (check #%d)", checkCount)
		}

		time.Sleep(500 * time.Millisecond)
	}

	return fmt.Errorf("timeout waiting for Codex prompt (waited %v, performed %d checks)", timeout, checkCount)
}
