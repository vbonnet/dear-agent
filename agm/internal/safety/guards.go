package safety

import (
	"fmt"
	"os/exec"
	"regexp"
	"slices"
	"strings"
	"unicode"

	"github.com/vbonnet/dear-agent/agm/internal/tmux"
)

// capturePaneContent captures the last N lines of a tmux pane.
func capturePaneContent(sessionName, socketPath string, lines int) (string, error) {
	normalizedName := tmux.NormalizeTmuxSessionName(sessionName)
	cmd := exec.Command("tmux", "-S", socketPath, "capture-pane", "-t", normalizedName, "-p", "-S", fmt.Sprintf("-%d", lines))
	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("failed to capture pane: %w", err)
	}
	return string(output), nil
}

// capturePaneWithEscape captures the last N lines of a tmux pane including ANSI escape codes.
func capturePaneWithEscape(sessionName, socketPath string, lines int) (string, error) {
	normalizedName := tmux.NormalizeTmuxSessionName(sessionName)
	cmd := exec.Command("tmux", "-S", socketPath, "capture-pane", "-t", normalizedName, "-p", "-e", "-S", fmt.Sprintf("-%d", lines))
	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("failed to capture pane with escape: %w", err)
	}
	return string(output), nil
}

// ansiStripRe matches ANSI CSI escape sequences for stripping.
var ansiStripRe = regexp.MustCompile(`\x1b\[[0-9;]*[a-zA-Z]`)

// stripANSI removes ANSI escape sequences from a string.
func stripANSI(s string) string {
	return ansiStripRe.ReplaceAllString(s, "")
}

// isGhostTextAfterPrompt reports whether the text after the ❯ prompt in the
// ANSI-rich pane capture is overseer ghost/placeholder text rather than real
// human input. Ghost text is styled with a dim or grey SGR attribute — Claude
// Code uses the dim attribute (\x1b[2m), while the vroom-meta-orchestrator uses
// 256-color grey (\x1b[38;5;241m). Detection is delegated to tmux.IsDimOrGreySGR
// so all dim/grey variants are recognized (ce-5miu), generalizing the original
// dim-only check (ce-v9in / PR #512). Only the portion of the line AFTER ❯ is
// checked to avoid false positives from dim/grey attributes that appear before
// the prompt marker.
func isGhostTextAfterPrompt(ansiContent string) bool {
	lines := strings.Split(ansiContent, "\n")
	for _, line := range slices.Backward(lines) {
		plainLine := stripANSI(line)
		if !strings.Contains(plainLine, "❯") {
			continue
		}
		// Find ❯ in the raw ANSI line and check only what follows it.
		idx := strings.Index(line, "❯")
		if idx < 0 {
			continue
		}
		return tmux.IsDimOrGreySGR(line[idx:])
	}
	return false
}

// permissionPromptPattern matches text rendered by Claude Code permission
// prompts and interactive UI elements. These appear at or near the prompt
// line but are not human input. The pattern is checked after trimming
// leading whitespace from the text after ❯.
var permissionPromptPattern = regexp.MustCompile(
	`(?i)` +
		`(` +
		`^\d+\.\s+` + // "1. Yes, allow" numbered options
		`|` +
		`^[yY]/[nN]` + // y/N prompt
		`|` +
		`^[nN]/[yY]` + // N/y prompt
		`|` +
		`^\([yYnN]\)` + // (Y)es/(N)o style
		`|` +
		`^(?:yes|no|allow|deny|skip|cancel|always allow|allow once|don't allow)\b` +
		`|` +
		`^(?:use arrows|press enter|do you want)\b` + // Navigation/confirmation UI hints
		`|` +
		`^❯\s*$` + // Bare prompt re-render artifact
		`)`,
)

// --- Human Typing Guard ---

// CheckHumanTyping reports whether a human is actively typing at the Claude
// prompt line. It uses a single ANSI capture to both read the composer text and
// classify the pane's dim/grey ghost-text state in one tmux round-trip.
//
// The detection is an ALLOWLIST of known-safe pane states, not a denylist of
// human-typing patterns (ce-py3x; retro 2026-06-17-vroom-overnight-human-typing-
// guard; policy docs/policies/harness-hygiene "invert to allowlist known-safe").
// The historical denylist presumed any text after ❯ was human typing unless it
// matched a hand-added exemption, so every novel Claude Code UI state read as
// typing — one such untracked state blocked the Overseer overnight (a crash-
// prevention control acting as total-work-prevention). The ghost/dim-and-grey
// state is one allowlist entry, handled here at the ANSI layer via
// isGhostTextAfterPrompt; the remaining known-safe states and the flipped
// default (unrecognized ⇒ not typing) live in detectHumanTyping.
func CheckHumanTyping(sessionName, socketPath string) *Violation {
	ansiContent, err := capturePaneWithEscape(sessionName, socketPath, 10)
	if err != nil {
		return nil // Can't capture = can't detect, allow through
	}
	// Ghost/dim/grey text after ❯ is a known-safe state (placeholder, not human
	// input). Classify it from the ANSI capture before the plain-text pass so
	// dim-styled content never reaches the human-input signature check.
	if isGhostTextAfterPrompt(ansiContent) {
		return nil
	}
	return detectHumanTyping(stripANSI(ansiContent))
}

// detectHumanTyping is the pure-logic detection function (testable without tmux).
//
// It is an ALLOWLIST classifier: a violation is reported only when the composer
// positively matches the human-input signature; every other pane state — known
// or unknown — is treated as not typing. This inverts the previous denylist,
// whose default was "assume typing" and which therefore misread each new UI
// state as a human at the keyboard (ce-py3x). Known-safe states (return nil):
//
//  1. no ❯ prompt line present;
//  2. empty / whitespace-only text after ❯;
//  3. an AGM sender header ([From:/[from:]) after ❯ (automated message);
//  4. a permission / navigation UI pattern after ❯ (permissionPromptPattern);
//  5. text after ❯ containing no alphanumeric input rune — the flipped default:
//     pure UI chrome (box-drawing, braille spinner glyphs, separators, arrows,
//     symbol-only decoration) is not human typing. Previously this fired.
//
// Human-typing signature (fires): text after ❯ that is none of the above and
// contains at least one Unicode letter or digit — i.e., positively looks like
// typed input. The check is Unicode-aware so non-ASCII human input still fires
// while box-drawing/braille/symbol chrome (categories So/Sk) is excluded.
//
// Biasing toward "not typing" on ambiguity is safe: the guard is advisory and
// the send path stashes the composer (Claude Code C-s auto-unstashes on the next
// submit), so a missed detection is recoverable — a false positive that blocks
// autonomous work is not.
func detectHumanTyping(paneContent string) *Violation {
	lines := strings.Split(paneContent, "\n")

	// Scan from bottom up to find the last line with the ❯ prompt
	for i := len(lines) - 1; i >= 0; i-- {
		line := lines[i]
		idx := strings.Index(line, "❯")
		if idx < 0 {
			continue
		}

		// Extract text after the prompt character
		after := strings.TrimSpace(line[idx+len("❯"):])
		if after == "" {
			return nil // (2) empty prompt, no one is typing
		}

		// (3) AGM sender header (automated message, not human)
		if strings.HasPrefix(after, "[From:") || strings.HasPrefix(after, "[from:") {
			return nil
		}

		// (4) permission / navigation UI pattern (not human typing)
		if permissionPromptPattern.MatchString(after) {
			return nil
		}

		// (5) flipped default: require a positive human-input signature. Content
		// with no letter or digit is UI chrome (box-drawing, spinner glyphs,
		// separators, symbols), not typed input — treat it as not typing rather
		// than assuming the worst about an unrecognized state.
		if !hasHumanInputRune(after) {
			return nil
		}

		// Truncate evidence for display
		evidence := after
		if len(evidence) > 50 {
			evidence = evidence[:50] + "..."
		}

		return &Violation{
			Guard:      ViolationHumanTyping,
			Message:    fmt.Sprintf("Unsent text detected in prompt: %q", evidence),
			Suggestion: "Wait for the human to finish typing before sending.",
			Evidence:   evidence,
		}
	}

	return nil // (1) no prompt found = not typing
}

// hasHumanInputRune reports whether s contains at least one Unicode letter or
// digit — the positive signature of typed human input. It is Unicode-aware so
// non-ASCII keyboard input counts, while pure UI chrome (box-drawing, braille
// spinner frames, arrows, separators, other symbols) does not.
func hasHumanInputRune(s string) bool {
	for _, r := range s {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			return true
		}
	}
	return false
}

// --- Session Uninitialized Guard ---

// isHarnessProcessRunning reports whether processName is running in the
// session pane's process tree. Delegates to the shared full-descendant scan
// in internal/tmux (ce-axsr): the previous direct-children-only version
// missed harnesses running as grandchildren under a shell after
// crash-resume.
func isHarnessProcessRunning(sessionName, socketPath, processName string) bool {
	return tmux.IsProcessInPaneTree(sessionName, socketPath, processName)
}

// CheckSessionUninitialized detects if the target harness has not reached its
// interactive composer yet. Empty harness preserves the historical Claude Code
// behavior for legacy callers.
func CheckSessionUninitialized(sessionName, socketPath, harness string) *Violation {
	content, err := capturePaneContent(sessionName, socketPath, 50)
	if err != nil {
		return nil
	}
	harness = normalizeHarnessForSafety(harness)
	if harness == "codex-cli" {
		codexRunning, procErr := tmux.IsProcessRunning(sessionName, "codex")
		if procErr != nil {
			codexRunning = true
		}
		if !codexRunning {
			codexRunning = isHarnessProcessRunning(sessionName, socketPath, "codex") || isHarnessProcessRunning(sessionName, socketPath, "node")
		}
		if !codexRunning {
			return &Violation{
				Guard:      ViolationSessionUninitialized,
				Message:    "Codex process is not running in this session.",
				Suggestion: "Wait for Codex to start, or verify the session: agm session list",
				Evidence:   "no codex process",
			}
		}
		return detectCodexSessionUninitialized(content)
	}
	if harness == "agy" {
		agyRunning, procErr := tmux.IsProcessRunning(sessionName, "agy")
		if procErr != nil {
			agyRunning = true
		}
		if !agyRunning {
			agyRunning = isHarnessProcessRunning(sessionName, socketPath, "agy")
		}
		if !agyRunning {
			return &Violation{
				Guard:      ViolationSessionUninitialized,
				Message:    "AGY process is not running in this session.",
				Suggestion: "Wait for AGY to start, or verify the session: agm session list",
				Evidence:   "no agy process",
			}
		}
		return detectAgySessionUninitialized(content)
	}
	if harness == "pi-cli" {
		piRunning, scanErr := tmux.IsPiProcessInPaneTree(sessionName, socketPath)
		if scanErr != nil || !piRunning {
			return &Violation{
				Guard:      ViolationSessionUninitialized,
				Message:    "Pi process is not running in this session.",
				Suggestion: "Wait for Pi to start, or verify the session: agm session list",
				Evidence:   "no pi process",
			}
		}
		return detectPiSessionUninitialized(content)
	}
	if harness == "opencode-cli" {
		if !isHarnessProcessRunning(sessionName, socketPath, "opencode") {
			return &Violation{
				Guard:      ViolationSessionUninitialized,
				Message:    "OpenCode process is not running in this session.",
				Suggestion: "Wait for OpenCode to start, or verify the session: agm session list",
				Evidence:   "no opencode process",
			}
		}
		return detectOpenCodeSessionUninitialized(content)
	}

	// Also check if Claude process is running
	claudeRunning, procErr := tmux.IsClaudeRunning(sessionName)
	if procErr != nil {
		claudeRunning = true // Assume running if we can't check
	}

	return detectSessionUninitialized(content, claudeRunning)
}

func normalizeHarnessForSafety(harness string) string {
	switch strings.ToLower(strings.TrimSpace(harness)) {
	case "codex", "codex-cli":
		return "codex-cli"
	case "agy", "antigravity":
		return "agy"
	case "opencode", "opencode-cli":
		return "opencode-cli"
	case "pi", "pi-cli":
		return "pi-cli"
	case "claude", "claude-code", "":
		return "claude-code"
	default:
		return strings.ToLower(strings.TrimSpace(harness))
	}
}

func detectCodexSessionUninitialized(paneContent string) *Violation {
	// Initial composer box (visible on first launch or before any exchange).
	// After the first exchange the bordered box scrolls off; only the footer
	// "gpt-X.Y quality · /path" remains — "gpt-" covers both states.
	if strings.Contains(paneContent, "OpenAI Codex") ||
		strings.Contains(paneContent, "/model to change") ||
		strings.Contains(paneContent, "gpt-") ||
		strings.Contains(paneContent, "Thought for") ||
		strings.Contains(paneContent, "Running...") ||
		strings.Contains(paneContent, "●") ||
		strings.Contains(paneContent, "▸") {
		return nil
	}
	if strings.Contains(paneContent, "Do you trust the contents of this directory") {
		return &Violation{
			Guard:      ViolationSessionUninitialized,
			Message:    "Codex is showing the trust prompt (not yet initialized).",
			Suggestion: "Attach to the session and answer the trust prompt first.",
			Evidence:   "codex trust prompt visible",
		}
	}
	if strings.Contains(paneContent, "Choose how you'd like Codex to proceed") {
		return &Violation{
			Guard:      ViolationSessionUninitialized,
			Message:    "Codex is showing the model selection prompt (not yet initialized).",
			Suggestion: "Attach to the session and choose a model first.",
			Evidence:   "codex model prompt visible",
		}
	}
	return &Violation{
		Guard:      ViolationSessionUninitialized,
		Message:    "No Codex composer detected. Codex may not have started yet.",
		Suggestion: "Wait for Codex to initialize, or attach to verify.",
		Evidence:   "no codex composer",
	}
}

func detectAgySessionUninitialized(paneContent string) *Violation {
	if strings.Contains(paneContent, "Do you trust the contents of this project?") ||
		strings.Contains(paneContent, "Yes, I trust this folder") {
		return &Violation{
			Guard:      ViolationSessionUninitialized,
			Message:    "AGY is showing the trust prompt (not yet initialized).",
			Suggestion: "Attach to the session and answer the trust prompt first.",
			Evidence:   "agy trust prompt visible",
		}
	}

	for line := range strings.SplitSeq(paneContent, "\n") {
		if strings.TrimSpace(line) == ">" {
			return nil
		}
	}

	// Bypass if the agent is actively working/thinking or executing tools
	if strings.Contains(paneContent, "Thought for") ||
		strings.Contains(paneContent, "Running...") ||
		strings.Contains(paneContent, "●") ||
		strings.Contains(paneContent, "▸") {
		return nil
	}

	return &Violation{
		Guard:      ViolationSessionUninitialized,
		Message:    "No AGY prompt (>) detected. AGY may not have started yet.",
		Suggestion: "Wait for AGY to initialize, or attach to verify.",
		Evidence:   "no agy prompt",
	}
}

func detectPiSessionUninitialized(paneContent string) *Violation {
	switch tmux.PiManagedState(paneContent) {
	case "ready", "working":
		return nil
	}
	return &Violation{
		Guard:      ViolationSessionUninitialized,
		Message:    "Pi has not reached an AGM-managed ready or working state.",
		Suggestion: "Wait for AGM Pi authorization controls to become ready, or attach to inspect the active overlay.",
		Evidence:   "no managed pi ready state",
	}
}

func detectOpenCodeSessionUninitialized(paneContent string) *Violation {
	for _, pattern := range tmux.OpenCodePromptPatterns {
		if strings.Contains(paneContent, pattern) {
			return nil
		}
	}
	if strings.Contains(strings.ToLower(paneContent), "opencode") ||
		strings.Contains(paneContent, "Running...") ||
		strings.Contains(paneContent, "●") ||
		strings.Contains(paneContent, "▸") {
		return nil
	}
	return &Violation{
		Guard:      ViolationSessionUninitialized,
		Message:    "No OpenCode composer or active-work indicator detected.",
		Suggestion: "Wait for OpenCode to initialize, or attach to verify.",
		Evidence:   "no opencode composer",
	}
}

// detectSessionUninitialized is the pure-logic detection function.
func detectSessionUninitialized(paneContent string, claudeRunning bool) *Violation {
	// If Claude process isn't running at all, session is uninitialized or dead
	if !claudeRunning {
		return &Violation{
			Guard:      ViolationSessionUninitialized,
			Message:    "Claude process is not running in this session.",
			Suggestion: "Wait for Claude to start, or verify the session: agm session list",
			Evidence:   "no claude process",
		}
	}

	// Only a live, tail-owning trust dialog is current onboarding. An answered
	// dialog retained above a newer composer is historical evidence and must not
	// keep an initialized session blocked.
	if tmux.TrustDialogOwnsInput(paneContent) {
		return &Violation{
			Guard:      ViolationSessionUninitialized,
			Message:    "Session is showing the trust prompt (not yet initialized).",
			Suggestion: "Attach to the session and answer the trust prompt first.",
			Evidence:   "trust prompt visible",
		}
	}

	// Check for Claude Code welcome screen
	if strings.Contains(paneContent, "Welcome to Claude Code") {
		// But only if there's no ❯ prompt (welcome screen hasn't been dismissed)
		if !strings.Contains(paneContent, "❯") {
			return &Violation{
				Guard:      ViolationSessionUninitialized,
				Message:    "Session is showing the welcome screen (no prompt yet).",
				Suggestion: "Wait for Claude to finish initializing.",
				Evidence:   "welcome screen visible",
			}
		}
	}

	// Check if there's no Claude prompt at all (might be a bash shell)
	if !strings.Contains(paneContent, "❯") {
		return &Violation{
			Guard:      ViolationSessionUninitialized,
			Message:    "No Claude prompt (❯) detected. Claude may not have started yet.",
			Suggestion: "Wait for Claude to initialize, or attach to verify.",
			Evidence:   "no prompt character",
		}
	}

	return nil
}

// --- Claude Mid-Response Guard ---

// Spinner patterns that indicate Claude is actively generating a response.
// These match the patterns from astrocyte's pane.go and Claude Code's UI.
var spinnerPatterns = []*regexp.Regexp{
	regexp.MustCompile(`[✶✢✻·]\s+\S+\.\.\.`), // Generic spinner: ✶ Thinking...
	regexp.MustCompile(`✻ Mustering`),
	regexp.MustCompile(`✶ Evaporating`),
	regexp.MustCompile(`[⣾⣽⣻⢿⡿⣟⣯⣷]`), // Braille spinner characters
}

// CheckClaudeMidResponse detects if Claude is actively generating a response.
func CheckClaudeMidResponse(sessionName, socketPath string) *Violation {
	content, err := capturePaneContent(sessionName, socketPath, 20)
	if err != nil {
		return nil
	}
	return detectClaudeMidResponse(content)
}

// detectClaudeMidResponse is the pure-logic detection function.
func detectClaudeMidResponse(paneContent string) *Violation {
	// If there's a ❯ prompt visible at the end, Claude is NOT mid-response
	// (prompt means ready for input)
	lines := strings.Split(strings.TrimSpace(paneContent), "\n")
	for i := len(lines) - 1; i >= 0 && i >= len(lines)-3; i-- {
		if strings.Contains(lines[i], "❯") {
			return nil // Prompt visible near bottom = ready
		}
	}

	// Check for spinner patterns
	for _, pattern := range spinnerPatterns {
		if loc := pattern.FindStringIndex(paneContent); loc != nil {
			matched := paneContent[loc[0]:loc[1]]
			if len(matched) > 40 {
				matched = matched[:40] + "..."
			}
			return &Violation{
				Guard:      ViolationClaudeMidResponse,
				Message:    "Claude is actively generating a response (spinner detected).",
				Suggestion: "Wait for Claude to finish generating its response.",
				Evidence:   matched,
			}
		}
	}

	return nil
}
