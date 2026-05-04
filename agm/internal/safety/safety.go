// Package safety provides session interaction safety guards.
//
// Before sending messages, approving prompts, or performing other automated
// interactions with tmux sessions, callers should run safety checks to avoid
// interfering with human operators or uninitialized sessions.
//
// The three guards detect:
//   - HumanTyping: unsent text in the prompt line (human mid-keystroke)
//   - SessionUninitialized: Claude hasn't started or is showing welcome screen
//   - ClaudeMidResponse: Claude is actively generating a response (spinner visible)
package safety

import "fmt"

// GuardViolation identifies which safety condition was triggered.
type GuardViolation string

// Recognized safety guard violation values.
const (
	ViolationHumanTyping          GuardViolation = "human_typing"
	ViolationSessionUninitialized GuardViolation = "session_uninitialized"
	ViolationClaudeMidResponse    GuardViolation = "claude_mid_response"
)

// Violation describes a single triggered guard.
type Violation struct {
	Guard      GuardViolation `json:"guard"`
	Message    string         `json:"message"`
	Suggestion string         `json:"suggestion"`
	Evidence   string         `json:"evidence,omitempty"`
}

// CheckResult is returned by safety checks.
type CheckResult struct {
	Safe       bool        `json:"safe"`
	Violations []Violation `json:"violations,omitempty"`
}

// HasViolation returns true if the result contains the given violation type.
func (r *CheckResult) HasViolation(v GuardViolation) bool {
	for _, viol := range r.Violations {
		if viol.Guard == v {
			return true
		}
	}
	return false
}

// Error returns a formatted multi-violation error string.
func (r *CheckResult) Error() string {
	if r.Safe {
		return ""
	}
	msg := ""
	for _, v := range r.Violations {
		msg += fmt.Sprintf("  %s: %s\n  -> %s\n\n", v.Guard, v.Message, v.Suggestion)
	}
	return msg
}

// GuardOptions controls which guards to run and overrides.
type GuardOptions struct {
	SkipHumanTyping   bool   // Skip human typing detection
	SkipUninitialized bool   // Skip session uninitialized detection
	SkipMidResponse   bool   // Skip Claude mid-response detection
	SocketPath        string // Override tmux socket path (empty = auto-detect)
}
