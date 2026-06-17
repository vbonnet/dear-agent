package safety

import (
	"fmt"
	"os/exec"
	"regexp"
	"strings"

	"github.com/vbonnet/dear-agent/agm/internal/tmux"
)

// capturePaneContent captures the last N lines of a tmux pane. When
// withEscapes is true, ANSI/SGR escape sequences are preserved (tmux -e) so
// callers can inspect rendering attributes (e.g. dim/faint ghost text).
func capturePaneContent(sessionName, socketPath string, lines int, withEscapes bool) (string, error) {
	normalizedName := tmux.NormalizeTmuxSessionName(sessionName)
	args := []string{"-S", socketPath, "capture-pane", "-t", normalizedName, "-p", "-S", fmt.Sprintf("-%d", lines)}
	if withEscapes {
		args = append(args, "-e")
	}
	cmd := exec.Command("tmux", args...)
	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("failed to capture pane: %w", err)
	}
	return string(output), nil
}

// ansiSGRPattern matches ANSI SGR escape sequences (e.g. "\x1b[2m", "\x1b[0m",
// "\x1b[38;5;105m"), used to strip rendering codes before text comparisons.
var ansiSGRPattern = regexp.MustCompile("\x1b\\[[0-9;]*m")

// dimSGR is the SGR "faint/dim" attribute that claude-code uses to render
// ghost/placeholder (AI-suggested) text in the prompt input line.
const dimSGR = "\x1b[2m"

// permissionPromptPattern matches text rendered by Claude Code permission prompts.
// These appear at or near the prompt line but are not human input.
var permissionPromptPattern = regexp.MustCompile(
	`(?i)` +
		`(` +
		`^\d+\.\s+` + // "1. Yes, allow" numbered options
		`|` +
		`^[yY]/[nN]` + // y/N prompt
		`|` +
		`^[nN]/[yY]` + // N/y prompt
		`|` +
		`^(?:yes|no|allow|deny|skip|cancel)\b` + // Common permission words
		`)`,
)

// --- Human Typing Guard ---

// CheckHumanTyping detects unsent text in the Claude prompt line.
// Text after the ❯ prompt without an AGM sender header indicates a human is typing.
func CheckHumanTyping(sessionName, socketPath string) *Violation {
	// Capture WITH escape sequences so detectHumanTyping can tell claude-code's
	// dim/faint ghost-text (AI suggestions) apart from real human input.
	content, err := capturePaneContent(sessionName, socketPath, 10, true)
	if err != nil {
		return nil // Can't capture = can't detect, allow through
	}
	return detectHumanTyping(content)
}

// detectHumanTyping is the pure-logic detection function (testable without tmux).
// paneContent may contain ANSI/SGR escape sequences (captured via tmux -e):
// claude-code renders ghost/placeholder (AI-suggested) text on the prompt line
// with the dim/faint SGR attribute (\x1b[2m), whereas real human input is not
// dimmed. Plain (escape-free) content is handled identically — it simply has no
// dim marker, so it can never be misclassified as ghost text.
func detectHumanTyping(paneContent string) *Violation {
	lines := strings.Split(paneContent, "\n")

	// Scan from bottom up to find the last line with the ❯ prompt
	for i := len(lines) - 1; i >= 0; i-- {
		rawLine := lines[i]
		// Strip SGR codes for text-based detection/extraction.
		line := ansiSGRPattern.ReplaceAllString(rawLine, "")
		idx := strings.Index(line, "❯")
		if idx < 0 {
			continue
		}

		// Extract text after the prompt character
		after := strings.TrimSpace(line[idx+len("❯"):])
		if after == "" {
			return nil // Empty prompt, no one is typing
		}

		// Claude-code renders ghost/placeholder text dim (\x1b[2m). If the raw
		// prompt line carries the dim attribute, the trailing text is the TUI's
		// own AI suggestion, not a human's unsent input — not a violation.
		if strings.Contains(rawLine, dimSGR) {
			return nil
		}

		// Check if it's an AGM sender header (automated message, not human)
		if strings.HasPrefix(after, "[From:") || strings.HasPrefix(after, "[from:") {
			return nil
		}

		// Check if it's a permission prompt option (not human typing)
		if permissionPromptPattern.MatchString(after) {
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

	return nil // No prompt found = not typing
}

// --- Session Uninitialized Guard ---

// CheckSessionUninitialized detects if Claude hasn't started or is showing the welcome screen.
func CheckSessionUninitialized(sessionName, socketPath string) *Violation {
	content, err := capturePaneContent(sessionName, socketPath, 50, false)
	if err != nil {
		return nil
	}

	// Also check if Claude process is running
	claudeRunning, procErr := tmux.IsClaudeRunning(sessionName)
	if procErr != nil {
		claudeRunning = true // Assume running if we can't check
	}

	return detectSessionUninitialized(content, claudeRunning)
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

	// Check for welcome/trust screen indicators
	if strings.Contains(paneContent, "Do you trust the files in this folder?") {
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
	content, err := capturePaneContent(sessionName, socketPath, 20, false)
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
