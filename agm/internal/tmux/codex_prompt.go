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

// CodexUpdatePromptPatterns match Codex's "Update available!" interstitial,
// which owns input before the composer renders whenever a newer codex-cli has
// shipped. Captured from a live pane on codex-cli 0.145.0 with 0.146.0 out:
//
//	✨ Update available! 0.145.0 -> 0.146.0
//	› 1. Update now (runs `brew upgrade --cask codex`)
//	  2. Skip
//	  3. Skip until next version
//
// The highlighted default upgrades the CLI, which would mutate the operator's
// toolchain mid-dispatch, so AGM selects "Skip" instead. Every phrase is
// required so ordinary transcript text mentioning updates cannot match.
var CodexUpdatePromptPatterns = []string{
	"Update available!",
	"Update now",
	"Skip until next version",
}

// ErrCodexHookReviewRequired marks the security-sensitive Codex startup screen
// that requires an operator to inspect new or changed executable hooks.
var ErrCodexHookReviewRequired = errors.New("codex hooks require explicit review")

const codexHookReviewGuidance = "open Codex interactively in this directory, review every new or changed hook, and choose whether to trust the audited hooks or continue without them; AGM will not trust executable hooks automatically"

// IsCodexHookReviewRequired reports whether Codex's numbered hook-review
// selector or interactive hooks dashboard currently owns input. Requiring each
// surface's complete controls avoids interpreting ordinary transcript text
// about hooks as an active blocker. A newer tail-owned composer supersedes a
// retained selector or dashboard, while the dashboard may itself be redrawn
// below an older composer that remains in scrollback.
func IsCodexHookReviewRequired(content string) bool {
	trimmed := strings.TrimSpace(content)
	if trimmed == "" {
		return false
	}
	plain := stripANSI(trimmed)
	lower := strings.ToLower(plain)

	if dashboardStart := strings.LastIndex(lower, "lifecycle hooks from config and enabled plugins."); dashboardStart >= 0 {
		lowerDashboard := lower[dashboardStart:]
		const dashboardControls = "press t to trust all; enter to review hooks; esc to close"
		controls := strings.LastIndex(lowerDashboard, dashboardControls)
		if controls >= 0 &&
			strings.Contains(lowerDashboard[:controls], "hooks need review before they can run") &&
			!hasCodexPaneOwnership(styledContentAfterLastLineContaining(trimmed, dashboardControls)) {
			return true
		}
	}

	selectorStart := strings.LastIndex(lower, "hooks need review")
	if selectorStart < 0 {
		return false
	}
	lowerSelector := lower[selectorStart:]
	if !strings.Contains(lowerSelector, "review hooks") ||
		!strings.Contains(lowerSelector, "continue without trusting") ||
		!strings.Contains(lowerSelector, "press enter to confirm") {
		return false
	}

	// Codex retains prior TUI output in scrollback. Once a later composer owns
	// the pane tail or a submitted turn is working, the earlier review selector
	// is no longer active. Start after the selector's controls so its highlighted
	// choice cannot be mistaken for a Codex composer.
	return !hasCodexPaneOwnership(styledContentAfterLastLineContaining(trimmed, "press enter to confirm"))
}

// hasCodexPaneOwnership reports whether Codex rendered a newer composer or
// working footer after a completed hook-review surface. Ownership is distinct
// from readiness: an occupied composer and an in-flight turn both prove that a
// retained dashboard no longer owns input even though neither accepts delivery.
func hasCodexPaneOwnership(content string) bool {
	for line := range strings.SplitSeq(content, "\n") {
		plain := strings.TrimSpace(stripANSI(line))
		if isCodexComposerAnchor(plain) || codexFooterPattern.MatchString(plain) {
			return true
		}
	}
	return false
}

func styledContentAfterLastLineContaining(content, lowerNeedle string) string {
	lines := strings.Split(content, "\n")
	for i, line := range slices.Backward(lines) {
		if strings.Contains(strings.ToLower(stripANSI(line)), lowerNeedle) {
			return strings.Join(lines[i+1:], "\n")
		}
	}
	return ""
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
		if !codexFooterPattern.MatchString(strings.TrimSpace(stripANSI(line))) {
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
			candidate := strings.TrimSpace(stripANSI(lines[j]))
			if candidate == "" {
				continue
			}
			// Only an empty cursor is idle. Typed drafts and collapsed paste chips
			// use the same glyph but accepting them would append a second prompt to
			// input the user has not submitted yet. Codex 0.145 renders current
			// welcome suggestions as dim placeholder text after the cursor; retain
			// ANSI so that placeholder remains distinguishable from human input.
			return candidate == "›" ||
				strings.HasPrefix(candidate, "›") && isCodexGhostComposerLine(lines[j])
		}
		return false
	}

	// Before the first exchange Codex renders a bordered welcome composer. Both
	// the header, its model-change hint, and an empty cursor must be present in the
	// same compact block; either substring alone can occur in stale or echoed
	// output, while an occupied cursor is an unsubmitted draft.
	for i, line := range lines {
		if !strings.Contains(stripANSI(line), CodexPromptPatterns[0]) {
			continue
		}
		for j := i + 1; j < len(lines) && j <= i+4; j++ {
			if strings.Contains(stripANSI(lines[j]), CodexPromptPatterns[1]) && codexInitialComposerOwnsTail(lines[j+1:]) {
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
		candidate := strings.TrimSpace(stripANSI(line))
		switch {
		case candidate == "":
		case strings.HasPrefix(candidate, "│") && strings.HasSuffix(candidate, "│"):
		case strings.HasPrefix(candidate, "╰") && strings.HasSuffix(candidate, "╯"):
		case candidate == "›":
			emptyCursor = true
		case strings.HasPrefix(candidate, "›") && isCodexGhostComposerLine(line):
			emptyCursor = true
		case strings.HasPrefix(candidate, "›"):
			return false
		default:
			return false
		}
	}
	return emptyCursor
}

func isCodexGhostComposerLine(line string) bool {
	_, afterPrompt, found := strings.Cut(line, "›")
	if !found {
		return false
	}
	plainInput := strings.TrimSpace(stripANSI(afterPrompt))
	if plainInput == "" || hasQueuedInput(plainInput) {
		return false
	}

	dim, grey := false, false
	for afterPrompt != "" {
		afterPrompt = strings.TrimLeft(afterPrompt, " \t")
		match := sgrParamRe.FindStringSubmatchIndex(afterPrompt)
		if match == nil || match[0] != 0 {
			return dim || grey
		}
		params := ""
		if match[2] >= 0 {
			params = afterPrompt[match[2]:match[3]]
		}
		dim, grey = updateCodexGhostStyle(strings.Split(params, ";"), dim, grey)
		afterPrompt = afterPrompt[match[1]:]
	}
	return false
}

func updateCodexGhostStyle(params []string, dim, grey bool) (bool, bool) {
	for i := 0; i < len(params); i++ {
		switch params[i] {
		case "", "0":
			dim, grey = false, false
		case "2":
			dim = true
		case "22":
			dim = false
		case "90":
			grey = true
		case "30", "31", "32", "33", "34", "35", "36", "37",
			"39", "91", "92", "93", "94", "95", "96", "97":
			grey = false
		case "38":
			var skip int
			grey, skip = consumeExtendedColor(params, i)
			i += skip
		case "48":
			_, skip := consumeExtendedColor(params, i)
			i += skip
		}
	}
	return dim, grey
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
		"capture-pane", "-t", NormalizeTmuxSessionName(sessionName), "-p", "-e", "-J").Output()
	if err != nil {
		return false, fmt.Errorf("capture-pane failed: %w", err)
	}
	return IsCodexComposerReady(string(output)), nil
}

// containsCodexTrustPromptPattern reports whether content contains a Codex
// first-run trust / onboarding consent prompt.
func containsCodexTrustPromptPattern(content string) bool {
	return codexSelectorOwnsInput(content, CodexTrustPromptPatterns, false)
}

// containsCodexModelUpgradePromptPattern reports whether the Codex
// model-upgrade interstitial currently owns input. A composer or working
// footer below a retained selector supersedes it.
func containsCodexModelUpgradePromptPattern(content string) bool {
	return codexSelectorOwnsInput(content, CodexModelUpgradePromptPatterns, false)
}

// containsCodexUpdatePromptPattern reports whether the Codex update-available
// interstitial currently owns input. Every phrase must be present: a release
// banner or transcript line mentioning an update must not be mistaken for the
// selector, because answering it sends keystrokes into whatever owns the pane.
func containsCodexUpdatePromptPattern(content string) bool {
	return codexSelectorOwnsInput(content, CodexUpdatePromptPatterns, true)
}

// codexSelectorOwnsInput recognizes an actionable startup selector and rejects
// retained selector text once newer Codex UI owns the pane. Some selectors
// have multiple compatible phrasings and match any one; the update prompt
// requires its complete control set to avoid treating release text as
// interactive UI.
func codexSelectorOwnsInput(content string, patterns []string, requireAll bool) bool {
	trimmed := strings.TrimSpace(content)
	if trimmed == "" {
		return false
	}
	latestPattern := ""
	latestIndex := -1
	matched := 0
	for _, pattern := range patterns {
		index := strings.LastIndex(trimmed, pattern)
		if index < 0 {
			if requireAll {
				return false
			}
			continue
		}
		matched++
		if index > latestIndex {
			latestPattern = pattern
			latestIndex = index
		}
	}
	if matched == 0 {
		return false
	}
	if requireAll && matched != len(patterns) {
		return false
	}
	return !hasCodexPaneOwnership(styledContentAfterLastLineContaining(
		trimmed,
		strings.ToLower(latestPattern),
	))
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
	updateAnswered := false

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
		output, err := exec.CommandContext(ctx, "tmux", "-S", GetSocketPath(), "capture-pane", "-t", sessionName, "-p", "-e", "-J", "-S", "-10").Output()
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
		plainContent := stripANSI(content)

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
		if !trustAccepted && containsCodexTrustPromptPattern(plainContent) {
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

		if !modelUpgradeAnswered && containsCodexModelUpgradePromptPattern(plainContent) {
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

		// A newer codex-cli release blocks startup on an update selector whose
		// highlighted default would run a package upgrade. Decline it and keep
		// polling; the composer renders immediately afterwards.
		if !updateAnswered && containsCodexUpdatePromptPattern(plainContent) {
			debug.Log("⬇️  Codex update prompt detected (check #%d) — skipping update", checkCount)
			if err := SendKeys(sessionName, "Down"); err != nil {
				debug.Log("⚠️  Failed to move to Codex skip-update option: %v", err)
			} else if err := SendKeys(sessionName, "Enter"); err != nil {
				debug.Log("⚠️  Failed to confirm skipping Codex update: %v", err)
			} else {
				updateAnswered = true
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
