// Package safety provides session interaction safety guards.
//
// Before sending messages, approving prompts, or performing other automated
// interactions with tmux sessions, callers should run safety checks to avoid
// interfering with human operators or uninitialized sessions.
//
// The guards detect:
//   - HumanTyping: unsent text in the prompt line (ADVISORY, non-blocking).
//   - SessionUninitialized: the selected harness has not reached a managed
//     interactive or working state.
//   - ClaudeMidResponse: Claude is actively generating a response (spinner visible).
//
// HumanTyping is ADVISORY only and never sets CheckResult.Safe=false. It is
// known to OVER-CAPTURE — stale scrollback, old prompts, and Claude Code
// ghost-text remnants all read as "a human is typing" — and blocking on it was
// the #1 cause of VROOM mesh stalls and reaper/dispatch failures (the ce-v9in
// family). Instead of blocking, the send path stashes the composer (Claude
// Code's C-s auto-unstashes after the next submit, preserving genuine human
// input) and delivers anyway, recording over-capture via telemetry so the
// heuristic can be improved later. HumanTyping detections are reported in
// CheckResult.Advisories (for observability / recovery introspection), not in
// Violations. See internal/tmux/stash.go and internal/telemetry human_typing.*.
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

// CheckResult is returned by safety checks. Violations are blocking; Advisories
// are non-blocking observations (currently only human_typing) surfaced for
// telemetry and recovery introspection and never affect Safe.
type CheckResult struct {
	Safe       bool        `json:"safe"`
	Violations []Violation `json:"violations,omitempty"`
	Advisories []Violation `json:"advisories,omitempty"`
}

// HasViolation returns true if the result contains the given blocking violation.
func (r *CheckResult) HasViolation(v GuardViolation) bool {
	for _, viol := range r.Violations {
		if viol.Guard == v {
			return true
		}
	}
	return false
}

// HasAdvisory returns true if the result contains the given advisory (non-
// blocking) observation, e.g. an over-capturing human_typing detection.
func (r *CheckResult) HasAdvisory(v GuardViolation) bool {
	for _, adv := range r.Advisories {
		if adv.Guard == v {
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
	AutonomousMode    bool   // Session is unattended; skip human_typing guard and enable cooldown
	Harness           string // Agent harness; empty preserves Claude Code behavior
}
