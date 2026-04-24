package reflection

import (
	"regexp"
	"strings"
)

// FailureDetector analyzes context to detect failures and classify errors
// (Task 1.1.2: Auto-detect failures and classify into error categories)
type FailureDetector struct {
	// Keyword patterns for failure detection
	failureKeywords *regexp.Regexp

	// Category-specific patterns for classification
	syntaxPatterns     *regexp.Regexp
	toolMisusePatterns *regexp.Regexp
	permissionPatterns *regexp.Regexp
	timeoutPatterns    *regexp.Regexp
}

// NewFailureDetector creates a new failure detector
func NewFailureDetector() *FailureDetector {
	return &FailureDetector{
		// General failure keywords (case-insensitive)
		failureKeywords: regexp.MustCompile(`(?i)\b(error|failed?|failure|broken|bug|issue|exception|panic|crash|incorrect|invalid)\b`),

		// Syntax error patterns
		syntaxPatterns: regexp.MustCompile(`(?i)\b(syntax error|parse error|unexpected token|unterminated|invalid syntax|compilation error|compile failed)\b`),

		// Tool misuse patterns
		toolMisusePatterns: regexp.MustCompile(`(?i)\b(invalid parameter|wrong argument|incorrect usage|bad request|invalid input|parameter validation|tool failed|command not found|unknown flag)\b`),

		// Permission denied patterns
		permissionPatterns: regexp.MustCompile(`(?i)\b(permission denied|access denied|forbidden|unauthorized|authentication failed|not allowed|insufficient permissions)\b`),

		// Timeout patterns
		timeoutPatterns: regexp.MustCompile(`(?i)\b(timeout|timed out|deadline exceeded|connection timeout|request timeout|hung|unresponsive)\b`),
	}
}

// DetectOutcome analyzes context to determine session outcome
// Returns OutcomeSuccess, OutcomeFailure, or OutcomePartial
func (d *FailureDetector) DetectOutcome(context DetectionContext) OutcomeType {
	// Check for explicit failure indicators
	if context.ErrorsEncountered > 0 {
		// If session completed despite errors, it's partial success
		if context.SessionCompleted {
			return OutcomePartial
		}
		return OutcomeFailure
	}

	// Check for failure keywords in description/learning
	combinedText := strings.ToLower(context.Description + " " + context.Learning)
	if d.failureKeywords.MatchString(combinedText) {
		// Keywords present but no explicit errors = partial (recovered failure)
		return OutcomePartial
	}

	// No errors, no failure keywords = success
	return OutcomeSuccess
}

// ClassifyError determines the error category based on context
// Returns empty string if outcome is not failure
func (d *FailureDetector) ClassifyError(context DetectionContext, outcome OutcomeType) ErrorCategory {
	// Only classify if this is a failure
	if outcome != OutcomeFailure && outcome != OutcomePartial {
		return ""
	}

	// Combine all text for pattern matching
	combinedText := strings.ToLower(
		context.Description + " " +
			context.Learning + " " +
			context.ErrorMessage,
	)

	// Check patterns in priority order (most specific first)

	// 1. Syntax errors (most specific)
	if d.syntaxPatterns.MatchString(combinedText) {
		return ErrorCategorySyntax
	}

	// 2. Permission errors (security-related)
	if d.permissionPatterns.MatchString(combinedText) {
		return ErrorCategoryPermissionDenied
	}

	// 3. Timeout errors (infrastructure)
	if d.timeoutPatterns.MatchString(combinedText) {
		return ErrorCategoryTimeout
	}

	// 4. Tool misuse errors (common operational issues)
	if d.toolMisusePatterns.MatchString(combinedText) {
		return ErrorCategoryToolMisuse
	}

	// 5. Default to "other" if no specific pattern matches
	return ErrorCategoryOther
}

// GenerateLessonLearned creates a concise lesson summary from failure context
func (d *FailureDetector) GenerateLessonLearned(context DetectionContext, category ErrorCategory) string {
	// If learning text already provided, use it as-is
	if context.Learning != "" {
		return context.Learning
	}

	// Otherwise, generate from error message and category
	if context.ErrorMessage != "" {
		// Extract first sentence or first 100 chars
		msg := strings.TrimSpace(context.ErrorMessage)
		if idx := strings.Index(msg, "."); idx > 0 && idx < 100 {
			return msg[:idx+1]
		}
		if len(msg) > 100 {
			return msg[:97] + "..."
		}
		return msg
	}

	// Fallback: generic lesson based on category
	switch category {
	case ErrorCategorySyntax:
		return "Verify syntax before execution to catch parse errors early."
	case ErrorCategoryToolMisuse:
		return "Validate tool parameters match expected schema before calling."
	case ErrorCategoryPermissionDenied:
		return "Check permissions and authentication before accessing resources."
	case ErrorCategoryTimeout:
		return "Implement timeout handling and retry logic for long operations."
	case ErrorCategoryOther:
		return "Review error context and implement appropriate error handling."
	default:
		return "Review error context and implement appropriate error handling."
	}
}

// EnrichReflection auto-detects failure and enriches reflection with failure tracking fields
// (Task 1.1.2: Main entry point for automatic failure detection)
func (d *FailureDetector) EnrichReflection(r *Reflection, context DetectionContext) {
	// Detect outcome
	outcome := d.DetectOutcome(context)
	r.Outcome = outcome

	// Classify error if failure/partial
	if outcome == OutcomeFailure || outcome == OutcomePartial {
		r.ErrorCategory = d.ClassifyError(context, outcome)
		r.LessonLearned = d.GenerateLessonLearned(context, r.ErrorCategory)
	}
}

// DetectionContext contains information used for failure detection
type DetectionContext struct {
	// Text fields to analyze
	Description  string // Trigger description or session summary
	Learning     string // User-provided learning text
	ErrorMessage string // Explicit error message if available

	// Metrics
	ErrorsEncountered int  // Number of errors in session
	SessionCompleted  bool // Did session complete successfully?

	// Optional: Command history for advanced detection
	Commands []string
}
