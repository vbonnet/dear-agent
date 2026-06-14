package fsguard

import "strings"

// Enforcement controls how the guard reacts when a write falls outside the
// allowed policy. The default is EnforceDeny (hard block). Operators can
// soften enforcement via FSGUARD_ENFORCEMENT or the config file without
// recompiling hooks or changing policy carveouts.
//
// The values map directly to the Claude Code PreToolUse JSON hook protocol:
// the hook binary writes a JSON hook response to stdout for Warn/Ask/Defer
// and falls back to the legacy exit-2 / stderr approach for Deny.
type Enforcement int

const (
	// EnforceDeny is the default hard block: the hook exits with code 2 and
	// writes the guidance message to stderr. The tool call is prevented.
	EnforceDeny Enforcement = iota

	// EnforceWarn allows the action but surfaces the guidance message to the
	// agent via a JSON hook response so it can course-correct without being
	// blocked. Violations are still recorded.
	EnforceWarn

	// EnforceAsk defers the decision to the user: the hook emits a JSON hook
	// response with permissionDecision "ask" and the guidance as the question.
	// Claude Code prompts the user before proceeding.
	EnforceAsk

	// EnforceDefer passes the decision entirely to the normal Claude Code
	// permission model (settings.json allow/ask/deny rules). The hook exits 0
	// with JSON permissionDecision "defer". Violations are still recorded so
	// the retro audit captures attempted out-of-policy writes.
	EnforceDefer
)

// EnvEnforcement is the environment variable that overrides the enforcement
// level. Accepted values (case-insensitive): "deny", "warn", "ask", "defer".
// An unrecognized value is silently ignored and the configured level is kept.
const EnvEnforcement = "FSGUARD_ENFORCEMENT"

// ParseEnforcement converts a string to an Enforcement value, returning
// (level, true) on success or (EnforceDeny, false) if unrecognized.
func ParseEnforcement(s string) (Enforcement, bool) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "deny", "":
		return EnforceDeny, true
	case "warn":
		return EnforceWarn, true
	case "ask":
		return EnforceAsk, true
	case "defer":
		return EnforceDefer, true
	}
	return EnforceDeny, false
}

// String returns the canonical name of the enforcement level.
func (e Enforcement) String() string {
	switch e {
	case EnforceDeny:
		return "deny"
	case EnforceWarn:
		return "warn"
	case EnforceAsk:
		return "ask"
	case EnforceDefer:
		return "defer"
	}
	return "deny"
}

// HookResponse is the JSON structure emitted to stdout by the hook binaries for
// the Warn, Ask, and Defer enforcement levels. The Claude Code PreToolUse
// protocol reads this from stdout and applies the embedded permissionDecision.
type HookResponse struct {
	PermissionDecision string `json:"permissionDecision"`
	// Message is surfaced to the agent as context. Used for Warn (guidance
	// without blocking) and Ask (question shown before user prompt).
	Message string `json:"message,omitempty"`
}
