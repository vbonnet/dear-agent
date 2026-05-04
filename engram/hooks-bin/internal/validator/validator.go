package validator

import (
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// getLogFile returns the log file path using the user's home directory.
func getLogFile() string {
	home, err := os.UserHomeDir()
	if err != nil {
		home = os.TempDir()
	}
	return filepath.Join(home, ".claude", "hooks", "logs", "pretool-bash-blocker.log")
}

// logDebug writes debug information to the log file
// Silently fails if logging is not possible (fail-open design)
func logDebug(format string, args ...any) {
	f, err := os.OpenFile(getLogFile(), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600)
	if err != nil {
		return // Silent fail if can't log
	}
	defer f.Close()
	timestamp := time.Now().Format("2006-01-02 15:04:05")
	fmt.Fprintf(f, "[%s] VALIDATOR: "+format+"\n", append([]any{timestamp}, args...)...)
}

// stripQuotedContents replaces the contents of quoted strings with safe
// placeholders so that English words inside --prompt "..." or -m "..."
// don't trigger bash-syntax patterns. Single-quoted contents are removed
// entirely (they're literal in bash). Double-quoted contents are removed
// except for $( sequences, which are executable command substitutions.
func stripQuotedContents(cmd string) string {
	var result []byte
	i := 0
	for i < len(cmd) {
		switch cmd[i] {
		case '\'':
			// Single quote: everything inside is literal, skip it all
			result = append(result, '\'')
			i++
			for i < len(cmd) && cmd[i] != '\'' {
				i++
			}
			if i < len(cmd) {
				result = append(result, '\'')
				i++
			}
		case '"':
			// Double quote: skip content but preserve $( sequences
			result = append(result, '"')
			i++
			for i < len(cmd) && cmd[i] != '"' {
				switch {
				case cmd[i] == '\\' && i+1 < len(cmd):
					// Skip escaped character pairs
					i += 2
				case cmd[i] == '$' && i+1 < len(cmd) && cmd[i+1] == '(':
					// Preserve $( — it's an executable substitution
					result = append(result, '$', '(')
					i += 2
				default:
					// Drop other content
					i++
				}
			}
			if i < len(cmd) {
				result = append(result, '"')
				i++
			}
		default:
			result = append(result, cmd[i])
			i++
		}
	}
	return string(result)
}

// pipeExemptPatterns lists pattern indices that should only be checked against
// the first segment of a pipeline. These are "standalone tool redirect" patterns
// that block commands like grep, tail, head when used standalone but should
// allow them as pipe targets (e.g., `tmux capture-pane | tail -20`).
var pipeExemptPatterns = map[int]bool{
	10: true, // find
	20: true, // ls
	21: true, // grep/rg
	22: true, // cat
	23: true, // head/tail
	24: true, // sed
	25: true, // awk
	26: true, // echo/printf
}

// firstPipeSegment returns the portion of cmd before the first pipe operator.
// Handles || (logical OR) by not splitting on it — only single | is a pipe.
// Operates on already-quote-stripped input so | inside quotes is already removed.
func firstPipeSegment(cmd string) string {
	for i := 0; i < len(cmd); i++ {
		if cmd[i] == '|' {
			if i+1 < len(cmd) && cmd[i+1] == '|' {
				i++ // skip ||
				continue
			}
			return cmd[:i]
		}
	}
	return cmd
}

// Result represents the outcome of command validation.
type Result struct {
	Allowed     bool
	PatternName string
	Remediation string
	Mode        Mode // ModeBlock or ModeWarn; empty string if no pattern matched
}

// ValidateCommand checks if command contains forbidden bash patterns.
// Returns (true, "", "") if allowed, (false, patternName, remediation) if blocked.
func ValidateCommand(cmd string) (bool, string, string) {
	r := ValidateCommandWithConfig(cmd, nil)
	return r.Allowed, r.PatternName, r.Remediation
}

// ValidateCommandWithConfig checks command against patterns using the given config.
// If cfg is nil, DefaultConfig() is used (all patterns block).
// When a pattern's category is set to ModeWarn, the command is allowed but
// Result.Mode is set to ModeWarn and the pattern info is populated.
func ValidateCommandWithConfig(cmd string, cfg *Config) Result {
	if cfg == nil {
		cfg = DefaultConfig()
	}
	logDebug("Checking %d patterns against command", len(forbiddenPatterns))

	// Strip quoted string contents to avoid false positives on English words
	// that look like bash syntax (e.g. "check the loop" triggering while-loop pattern).
	stripped := stripQuotedContents(cmd)

	// For pipe-exempt patterns, only check the first pipeline segment.
	firstSeg := firstPipeSegment(stripped)

	for i, pattern := range forbiddenPatterns {
		target := stripped
		if pipeExemptPatterns[i] {
			target = firstSeg
		}
		if pattern.MatchString(target) {
			// Try to generate a specific tool call suggestion with extracted args
			// Use original cmd for arg extraction (quotes intact)
			remedy := SuggestToolCall(i, cmd)
			if remedy == "" && i < len(remediations) {
				// Fall back to generic remediation
				remedy = remediations[i]
			}

			cat := PatternCategory(i)
			mode := cfg.CategoryMode(cat)

			logDebug("Pattern #%d MATCHED: %s (category: %s, mode: %s)", i, patternNames[i], cat, mode)

			if mode == ModeWarn {
				return Result{
					Allowed:     true,
					PatternName: patternNames[i],
					Remediation: remedy,
					Mode:        ModeWarn,
				}
			}

			return Result{
				Allowed:     false,
				PatternName: patternNames[i],
				Remediation: remedy,
				Mode:        ModeBlock,
			}
		}
	}

	logDebug("No patterns matched - command allowed")
	return Result{Allowed: true}
}
