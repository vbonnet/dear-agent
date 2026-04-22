package ecphory

import (
	"testing"

	"github.com/vbonnet/dear-agent/engram/internal/reflection"
)

// TestNewContextDetector verifies detector initialization
func TestNewContextDetector(t *testing.T) {
	detector := NewContextDetector()
	if detector == nil {
		t.Fatal("NewContextDetector() returned nil")
	}

	if detector.debuggingKeywords == nil {
		t.Error("debuggingKeywords not initialized")
	}
	if detector.syntaxPatterns == nil {
		t.Error("syntaxPatterns not initialized")
	}
	if detector.toolMisusePatterns == nil {
		t.Error("toolMisusePatterns not initialized")
	}
	if detector.permissionPatterns == nil {
		t.Error("permissionPatterns not initialized")
	}
	if detector.timeoutPatterns == nil {
		t.Error("timeoutPatterns not initialized")
	}
}

// TestDetectContext_DebuggingQueries tests detection of debugging contexts
func TestDetectContext_DebuggingQueries(t *testing.T) {
	detector := NewContextDetector()

	testCases := []struct {
		name          string
		query         string
		wantDebugging bool
		wantCategory  reflection.ErrorCategory
	}{
		// Syntax errors
		{
			name:          "syntax_error_explicit",
			query:         "syntax error in my code",
			wantDebugging: true,
			wantCategory:  reflection.ErrorCategorySyntax,
		},
		{
			name:          "parsing_error",
			query:         "parsing error in JSON file",
			wantDebugging: true,
			wantCategory:  reflection.ErrorCategorySyntax,
		},
		{
			name:          "compilation_error",
			query:         "compilation failed with unexpected token",
			wantDebugging: true,
			wantCategory:  reflection.ErrorCategorySyntax,
		},
		{
			name:          "missing_semicolon",
			query:         "error: missing semicolon",
			wantDebugging: true,
			wantCategory:  reflection.ErrorCategorySyntax,
		},

		// Permission errors
		{
			name:          "permission_denied",
			query:         "permission denied when accessing file",
			wantDebugging: true,
			wantCategory:  reflection.ErrorCategoryPermissionDenied,
		},
		{
			name:          "access_denied",
			query:         "access denied error",
			wantDebugging: true,
			wantCategory:  reflection.ErrorCategoryPermissionDenied,
		},
		{
			name:          "unauthorized",
			query:         "unauthorized API request",
			wantDebugging: true,
			wantCategory:  reflection.ErrorCategoryPermissionDenied,
		},
		{
			name:          "403_error",
			query:         "403 forbidden error",
			wantDebugging: true,
			wantCategory:  reflection.ErrorCategoryPermissionDenied,
		},

		// Timeout errors
		{
			name:          "timeout_explicit",
			query:         "request timeout error",
			wantDebugging: true,
			wantCategory:  reflection.ErrorCategoryTimeout,
		},
		{
			name:          "timed_out",
			query:         "operation timed out",
			wantDebugging: true,
			wantCategory:  reflection.ErrorCategoryTimeout,
		},
		{
			name:          "hung_process",
			query:         "process hung and not responding",
			wantDebugging: true,
			wantCategory:  reflection.ErrorCategoryTimeout,
		},
		{
			name:          "deadlock",
			query:         "deadlock detected in database",
			wantDebugging: true,
			wantCategory:  reflection.ErrorCategoryTimeout,
		},

		// Tool misuse errors
		{
			name:          "wrong_tool",
			query:         "wrong tool for the job",
			wantDebugging: true,
			wantCategory:  reflection.ErrorCategoryToolMisuse,
		},
		{
			name:          "incorrect_usage",
			query:         "incorrect usage of API",
			wantDebugging: true,
			wantCategory:  reflection.ErrorCategoryToolMisuse,
		},
		{
			name:          "invalid_call",
			query:         "invalid API call",
			wantDebugging: true,
			wantCategory:  reflection.ErrorCategoryToolMisuse,
		},
		{
			name:          "wrong_parameter",
			query:         "error: wrong parameter passed",
			wantDebugging: true,
			wantCategory:  reflection.ErrorCategoryToolMisuse,
		},

		// Hook denial patterns (detected as tool misuse)
		{
			name:          "hook_blocked",
			query:         "hook blocked my command",
			wantDebugging: true,
			wantCategory:  reflection.ErrorCategoryToolMisuse,
		},
		{
			name:          "hook_denied",
			query:         "hook denied the operation",
			wantDebugging: true,
			wantCategory:  reflection.ErrorCategoryToolMisuse,
		},
		{
			name:          "hook_rejected",
			query:         "hook rejected the bash command",
			wantDebugging: true,
			wantCategory:  reflection.ErrorCategoryToolMisuse,
		},
		{
			name:          "pretool_denied",
			query:         "pretool denied my request",
			wantDebugging: true,
			wantCategory:  reflection.ErrorCategoryToolMisuse,
		},
		{
			name:          "pretool_blocked",
			query:         "pretool blocked dangerous command",
			wantDebugging: true,
			wantCategory:  reflection.ErrorCategoryToolMisuse,
		},
		{
			name:          "command_not_allowed",
			query:         "command not allowed by policy",
			wantDebugging: true,
			wantCategory:  reflection.ErrorCategoryPermissionDenied, // "not allowed" matches permission patterns first
		},
		{
			name:          "command_blocked",
			query:         "command blocked by hook",
			wantDebugging: true,
			wantCategory:  reflection.ErrorCategoryToolMisuse,
		},
		{
			name:          "bash_blocker",
			query:         "bash blocker prevented execution",
			wantDebugging: true,
			wantCategory:  reflection.ErrorCategoryToolMisuse,
		},

		// Other errors (general debugging)
		{
			name:          "generic_error",
			query:         "why did my API call fail?",
			wantDebugging: true,
			wantCategory:  reflection.ErrorCategoryOther,
		},
		{
			name:          "broken_feature",
			query:         "broken feature in production",
			wantDebugging: true,
			wantCategory:  reflection.ErrorCategoryOther,
		},
		{
			name:          "bug_report",
			query:         "bug in authentication flow",
			wantDebugging: true,
			wantCategory:  reflection.ErrorCategoryOther,
		},
		{
			name:          "issue_tracker",
			query:         "issue with database connection",
			wantDebugging: true,
			wantCategory:  reflection.ErrorCategoryOther,
		},
		{
			name:          "crash_report",
			query:         "application crashed unexpectedly",
			wantDebugging: true,
			wantCategory:  reflection.ErrorCategoryOther,
		},
		{
			name:          "exception_handling",
			query:         "exception thrown in handler",
			wantDebugging: true,
			wantCategory:  reflection.ErrorCategoryOther,
		},
		{
			name:          "troubleshooting",
			query:         "troubleshooting network issue",
			wantDebugging: true,
			wantCategory:  reflection.ErrorCategoryOther,
		},
		{
			name:          "fixing_bug",
			query:         "fixing authentication bug",
			wantDebugging: true,
			wantCategory:  reflection.ErrorCategoryOther,
		},
		{
			name:          "resolve_issue",
			query:         "how to resolve this problem",
			wantDebugging: true,
			wantCategory:  reflection.ErrorCategoryOther,
		},
		{
			name:          "diagnose_problem",
			query:         "diagnosing performance problem",
			wantDebugging: true,
			wantCategory:  reflection.ErrorCategoryOther,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			isDebugging, category := detector.DetectContext(tc.query)

			if isDebugging != tc.wantDebugging {
				t.Errorf("DetectContext(%q) isDebugging = %v, want %v",
					tc.query, isDebugging, tc.wantDebugging)
			}

			if category != tc.wantCategory {
				t.Errorf("DetectContext(%q) category = %q, want %q",
					tc.query, category, tc.wantCategory)
			}
		})
	}
}

// TestDetectContext_NormalQueries tests that normal queries are not flagged as debugging
func TestDetectContext_NormalQueries(t *testing.T) {
	detector := NewContextDetector()

	testCases := []struct {
		name  string
		query string
	}{
		{"how_to_question", "how to write a for loop in Go"},
		{"tutorial_request", "show me an example of dependency injection"},
		{"concept_explanation", "what is the difference between async and sync"},
		{"code_example", "example of factory pattern"},
		{"best_practices", "best practices for API design"},
		{"architecture_question", "how to structure a microservices application"},
		{"learning_query", "explain how recursion works"},
		{"comparison", "React vs Vue performance comparison"},
		{"documentation", "where is the documentation for this library"},
		{"usage_guide", "how to use this tool effectively"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			isDebugging, category := detector.DetectContext(tc.query)

			if isDebugging {
				t.Errorf("DetectContext(%q) incorrectly flagged as debugging (category: %q)",
					tc.query, category)
			}

			if category != "" {
				t.Errorf("DetectContext(%q) returned category %q for non-debugging query",
					tc.query, category)
			}
		})
	}
}

// TestDetectContext_CaseInsensitive verifies case-insensitive matching
func TestDetectContext_CaseInsensitive(t *testing.T) {
	detector := NewContextDetector()

	testCases := []string{
		"ERROR in my code",
		"Error In My Code",
		"error in my code",
		"SYNTAX ERROR",
		"Permission Denied",
	}

	for _, query := range testCases {
		t.Run(query, func(t *testing.T) {
			isDebugging, _ := detector.DetectContext(query)
			if !isDebugging {
				t.Errorf("DetectContext(%q) should detect debugging context (case-insensitive)",
					query)
			}
		})
	}
}

// TestDetectContext_WordBoundaries verifies word boundary matching
func TestDetectContext_WordBoundaries(t *testing.T) {
	detector := NewContextDetector()

	// These should NOT match (substring within word)
	falsePositives := []string{
		"terrific example",     // contains "error" but not as word
		"unfailing commitment", // contains "fail" but not as word
		"rebroken ceramic",     // contains "broken" but not as word
	}

	for _, query := range falsePositives {
		t.Run("false_positive_"+query, func(t *testing.T) {
			isDebugging, _ := detector.DetectContext(query)
			if isDebugging {
				t.Errorf("DetectContext(%q) incorrectly detected debugging (word boundary issue)",
					query)
			}
		})
	}

	// These SHOULD match (actual words)
	truePositives := []string{
		"an error occurred",
		"the request failed",
		"it's broken",
	}

	for _, query := range truePositives {
		t.Run("true_positive_"+query, func(t *testing.T) {
			isDebugging, _ := detector.DetectContext(query)
			if !isDebugging {
				t.Errorf("DetectContext(%q) should detect debugging context",
					query)
			}
		})
	}
}

// TestDetectContext_CategoryPriority verifies category priority order
func TestDetectContext_CategoryPriority(t *testing.T) {
	detector := NewContextDetector()

	// Syntax has highest priority
	query1 := "permission denied due to syntax error"
	_, category1 := detector.DetectContext(query1)
	if category1 != reflection.ErrorCategorySyntax {
		t.Errorf("DetectContext(%q) category = %q, want %q (syntax priority)",
			query1, category1, reflection.ErrorCategorySyntax)
	}

	// Permission over timeout
	query2 := "permission denied timeout"
	_, category2 := detector.DetectContext(query2)
	if category2 != reflection.ErrorCategoryPermissionDenied {
		t.Errorf("DetectContext(%q) category = %q, want %q (permission priority)",
			query2, category2, reflection.ErrorCategoryPermissionDenied)
	}

	// Timeout over tool misuse
	query3 := "timeout using wrong tool"
	_, category3 := detector.DetectContext(query3)
	if category3 != reflection.ErrorCategoryTimeout {
		t.Errorf("DetectContext(%q) category = %q, want %q (timeout priority)",
			query3, category3, reflection.ErrorCategoryTimeout)
	}

	// Tool misuse over other
	query4 := "failed with wrong parameter"
	_, category4 := detector.DetectContext(query4)
	if category4 != reflection.ErrorCategoryToolMisuse {
		t.Errorf("DetectContext(%q) category = %q, want %q (tool_misuse priority)",
			query4, category4, reflection.ErrorCategoryToolMisuse)
	}
}

// TestIsDebuggingContext verifies convenience method
func TestIsDebuggingContext(t *testing.T) {
	detector := NewContextDetector()

	if !detector.IsDebuggingContext("error in my code") {
		t.Error("IsDebuggingContext should return true for debugging query")
	}

	if detector.IsDebuggingContext("how to write a function") {
		t.Error("IsDebuggingContext should return false for normal query")
	}
}

// TestDetectContext_Accuracy validates 90%+ accuracy requirement
func TestDetectContext_Accuracy(t *testing.T) {
	detector := NewContextDetector()

	// Test cases with expected results
	testCases := []struct {
		query             string
		expectedDebugging bool
	}{
		// Debugging queries (30 cases)
		{"error in my code", true},
		{"why did this fail", true},
		{"broken feature", true},
		{"bug in authentication", true},
		{"issue with database", true},
		{"problem with API", true},
		{"crash on startup", true},
		{"exception thrown", true},
		{"failure to connect", true},
		{"debugging network issue", true},
		{"troubleshooting timeout", true},
		{"fixing permission problem", true},
		{"resolve syntax error", true},
		{"diagnose performance issue", true},
		{"not working correctly", true},
		{"something went wrong", true},
		{"incorrect behavior", true},
		{"syntax error in line 10", true},
		{"permission denied error", true},
		{"request timed out", true},
		{"wrong tool used", true},
		{"invalid API call", true},
		{"access denied", true},
		{"compilation failed", true},
		{"hung process", true},
		{"deadlock detected", true},
		{"unauthorized request", true},
		{"parsing error", true},
		{"tool misuse", true},
		{"operation failed", true},

		// Normal queries (20 cases)
		{"how to write a loop", false},
		{"show me an example", false},
		{"what is recursion", false},
		{"best practices for testing", false},
		{"documentation for library", false},
		{"tutorial on async programming", false},
		{"explain dependency injection", false},
		{"React vs Vue comparison", false},
		{"how to use this tool", false},
		{"guide for beginners", false},
		{"architecture patterns", false},
		{"design principles", false},
		{"code review checklist", false},
		{"performance optimization tips", false},
		{"security best practices", false},
		{"testing strategies", false},
		{"refactoring techniques", false},
		{"API design guidelines", false},
		{"database schema design", false},
		{"microservices overview", false},
	}

	correct := 0
	total := len(testCases)

	for _, tc := range testCases {
		isDebugging, _ := detector.DetectContext(tc.query)
		if isDebugging == tc.expectedDebugging {
			correct++
		} else {
			t.Logf("MISS: query=%q expected=%v got=%v",
				tc.query, tc.expectedDebugging, isDebugging)
		}
	}

	accuracy := float64(correct) / float64(total) * 100.0

	if accuracy < 90.0 {
		t.Errorf("Accuracy = %.1f%%, want >= 90.0%%", accuracy)
	}

	t.Logf("Accuracy: %.1f%% (%d/%d correct)", accuracy, correct, total)
}
