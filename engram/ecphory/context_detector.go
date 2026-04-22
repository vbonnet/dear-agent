package ecphory

import (
	"regexp"
	"strings"

	"github.com/vbonnet/dear-agent/engram/internal/reflection"
)

// ContextDetector identifies the type of context from a query.
// It detects when a query is in a debugging/failure context vs normal retrieval,
// and identifies the relevant error category for failure boosting.
//
// Task 1.3.1: Implement failure context detection
//
// Usage:
//
//	detector := NewContextDetector()
//	isDebugging, category := detector.DetectContext("why did my API call fail?")
//	// isDebugging = true, category = "other"
type ContextDetector struct {
	// Keywords that indicate debugging context
	debuggingKeywords *regexp.Regexp

	// Patterns for specific error categories
	syntaxPatterns     *regexp.Regexp
	toolMisusePatterns *regexp.Regexp
	permissionPatterns *regexp.Regexp
	timeoutPatterns    *regexp.Regexp
}

// NewContextDetector creates a new context detector with pre-compiled patterns
func NewContextDetector() *ContextDetector {
	return &ContextDetector{
		// Keywords: error, failed, broken, bug, issue, problem, crash, exception, failure
		// Also includes: debugging, troubleshoot, fix, resolve, diagnose
		debuggingKeywords: regexp.MustCompile(`(?i)\b(error|fails?|failed|failing|broken|bugs?|issues?|problems?|crash(ed)?|exceptions?|failures?|debugging?|troubleshoot(ing)?|fix(ing)?|resolve|diagnos(e|ing)|wrong|incorrect|not work(ing)?)\b`),

		// Syntax error patterns (exclude "invalid" alone to avoid matching "invalid API call")
		syntaxPatterns: regexp.MustCompile(`(?i)\b(syntax|parse|parsing|compilation|compiler|unexpected token|missing (semicolon|bracket|quote)|semicolon|bracket|parenthes(is|es)|quote|indentation)\b`),

		// Tool misuse patterns (includes hook denial patterns — hook denials ARE tool misuse)
		toolMisusePatterns: regexp.MustCompile(`(?i)\b(wrong (tool|command|function|method|parameter|argument)|incorrect usage|misuse|invalid.*(call|invocation|usage)|tool (error|fail)|command (error|fail)|API (error|misuse)|hook (blocked|denied|rejected)|pretool (denied|blocked)|command (not allowed|blocked)|bash.blocker)\b`),

		// Permission patterns
		permissionPatterns: regexp.MustCompile(`(?i)\b(permission denied|access denied|unauthorized|forbidden|403|401|auth(entication)? fail|credential|not allowed)\b`),

		// Timeout patterns
		timeoutPatterns: regexp.MustCompile(`(?i)\b(timeout|time[ds]? out|hung|hanging|slow|unresponsive|not responding|deadlock|stuck)\b`),
	}
}

// DetectContext analyzes a query to determine if it's in a debugging context
// and identifies the most relevant error category.
//
// Returns:
//   - isDebugging: true if query indicates debugging/troubleshooting context
//   - category: error category if applicable (empty string if not debugging)
//
// Examples:
//   - "why did my API call fail?" → (true, "other")
//   - "syntax error in my code" → (true, "syntax_error")
//   - "permission denied when accessing file" → (true, "permission_denied")
//   - "how to write a for loop" → (false, "")
func (d *ContextDetector) DetectContext(query string) (bool, reflection.ErrorCategory) {
	normalized := strings.ToLower(query)

	// Check for specific error categories first - these indicate debugging context
	// Priority order: syntax > permission > timeout > tool_misuse

	if d.syntaxPatterns.MatchString(normalized) {
		return true, reflection.ErrorCategorySyntax
	}

	if d.permissionPatterns.MatchString(normalized) {
		return true, reflection.ErrorCategoryPermissionDenied
	}

	if d.timeoutPatterns.MatchString(normalized) {
		return true, reflection.ErrorCategoryTimeout
	}

	if d.toolMisusePatterns.MatchString(normalized) {
		return true, reflection.ErrorCategoryToolMisuse
	}

	// Check if this is a general debugging context
	if d.debuggingKeywords.MatchString(normalized) {
		// Debugging context but no specific category
		return true, reflection.ErrorCategoryOther
	}

	// Not a debugging context
	return false, ""
}

// IsDebuggingContext is a convenience method that only returns the boolean
func (d *ContextDetector) IsDebuggingContext(query string) bool {
	isDebugging, _ := d.DetectContext(query)
	return isDebugging
}
