package daemon

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/vbonnet/dear-agent/pkg/enforcement"
)

// HookError represents a parsed PreTool hook error.
type HookError struct {
	Tool      string // tool name, e.g. "Bash"
	HookName  string // hook script name, e.g. "pretool-bash-blocker"
	Reason    string // human-readable reason for blocking
	PatternID string // matched pattern ID (populated by MatchPattern)
	Raw       string // original error text
}

// Hook error output patterns from Claude Code PreToolUse hooks:
//
//	"Hook PreToolUse:Bash denied this tool"
//	"Hook PreToolUse:Bash blocking error from command: \"~/.claude/hooks/pretool-bash-blocker\": reason text"
var (
	hookDeniedRe   = regexp.MustCompile(`Hook PreToolUse:(\w+) denied this tool`)
	hookBlockingRe = regexp.MustCompile(`Hook PreToolUse:(\w+) (?:denied|blocking error from) (?:this tool|command): "([^"]+)"(?:: (.+))?`)
)

// ParseHookError parses a single hook error line into a HookError.
// Returns an error if the input doesn't match any known hook error format.
func ParseHookError(raw string) (*HookError, error) {
	raw = strings.TrimSpace(raw)

	// Try blocking error format first (more specific)
	if m := hookBlockingRe.FindStringSubmatch(raw); m != nil {
		hookName := extractHookName(m[2])
		return &HookError{
			Tool:     m[1],
			HookName: hookName,
			Reason:   strings.TrimSpace(m[3]),
			Raw:      raw,
		}, nil
	}

	// Try simple denied format
	if m := hookDeniedRe.FindStringSubmatch(raw); m != nil {
		return &HookError{
			Tool: m[1],
			Raw:  raw,
		}, nil
	}

	return nil, fmt.Errorf("unrecognized hook error format: %s", raw)
}

// ParseHookErrors extracts all hook errors from multi-line content.
// Lines that don't match a hook error format are silently skipped.
func ParseHookErrors(content string) []*HookError {
	var errors []*HookError
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if he, err := ParseHookError(line); err == nil {
			errors = append(errors, he)
		}
	}
	return errors
}

// MatchPattern attempts to match this hook error's reason against the given
// violation detector's pattern database. If a match is found, sets PatternID
// and returns the matched pattern.
func (e *HookError) MatchPattern(detector *enforcement.ViolationDetector) *enforcement.Pattern {
	if detector == nil || e.Reason == "" {
		return nil
	}

	pattern := detector.Detect(e.Reason)
	if pattern == nil {
		return nil
	}

	e.PatternID = pattern.ID
	return pattern
}

// extractHookName returns the base name from a hook path.
// e.g. "~/.claude/hooks/pretool-bash-blocker" -> "pretool-bash-blocker"
func extractHookName(path string) string {
	if idx := strings.LastIndex(path, "/"); idx >= 0 {
		return path[idx+1:]
	}
	return path
}
