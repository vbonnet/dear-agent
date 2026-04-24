// Package tmux provides tmux session management.
package tmux

import (
	"regexp"
	"strings"
	"time"
)

// PaneInfo contains captured state of a tmux pane.
type PaneInfo struct {
	SessionName string    // Name of the tmux session
	Content     string    // Full pane content (up to 500 lines)
	CursorX     int       // Cursor X position
	CursorY     int       // Cursor Y position
	CapturedAt  time.Time // When this state was captured
	LastCommand string    // Last command extracted from pane (if detectable)
}

// Stuck detection patterns
var (
	// Mustering patterns - session stuck during initialization
	// These are checked FIRST to prevent overlap with waiting patterns
	musteringPatterns = []*regexp.Regexp{
		regexp.MustCompile(`✻ Mustering\.\.\.`),
		regexp.MustCompile(`✶ Evaporating\.\.\.`),
		regexp.MustCompile(`✢ Mustering\.\.\.`),
	}

	// Waiting patterns - session stuck with spinner
	// NOTE: We use specific patterns first, then generic pattern
	waitingPatterns = []*regexp.Regexp{
		// Specific patterns (checked first for fast path)
		regexp.MustCompile(`✶ Thinking\.\.\.`),
		regexp.MustCompile(`✢ Processing\.\.\.`),
		regexp.MustCompile(`✶ Processing\.\.\.`),
		regexp.MustCompile(`✻ Working\.\.\.`),
		regexp.MustCompile(`· Waiting\.\.\.`),
		// Generic pattern: any spinner with dots (checked AFTER mustering patterns to avoid overlap)
		regexp.MustCompile(`[✶✢✻·]\s+.+\.\.\.`),
	}

	// Permission prompt patterns - Claude asking for tool permission.
	// Includes both generic patterns and Claude Code-specific UI patterns.
	permissionPromptPatterns = []*regexp.Regexp{
		// Claude Code-specific patterns (checked first for fast path)
		// Claude Code shows: "Allow Bash" / "Allow Read" / "Allow Edit" etc.
		regexp.MustCompile(`(?m)^\s*Allow\s+(Bash|Read|Edit|Write|Glob|Grep|Agent|Skill|NotebookEdit|WebFetch|WebSearch)\b`),
		// Claude Code choice format: "(y)es | (n)o" or "(Y)es | (N)o"
		regexp.MustCompile(`\([yY]\)es\s*\|\s*\([nN]\)o`),
		// Claude Code "Allow all" / "don't ask again" format
		regexp.MustCompile(`(?i)don't ask again`),
		// Claude Code "Allow in this session" option
		regexp.MustCompile(`(?i)\(A\)llow\s+in\s+this\s+session`),

		// Generic patterns (fallback)
		regexp.MustCompile(`(?i)allow.*to.*\?`),
		regexp.MustCompile(`(?i)permission.*to.*\?`),
		regexp.MustCompile(`(?i)proceed.*\?`),
		regexp.MustCompile(`(?i)continue.*\?`),
		regexp.MustCompile(`\(y/n\)`),
		regexp.MustCompile(`\[y/n\]`),
	}

	// AskUserQuestion / interactive prompt patterns - Claude waiting for human input.
	// These TUI dialogs show numbered options, selection prompts, or plan approval.
	// Sessions in this state are legitimately waiting for human input, NOT stuck.
	askUserQuestionPatterns = []*regexp.Regexp{
		// Numbered option lists (e.g., "  1. Option A\n  2. Option B") - require 2+ items
		regexp.MustCompile(`(?m)^\s*1\.\s+\S+[\s\S]*?^\s*2\.\s+\S+`),
		// "Enter to select/confirm" prompts
		regexp.MustCompile(`(?i)enter\s+to\s+(select|confirm|continue|submit)`),
		// Multi-choice / selection menus
		regexp.MustCompile(`(?i)(select|choose|pick)\s+(an?\s+)?(option|choice|item|number)`),
		// Plan approval prompts
		regexp.MustCompile(`(?i)(approve|reject)\s+(this\s+)?(plan|approach)`),
		// Arrow key navigation hints
		regexp.MustCompile(`(?i)(use\s+)?arrow\s+keys?\s+to\s+(navigate|select|move)`),
		// "Type a number" or "Enter your choice"
		regexp.MustCompile(`(?i)(type|enter)\s+(a\s+)?(number|your\s+choice)`),
	}

	// Completion patterns - session finished work
	completionPatterns = []*regexp.Regexp{
		regexp.MustCompile(`✅`),
		regexp.MustCompile(`✓`),
		regexp.MustCompile(`(?i)Task.*completed`),
		regexp.MustCompile(`(?i)Task.*finished`),
		regexp.MustCompile(`(?i)Task.*done`),
		regexp.MustCompile(`(?i)All.*complete`),
		regexp.MustCompile(`(?i)Successfully.*completed`),
		regexp.MustCompile(`(?i)Ready to proceed`),
		regexp.MustCompile(`(?i)What would you like`),
		regexp.MustCompile(`(?i)How can I help`),
	}

	// Idle prompt - ❯ character indicating Claude is ready (must be at end)
	idlePromptPattern = regexp.MustCompile(`❯\s*$`)
)

// ExtractLastCommand attempts to extract the last command from pane content.
// Looks for "Bash command:" header or similar indicators.
// Returns empty string if no command detected.
func (p *PaneInfo) ExtractLastCommand() string {
	lines := strings.Split(p.Content, "\n")

	// Search backwards through recent lines for command headers
	for i := len(lines) - 1; i >= 0 && i >= len(lines)-50; i-- {
		line := lines[i]

		// Look for "Bash command:" or similar headers
		if strings.Contains(line, "Bash command:") ||
			strings.Contains(line, "Running command:") ||
			strings.Contains(line, "Executing:") {

			// Command is usually on the next line
			if i+1 < len(lines) {
				return strings.TrimSpace(lines[i+1])
			}
		}
	}

	return ""
}

// DetectPermissionPrompt checks if the pane contains a Claude permission prompt.
// Permission prompts indicate Claude is waiting for user approval to use a tool.
// Uses a larger content window (1000 chars) because Claude Code permission prompts
// span multiple lines (tool name, parameters, choice options).
func (p *PaneInfo) DetectPermissionPrompt() bool {
	// Check last 1000 characters — Claude Code permission prompts span multiple lines
	recentContent := p.getRecentContent(1000)

	for _, pattern := range permissionPromptPatterns {
		if pattern.MatchString(recentContent) {
			return true
		}
	}

	return false
}

// DetectStuckIndicators checks for various stuck session indicators.
// Returns a map of indicator names to boolean values.
func (p *PaneInfo) DetectStuckIndicators() map[string]bool {
	indicators := make(map[string]bool)
	recentContent := p.getRecentContent(500)

	// Check for mustering patterns
	indicators["mustering"] = false
	for _, pattern := range musteringPatterns {
		if pattern.MatchString(recentContent) {
			indicators["mustering"] = true
			break
		}
	}

	// Check for waiting/spinner patterns (but NOT if it's mustering/evaporating)
	indicators["waiting"] = false
	if !indicators["mustering"] { // Only check waiting if NOT mustering
		for _, pattern := range waitingPatterns {
			if pattern.MatchString(recentContent) {
				indicators["waiting"] = true
				break
			}
		}
	}

	// Check for permission prompts
	indicators["permission_prompt"] = p.DetectPermissionPrompt()

	// Check for AskUserQuestion / interactive prompts (legitimate human-waiting state)
	indicators["ask_user_question"] = p.hasAskUserQuestion()

	// Check for completion language
	indicators["completed"] = p.hasCompletionLanguage()

	// Check for idle prompt
	indicators["idle_prompt"] = p.hasIdlePrompt()

	// Check for zero token waiting (waiting pattern + no idle prompt + not waiting for human)
	indicators["zero_token_waiting"] = indicators["waiting"] && !indicators["idle_prompt"] && !indicators["ask_user_question"]

	return indicators
}

// hasCompletionLanguage checks if pane contains completion/done language.
func (p *PaneInfo) hasCompletionLanguage() bool {
	recentContent := p.getRecentContent(500)

	for _, pattern := range completionPatterns {
		if pattern.MatchString(recentContent) {
			return true
		}
	}

	return false
}

// hasAskUserQuestion checks if the pane contains an AskUserQuestion or
// interactive selection prompt. These are legitimate human-waiting states
// that must never be interrupted by recovery actions.
func (p *PaneInfo) hasAskUserQuestion() bool {
	// Use more content for multi-line pattern matching (numbered lists span lines)
	recentContent := p.getRecentContent(1000)

	for _, pattern := range askUserQuestionPatterns {
		if pattern.MatchString(recentContent) {
			return true
		}
	}

	return false
}

// hasIdlePrompt checks if the idle prompt (❯) is visible.
func (p *PaneInfo) hasIdlePrompt() bool {
	// Check last 100 chars (prompt is always at end)
	recentContent := p.getRecentContent(100)
	return idlePromptPattern.MatchString(recentContent)
}

// getRecentContent returns the last N characters from pane content.
func (p *PaneInfo) getRecentContent(n int) string {
	if len(p.Content) <= n {
		return p.Content
	}
	return p.Content[len(p.Content)-n:]
}

// IsStuck performs a simple stuck check combining multiple indicators.
// A session is considered stuck if:
// - Has waiting/mustering pattern AND
// - No idle prompt AND
// - No completion language AND
// - Not waiting for human input (AskUserQuestion)
func (p *PaneInfo) IsStuck() bool {
	indicators := p.DetectStuckIndicators()

	// Never consider stuck if waiting for human input
	if indicators["ask_user_question"] {
		return false
	}

	// Stuck if showing spinner but no idle prompt and not completed
	if (indicators["waiting"] || indicators["mustering"]) &&
		!indicators["idle_prompt"] &&
		!indicators["completed"] {
		return true
	}

	return false
}

// GetStuckReason returns a human-readable reason why the session appears stuck.
// Returns empty string if session does not appear stuck.
func (p *PaneInfo) GetStuckReason() string {
	indicators := p.DetectStuckIndicators()

	// AskUserQuestion is a legitimate waiting state — never stuck
	if indicators["ask_user_question"] {
		return ""
	}

	if indicators["mustering"] && !indicators["idle_prompt"] {
		return "stuck_mustering"
	}

	if indicators["zero_token_waiting"] {
		return "stuck_zero_token_waiting"
	}

	if indicators["waiting"] && !indicators["idle_prompt"] && !indicators["completed"] {
		return "stuck_waiting"
	}

	if indicators["permission_prompt"] {
		return "stuck_permission_prompt"
	}

	return ""
}

// CapturePaneInfo creates a PaneInfo snapshot from a tmux client.
// Returns error if session cannot be captured.
func CapturePaneInfo(client *Client, sessionName string) (*PaneInfo, error) {
	content, err := client.GetPaneContent(sessionName)
	if err != nil {
		return nil, err
	}

	x, y, err := client.GetCursorPosition(sessionName)
	if err != nil {
		return nil, err
	}

	pane := &PaneInfo{
		SessionName: sessionName,
		Content:     content,
		CursorX:     x,
		CursorY:     y,
		CapturedAt:  time.Now(),
	}

	pane.LastCommand = pane.ExtractLastCommand()

	return pane, nil
}
