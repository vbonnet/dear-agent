package tmux

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"regexp"
	"slices"
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
//	›
//
// Decorative shell prompts can also use rounded box-drawing characters, so
// readiness must remain keyed to Codex-specific text instead of generic TUI
// chrome. If Codex changes its composer wording, refine this list against a live
// `codex` pane.
//
// The bare "›" input cursor is deliberately NOT a pattern here: it also marks
// the highlighted option of the trust dialog ("› 1. Yes, continue"), so keying
// on it would risk a false "ready" before the trust prompt is answered.
//
// Post-conversation state: after the first exchange, the bordered composer box
// scrolls off screen. Codex then shows an input cursor followed by a minimal
// footer: "gpt-X.Y quality · /path". The model name alone is not sufficient:
// it also appears in echoed launch commands and while Codex is working.
var CodexPromptPatterns = []string{
	"OpenAI Codex",     // composer box header — present once the TUI renders
	"/model to change", // composer status-line hint
}

var codexFooterPattern = regexp.MustCompile(`^gpt-\d[^\n]*\s·\s[^\n]+$`)

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

// CodexModelUpgradePromptPatterns match Codex's model-upgrade interstitial.
// When a caller explicitly requested a model, AGM should keep that model rather
// than accepting the highlighted "Try new model" option.
var CodexModelUpgradePromptPatterns = []string{
	"Choose how you'd like Codex to proceed",
	"Try new model",
	"Use existing model",
}

// ErrCodexHookReviewRequired marks the security-sensitive Codex startup screen
// that requires an operator to inspect new or changed executable hooks.
var ErrCodexHookReviewRequired = errors.New("codex hooks require explicit review")

const codexHookReviewGuidance = "open Codex interactively in this directory, review every new or changed hook, and choose whether to trust the audited hooks or continue without them; AGM will not trust executable hooks automatically"

// IsCodexHookReviewRequired reports whether content ends in Codex's structured
// hook-review selector. Requiring the title and both safe menu choices avoids
// interpreting ordinary transcript text about hooks as an active blocker. A
// newer tail-owned composer supersedes retained review text.
func IsCodexHookReviewRequired(content string) bool {
	trimmed := strings.TrimSpace(content)
	if trimmed == "" {
		return false
	}
	lower := strings.ToLower(trimmed)
	start := strings.LastIndex(lower, "hooks need review")
	if start < 0 {
		return false
	}
	review := trimmed[start:]
	lowerReview := lower[start:]
	if !strings.Contains(lowerReview, "review hooks") ||
		!strings.Contains(lowerReview, "continue without trusting") ||
		!strings.Contains(lowerReview, "press enter to confirm") {
		return false
	}

	// Codex retains prior TUI output in scrollback. Once a later composer owns
	// the pane tail, the earlier review selector is no longer active.
	return !IsCodexComposerReady(review)
}

// CodexHookReviewError returns the typed, actionable startup failure used by
// every Codex readiness path.
func CodexHookReviewError() error {
	return fmt.Errorf("%w: %s", ErrCodexHookReviewRequired, codexHookReviewGuidance)
}

// IsCodexComposerReady reports whether content contains a complete Codex
// composer-ready signal. It is the single owner of Codex visual readiness for
// tmux waits, generic delivery, and shared state classification.
func IsCodexComposerReady(content string) bool {
	trimmed := strings.TrimSpace(content)
	if trimmed == "" {
		return false
	}
	lines := strings.Split(trimmed, "\n")

	// A structured footer is the current post-turn status signal. Inspect the
	// last one first so stale ready footer text above a newer working footer
	// or shell output cannot produce a false-ready result.
	for i, line := range slices.Backward(lines) {
		if !codexFooterPattern.MatchString(strings.TrimSpace(line)) {
			continue
		}
		if i != len(lines)-1 {
			// The latest footer is stale, but a restarted Codex process may have
			// rendered a newer initial composer below it. Let the initial-composer
			// scan prove whether that newer structure owns the pane tail.
			break
		}
		// The footer is ready only when paired with the nearby composer cursor.
		// A working view has the same footer but a "Working" status line.
		for j := i - 1; j >= 0 && j >= i-3; j-- {
			candidate := strings.TrimSpace(lines[j])
			if candidate == "" {
				continue
			}
			// Only an empty cursor is idle. Typed drafts and collapsed paste chips
			// use the same glyph but accepting them would append a second prompt to
			// input the user has not submitted yet.
			return candidate == "›"
		}
		return false
	}

	// Before the first exchange Codex renders a bordered welcome composer. Both
	// the header, its model-change hint, and an empty cursor must be present in the
	// same compact block; either substring alone can occur in stale or echoed
	// output, while an occupied cursor is an unsubmitted draft.
	for i, line := range lines {
		if !strings.Contains(line, CodexPromptPatterns[0]) {
			continue
		}
		for j := i + 1; j < len(lines) && j <= i+4; j++ {
			if strings.Contains(lines[j], CodexPromptPatterns[1]) && codexInitialComposerOwnsTail(lines[j+1:]) {
				return true
			}
		}
	}
	return false
}

// codexInitialComposerOwnsTail rejects ordinary output rendered after the
// welcome composer. The hint may be followed by the remaining bordered rows,
// the bottom border, and an empty Codex input cursor, but not a draft, newer
// shell prompt, or process-exit message.
func codexInitialComposerOwnsTail(lines []string) bool {
	emptyCursor := false
	for _, line := range lines {
		candidate := strings.TrimSpace(line)
		switch {
		case candidate == "":
		case strings.HasPrefix(candidate, "│") && strings.HasSuffix(candidate, "│"):
		case strings.HasPrefix(candidate, "╰") && strings.HasSuffix(candidate, "╯"):
		case candidate == "›":
			emptyCursor = true
		case strings.HasPrefix(candidate, "›"):
			return false
		default:
			return false
		}
	}
	return emptyCursor
}

// IsCodexIdle reports whether the Codex TUI composer is currently visible in
// the pane, i.e. the session is idle and ready for input rather than processing
// a prompt.
//
// It is the Codex counterpart to Claude's idle-prompt detection used by the
// supervisor: a live `codex-cli` pane shows the bordered composer box (whose
// header reads "OpenAI Codex" with a "/model to change" hint) only when it is
// waiting for input. After a turn, the cursor plus structured footer replaces
// that welcome composer. While Codex is working, the footer remains but the
// cursor is replaced by a working-status line. Callers can therefore treat a
// true result as "idle/ready" and false as "working".
//
// The capture mirrors WaitForCodexPrompt: it reads the visible pane through the
// AGM-specific tmux socket. An error is returned only when the pane cannot be
// captured at all (e.g. the tmux session does not exist); callers that already
// know the session is alive can treat that as "not idle".
func IsCodexIdle(sessionName string) (bool, error) {
	output, err := exec.Command("tmux", "-S", GetSocketPath(),
		"capture-pane", "-t", NormalizeTmuxSessionName(sessionName), "-p").Output()
	if err != nil {
		return false, fmt.Errorf("capture-pane failed: %w", err)
	}
	return IsCodexComposerReady(string(output)), nil
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

// containsCodexModelUpgradePromptPattern reports whether content contains the
// Codex model-upgrade interstitial.
func containsCodexModelUpgradePromptPattern(content string) bool {
	trimmed := strings.TrimSpace(content)
	if trimmed == "" {
		return false
	}
	for _, pattern := range CodexModelUpgradePromptPatterns {
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
	return WaitForCodexPromptContext(context.Background(), sessionName, timeout)
}

// WaitForCodexPromptContext is the command-scoped variant of
// WaitForCodexPrompt. It stops polling immediately when the caller cancels.
//
//nolint:gocyclo // Readiness is a stateful polling protocol with trust and model-upgrade interstitials.
func WaitForCodexPromptContext(parent context.Context, sessionName string, timeout time.Duration) error {
	debug.Log("\n🔍 Starting Codex prompt detection for session: %s", sessionName)

	ctx, cancel := context.WithTimeout(parent, timeout)
	defer cancel()
	checkCount := 0
	trustAccepted := false
	modelUpgradeAnswered := false

	for {
		if err := ctx.Err(); err != nil {
			if errors.Is(err, context.DeadlineExceeded) {
				return fmt.Errorf("timeout waiting for Codex prompt (waited %v, performed %d checks)", timeout, checkCount)
			}
			return err
		}
		checkCount++

		// Capture the recent pane tail (10 lines into scrollback through the
		// visible region) so a trust dialog and the composer that follows are
		// both observable across consecutive checks.
		output, err := exec.CommandContext(ctx, "tmux", "-S", GetSocketPath(), "capture-pane", "-t", sessionName, "-p", "-S", "-10").Output()
		if err != nil {
			if ctx.Err() != nil {
				continue
			}
			// Session might not exist yet or not be accessible.
			if err := sleepWithContext(ctx, 500*time.Millisecond); err != nil {
				continue
			}
			continue
		}

		content := string(output)

		// Executable hooks can run outside Codex's sandbox after they are
		// trusted. This decision belongs to an operator who has inspected the
		// hook definitions; AGM must neither press the highlighted choice nor
		// wait until the generic readiness deadline obscures the cause.
		if IsCodexHookReviewRequired(content) {
			return CodexHookReviewError()
		}

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
			if err := sleepWithContext(ctx, time.Second); err != nil {
				continue
			}
			continue
		}

		if !modelUpgradeAnswered && containsCodexModelUpgradePromptPattern(content) {
			debug.Log("⬇️  Codex model upgrade prompt detected (check #%d) — selecting existing model", checkCount)
			if err := SendKeys(sessionName, "Down"); err != nil {
				debug.Log("⚠️  Failed to select existing Codex model: %v", err)
			} else if err := SendKeys(sessionName, "Enter"); err != nil {
				debug.Log("⚠️  Failed to confirm existing Codex model: %v", err)
			} else {
				modelUpgradeAnswered = true
			}
			if err := sleepWithContext(ctx, time.Second); err != nil {
				continue
			}
			continue
		}

		if IsCodexComposerReady(content) {
			debug.Log("✓ Codex composer detected (check #%d)", checkCount)
			// Found the composer — wait a beat to ensure it's stable.
			if err := sleepWithContext(ctx, 500*time.Millisecond); err != nil {
				return err
			}
			return nil
		}

		if checkCount%10 == 0 {
			debug.Log("⏳ Still waiting for Codex prompt... (check #%d)", checkCount)
		}

		if err := sleepWithContext(ctx, 500*time.Millisecond); err != nil {
			continue
		}
	}
}
