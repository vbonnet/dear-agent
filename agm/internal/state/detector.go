// Package state provides state functionality.
package state

import (
	"fmt"
	"regexp"
	"strings"
	"time"
)

// State represents Claude Code session states
type State string

const (
	// StateReady indicates Claude is idle, waiting for input
	StateReady State = "ready"

	// StateThinking indicates Claude is processing (spinner visible)
	StateThinking State = "thinking"

	// StateBlockedAuth indicates y/N authentication prompt
	StateBlockedAuth State = "blocked_auth"

	// StateBlockedInput indicates AskUserQuestion prompt
	StateBlockedInput State = "blocked_input"

	// StateBlockedPermission indicates a permission prompt (Do you want to proceed?)
	StateBlockedPermission State = "blocked_permission"

	// StateStuck indicates no token output for > threshold duration
	StateStuck State = "stuck"

	// StateWaitingAgent indicates waiting for a sub-agent or background task
	StateWaitingAgent State = "waiting_agent"

	// StateLooping indicates an orchestrator or monitoring loop
	StateLooping State = "looping"

	// StateBackgroundTasksView indicates the "Background Tasks" read-only overlay is open.
	// This overlay blocks message delivery but can be dismissed by sending Left/Escape.
	StateBackgroundTasksView State = "background_tasks_view"

	// StateUnknown indicates unable to determine state
	StateUnknown State = "unknown"
)

// DetectionResult contains state detection outcome
type DetectionResult struct {
	State      State     // Detected state
	Timestamp  time.Time // When detection occurred
	Evidence   string    // Text evidence for detection
	Confidence string    // high, medium, low
}

// Detector provides visual parsing of Claude Code session state
type Detector struct {
	// Regex patterns for state detection
	thinkingPattern            *regexp.Regexp
	blockedAuthPattern         *regexp.Regexp
	blockedInputPattern        *regexp.Regexp
	blockedPermissionPattern   *regexp.Regexp
	readyPattern               *regexp.Regexp
	waitingAgentPattern        *regexp.Regexp
	loopingPattern             *regexp.Regexp
	backgroundTasksPattern     *regexp.Regexp

	// Stuck detection threshold
	stuckThreshold time.Duration
}

// NewDetector creates a new state detector with default patterns
func NewDetector() *Detector {
	return &Detector{
		// Thinking: Spinner characters (⣾ ⣽ ⣻ ⢿ ⡿ ⣟ ⣯ ⣷)
		thinkingPattern: regexp.MustCompile(`[⣾⣽⣻⢿⡿⣟⣯⣷]`),

		// Blocked Auth: "y/N" or "Y/n" patterns (case insensitive)
		blockedAuthPattern: regexp.MustCompile(`(?i)\b([yY]/[nN]|[nN]/[yY])\b`),

		// Blocked Input: AskUserQuestion indicators
		// Must have BOTH a question keyword AND numbered/lettered options to avoid
		// false positives from Claude's output containing numbered lists.
		blockedInputPattern: regexp.MustCompile(
			`(?ms)` + // Multiline + dotall mode
				`(` +
				`(?:Choose|Select|Pick|Which).*[:?][\s\S]*?\b(?:1\.|2\.|3\.|A\.|B\.|C\.)\s+` + // Question keyword followed by options
				`|` +
				`\b(?:1\.|2\.|3\.|A\.|B\.|C\.)\s+[\s\S]*?(?:Choose|Select|Pick|Which).*[:?]` + // Options followed by question keyword
				`|` +
				`\[.*\].*\[.*\]` + // [Option 1] [Option 2] pattern (already specific enough)
				`)`,
		),

		// Blocked Permission: Claude Code permission prompt with ❯ selector
		// Pattern: "Do you want to proceed?" followed by "❯ 1. Yes" / "2. No"
		// This MUST be checked before the ready pattern because the ❯ in
		// "❯ 1. Yes" is a selector arrow, not the idle prompt.
		blockedPermissionPattern: regexp.MustCompile(
			`(?ms)` +
				`(` +
				`Do you want to proceed\?` + // Claude Code permission question
				`|` +
				`❯\s+\d+\.\s+(?:Yes|No|Allow|Deny|Approve|Reject)` + // ❯ selector on Yes/No option
				`)`,
		),

		// Ready: Claude prompt (❯) at end of a line in the output.
		// Uses (?m) so $ matches end-of-line, not just end-of-text.
		// Includes \x{00a0} (NBSP) because Claude Code renders ❯ followed by NBSP.
		// The pane may have a status bar (━━━) and trailing blank lines after the prompt.
		readyPattern: regexp.MustCompile(`(?m)❯[\s\x{00a0}]*$`),

		// Waiting Agent: sub-agent or background task indicators
		// Matches "Agent:", "Launching agent", "agent to", spinner with agent context
		waitingAgentPattern: regexp.MustCompile(
			`(?mi)` +
				`(` +
				`\bAgent\b.*(?:running|launched|starting|working)` +
				`|` +
				`(?:Launching|Spawning|Starting)\s+(?:agent|sub-?agent)` +
				`|` +
				`\bagent\s+to\s+` + // "I'll use the agent to..."
				`|` +
				`run_in_background` +
				`)`,
		),

		// Looping: orchestrator or monitoring loop indicators
		loopingPattern: regexp.MustCompile(
			`(?mi)` +
				`(` +
				`/loop\s+` +
				`|` +
				`(?:iteration|interval|polling|monitoring)\s+\d+` +
				`|` +
				`(?:Checking|Polling|Monitoring)\s+(?:every|each)\s+` +
				`|` +
				`(?:Running|Executing)\s+(?:iteration|cycle)\b` +
				`)`,
		),

		// Background Tasks: Claude Code read-only overlay
		// Pattern: "Background tasks" heading AND navigation hint on same screen.
		// The overlay shows: "Background tasks / No tasks currently running /
		// ↑/↓ to select · Enter to view · ←/Esc to close"
		backgroundTasksPattern: regexp.MustCompile(
			`(?ms)Background tasks[\s\S]*?to select.*(?:Esc|←).*to close`,
		),

		// Stuck threshold: 60 seconds of no tokens
		stuckThreshold: 60 * time.Second,
	}
}

// DetectState analyzes pane output to determine current state
func (d *Detector) DetectState(output string, lastOutputTime time.Time) DetectionResult {
	now := time.Now()

	// Priority order: Permission > Ready > Thinking > Blocked > Stuck > Unknown
	//
	// Permission prompts are checked FIRST because they contain ❯ as a selector
	// arrow (e.g., "❯ 1. Yes"), which the ready pattern would otherwise match.
	// Ready is checked next because the ❯ prompt at the end of output is the
	// strongest signal for idle state.

	// 1. Check for permission prompt (❯ as selector, not idle prompt)
	// Permission prompts show: "Do you want to proceed?\n❯ 1. Yes\n  2. No"
	// The ❯ here is a menu selector, not the Claude prompt.
	if d.blockedPermissionPattern.MatchString(output) {
		evidence := d.extractEvidence(output, d.blockedPermissionPattern, 80)
		return DetectionResult{
			State:      StateBlockedPermission,
			Timestamp:  now,
			Evidence:   evidence,
			Confidence: "high",
		}
	}

	// 1b. Check for Background Tasks overlay (blocks input even if prompt is visible underneath)
	if d.backgroundTasksPattern.MatchString(output) {
		evidence := d.extractEvidence(output, d.backgroundTasksPattern, 80)
		return DetectionResult{
			State:      StateBackgroundTasksView,
			Timestamp:  now,
			Evidence:   evidence,
			Confidence: "high",
		}
	}

	// 2. Check for ready (Claude prompt at end — highest priority after permission)
	if d.readyPattern.MatchString(output) {
		return DetectionResult{
			State:      StateReady,
			Timestamp:  now,
			Evidence:   "Claude prompt (❯) detected",
			Confidence: "high",
		}
	}

	// 2. Check for waiting agent (spinner + agent markers — more specific than thinking)
	if d.thinkingPattern.MatchString(output) && d.waitingAgentPattern.MatchString(output) {
		evidence := d.extractEvidence(output, d.waitingAgentPattern, 80)
		return DetectionResult{
			State:      StateWaitingAgent,
			Timestamp:  now,
			Evidence:   evidence,
			Confidence: "medium",
		}
	}

	// 3. Check for looping (spinner + loop/monitor keywords — requires spinner to avoid
	//    false positives from historical output mentioning "iteration", "monitoring", etc.)
	if d.thinkingPattern.MatchString(output) && d.loopingPattern.MatchString(output) {
		evidence := d.extractEvidence(output, d.loopingPattern, 80)
		return DetectionResult{
			State:      StateLooping,
			Timestamp:  now,
			Evidence:   evidence,
			Confidence: "medium",
		}
	}

	// 4. Check for thinking (spinner visible means actively working)
	if d.thinkingPattern.MatchString(output) {
		evidence := d.extractEvidence(output, d.thinkingPattern, 50)
		return DetectionResult{
			State:      StateThinking,
			Timestamp:  now,
			Evidence:   evidence,
			Confidence: "high",
		}
	}

	// 5. Check for blocked auth (y/N prompt — only matters if no ready prompt visible)
	if d.blockedAuthPattern.MatchString(output) {
		evidence := d.extractEvidence(output, d.blockedAuthPattern, 100)
		return DetectionResult{
			State:      StateBlockedAuth,
			Timestamp:  now,
			Evidence:   evidence,
			Confidence: "high",
		}
	}

	// 6. Check for blocked input (numbered options — only matters if no ready prompt visible)
	if d.blockedInputPattern.MatchString(output) {
		evidence := d.extractEvidence(output, d.blockedInputPattern, 150)
		return DetectionResult{
			State:      StateBlockedInput,
			Timestamp:  now,
			Evidence:   evidence,
			Confidence: "high",
		}
	}

	// 7. Check for stuck (no output for > threshold)
	timeSinceLastOutput := now.Sub(lastOutputTime)
	if timeSinceLastOutput > d.stuckThreshold {
		return DetectionResult{
			State:      StateStuck,
			Timestamp:  now,
			Evidence:   fmt.Sprintf("No tokens for %v", timeSinceLastOutput),
			Confidence: "medium",
		}
	}

	// 8. Unknown state
	return DetectionResult{
		State:      StateUnknown,
		Timestamp:  now,
		Evidence:   "No recognizable pattern",
		Confidence: "low",
	}
}

// extractEvidence extracts context around matched pattern
func (d *Detector) extractEvidence(output string, pattern *regexp.Regexp, contextChars int) string {
	match := pattern.FindStringIndex(output)
	if match == nil {
		return ""
	}

	start := match[0] - contextChars
	if start < 0 {
		start = 0
	}

	end := match[1] + contextChars
	if end > len(output) {
		end = len(output)
	}

	evidence := output[start:end]

	// Truncate to single line if multiline
	lines := strings.Split(evidence, "\n")
	if len(lines) > 3 {
		evidence = strings.Join(lines[:3], "\n") + "..."
	}

	return strings.TrimSpace(evidence)
}

// SetStuckThreshold allows customizing stuck detection duration
func (d *Detector) SetStuckThreshold(duration time.Duration) {
	d.stuckThreshold = duration
}

// String returns human-readable state name
func (s State) String() string {
	return string(s)
}

// IsBlocked returns true if state requires user intervention
func (s State) IsBlocked() bool {
	return s == StateBlockedAuth || s == StateBlockedInput || s == StateBlockedPermission
}

// IsOverlay returns true if state is a dismissible UI overlay that can be auto-recovered
func (s State) IsOverlay() bool {
	return s == StateBackgroundTasksView
}

// IsActive returns true if Claude is actively processing
func (s State) IsActive() bool {
	return s == StateThinking || s == StateWaitingAgent || s == StateLooping
}

// IsIdle returns true if Claude is waiting for input
func (s State) IsIdle() bool {
	return s == StateReady
}

// IsWaiting returns true if session is waiting for external response
func (s State) IsWaiting() bool {
	return s == StateWaitingAgent
}

// CheckCanReceive determines if the session can accept input based on pane content.
// This is orthogonal to display state — it only cares about whether typing into
// the tmux pane right now would be received by Claude as a new prompt.
//
// Returns:
//   - CanReceiveYes:   prompt (❯) visible at end, no permission dialog → send directly
//   - CanReceiveNo:    permission dialog active → cannot send, needs human intervention
//   - CanReceiveQueue: no prompt visible (busy/working) → queue for later delivery
//
// Note: Alive status (Stopped/Archived/NotFound) is checked at the session level.
func (d *Detector) CheckCanReceive(output string) CanReceive {
	// Permission dialog blocks input — ❯ appears as selector, not prompt
	if d.blockedPermissionPattern.MatchString(output) {
		return CanReceiveNo
	}

	// Background Tasks overlay blocks input but is auto-dismissible
	if d.backgroundTasksPattern.MatchString(output) {
		return CanReceiveOverlay
	}

	// Prompt chevron at end of output = session is at idle prompt, can receive
	if d.readyPattern.MatchString(output) {
		return CanReceiveYes
	}

	// No prompt visible = session is busy, queue for later
	return CanReceiveQueue
}

// CanReceive represents whether a session can accept input right now.
type CanReceive string

const (
	CanReceiveYes      CanReceive = "YES"       // Prompt visible, send directly
	CanReceiveNo       CanReceive = "NO"        // Permission dialog, needs human
	CanReceiveQueue    CanReceive = "QUEUE"     // Busy, queue for later
	CanReceiveOverlay  CanReceive = "OVERLAY"   // Dismissible overlay active, auto-recoverable
	CanReceiveNotFound CanReceive = "NOT_FOUND" // Tmux session does not exist
)
