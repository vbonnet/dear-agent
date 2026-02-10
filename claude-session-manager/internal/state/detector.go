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

	// StateStuck indicates no token output for > threshold duration
	StateStuck State = "stuck"

	// StateUnknown indicates unable to determine state
	StateUnknown State = "unknown"
)

// DetectionResult contains state detection outcome
type DetectionResult struct {
	State     State     // Detected state
	Timestamp time.Time // When detection occurred
	Evidence  string    // Text evidence for detection
	Confidence string   // high, medium, low
}

// Detector provides visual parsing of Claude Code session state
type Detector struct {
	// Regex patterns for state detection
	thinkingPattern     *regexp.Regexp
	blockedAuthPattern  *regexp.Regexp
	blockedInputPattern *regexp.Regexp
	readyPattern        *regexp.Regexp

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
		// Looks for numbered options (1., 2., 3.) or lettered (A., B., C.)
		// followed by question patterns
		blockedInputPattern: regexp.MustCompile(
			`(?m)` + // Multiline mode
			`(` +
				`\b(?:1\.|2\.|3\.|A\.|B\.|C\.)\s+` + // Numbered/lettered options
				`|` +
				`(?:Choose|Select|Pick|Which).*:` + // Choice keywords
				`|` +
				`\[.*\].*\[.*\]` + // [Option 1] [Option 2] pattern
			`)`,
		),

		// Ready: Claude prompt (❯) at end of output
		readyPattern: regexp.MustCompile(`❯\s*$`),

		// Stuck threshold: 60 seconds of no tokens
		stuckThreshold: 60 * time.Second,
	}
}

// DetectState analyzes pane output to determine current state
func (d *Detector) DetectState(output string, lastOutputTime time.Time) DetectionResult {
	now := time.Now()

	// Priority order: Blocked states > Thinking > Stuck > Ready > Unknown

	// 1. Check for blocked auth (highest priority - needs immediate user response)
	if d.blockedAuthPattern.MatchString(output) {
		evidence := d.extractEvidence(output, d.blockedAuthPattern, 100)
		return DetectionResult{
			State:      StateBlockedAuth,
			Timestamp:  now,
			Evidence:   evidence,
			Confidence: "high",
		}
	}

	// 2. Check for blocked input (AskUserQuestion)
	if d.blockedInputPattern.MatchString(output) {
		evidence := d.extractEvidence(output, d.blockedInputPattern, 150)
		return DetectionResult{
			State:      StateBlockedInput,
			Timestamp:  now,
			Evidence:   evidence,
			Confidence: "high",
		}
	}

	// 3. Check for thinking (spinner visible)
	if d.thinkingPattern.MatchString(output) {
		evidence := d.extractEvidence(output, d.thinkingPattern, 50)
		return DetectionResult{
			State:      StateThinking,
			Timestamp:  now,
			Evidence:   evidence,
			Confidence: "high",
		}
	}

	// 4. Check for stuck (no output for > threshold)
	timeSinceLastOutput := now.Sub(lastOutputTime)
	if timeSinceLastOutput > d.stuckThreshold {
		// Only consider stuck if NOT at ready prompt
		if !d.readyPattern.MatchString(output) {
			return DetectionResult{
				State:      StateStuck,
				Timestamp:  now,
				Evidence:   fmt.Sprintf("No tokens for %v", timeSinceLastOutput),
				Confidence: "medium",
			}
		}
	}

	// 5. Check for ready (Claude prompt at end)
	if d.readyPattern.MatchString(output) {
		return DetectionResult{
			State:      StateReady,
			Timestamp:  now,
			Evidence:   "Claude prompt (❯) detected",
			Confidence: "high",
		}
	}

	// 6. Unknown state
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
	return s == StateBlockedAuth || s == StateBlockedInput
}

// IsActive returns true if Claude is actively processing
func (s State) IsActive() bool {
	return s == StateThinking
}

// IsIdle returns true if Claude is waiting for input
func (s State) IsIdle() bool {
	return s == StateReady
}
