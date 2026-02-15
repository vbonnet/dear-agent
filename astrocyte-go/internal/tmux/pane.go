package tmux

import (
	"regexp"
	"strings"
	"time"
)

// PaneInfo contains captured state of a tmux pane.
type PaneInfo struct {
	SessionName    string    // Name of the tmux session
	Content        string    // Full pane content (up to 500 lines)
	CursorX        int       // Cursor X position
	CursorY        int       // Cursor Y position
	CapturedAt     time.Time // When this state was captured
	LastCommand    string    // Last command extracted from pane (if detectable)
}

// Stuck detection patterns
var (
	// Mustering patterns - session stuck during initialization
	musteringPatterns = []*regexp.Regexp{
		regexp.MustCompile(`✻ Mustering\.\.\.`),
		regexp.MustCompile(`✶ Evaporating\.\.\.`),
		regexp.MustCompile(`✢ Mustering\.\.\.`),
	}

	// Waiting patterns - session stuck with spinner
	waitingPatterns = []*regexp.Regexp{
		// Generic Claude spinner pattern (✶✢✻· symbol + verb…)
		regexp.MustCompile(`[✶✢✻·]\s+\w+\.\.\.`),
		// Common specific patterns
		regexp.MustCompile(`✶ Thinking\.\.\.`),
		regexp.MustCompile(`✢ Processing\.\.\.`),
		regexp.MustCompile(`✻ Working\.\.\.`),
		regexp.MustCompile(`· Waiting\.\.\.`),
	}

	// Permission prompt patterns - Claude asking for tool permission
	permissionPromptPatterns = []*regexp.Regexp{
		regexp.MustCompile(`(?i)allow.*to.*\?`),
		regexp.MustCompile(`(?i)permission.*to.*\?`),
		regexp.MustCompile(`(?i)proceed.*\?`),
		regexp.MustCompile(`(?i)continue.*\?`),
		regexp.MustCompile(`\(y/n\)`),
		regexp.MustCompile(`\[y/n\]`),
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

	// Idle prompt - ❯ character indicating Claude is ready
	idlePromptPattern = regexp.MustCompile(`❯`)
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
func (p *PaneInfo) DetectPermissionPrompt() bool {
	// Check last 500 characters (recent output)
	recentContent := p.getRecentContent(500)

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

	// Check for waiting/spinner patterns
	indicators["waiting"] = false
	for _, pattern := range waitingPatterns {
		if pattern.MatchString(recentContent) {
			indicators["waiting"] = true
			break
		}
	}

	// Check for permission prompts
	indicators["permission_prompt"] = p.DetectPermissionPrompt()

	// Check for completion language
	indicators["completed"] = p.hasCompletionLanguage()

	// Check for idle prompt
	indicators["idle_prompt"] = p.hasIdlePrompt()

	// Check for zero token waiting (waiting pattern + no idle prompt)
	indicators["zero_token_waiting"] = indicators["waiting"] && !indicators["idle_prompt"]

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
// - No completion language
func (p *PaneInfo) IsStuck() bool {
	indicators := p.DetectStuckIndicators()

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
