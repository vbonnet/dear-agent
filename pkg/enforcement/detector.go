package enforcement

import (
	"regexp"
	"sort"
)

// Context provides environmental context for pattern matching.
type Context struct {
	HasWorktrees   bool
	IsMainWorktree bool
	WorkingDir     string
}

// ViolationDetector detects anti-pattern violations in commands and content.
// Patterns are pre-compiled at construction time for performance.
type ViolationDetector struct {
	patterns        *PatternDatabase
	compiledRegex   map[string]*regexp.Regexp
	skippedPatterns []string
}

// NewDetector creates a new violation detector from a pattern database.
// Patterns with unsupported regex features (e.g., lookahead/lookbehind) are
// skipped gracefully.
func NewDetector(patterns *PatternDatabase) (*ViolationDetector, error) {
	detector := &ViolationDetector{
		patterns:        patterns,
		compiledRegex:   make(map[string]*regexp.Regexp),
		skippedPatterns: make([]string, 0),
	}

	for i := range patterns.Patterns {
		pattern := &patterns.Patterns[i]
		regexStr := pattern.Regex
		if regexStr == "" {
			regexStr = pattern.RE2Regex
		}
		if regexStr == "" {
			continue
		}
		regex, err := regexp.Compile(regexStr)
		if err != nil {
			detector.skippedPatterns = append(detector.skippedPatterns, pattern.ID)
			continue
		}
		detector.compiledRegex[pattern.ID] = regex
	}

	return detector, nil
}

// NewDetectorRE2 creates a detector that uses only RE2-compatible patterns
// (the re2_regex field). This is suitable for PreToolUse hook enforcement
// where only RE2 regexes are supported.
func NewDetectorRE2(patterns *PatternDatabase) (*ViolationDetector, error) {
	detector := &ViolationDetector{
		patterns:        patterns,
		compiledRegex:   make(map[string]*regexp.Regexp),
		skippedPatterns: make([]string, 0),
	}

	for i := range patterns.Patterns {
		pattern := &patterns.Patterns[i]
		if pattern.RE2Regex == "" {
			continue
		}
		regex, err := regexp.Compile(pattern.RE2Regex)
		if err != nil {
			detector.skippedPatterns = append(detector.skippedPatterns, pattern.ID)
			continue
		}
		detector.compiledRegex[pattern.ID] = regex
	}

	return detector, nil
}

// GetSkippedPatterns returns the list of pattern IDs that couldn't be compiled.
func (d *ViolationDetector) GetSkippedPatterns() []string {
	return d.skippedPatterns
}

// Detect finds the first violation in the given content.
// Returns the matched pattern or nil if no violation is found.
func (d *ViolationDetector) Detect(content string) *Pattern {
	for i := range d.patterns.Patterns {
		pattern := &d.patterns.Patterns[i]
		regex, ok := d.compiledRegex[pattern.ID]
		if !ok {
			continue
		}
		if regex.MatchString(content) {
			return pattern
		}
	}
	return nil
}

// DetectWithContext performs context-aware violation detection.
func (d *ViolationDetector) DetectWithContext(content string, ctx Context) *Pattern {
	for i := range d.patterns.Patterns {
		pattern := &d.patterns.Patterns[i]
		regex, ok := d.compiledRegex[pattern.ID]
		if !ok {
			continue
		}
		if !regex.MatchString(content) {
			continue
		}
		if pattern.ContextCheck != "" && !checkContext(pattern, ctx) {
			continue
		}
		return pattern
	}
	return nil
}

// DetectAll finds all violations in the given content.
func (d *ViolationDetector) DetectAll(content string) []*Pattern {
	var violations []*Pattern
	for i := range d.patterns.Patterns {
		pattern := &d.patterns.Patterns[i]
		regex, ok := d.compiledRegex[pattern.ID]
		if !ok {
			continue
		}
		if regex.MatchString(content) {
			violations = append(violations, pattern)
		}
	}
	return violations
}

// DetectAllWithContext finds all violations with context awareness.
func (d *ViolationDetector) DetectAllWithContext(content string, ctx Context) []*Pattern {
	var violations []*Pattern
	for i := range d.patterns.Patterns {
		pattern := &d.patterns.Patterns[i]
		regex, ok := d.compiledRegex[pattern.ID]
		if !ok {
			continue
		}
		if !regex.MatchString(content) {
			continue
		}
		if pattern.ContextCheck != "" && !checkContext(pattern, ctx) {
			continue
		}
		violations = append(violations, pattern)
	}
	return violations
}

// ValidateCommand checks if a command contains forbidden patterns.
// Patterns are checked in order (by Order field); first match wins.
// Returns the matched pattern or nil if the command is allowed.
//
// This is the primary entry point for PreToolUse hook enforcement,
// replacing the generated patterns.go approach with runtime detection.
func (d *ViolationDetector) ValidateCommand(cmd string) *Pattern {
	// Build ordered list of active patterns with compiled regexes
	type ordered struct {
		order   int
		pattern *Pattern
		regex   *regexp.Regexp
	}
	var entries []ordered
	for i := range d.patterns.Patterns {
		p := &d.patterns.Patterns[i]
		if p.Relaxed || p.ConsolidatedInto != "" {
			continue
		}
		regex, ok := d.compiledRegex[p.ID]
		if !ok {
			continue
		}
		entries = append(entries, ordered{order: p.Order, pattern: p, regex: regex})
	}
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].order < entries[j].order
	})

	for _, e := range entries {
		if e.regex.MatchString(cmd) {
			return e.pattern
		}
	}
	return nil
}

func checkContext(pattern *Pattern, ctx Context) bool {
	switch pattern.ContextCheck {
	case "has_worktrees":
		return ctx.HasWorktrees
	case "is_main_worktree":
		return ctx.IsMainWorktree
	case "not_main_worktree":
		return !ctx.IsMainWorktree
	default:
		return true
	}
}
