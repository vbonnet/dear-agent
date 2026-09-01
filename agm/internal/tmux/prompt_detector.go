package tmux

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/vbonnet/dear-agent/agm/internal/debug"
)

// ClaudePromptPatterns are patterns that indicate Claude is ready for input
var ClaudePromptPatterns = []string{
	"❯",  // Claude Code primary prompt (Unicode U+276F)
	"▌",  // Claude cursor
	"> ", // Common prompt
	"$ ", // Shell-style prompt
	"# ", // Root prompt
}

// WaitForClaudePrompt waits for Claude prompt using capture-pane polling.
// This replaces the control-mode approach which only sees NEW output after attachment.
// capture-pane reads the pane's historical buffer, allowing us to detect prompts
// that appeared before we started monitoring.
//
// If a trust prompt ("Do you trust the files in this folder?") appears during the
// wait, this function auto-answers it by sending Enter (selecting the default
// "Yes, proceed" option) and continues waiting for the Claude prompt. This is
// critical when starting Claude in a sandbox where --add-dir does not pre-trust
// the workspace path.
func WaitForClaudePrompt(sessionName string, timeout time.Duration) error {
	return WaitForClaudePromptContext(context.Background(), sessionName, timeout)
}

// WaitForClaudePromptContext is the command-scoped variant of
// WaitForClaudePrompt. It stops readiness polling when the caller cancels.
//
//nolint:gocyclo // Readiness is a stateful polling protocol with trust and harness-liveness transitions.
func WaitForClaudePromptContext(parent context.Context, sessionName string, timeout time.Duration) error {
	debug.Log("\n🔍 Starting prompt detection for session: %s (using capture-pane polling)", sessionName)
	ctx, cancel := context.WithTimeout(parent, timeout)
	defer cancel()
	if err := ctx.Err(); err != nil {
		return err
	}

	// Find which socket the session is on
	socketPath := findSessionSocket(sessionName)

	pollInterval := 500 * time.Millisecond
	checksPerformed := 0
	lastLog := time.Now()
	trustAnswered := false
	var trustAnsweredAt time.Time

	// Fast-fail bookkeeping (ce-5zbg): if the harness process dies at startup
	// (e.g. Claude launched in a deleted cwd exits instantly with a Bun
	// ENOENT), the ❯ prompt can never appear — detect that and surface the
	// pane's actual output within seconds instead of burning the full timeout.
	sawHarness := false
	consecutiveShell := 0
	captureFailures := 0
	lastContent := ""

	for {
		if err := ctx.Err(); err != nil {
			if errors.Is(err, context.DeadlineExceeded) {
				return fmt.Errorf("timeout waiting for Claude prompt (waited %v, checked %d times)", timeout, checksPerformed)
			}
			return err
		}
		checksPerformed++

		// Log progress every 10 seconds
		if time.Since(lastLog) > 10*time.Second {
			debug.Log("⏳ Still waiting for prompt... (performed %d checks)", checksPerformed)
			lastLog = time.Now()
		}

		// Resolve and capture one exact pane with ANSI styling and wrapped rows
		// preserved. The shared probe is the single source of truth for both
		// trust-dialog ownership and Claude composer readiness.
		observation, err := probeClaudeInputContext(ctx, sessionName, !trustAnswered)
		if observation.content != "" {
			lastContent = observation.content
		}
		if err != nil {
			if ctx.Err() != nil {
				continue
			}
			debug.Log("⚠️  Claude input probe failed (attempt %d): %v", checksPerformed, err)
			// Repeated capture failures usually mean the session itself is
			// gone (harness exited and closed the pane) — check and fail fast
			// instead of silently polling out the whole timeout.
			captureFailures++
			if captureFailures >= sessionGoneChecks {
				if exists, hasErr := HasSession(sessionName); hasErr == nil && !exists {
					return fmt.Errorf("tmux session %q no longer exists while waiting for Claude prompt (harness exited during startup?); last pane output:\n%s",
						sessionName, paneTail(lastContent, 6))
				}
				captureFailures = 0
			}
			if err := sleepWithContext(ctx, pollInterval); err != nil {
				continue
			}
			continue
		}
		captureFailures = 0

		content := observation.content

		// Log a sample on first check to verify we're reading output
		if checksPerformed == 1 {
			lines := strings.Split(strings.TrimSpace(content), "\n")
			if len(lines) > 0 {
				lastLine := lines[len(lines)-1]
				debug.Log("📝 Sample output (last line): %q", truncate(lastLine, 100))
			}
		}

		// Require the live, tail-owning Claude composer. Trust selectors and
		// historical prompt glyphs are not readiness evidence, even after AGM has
		// sent Enter; trustAnswered controls only the short transition settle.
		if observation.probe.ComposerOwnsInput {
			if trustAnswered && time.Since(trustAnsweredAt) < 2*time.Second {
				// Trust prompt UI may still be on screen — ignore false match.
			} else {
				debug.Log("✓ Claude prompt detected after %d checks", checksPerformed)
				// Brief sleep to ensure prompt is stable
				if err := sleepWithContext(ctx, 500*time.Millisecond); err != nil {
					return err
				}
				return nil
			}
		}

		// Detect and auto-answer trust prompt inline.
		// Without this, the trust prompt blocks Claude's main UI from rendering,
		// so the ❯ prompt never appears and we time out. Only answer when the
		// selector currently OWNS input, so a historical answered selector still
		// in the capture, or the question before its options render, never causes
		// an Enter to land on whatever owns input now (ce-wn4qe).
		if !trustAnswered && observation.probe.DialogOwnsInput {
			debug.Log("🛡️  Trust dialog detected from exact-pane capture")
			if observation.probe.TrustAnswered {
				trustAnswered = true
				trustAnsweredAt = time.Now()
				debug.Log("✓ Trust prompt answered, continuing to wait for ❯")
				// Brief sleep so Claude can transition past the trust UI
				if err := sleepWithContext(ctx, time.Second); err != nil {
					continue
				}
				continue
			}
		}

		// Fast-fail when the harness process is not (or no longer) running in
		// the pane. The claude command was already sent, so the pane's
		// foreground process should exec into the harness almost immediately
		// and stay there; a shell in the foreground means the harness either
		// failed to start or died (ce-5zbg: instant Bun ENOENT in deleted cwd).
		if fg, ok := paneForegroundCommand(ctx, socketPath, sessionName); ok {
			if IsShellCommand(fg) {
				consecutiveShell++
				if sawHarness && consecutiveShell >= harnessExitedChecks {
					return fmt.Errorf("harness process exited before becoming ready (pane foreground returned to %q); last pane output:\n%s",
						fg, paneTail(content, 6))
				}
				if !sawHarness && consecutiveShell >= harnessNeverStartedChecks {
					return fmt.Errorf("harness process never started (pane foreground still %q after %d checks — the launch command likely failed immediately); last pane output:\n%s",
						fg, checksPerformed, paneTail(content, 6))
				}
			} else {
				sawHarness = true
				consecutiveShell = 0
			}
		}

		// Sleep before next poll
		if err := sleepWithContext(ctx, pollInterval); err != nil {
			continue
		}
	}
}

// Fast-fail thresholds for WaitForClaudePrompt, in units of poll iterations
// (500ms each). Package-level vars so tests can tighten them.
var (
	// harnessExitedChecks: consecutive shell-foreground polls after the
	// harness was seen running before we declare it dead (~1.5s).
	harnessExitedChecks = 3
	// harnessNeverStartedChecks: consecutive shell-foreground polls with the
	// harness never seen before we declare the launch failed (~15s — generous
	// so a slow-booting harness on a loaded machine is not misdiagnosed).
	harnessNeverStartedChecks = 30
	// sessionGoneChecks: consecutive capture-pane failures before we probe
	// whether the session still exists (~3s).
	sessionGoneChecks = 6
)

// paneForegroundCommand returns the name of the process currently in the
// foreground of the session's active pane (#{pane_current_command}). The
// boolean is false when the value could not be determined. The tmux call is
// timeout-bounded so a wedged server cannot stall the caller's poll loop.
func paneForegroundCommand(ctx context.Context, socketPath, sessionName string) (string, bool) {
	output, err := RunWithTimeout(ctx, globalTimeout, "tmux", "-S", socketPath,
		"display-message", "-p", "-t", sessionName, "#{pane_current_command}")
	if err != nil {
		return "", false
	}
	fg := strings.TrimSpace(string(output))
	if fg == "" {
		return "", false
	}
	return fg, true
}

// IsShellCommand reports whether a pane_current_command or process comm value is a plain
// interactive shell (as opposed to a harness CLI like claude/codex/gemini).
func IsShellCommand(name string) bool {
	name = strings.TrimPrefix(filepath.Base(name), "-") // login shells may report as "-zsh"
	switch name {
	case "zsh", "bash", "sh", "ash", "fish", "dash", "ksh", "tcsh", "csh":
		return true
	}
	return false
}

// paneTail returns the last n non-empty lines of captured pane content with
// ANSI escapes stripped, for inclusion in error messages.
func paneTail(content string, n int) string {
	lines := strings.Split(strings.TrimSpace(stripANSI(content)), "\n")
	var kept []string
	for _, line := range lines {
		if strings.TrimSpace(line) != "" {
			kept = append(kept, line)
		}
	}
	if len(kept) > n {
		kept = kept[len(kept)-n:]
	}
	if len(kept) == 0 {
		return "  (pane empty)"
	}
	return "  " + strings.Join(kept, "\n  ")
}

// WaitForClaudePromptControlMode is the old control-mode based implementation
//
// Deprecated: Control mode only sees NEW output after attachment, missing historical output.
// Preserved for reference but should not be used for session startup detection.
//
//nolint:gocyclo // reason: stateful tmux control-mode loop with many concurrent termination conditions; helpers would obscure the per-event flow.
func WaitForClaudePromptControlMode(sessionName string, timeout time.Duration) error {
	debug.Log("\n🔍 Starting prompt detection for session: %s (control mode - DEPRECATED)", sessionName)

	// Start control mode
	ctrl, err := StartControlMode(sessionName)
	if err != nil {
		return fmt.Errorf("failed to start control mode: %w", err)
	}
	defer ctrl.Close()

	// Create output watcher
	watcher := NewOutputWatcher(ctrl.Stdout)

	// Wait for prompt pattern
	deadline := time.Now().Add(timeout)
	consecutiveIdleLines := 0
	lastContent := ""
	linesChecked := 0

	lastLog := time.Now()

	for time.Now().Before(deadline) {
		// Log progress every 10 seconds for debugging hangs
		if time.Since(lastLog) > 10*time.Second {
			debug.Log("⏳ Still waiting for prompt... (checked %d lines, %d consecutive idles)", linesChecked, consecutiveIdleLines)
			lastLog = time.Now()
		}

		// Read next output line (short timeout per line - 200ms for faster detection)
		// Using ReadLine instead of GetRawLine to ensure timeout is enforced via goroutine + select
		line, err := watcher.ReadLine(200 * time.Millisecond)
		if err != nil {
			// Timeout on individual read - check if we've seen enough idle time
			consecutiveIdleLines++

			// If we've seen a prompt-like pattern and then idle, assume ready
			// Increased to 10 consecutive idles (2 seconds) to avoid false detection
			// during slash command execution where output might contain ">" characters
			if consecutiveIdleLines >= 10 && containsPromptPattern(lastContent) {
				debug.Log("✓ Detected prompt pattern after idle period: %q", lastContent)
				return nil
			}

			// If we've checked many lines and seen idle, likely ready
			// Increased to 15 consecutive idles (3 seconds) for more conservative detection
			if linesChecked > 10 && consecutiveIdleLines >= 15 {
				debug.Log("✓ Stable idle state detected after %d lines", linesChecked)
				return nil
			}

			continue
		}

		// Reset idle counter
		consecutiveIdleLines = 0
		linesChecked++

		// Extract content if it's an %output line
		if strings.HasPrefix(line, "%output") {
			content := ExtractOutputContent(line)
			lastContent = content

			// Log output for debugging (limit verbosity and filter escape sequences)
			if linesChecked <= 5 || linesChecked%10 == 0 {
				// Only log if content is meaningful (not just escape sequences)
				if isVisibleContent(content) {
					// Strip ANSI escape sequences before logging
					cleanContent := stripANSI(content)
					if strings.TrimSpace(cleanContent) != "" {
						debug.Log("📝 Output [%d]: %q", linesChecked, truncate(cleanContent, 80))
					}
				}
			}

			// Check for prompt patterns
			if containsPromptPattern(content) {
				debug.Log("✓ Prompt pattern detected in line %d: %q", linesChecked, content)
				// Wait a bit more to ensure it's stable (increased to 2s to avoid false positives)
				time.Sleep(2 * time.Second)
				return nil
			}
		}

		// Check for %end notification (command completed)
		if strings.HasPrefix(line, "%end") {
			debug.Log("📋 Command completion detected (%%end) at line %d", linesChecked)
			// Command finished, likely ready for input soon
			// Continue monitoring to confirm
		}
	}

	return fmt.Errorf("timeout waiting for Claude prompt (waited %v, checked %d lines)", timeout, linesChecked)
}

// containsClaudePromptPattern checks if content contains Claude's unique prompt pattern.
// This function is more strict than containsPromptPattern - it ONLY matches
// Claude Code's specific "❯" prompt, not generic shell prompts.
//
// This is used to avoid false positives when bash shell is visible before Claude starts.
// The bash prompt ("$", ">", "#") should NOT be detected as Claude being ready.
func containsClaudePromptPattern(content string) bool {
	trimmed := strings.TrimSpace(content)
	if trimmed == "" {
		return false
	}

	// Only check for Claude's specific prompt character (U+276F)
	// This excludes bash prompts like "$", ">", "#"
	return strings.Contains(trimmed, "❯")
}

func claudePromptEligible(content string) bool {
	return hasTailOwnedClaudeComposer(content) && !TrustDialogOwnsInput(content)
}

// Claude Code's trust/onboarding dialog wording has changed across releases.
// AGM must recognise every known variant, otherwise it never auto-answers the
// prompt: Claude sits on the dialog forever and harness-readiness detection
// times out with a misleading AGM-011 (ce-wn4qe). Keep these lists in sync with
// the shipping Claude Code TUI.
//
//	Legacy (≤ 2025): "Do you trust the files in this folder?"  / "Yes, proceed"
//	Current (2.x):   "Quick safety check: Is this a project you created or one
//	                  you trust?"                                / "Yes, I trust this folder"
var claudeTrustQuestionMarkers = []string{
	"Do you trust the files in this folder?",
	"Is this a project you created or one you trust",
}

// The affirmative selector patterns match only option 1. Both current and
// legacy wording require their adjacent trust question: either phrase can be
// typed into Claude's composer, and lexical row shape is not input ownership.
var (
	claudeTrustCurrentAffirmativeSelectorPattern = regexp.MustCompile(`(?mi)^[ \t]*❯[ \t]*1[.)][ \t]+Yes,[ \t]+I trust this folder[ \t\r]*$`)
	claudeTrustLegacyAffirmativeSelectorPattern  = regexp.MustCompile(`(?mi)^[ \t]*❯[ \t]*1[.)][ \t]+Yes,[ \t]+proceed[ \t\r]*$`)
)

// A selected negative row is only trust-dialog evidence when the immediately
// preceding option is the trust-specific affirmative choice. "No, exit" also
// appears in other interactive surfaces, so the negative row is not sufficient
// on its own.
var (
	claudeTrustCurrentAffirmativeOptionPattern = regexp.MustCompile(`(?mi)^[ \t]*1[.)][ \t]+Yes,[ \t]+I trust this folder[ \t\r]*$`)
	claudeTrustLegacyAffirmativeOptionPattern  = regexp.MustCompile(`(?mi)^[ \t]*1[.)][ \t]+Yes,[ \t]+proceed[ \t\r]*$`)
	claudeTrustNegativeSelectorPattern         = regexp.MustCompile(`(?mi)^[ \t]*❯[ \t]*2[.)][ \t]+No,[ \t]+(?:exit|quit)[ \t\r]*$`)
	claudeTrustNegativeOptionPattern           = regexp.MustCompile(`(?mi)^[ \t]*2[.)][ \t]+No,[ \t]+(?:exit|quit)[ \t\r]*$`)
	claudeTrustDialogHintPattern               = regexp.MustCompile(`(?i)^(?:Enter to confirm(?:[ \t]*·[ \t]*Esc to cancel)?|Esc to cancel|Use arrow keys(?: to navigate)?)[ \t\r]*$`)
)

// ContainsClaudeTrustPrompt reports lexical evidence of the Claude Code trust
// dialog in any known wording. An affirmative row qualifies only when its
// same-dialog question remains in the capture; a matching composer draft is
// not onboarding evidence.
func ContainsClaudeTrustPrompt(content string) bool {
	trimmed := strings.TrimSpace(content)
	if trimmed == "" {
		return false
	}
	for _, marker := range claudeTrustQuestionMarkers {
		if strings.Contains(trimmed, marker) {
			return true
		}
	}
	return ContainsClaudeTrustAffirmative(trimmed)
}

// ContainsClaudeTrustAffirmative reports lexical selected-row evidence in one
// output fragment. It can wake a fresh ownership probe, but never authorizes
// Enter by itself because control-mode events may be fragmented or historical.
func ContainsClaudeTrustAffirmative(content string) bool {
	lines := strings.Split(stripANSI(content), "\n")
	for i := range lines {
		if isClaudeTrustAffirmativeSelector(lines, i) {
			return true
		}
	}
	return false
}

const claudeTrustOwnershipCaptureLines = 50

// ClaudeInputProbe is the current exact-pane input ownership observed during a
// bounded startup probe. TrustAnswered means the probe itself sent raw Enter
// to the same pane it captured.
type ClaudeInputProbe struct {
	DialogOwnsInput   bool
	ComposerOwnsInput bool
	TrustAnswered     bool
}

type claudeInputProbeRuntime struct {
	resolve   func(context.Context, string) (activePaneTarget, bool, error)
	capture   func(context.Context, string) (string, error)
	liveness  func(context.Context, activePaneTarget) (PaneLiveness, error)
	sendEnter func(context.Context, string) error
	// sendKey delivers a named tmux key (only "Up"/"Down" here) to move the
	// trust dialog's selector. It is separate from sendEnter so the two
	// authorizations stay distinct: moving a selector is reversible, confirming
	// it is not.
	sendKey func(context.Context, string, string) error
}

type claudeInputObservation struct {
	probe   ClaudeInputProbe
	content string
}

// ProbeClaudeInputContext resolves one active pane, captures that exact target,
// and optionally answers a live affirmative trust selector on the same pane.
// Control-mode output is intentionally absent because events may be fragmented
// or historical; callers use each event only as a wake-up for this probe.
func ProbeClaudeInputContext(ctx context.Context, sessionName string, autoAnswerTrust bool) (ClaudeInputProbe, error) {
	observation, err := probeClaudeInputContext(ctx, sessionName, autoAnswerTrust)
	return observation.probe, err
}

func probeClaudeInputContext(ctx context.Context, sessionName string, autoAnswerTrust bool) (claudeInputObservation, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	runtime := claudeInputProbeRuntime{
		resolve: func(ctx context.Context, sessionName string) (activePaneTarget, bool, error) {
			return resolveActivePaneTarget(ctx, sessionName, GetSocketPath())
		},
		capture: func(ctx context.Context, targetPane string) (string, error) {
			return capturePaneTargetWithOptions(ctx, targetPane, claudeTrustOwnershipCaptureLines, true, true)
		},
		liveness: func(ctx context.Context, targetPane activePaneTarget) (PaneLiveness, error) {
			scanCtx, cancel := context.WithTimeout(ctx, livenessScanTimeout)
			defer cancel()
			return checkExpectedHarnessLivenessForPane(scanCtx, targetPane, "claude-code")
		},
		sendEnter: func(ctx context.Context, targetPane string) error {
			_, err := RunWithTimeout(ctx, globalTimeout, "tmux", "-S", GetSocketPath(), "send-keys", "-t", targetPane, "-H", "0d")
			return err
		},
		sendKey: func(ctx context.Context, targetPane, key string) error {
			_, err := RunWithTimeout(ctx, globalTimeout, "tmux", "-S", GetSocketPath(), "send-keys", "-t", targetPane, key)
			return err
		},
	}
	var observation claudeInputObservation
	err := withTmuxLockContext(ctx, func() error {
		var err error
		observation, err = observeClaudeInput(ctx, sessionName, autoAnswerTrust, runtime)
		return err
	})
	return observation, err
}

func probeClaudeInput(
	ctx context.Context,
	sessionName string,
	autoAnswerTrust bool,
	runtime claudeInputProbeRuntime,
) (ClaudeInputProbe, error) {
	observation, err := observeClaudeInput(ctx, sessionName, autoAnswerTrust, runtime)
	return observation.probe, err
}

func observeClaudeInput(
	ctx context.Context,
	sessionName string,
	autoAnswerTrust bool,
	runtime claudeInputProbeRuntime,
) (claudeInputObservation, error) {
	targetPane, exists, err := runtime.resolve(ctx, sessionName)
	if err != nil {
		return claudeInputObservation{}, fmt.Errorf("resolve current Claude pane: %w", err)
	}
	if !exists {
		return claudeInputObservation{}, fmt.Errorf("claude tmux session %q has no active pane", sessionName)
	}
	content, err := runtime.capture(ctx, targetPane.ID)
	if err != nil {
		return claudeInputObservation{}, fmt.Errorf("capture current Claude pane %q: %w", targetPane.ID, err)
	}
	observation, selection := classifyClaudeInputObservation(content)
	answerableTrust := selection == claudeTrustAffirmativeSelected || selection == claudeTrustNegativeSelected
	needsLiveClaude := observation.probe.ComposerOwnsInput || autoAnswerTrust && answerableTrust
	if !needsLiveClaude {
		return observation, nil
	}
	liveness, err := runtime.liveness(ctx, targetPane)
	if err != nil {
		return observation, fmt.Errorf("verify Claude liveness on pane %q: %w", targetPane.ID, err)
	}
	if !liveness.SessionExists || !liveness.HarnessAlive {
		return observation, fmt.Errorf("refuse Claude input ownership on pane %q without live Claude process evidence", targetPane.ID)
	}
	if observation.probe.ComposerOwnsInput {
		return observation, nil
	}

	// The liveness scan is intentionally before the final authority capture:
	// process-table round trips can take long enough for a human or the TUI to
	// move the selector. Re-capture and reclassify immediately before Enter.
	content, err = runtime.capture(ctx, targetPane.ID)
	if err != nil {
		return observation, fmt.Errorf("re-capture current Claude pane %q before trust answer: %w", targetPane.ID, err)
	}
	observation, selection = classifyClaudeInputObservation(content)

	// The dialog opens with "No, exit" selected, so observing it is not enough:
	// left alone it never resolves, and a blind Enter kills the harness. Move
	// the selector onto the affirmative option first, then re-verify before
	// confirming — the capture after the move is what authorizes Enter, never
	// the assumption that the move landed.
	if selection == claudeTrustNegativeSelected {
		key, needsMove := trustAffirmativeNavigationKey(content)
		if !needsMove || runtime.sendKey == nil {
			return observation, nil
		}
		if err := runtime.sendKey(ctx, targetPane.ID, key); err != nil {
			return observation, fmt.Errorf("move Claude trust selector on pane %q: %w", targetPane.ID, err)
		}
		content, err = runtime.capture(ctx, targetPane.ID)
		if err != nil {
			return observation, fmt.Errorf("re-capture current Claude pane %q after trust selector move: %w", targetPane.ID, err)
		}
		observation, selection = classifyClaudeInputObservation(content)
	}

	if selection != claudeTrustAffirmativeSelected {
		return observation, nil
	}
	if err := runtime.sendEnter(ctx, targetPane.ID); err != nil {
		return observation, fmt.Errorf("answer Claude trust selector on pane %q: %w", targetPane.ID, err)
	}
	observation.probe.TrustAnswered = true
	return observation, nil
}

func classifyClaudeInputObservation(content string) (claudeInputObservation, claudeTrustSelection) {
	selection := classifyTrustDialogOwnership(content)
	probe := ClaudeInputProbe{
		DialogOwnsInput:   selection != claudeTrustNotSelected,
		ComposerOwnsInput: selection == claudeTrustNotSelected && hasTailOwnedClaudeComposer(content),
	}
	return claudeInputObservation{probe: probe, content: content}, selection
}

// TrustSelectorOwnsInput reports whether the trust dialog's affirmative selector
// is the current tail-owning interactive element: the same-dialog question and
// numbered "Yes, ..." option are present and only trust-dialog chrome (the other
// option, the confirm/cancel hint, borders, blanks) follows it. This is the safe precondition for auto-
// answering the dialog — a historical, already-answered selector with newer
// content below it (a composer, a permission prompt, model output), or the
// question text before its options have rendered, does NOT own input and must
// not be answered (ce-wn4qe: otherwise the Enter could approve a permission
// selector or submit a user draft).
func TrustSelectorOwnsInput(content string) bool {
	return classifyTrustDialogOwnership(content) == claudeTrustAffirmativeSelected
}

// TrustDialogOwnsInput reports whether the current tail-owning interactive
// element is Claude's trust dialog, regardless of whether Yes or No is
// selected. This broader predicate blocks readiness and safety checks while
// the dialog owns input; only TrustSelectorOwnsInput may authorize Enter.
func TrustDialogOwnsInput(content string) bool {
	return classifyTrustDialogOwnership(content) != claudeTrustNotSelected
}

type claudeTrustSelection uint8

const (
	claudeTrustNotSelected claudeTrustSelection = iota
	claudeTrustIndeterminateSelected
	claudeTrustAffirmativeSelected
	claudeTrustNegativeSelected
)

func classifyTrustDialogOwnership(content string) claudeTrustSelection {
	// Normalize once before splitting. Control-mode captures preserve ANSI
	// styling, and repeatedly stripping each row is both slower and easier to
	// apply inconsistently across selector and chrome checks.
	lines := strings.Split(stripANSI(content), "\n")
	selectorIndex := -1
	selection := claudeTrustNotSelected
	for i, line := range lines {
		if isClaudeTrustAffirmativeSelector(lines, i) {
			selectorIndex = i
			selection = claudeTrustIndeterminateSelected
			if hasTrustNegativeOptionImmediatelyBelow(lines, i) {
				selection = claudeTrustAffirmativeSelected
			}
			continue
		}
		if claudeTrustNegativeSelectorPattern.MatchString(line) &&
			hasTrustAffirmativeOptionImmediatelyAbove(lines, i) {
			selectorIndex = i
			selection = claudeTrustNegativeSelected
		}
	}
	if selectorIndex < 0 {
		// The numbered, Yes-first patterns above match nothing against Claude
		// Code 2.1.234, which renders the options unnumbered and No-first. Fall
		// back to the tail-shaped option-block recognizer before the partial
		// heuristic, so a fully rendered current-layout dialog is classified
		// precisely rather than as merely indeterminate.
		if selection := classifyCurrentTrustOptionBlock(content); selection != claudeTrustNotSelected {
			return selection
		}
		if partialTrustDialogOwnsTail(content) {
			return claudeTrustIndeterminateSelected
		}
		return claudeTrustNotSelected
	}
	for _, line := range lines[selectorIndex+1:] {
		if !isClaudeTrustDialogChrome(line) {
			return claudeTrustNotSelected
		}
	}
	return selection
}

// partialTrustDialogOwnsTail catches redraws before Claude has rendered a
// complete numbered option. A known question or unselected affirmative row
// followed only by trust chrome and a bare cursor still owns input; treating
// that cursor as the main composer can submit text into onboarding.
func partialTrustDialogOwnsTail(content string) bool {
	styledLines := strings.Split(content, "\n")
	lines := strings.Split(stripANSI(content), "\n")
	anchor, anchorIsQuestion := latestClaudeTrustPartialAnchor(lines)
	if anchor < 0 {
		return false
	}
	lastPrompt := latestClaudePromptAfter(styledLines, lines, anchor)
	if lastPrompt < 0 {
		return partialTrustBodyOwnsTail(lines, anchor+1)
	}
	body := lines[anchor+1 : lastPrompt]
	if anchorIsQuestion && !validClaudeTrustPartialBody(body) ||
		!anchorIsQuestion && !onlyClaudeTrustDialogChrome(body) {
		return false
	}
	// Styled ghost text or a real status-footer line proves that the cursor
	// belongs to a newer main composer. A bare cursor without that structure
	// remains indeterminate when only a known trust-body prefix precedes it.
	return !HasGhostTextInANSI(styledLines[lastPrompt]) &&
		!hasDefinitiveClaudeComposerFooter(styledLines[lastPrompt+1:])
}

func latestClaudeTrustPartialAnchor(lines []string) (int, bool) {
	anchor := -1
	isQuestion := false
	for i, line := range lines {
		plain := strings.TrimSpace(line)
		if containsClaudeTrustQuestion(plain) {
			anchor = i
			isQuestion = true
			continue
		}
		if isClaudeTrustAffirmativeOption(lines, i) {
			anchor = i
			isQuestion = false
		}
	}
	return anchor, isQuestion
}

func latestClaudePromptAfter(styledLines, lines []string, anchor int) int {
	lastPrompt := -1
	for i := anchor + 1; i < len(lines); i++ {
		plain := strings.TrimSpace(lines[i])
		if plain == "❯" || strings.HasPrefix(plain, "❯") && HasGhostTextInANSI(styledLines[i]) {
			lastPrompt = i
		}
	}
	return lastPrompt
}

func partialTrustBodyOwnsTail(lines []string, start int) bool {
	for i := start; i < len(lines); i++ {
		plain := strings.TrimSpace(lines[i])
		if isClaudeTrustDialogChrome(plain) || isClaudeTrustAffirmativeOption(lines, i) {
			continue
		}
		return false
	}
	return true
}

func onlyClaudeTrustDialogChrome(lines []string) bool {
	for _, line := range lines {
		if !isClaudeTrustDialogChrome(line) {
			return false
		}
	}
	return true
}

func hasDefinitiveClaudeComposerFooter(lines []string) bool {
	for _, line := range lines {
		plain := strings.TrimSpace(stripANSI(line))
		if plain == "" || strings.Trim(plain, "─━┄┈╌╍═│┃┆┊╎╏┌┐└┘├┤┬┴┼╭╮╰╯ ") == "" {
			continue
		}
		if isClaudeComposerFooterChrome(line) {
			return true
		}
	}
	return false
}

func containsClaudeTrustQuestion(line string) bool {
	for _, marker := range claudeTrustQuestionMarkers {
		if strings.Contains(line, marker) {
			return true
		}
	}
	return false
}

func hasTrustAffirmativeOptionImmediatelyAbove(lines []string, selectorIndex int) bool {
	for i := selectorIndex - 1; i >= 0; i-- {
		if strings.TrimSpace(lines[i]) == "" {
			continue
		}
		return isClaudeTrustAffirmativeOption(lines, i)
	}
	return false
}

func isClaudeTrustAffirmativeSelector(lines []string, index int) bool {
	if index < 0 || index >= len(lines) {
		return false
	}
	if claudeTrustCurrentAffirmativeSelectorPattern.MatchString(lines[index]) {
		return hasTrustQuestionImmediatelyBefore(lines, index, claudeTrustQuestionMarkers[1])
	}
	return claudeTrustLegacyAffirmativeSelectorPattern.MatchString(lines[index]) &&
		hasTrustQuestionImmediatelyBefore(lines, index, claudeTrustQuestionMarkers[0])
}

func isClaudeTrustAffirmativeOption(lines []string, index int) bool {
	if index < 0 || index >= len(lines) {
		return false
	}
	if claudeTrustCurrentAffirmativeOptionPattern.MatchString(lines[index]) {
		return hasTrustQuestionImmediatelyBefore(lines, index, claudeTrustQuestionMarkers[1])
	}
	return claudeTrustLegacyAffirmativeOptionPattern.MatchString(lines[index]) &&
		hasTrustQuestionImmediatelyBefore(lines, index, claudeTrustQuestionMarkers[0])
}

func hasTrustQuestionImmediatelyBefore(lines []string, optionIndex int, marker string) bool {
	for i := optionIndex - 1; i >= 0; i-- {
		plain := strings.TrimSpace(lines[i])
		if strings.Contains(plain, marker) {
			return validClaudeTrustDialogBody(lines[i+1 : optionIndex])
		}
	}
	return false
}

func validClaudeTrustDialogBody(lines []string) bool {
	body := normalizedClaudeTrustBody(lines)
	if len(body) == 0 {
		return true
	}
	if !strings.Contains(strings.ToLower(body[0]), "this folder pre-approves") {
		return false
	}

	index := 1
	summaryRows := 0
	for index < len(body) && validClaudeTrustPermissionSummary(body[index]) {
		summaryRows++
		index++
	}
	if summaryRows == 0 {
		return false
	}

	sawApplyWarning := false
	sawTrustWarning := false
	for index < len(body) {
		lower := strings.ToLower(body[index])
		if lower == "security guide" {
			return sawApplyWarning && sawTrustWarning && index == len(body)-1
		}
		applyWarning := strings.Contains(lower, "these will apply without asking")
		trustWarning := strings.Contains(lower, "only proceed if you trust this configuration")
		if !applyWarning && !trustWarning {
			return false
		}
		sawApplyWarning = sawApplyWarning || applyWarning
		sawTrustWarning = sawTrustWarning || trustWarning
		index++
	}
	return sawApplyWarning && sawTrustWarning
}

func validClaudeTrustPartialBody(lines []string) bool {
	body := normalizedClaudeTrustBody(lines)
	if len(body) == 0 {
		return true
	}
	if !strings.Contains(strings.ToLower(body[0]), "this folder pre-approves") {
		return false
	}
	consequenceStarted := false
	for index, line := range body[1:] {
		lower := strings.ToLower(line)
		if !consequenceStarted && validClaudeTrustPermissionSummary(line) {
			continue
		}
		if strings.Contains(lower, "these will apply without asking") ||
			strings.Contains(lower, "only proceed if you trust this configuration") {
			consequenceStarted = true
			continue
		}
		if consequenceStarted && lower == "security guide" && index == len(body)-2 {
			continue
		}
		return false
	}
	return true
}

func normalizedClaudeTrustBody(lines []string) []string {
	body := make([]string, 0, len(lines))
	for _, line := range lines {
		plain := strings.TrimSpace(line)
		if plain == "" || strings.Trim(plain, "─━┄┈╌╍═│┃┆┊╎╏┌┐└┘├┤┬┴┼╭╮╰╯ ") == "" {
			continue
		}
		body = append(body, plain)
	}
	return body
}

func validClaudeTrustPermissionSummary(line string) bool {
	line = strings.TrimSpace(line)
	if remainder, ok := strings.CutPrefix(line, "- "); ok {
		line = strings.TrimSpace(remainder)
	} else if remainder, ok := strings.CutPrefix(line, "• "); ok {
		line = strings.TrimSpace(remainder)
	}
	if line == "" || len(line) > 4096 {
		return false
	}
	rules, ok := splitClaudeTrustPermissionRules(line)
	if !ok || len(rules) == 0 {
		return false
	}
	for _, rule := range rules {
		if !validClaudeTrustPermissionRule(rule) {
			return false
		}
	}
	return true
}

func splitClaudeTrustPermissionRules(line string) ([]string, bool) {
	const maxNesting = 8
	rules := make([]string, 0, 4)
	start := 0
	depth := 0
	for i, char := range line {
		switch char {
		case '(':
			depth++
			if depth > maxNesting {
				return nil, false
			}
		case ')':
			depth--
			if depth < 0 {
				return nil, false
			}
		case ',':
			if depth == 0 {
				rules = append(rules, strings.TrimSpace(line[start:i]))
				start = i + 1
			}
		case '\r', '\n':
			return nil, false
		}
	}
	if depth != 0 {
		return nil, false
	}
	return append(rules, strings.TrimSpace(line[start:])), true
}

func validClaudeTrustPermissionRule(rule string) bool {
	if rule == "" || strings.TrimSpace(rule) != rule {
		return false
	}
	name := rule
	if open := strings.IndexByte(rule, '('); open >= 0 {
		if !strings.HasSuffix(rule, ")") || open == 0 || open == len(rule)-1 {
			return false
		}
		name = rule[:open]
	}
	if strings.HasPrefix(name, "mcp__") {
		return validClaudePermissionName(name, true)
	}
	return name[0] >= 'A' && name[0] <= 'Z' && validClaudePermissionName(name, false)
}

func validClaudePermissionName(name string, allowWildcard bool) bool {
	for _, char := range name {
		if char >= 'A' && char <= 'Z' || char >= 'a' && char <= 'z' || char >= '0' && char <= '9' || char == '_' || char == '-' {
			continue
		}
		if allowWildcard && (char == '*' || char == '?') {
			continue
		}
		return false
	}
	return name != ""
}

func hasTrustNegativeOptionImmediatelyBelow(lines []string, selectorIndex int) bool {
	for i := selectorIndex + 1; i < len(lines); i++ {
		if strings.TrimSpace(lines[i]) == "" {
			continue
		}
		return claudeTrustNegativeOptionPattern.MatchString(lines[i])
	}
	return false
}

// isClaudeTrustDialogChrome reports whether a normalized line below the
// selected option is part of the trust dialog itself (the reject option, the
// confirm/cancel hint, a box border, or blank) rather than newer content that
// has taken over input.
func isClaudeTrustDialogChrome(line string) bool {
	plain := strings.TrimSpace(line)
	if plain == "" {
		return true
	}
	if strings.Trim(plain, "─━┄┈╌╍═│┃┆┊╎╏┌┐└┘├┤┬┴┼╭╮╰╯ ") == "" {
		return true
	}
	return claudeTrustNegativeOptionPattern.MatchString(plain) ||
		claudeTrustDialogHintPattern.MatchString(plain)
}

// containsTrustPromptPattern checks if content contains the Claude Code trust
// prompt in any known wording. It is used during readiness detection and
// InitSequence to auto-answer trust prompts that appear during session
// creation (e.g., after /rename or /agm:agm-assoc commands).
func containsTrustPromptPattern(content string) bool {
	return ContainsClaudeTrustPrompt(content)
}

// containsPromptPattern is deprecated. Use containsClaudePromptPattern instead.
// This function matches bash prompts ("$", ">", "#") which causes false positives
// when bash shell is visible before Claude starts.
// Preserved for backward compatibility with WaitForClaudePrompt (control mode function).
func containsPromptPattern(content string) bool {
	// Trim whitespace for comparison
	trimmed := strings.TrimSpace(content)

	// Empty content is not a prompt
	if trimmed == "" {
		return false
	}

	// Check against known patterns
	for _, pattern := range ClaudePromptPatterns {
		if strings.Contains(trimmed, pattern) {
			return true
		}
	}

	// Check if ends with common prompt characters
	if strings.HasSuffix(trimmed, ">") ||
		strings.HasSuffix(trimmed, "$") ||
		strings.HasSuffix(trimmed, "#") {
		return true
	}

	return false
}

// WaitForPromptSimple waits for any supported harness prompt using simple capture-pane approach.
// This is a simplified version that doesn't use control mode (which has issues).
// It periodically captures the pane content and checks for prompt patterns.
// Detects both Claude (❯) and Gemini (">   Type your message") prompts.
func WaitForPromptSimple(sessionName string, timeout time.Duration) error {
	return WaitForPromptSimpleContext(context.Background(), sessionName, timeout)
}

// WaitForPromptSimpleContext is the command-scoped variant of
// WaitForPromptSimple. It stops polling when the caller cancels.
func WaitForPromptSimpleContext(parent context.Context, sessionName string, timeout time.Duration) error {
	return WaitForPromptSimpleForHarnessContext(parent, sessionName, timeout, "")
}

// WaitForPromptSimpleForHarnessContext scopes harness-specific blockers while
// retaining the shared composer polling used by generic delivery.
//
//nolint:gocyclo // Stateful readiness keeps capture, liveness, harness-specific blockers, and cancellation in one polling protocol.
func WaitForPromptSimpleForHarnessContext(parent context.Context, sessionName string, timeout time.Duration, expectedHarness string) error {
	debug.Log("\n🔍 Starting simple prompt detection for session: %s", sessionName)

	ctx, cancel := context.WithTimeout(parent, timeout)
	defer cancel()
	checkCount := 0

	for {
		if err := ctx.Err(); err != nil {
			if errors.Is(err, context.DeadlineExceeded) {
				return fmt.Errorf("timeout waiting for harness prompt (waited %v, performed %d checks)", timeout, checkCount)
			}
			return err
		}
		checkCount++

		// Capture enough of the visible pane to include multi-line TUI
		// composers. Codex's stable readiness marker ("OpenAI Codex") can sit
		// well above the footer line after previous prompts/responses.
		cmdCtx, cmdCancel := context.WithTimeout(ctx, 5*time.Second)
		output, err := exec.CommandContext(cmdCtx, "tmux", "-S", GetSocketPath(), "capture-pane", "-t", sessionName, "-p", "-e", "-J", "-S", "-30").Output()
		cmdErr := cmdCtx.Err()
		cmdCancel()
		if cmdErr != nil {
			if ctx.Err() != nil {
				continue
			}
			return fmt.Errorf("tmux capture-pane timed out while waiting for prompt: %w", cmdErr)
		}
		if err != nil {
			// Session might not exist or not accessible
			if err := sleepWithContext(ctx, 500*time.Millisecond); err != nil {
				continue
			}
			continue
		}

		styledContent := string(output)
		if expectedHarness == "codex-cli" && IsCodexHookReviewRequired(styledContent) {
			return CodexHookReviewError()
		}
		if IsCodexComposerReady(styledContent) {
			debug.Log("✓ Codex composer detected (check #%d)", checkCount)
			if err := sleepWithContext(ctx, 500*time.Millisecond); err != nil {
				return err
			}
			return nil
		}
		content := stripANSI(styledContent)
		if containsPiReadyPattern(content) {
			debug.Log("✓ Managed Pi prompt detected (check #%d)", checkCount)
			if err := sleepWithContext(ctx, 500*time.Millisecond); err != nil {
				return err
			}
			return nil
		}
		// Codex readiness is a multi-line contract: the initial header must be
		// paired with its hint, and the post-turn cursor with its footer. Evaluate
		// the styled pane before stripping terminal controls for legacy
		// line-oriented harness checks.
		lines := strings.Split(content, "\n")

		// Check each line for any harness prompt pattern (Claude or Gemini)
		for i, line := range lines {
			if containsAnyNonPiHarnessPromptPattern(line) {
				debug.Log("✓ Harness prompt detected in line %d (check #%d): %q", i, checkCount, strings.TrimSpace(line))
				// Found prompt - wait a bit to ensure it's stable
				if err := sleepWithContext(ctx, 500*time.Millisecond); err != nil {
					return err
				}
				return nil
			}
		}

		// Log progress every 10 checks (5 seconds)
		if checkCount%10 == 0 {
			debug.Log("⏳ Still waiting for prompt... (check #%d)", checkCount)
		}

		// Wait before next check
		if err := sleepWithContext(ctx, 500*time.Millisecond); err != nil {
			continue
		}
	}
}

// ResumeFailurePatterns are substrings that indicate `claude --resume` failed
// fatally and will never reach a prompt. The canonical example is a UUID that
// has no matching conversation in the current project directory. Detecting
// these lets resume abort fast with an actionable error instead of waiting out
// the full prompt timeout and then attaching to a dead pane.
//
// Keep these in sync with the patterns classified in
// internal/validate/validator.go (classifyResumeError).
var ResumeFailurePatterns = []string{
	"No conversation found",
	"No messages returned",
}

// ResumeFailureError signals that a fatal `claude --resume` error string was
// detected in the pane before any harness prompt appeared. Detail holds the
// offending line so callers can surface it to the user.
type ResumeFailureError struct {
	Detail string
}

func (e *ResumeFailureError) Error() string {
	return fmt.Sprintf("claude resume failed: %s", e.Detail)
}

// containsResumeFailurePattern reports whether line contains a known fatal
// resume-failure substring, returning the matched substring for context.
func containsResumeFailurePattern(line string) (string, bool) {
	for _, pattern := range ResumeFailurePatterns {
		if strings.Contains(line, pattern) {
			return pattern, true
		}
	}
	return "", false
}

// WaitForPromptOrResumeFailure polls the pane for either a harness prompt
// (success, returns nil) or a known fatal resume-failure pattern (returns a
// *ResumeFailureError). It otherwise behaves like WaitForPromptSimple,
// returning a timeout error if neither appears before the deadline.
//
// This is the resume-aware variant of WaitForPromptSimple: when
// `claude --resume <uuid>` cannot find the conversation, it prints
// "No conversation found ..." and the shell prompt returns, so the harness
// prompt never renders. Without this check the caller would block for the
// full timeout and then attach to a broken pane.
func WaitForPromptOrResumeFailure(sessionName string, timeout time.Duration) error {
	return WaitForPromptOrResumeFailureContext(context.Background(), sessionName, timeout)
}

// WaitForPromptOrResumeFailureContext is the command-scoped variant of
// WaitForPromptOrResumeFailure. It stops polling when the caller cancels.
func WaitForPromptOrResumeFailureContext(parent context.Context, sessionName string, timeout time.Duration) error {
	return WaitForPromptOrResumeFailureForHarnessContext(parent, sessionName, timeout, "")
}

// WaitForPromptOrResumeFailureForHarnessContext scopes harness-specific
// blockers while retaining generic fatal-resume and composer detection.
//
//nolint:gocyclo // Stateful resume readiness keeps fatal output, harness blockers, composer detection, and cancellation in one polling protocol.
func WaitForPromptOrResumeFailureForHarnessContext(parent context.Context, sessionName string, timeout time.Duration, expectedHarness string) error {
	debug.Log("\n🔍 Starting resume-aware prompt detection for session: %s", sessionName)

	ctx, cancel := context.WithTimeout(parent, timeout)
	defer cancel()
	checkCount := 0

	for {
		if err := ctx.Err(); err != nil {
			if errors.Is(err, context.DeadlineExceeded) {
				return fmt.Errorf("timeout waiting for harness prompt (waited %v, performed %d checks)", timeout, checkCount)
			}
			return err
		}
		checkCount++

		// Capture the recent pane tail (from 10 lines into scrollback through
		// the visible region) so a multi-line failure message - the error plus
		// the returned shell prompt - is visible in a single check.
		output, err := exec.CommandContext(ctx, "tmux", "-S", GetSocketPath(), "capture-pane", "-t", sessionName, "-p", "-e", "-J", "-S", "-10").Output()
		if err != nil {
			if err := sleepWithContext(ctx, 500*time.Millisecond); err != nil {
				continue
			}
			continue
		}

		styledContent := string(output)
		content := stripANSI(styledContent)
		lines := strings.Split(content, "\n")

		// Check for a fatal resume failure first - it is the more specific
		// signal and a returned shell prompt could otherwise look like success.
		for _, line := range lines {
			if _, ok := containsResumeFailurePattern(line); ok {
				debug.Log("✗ Resume failure detected (check #%d): %q", checkCount, strings.TrimSpace(line))
				return &ResumeFailureError{Detail: strings.TrimSpace(line)}
			}
		}

		if expectedHarness == "codex-cli" && IsCodexHookReviewRequired(styledContent) {
			return CodexHookReviewError()
		}
		if IsCodexComposerReady(styledContent) {
			debug.Log("✓ Codex composer detected (check #%d)", checkCount)
			if err := sleepWithContext(ctx, 500*time.Millisecond); err != nil {
				return err
			}
			return nil
		}

		for i, line := range lines {
			if containsAnyHarnessPromptPattern(line) {
				debug.Log("✓ Harness prompt detected in line %d (check #%d): %q", i, checkCount, strings.TrimSpace(line))
				if err := sleepWithContext(ctx, 500*time.Millisecond); err != nil {
					return err
				}
				return nil
			}
		}

		if checkCount%10 == 0 {
			debug.Log("⏳ Still waiting for prompt... (check #%d)", checkCount)
		}

		if err := sleepWithContext(ctx, 500*time.Millisecond); err != nil {
			continue
		}
	}
}

// WaitForClaudeReady waits for Claude's current exact pane to expose the main
// composer, handling a live affirmative trust selector first when needed.
// Pane capture is authoritative: control-mode events can be historical,
// fragmented, or lost while a reader times out.
func WaitForClaudeReady(sessionName string, timeout time.Duration) error {
	debug.Log("🔍 Waiting for Claude to be ready (session: %s)", sessionName)
	return waitForClaudeReadyWithProbe(
		context.Background(), sessionName, timeout, 200*time.Millisecond, 2*time.Second, probeClaudeInputContext,
	)
}

func waitForClaudeReadyWithProbe(
	parent context.Context,
	sessionName string,
	timeout, pollInterval, trustSettle time.Duration,
	probeInput func(context.Context, string, bool) (claudeInputObservation, error),
) error {
	ctx, cancel := context.WithTimeout(parent, timeout)
	defer cancel()
	if pollInterval <= 0 {
		pollInterval = 200 * time.Millisecond
	}

	trustPromptAnswered := false
	checks := 0
	var trustAnsweredAt time.Time
	for {
		if err := ctx.Err(); err != nil {
			if errors.Is(err, context.DeadlineExceeded) {
				return fmt.Errorf("timeout waiting for Claude to be ready (waited %v, checked %d times)", timeout, checks)
			}
			return err
		}
		checks++
		observation, probeErr := probeInput(ctx, sessionName, !trustPromptAnswered)
		if probeErr != nil {
			return probeErr
		}
		if observation.probe.TrustAnswered {
			trustPromptAnswered = true
			trustAnsweredAt = time.Now()
			debug.Log("✓ Trust prompt answer sent to captured pane; waiting for Claude composer")
		}
		if observation.probe.ComposerOwnsInput &&
			(!trustPromptAnswered || time.Since(trustAnsweredAt) >= trustSettle) {
			debug.Log("✓ Claude composer owns current pane input")
			if err := sleepWithContext(ctx, 500*time.Millisecond); err != nil {
				return err
			}
			return nil
		}
		if err := sleepWithContext(ctx, pollInterval); err != nil {
			continue
		}
	}
}

// truncate truncates a string to maxLen characters with "..." suffix
func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

// isVisibleContent returns true if content contains visible characters
// (not just ANSI escape sequences)
func isVisibleContent(s string) bool {
	// Empty or whitespace-only strings are not visible
	trimmed := strings.TrimSpace(s)
	if trimmed == "" {
		return false
	}

	// If content is mostly escape sequences, don't consider it visible
	// Escape sequences typically start with \x1b or \033
	if strings.HasPrefix(trimmed, "\x1b") {
		// Check if there's any non-escape content
		// Simple heuristic: if more than 50% is escape codes, skip it
		escapeCount := strings.Count(trimmed, "\x1b")
		if escapeCount*4 > len(trimmed) { // Escape sequences are typically 4+ chars
			return false
		}
	}

	return true
}

// stripANSI removes ANSI escape sequences from a string
func stripANSI(s string) string {
	result := stripCSISequences(s)
	result = stripOSCSequences(result)
	return stripBracketedPasteSequences(result)
}

// stripCSISequences removes CSI sequences (ESC [ ... letter).
func stripCSISequences(result string) string {
	for {
		start := strings.Index(result, "\x1b[")
		if start == -1 {
			return result
		}
		end := start + 2
		for end < len(result) {
			ch := result[end]
			if (ch >= 'A' && ch <= 'Z') || (ch >= 'a' && ch <= 'z') {
				end++
				break
			}
			end++
		}
		result = result[:start] + result[end:]
	}
}

// stripOSCSequences removes OSC sequences (ESC ] ... BEL or ESC \).
func stripOSCSequences(result string) string {
	for {
		start := strings.Index(result, "\x1b]")
		if start == -1 {
			return result
		}
		end := strings.IndexAny(result[start:], "\x07")
		if end == -1 {
			stIdx := strings.Index(result[start:], "\x1b\\")
			if stIdx == -1 {
				return result
			}
			end = stIdx + 2
		} else {
			end++
		}
		result = result[:start] + result[start+end:]
	}
}

// stripBracketedPasteSequences removes ESC ? ... h/l sequences.
func stripBracketedPasteSequences(result string) string {
	for {
		start := strings.Index(result, "\x1b?")
		if start == -1 {
			return result
		}
		end := start + 2
		for end < len(result) && result[end] != 'h' && result[end] != 'l' {
			end++
		}
		if end < len(result) {
			end++
		}
		result = result[:start] + result[end:]
	}
}

// GeminiPromptPatterns are patterns that indicate Gemini is ready for input
var GeminiPromptPatterns = []string{
	">   Type your message", // Gemini's input prompt text
	"@path/to/file",         // Part of Gemini's input prompt
	"╭─",                    // Box drawing characters from Gemini UI
	"╰─",                    // Box drawing characters from Gemini UI
}

// OpenCodePromptPatterns are patterns that indicate OpenCode is ready for input
var OpenCodePromptPatterns = []string{
	"> ",  // OpenCode input prompt
	"❯",   // OpenCode may use similar prompt to Claude
	">> ", // Alternative OpenCode prompt pattern
}

// WaitForGeminiPrompt waits for Gemini to return to the input prompt
// Uses control mode to monitor output stream and detect prompt patterns
// Similar to WaitForClaudePrompt but adapted for Gemini's UI patterns
//
//nolint:gocyclo // reason: stateful tmux control-mode loop with many concurrent termination conditions; helpers would obscure the per-event flow.
func WaitForGeminiPrompt(sessionName string, timeout time.Duration) error {
	debug.Log("\n🔍 Starting Gemini prompt detection for session: %s", sessionName)

	// Start control mode
	ctrl, err := StartControlMode(sessionName)
	if err != nil {
		return fmt.Errorf("failed to start control mode: %w", err)
	}
	defer ctrl.Close()

	// Create output watcher
	watcher := NewOutputWatcher(ctrl.Stdout)

	// Wait for prompt pattern
	deadline := time.Now().Add(timeout)
	consecutiveIdleLines := 0
	linesChecked := 0
	promptPatternsSeen := 0

	for time.Now().Before(deadline) {
		// Read next output line (200ms timeout for faster detection)
		line, err := watcher.ReadLine(200 * time.Millisecond)
		if err != nil {
			// Timeout on individual read - check if we've seen enough idle time
			consecutiveIdleLines++

			// If we've seen prompt patterns and then idle, assume ready
			// Increased to 10 consecutive idles (2 seconds) to avoid false positives
			if consecutiveIdleLines >= 10 && promptPatternsSeen >= 2 {
				debug.Log("✓ Detected Gemini prompt after idle period (saw %d patterns)", promptPatternsSeen)
				return nil
			}

			// If we've checked many lines and seen idle, likely ready
			if linesChecked > 10 && consecutiveIdleLines >= 15 {
				debug.Log("✓ Stable idle state detected after %d lines", linesChecked)
				return nil
			}

			continue
		}

		// Reset idle counter
		consecutiveIdleLines = 0
		linesChecked++

		// Extract content if it's an %output line
		if strings.HasPrefix(line, "%output") {
			content := ExtractOutputContent(line)

			// Log output for debugging (limit verbosity)
			if linesChecked <= 5 || linesChecked%10 == 0 {
				if isVisibleContent(content) {
					cleanContent := stripANSI(content)
					if strings.TrimSpace(cleanContent) != "" {
						debug.Log("📝 Output [%d]: %q", linesChecked, truncate(cleanContent, 80))
					}
				}
			}

			// Check for Gemini prompt patterns
			if containsGeminiPromptPattern(content) {
				promptPatternsSeen++
				debug.Log("✓ Gemini prompt pattern detected in line %d: %q (count: %d)", linesChecked, truncate(content, 50), promptPatternsSeen)

				// Need to see multiple patterns to confirm (Gemini's UI has box drawing + text)
				if promptPatternsSeen >= 2 {
					// Wait a bit more to ensure it's stable
					time.Sleep(1 * time.Second)
					return nil
				}
			}
		}

		// Check for %end notification (command completed)
		if strings.HasPrefix(line, "%end") {
			debug.Log("📋 Command completion detected (%%end) at line %d", linesChecked)
		}
	}

	return fmt.Errorf("timeout waiting for Gemini prompt (waited %v, checked %d lines)", timeout, linesChecked)
}

// containsAnyHarnessPromptPattern checks if content contains prompt patterns from
// ANY supported harness (Claude, Gemini, OpenCode, Codex, AGY, or Pi). Used by SendMultiLinePromptSafe and
// SendPromptLiteral which don't know the harness type but need to detect readiness.
func containsAnyHarnessPromptPattern(content string) bool {
	return containsAnyNonPiHarnessPromptPattern(content) || containsPiReadyPattern(stripANSI(content))
}

func containsAnyNonPiHarnessPromptPattern(content string) bool {
	plainContent := stripANSI(content)
	return containsClaudePromptPattern(plainContent) || containsGeminiPromptPattern(plainContent) ||
		containsOpenCodePromptPattern(plainContent) || IsCodexComposerReady(content) ||
		containsAgyPromptPattern(plainContent)
}

// containsGeminiPromptPattern checks if content contains any Gemini prompt pattern
func containsGeminiPromptPattern(content string) bool {
	// Trim whitespace for comparison
	trimmed := strings.TrimSpace(content)

	// Empty content is not a prompt
	if trimmed == "" {
		return false
	}

	// Check against known Gemini patterns
	for _, pattern := range GeminiPromptPatterns {
		if strings.Contains(trimmed, pattern) {
			return true
		}
	}

	return false
}

// containsOpenCodePromptPattern checks if content contains any OpenCode prompt pattern
func containsOpenCodePromptPattern(content string) bool {
	// Trim whitespace for comparison
	trimmed := strings.TrimSpace(content)

	// Empty content is not a prompt
	if trimmed == "" {
		return false
	}

	// Check against known OpenCode patterns
	for _, pattern := range OpenCodePromptPatterns {
		if strings.Contains(trimmed, pattern) {
			return true
		}
	}

	return false
}

// WaitForGeminiReady waits for Gemini to be fully ready
// This function waits for the Gemini prompt to appear after startup
//
//nolint:gocyclo // reason: stateful readiness loop with many termination conditions; per-event helpers would obscure the polling protocol.
func WaitForGeminiReady(sessionName string, timeout time.Duration) error {
	debug.Log("🔍 Waiting for Gemini to be ready (session: %s)", sessionName)

	// Start control mode
	ctrl, err := StartControlMode(sessionName)
	if err != nil {
		return fmt.Errorf("failed to start control mode: %w", err)
	}
	defer ctrl.Close()

	// Create output watcher
	watcher := NewOutputWatcher(ctrl.Stdout)

	// State tracking
	deadline := time.Now().Add(timeout)
	linesChecked := 0
	promptPatternsSeen := 0
	bannerSeen := false

	for time.Now().Before(deadline) {
		// Read next output line
		line, err := watcher.ReadLine(2 * time.Second)
		if err != nil {
			// Timeout on individual read - might be ready
			if promptPatternsSeen >= 2 && linesChecked > 10 {
				debug.Log("✓ Gemini appears ready (saw %d prompt patterns)", promptPatternsSeen)
				return nil
			}
			continue
		}

		linesChecked++

		// Extract content if it's an %output line
		var content string
		if strings.HasPrefix(line, "%output") {
			content = ExtractOutputContent(line)
		} else {
			content = line
		}

		// Log output for debugging (first few lines and periodically)
		if linesChecked <= 10 || linesChecked%20 == 0 {
			if isVisibleContent(content) {
				cleanContent := stripANSI(content)
				if strings.TrimSpace(cleanContent) != "" {
					debug.Log("📝 Output [%d]: %q", linesChecked, truncate(cleanContent, 100))
				}
			}
		}

		// Check for Gemini ASCII banner (indicates startup)
		if strings.Contains(content, "███") || strings.Contains(content, "GEMINI") {
			if !bannerSeen {
				bannerSeen = true
				debug.Log("🎨 Gemini banner detected at line %d", linesChecked)
			}
		}

		// Check for Gemini prompt patterns
		if containsGeminiPromptPattern(content) {
			promptPatternsSeen++
			debug.Log("✓ Gemini prompt pattern detected at line %d: %q (count: %d)",
				linesChecked, truncate(content, 50), promptPatternsSeen)

			// Need to see multiple patterns to confirm (box drawing + text)
			if promptPatternsSeen >= 2 {
				debug.Log("✓ Gemini prompt fully detected, waiting for stability...")
				time.Sleep(500 * time.Millisecond)
				return nil
			}
		}
	}

	return fmt.Errorf("timeout waiting for Gemini to be ready (waited %v, checked %d lines)", timeout, linesChecked)
}

// WaitForOutputIdle detects when tmux pane output has been idle (no new output) for a specified duration.
// This uses capture-pane polling to track output changes over time.
// Returns nil when output has been idle for idleDuration, error on timeout.
//
// This is useful for detecting when a skill or command has finished producing output,
// even if it doesn't print an explicit completion marker.
//
// Example:
//
//	// Wait for output to be idle for 1 second, with 15 second total timeout
//	err := WaitForOutputIdle("my-session", 1*time.Second, 15*time.Second)
func WaitForOutputIdle(sessionName string, idleDuration time.Duration, timeout time.Duration) error {
	debug.Log("🔍 Starting idle detection for session: %s (idle threshold: %v, timeout: %v)",
		sessionName, idleDuration, timeout)

	// Find which socket the session is on
	socketPath := findSessionSocket(sessionName)

	deadline := time.Now().Add(timeout)
	pollInterval := 200 * time.Millisecond // Faster polling for responsive detection

	var lastContent string
	var lastChangeTime time.Time
	checksPerformed := 0

	for time.Now().Before(deadline) {
		checksPerformed++

		// Capture last 50 lines from pane
		cmd := exec.Command("tmux", "-S", socketPath, "capture-pane", "-t", sessionName, "-p", "-S", "-50")
		output, err := cmd.CombinedOutput()
		if err != nil {
			debug.Log("⚠️  capture-pane failed (attempt %d): %v", checksPerformed, err)
			time.Sleep(pollInterval)
			continue
		}

		content := string(output)

		// Initialize on first check
		if checksPerformed == 1 {
			lastContent = content
			lastChangeTime = time.Now()
			debug.Log("📝 Initial output captured (%d bytes)", len(content))
		} else {
			// Check if content has changed
			if content != lastContent {
				// Output changed - reset idle timer
				lastContent = content
				lastChangeTime = time.Now()
				debug.Log("📝 Output changed (check #%d, idle timer reset)", checksPerformed)
			} else {
				// Output unchanged - check idle duration
				idleTime := time.Since(lastChangeTime)
				if idleTime >= idleDuration {
					debug.Log("✓ Output idle detected after %d checks (idle for %v)",
						checksPerformed, idleTime)
					return nil
				}

				// Log progress every 5 checks when close to idle threshold
				if checksPerformed%5 == 0 {
					debug.Log("⏳ Output idle for %v (threshold: %v)", idleTime, idleDuration)
				}
			}
		}

		// Sleep before next poll
		time.Sleep(pollInterval)
	}

	return fmt.Errorf("timeout waiting for output idle (waited %v, checked %d times)", timeout, checksPerformed)
}

// WaitForPattern waits for a specific text pattern to appear in tmux pane output.
// This uses capture-pane polling to check for the pattern.
// Returns nil when pattern is found, error on timeout.
//
// This is useful for detecting explicit completion markers or messages from skills/commands.
//
// Example:
//
//	// Wait for skill completion marker
//	err := WaitForPattern("my-session", "[AGM_SKILL_COMPLETE]", 10*time.Second)
func WaitForPattern(sessionName string, pattern string, timeout time.Duration) error {
	debug.Log("🔍 Starting pattern detection for session: %s (pattern: %q, timeout: %v)",
		sessionName, pattern, timeout)

	// Find which socket the session is on
	socketPath := findSessionSocket(sessionName)

	deadline := time.Now().Add(timeout)
	pollInterval := 200 * time.Millisecond
	checksPerformed := 0

	for time.Now().Before(deadline) {
		checksPerformed++

		// Capture last 100 lines from pane (more lines to catch pattern)
		cmd := exec.Command("tmux", "-S", socketPath, "capture-pane", "-t", sessionName, "-p", "-S", "-100")
		output, err := cmd.CombinedOutput()
		if err != nil {
			debug.Log("⚠️  capture-pane failed (attempt %d): %v", checksPerformed, err)
			time.Sleep(pollInterval)
			continue
		}

		content := string(output)

		// Check if pattern exists in output
		if strings.Contains(content, pattern) {
			debug.Log("✓ Pattern found after %d checks: %q", checksPerformed, pattern)
			return nil
		}

		// Log progress every 10 checks
		if checksPerformed%10 == 0 {
			debug.Log("⏳ Still searching for pattern... (check #%d)", checksPerformed)
		}

		// Sleep before next poll
		time.Sleep(pollInterval)
	}

	return fmt.Errorf("timeout waiting for pattern %q (waited %v, checked %d times)", pattern, timeout, checksPerformed)
}
