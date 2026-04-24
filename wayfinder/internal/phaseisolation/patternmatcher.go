package phaseisolation

import (
	"regexp"
	"strings"
)

// MatchOptions controls pattern matching behavior.
type MatchOptions struct {
	CaseSensitive bool // Default: false
	WholeWord     bool // Default: true
}

// DefaultMatchOptions returns the default match options.
func DefaultMatchOptions() MatchOptions {
	return MatchOptions{
		CaseSensitive: false,
		WholeWord:     true,
	}
}

// MatchesPattern tests if text matches a pipe-separated pattern string.
// Pattern examples: "because|issue|problem", "must|constraint|requirement"
func MatchesPattern(text, pattern string, opts ...MatchOptions) bool {
	if text == "" || pattern == "" {
		return false
	}

	opt := DefaultMatchOptions()
	if len(opts) > 0 {
		opt = opts[0]
	}

	regexPattern := pattern
	if opt.WholeWord {
		regexPattern = `\b(` + regexPattern + `)\b`
	}

	flags := ""
	if !opt.CaseSensitive {
		flags = "(?i)"
	}

	re, err := regexp.Compile(flags + regexPattern)
	if err != nil {
		return false
	}

	return re.MatchString(text)
}

// MatchesRegexp tests if text matches a compiled regexp.
func MatchesRegexp(text string, re *regexp.Regexp) bool {
	if text == "" || re == nil {
		return false
	}
	return re.MatchString(text)
}

// MatchesAnyPattern tests if text matches any of the given patterns.
func MatchesAnyPattern(text string, patterns []string, opts ...MatchOptions) bool {
	for _, pattern := range patterns {
		if MatchesPattern(text, pattern, opts...) {
			return true
		}
	}
	return false
}

// CountWords counts words in text.
func CountWords(text string) int {
	fields := strings.Fields(strings.TrimSpace(text))
	count := 0
	for _, f := range fields {
		if len(f) > 0 {
			count++
		}
	}
	return count
}
