package enforcement

import (
	"regexp"
)

// Context provides environmental context for pattern matching.
// Used for context-aware violation detection (e.g., git worktree checks).
type Context struct {
	HasWorktrees   bool   // Whether the repository has worktrees
	IsMainWorktree bool   // Whether the current directory is the main worktree
	WorkingDir     string // Current working directory
}

// ViolationDetector detects anti-pattern violations in commands and content.
type ViolationDetector struct {
	patterns       *PatternDatabase
	compiledRegex  map[string]*regexp.Regexp
	skippedPatterns []string // Patterns that couldn't be compiled (e.g., unsupported regex features)
}

// NewDetector creates a new violation detector from a pattern database.
// Patterns are pre-compiled for performance. Patterns with unsupported regex
// features (e.g., lookahead/lookbehind) are skipped and logged.
func NewDetector(patterns *PatternDatabase) (*ViolationDetector, error) {
	detector := &ViolationDetector{
		patterns:        patterns,
		compiledRegex:   make(map[string]*regexp.Regexp),
		skippedPatterns: make([]string, 0),
	}

	// Pre-compile all regex patterns
	// Skip patterns with unsupported features (Go's RE2 doesn't support lookahead/lookbehind)
	for i := range patterns.Patterns {
		pattern := &patterns.Patterns[i]
		regex, err := regexp.Compile(pattern.Regex)
		if err != nil {
			// Skip patterns that can't be compiled (e.g., unsupported Perl regex features)
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
func (d *ViolationDetector) Detect(content string) (*Pattern, error) {
	for i := range d.patterns.Patterns {
		pattern := &d.patterns.Patterns[i]
		regex, ok := d.compiledRegex[pattern.ID]
		if !ok {
			// Skip patterns that couldn't be compiled
			continue
		}

		if regex.MatchString(content) {
			return pattern, nil
		}
	}

	return nil, nil
}

// DetectWithContext performs context-aware violation detection.
// Some patterns require environmental context (e.g., checking if worktrees exist).
// Returns the matched pattern or nil if no violation is found.
func (d *ViolationDetector) DetectWithContext(content string, ctx Context) (*Pattern, error) {
	for i := range d.patterns.Patterns {
		pattern := &d.patterns.Patterns[i]
		regex, ok := d.compiledRegex[pattern.ID]
		if !ok {
			// Skip patterns that couldn't be compiled
			continue
		}

		// First check if the regex matches
		if !regex.MatchString(content) {
			continue
		}

		// If pattern has context check, validate it
		if pattern.ContextCheck != "" {
			if !d.checkContext(pattern, ctx) {
				continue
			}
		}

		return pattern, nil
	}

	return nil, nil
}

// checkContext validates context-specific conditions for a pattern.
func (d *ViolationDetector) checkContext(pattern *Pattern, ctx Context) bool {
	// Context checks are pattern-specific
	// For now, we implement common checks
	switch pattern.ContextCheck {
	case "has_worktrees":
		return ctx.HasWorktrees
	case "is_main_worktree":
		return ctx.IsMainWorktree
	case "not_main_worktree":
		return !ctx.IsMainWorktree
	default:
		// Unknown context check - assume it passes
		return true
	}
}

// DetectAll finds all violations in the given content.
// Returns a slice of all matched patterns.
func (d *ViolationDetector) DetectAll(content string) ([]*Pattern, error) {
	var violations []*Pattern

	for i := range d.patterns.Patterns {
		pattern := &d.patterns.Patterns[i]
		regex, ok := d.compiledRegex[pattern.ID]
		if !ok {
			// Skip patterns that couldn't be compiled
			continue
		}

		if regex.MatchString(content) {
			violations = append(violations, pattern)
		}
	}

	return violations, nil
}

// DetectAllWithContext finds all violations with context awareness.
func (d *ViolationDetector) DetectAllWithContext(content string, ctx Context) ([]*Pattern, error) {
	var violations []*Pattern

	for i := range d.patterns.Patterns {
		pattern := &d.patterns.Patterns[i]
		regex, ok := d.compiledRegex[pattern.ID]
		if !ok {
			// Skip patterns that couldn't be compiled
			continue
		}

		if !regex.MatchString(content) {
			continue
		}

		if pattern.ContextCheck != "" {
			if !d.checkContext(pattern, ctx) {
				continue
			}
		}

		violations = append(violations, pattern)
	}

	return violations, nil
}
