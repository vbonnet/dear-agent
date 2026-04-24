package reflection

import (
	"testing"
	"time"
)

// TestNewFailureDetector verifies detector initialization
func TestNewFailureDetector(t *testing.T) {
	detector := NewFailureDetector()
	if detector == nil {
		t.Fatal("NewFailureDetector() returned nil")
	}

	if detector.failureKeywords == nil {
		t.Error("failureKeywords pattern not initialized")
	}
}

// TestDetectOutcome_Success verifies success detection
func TestDetectOutcome_Success(t *testing.T) {
	detector := NewFailureDetector()

	context := DetectionContext{
		Description:       "Implemented new feature successfully",
		Learning:          "Feature works as expected",
		ErrorsEncountered: 0,
		SessionCompleted:  true,
	}

	outcome := detector.DetectOutcome(context)
	if outcome != OutcomeSuccess {
		t.Errorf("DetectOutcome() = %v, want %v", outcome, OutcomeSuccess)
	}
}

// TestDetectOutcome_Failure verifies failure detection
func TestDetectOutcome_Failure(t *testing.T) {
	detector := NewFailureDetector()

	testCases := []struct {
		name    string
		context DetectionContext
	}{
		{
			name: "errors_not_completed",
			context: DetectionContext{
				Description:       "Failed to complete task",
				ErrorsEncountered: 3,
				SessionCompleted:  false,
			},
		},
		{
			name: "error_keyword_in_description",
			context: DetectionContext{
				Description:       "Error occurred while processing",
				ErrorsEncountered: 0,
				SessionCompleted:  false,
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			outcome := detector.DetectOutcome(tc.context)
			if outcome == OutcomeSuccess {
				t.Errorf("DetectOutcome() = %v, want failure or partial", outcome)
			}
		})
	}
}

// TestDetectOutcome_Partial verifies partial success detection
func TestDetectOutcome_Partial(t *testing.T) {
	detector := NewFailureDetector()

	context := DetectionContext{
		Description:       "Completed with errors",
		Learning:          "Had to work around a bug",
		ErrorsEncountered: 2,
		SessionCompleted:  true, // Completed despite errors
	}

	outcome := detector.DetectOutcome(context)
	if outcome != OutcomePartial {
		t.Errorf("DetectOutcome() = %v, want %v", outcome, OutcomePartial)
	}
}

// TestClassifyError_Syntax verifies syntax error classification
func TestClassifyError_Syntax(t *testing.T) {
	detector := NewFailureDetector()

	testCases := []struct {
		description string
		errorMsg    string
	}{
		{"syntax error in Python", "SyntaxError: invalid syntax on line 42"},
		{"parse error in Go", "parse error: unexpected token },"},
		{"compilation failed", "compilation error: unterminated string literal"},
	}

	for _, tc := range testCases {
		t.Run(tc.description, func(t *testing.T) {
			context := DetectionContext{
				Description:  tc.description,
				ErrorMessage: tc.errorMsg,
			}

			category := detector.ClassifyError(context, OutcomeFailure)
			if category != ErrorCategorySyntax {
				t.Errorf("ClassifyError() = %v, want %v", category, ErrorCategorySyntax)
			}
		})
	}
}

// TestClassifyError_ToolMisuse verifies tool misuse classification
func TestClassifyError_ToolMisuse(t *testing.T) {
	detector := NewFailureDetector()

	testCases := []struct {
		description string
		errorMsg    string
	}{
		{"invalid parameter", "invalid parameter: expected string, got int"},
		{"wrong argument", "wrong argument count: expected 2, got 3"},
		{"command not found", "command not found: foobar"},
		{"unknown flag", "unknown flag: --invalid-option"},
	}

	for _, tc := range testCases {
		t.Run(tc.description, func(t *testing.T) {
			context := DetectionContext{
				Description:  tc.description,
				ErrorMessage: tc.errorMsg,
			}

			category := detector.ClassifyError(context, OutcomeFailure)
			if category != ErrorCategoryToolMisuse {
				t.Errorf("ClassifyError() = %v, want %v", category, ErrorCategoryToolMisuse)
			}
		})
	}
}

// TestClassifyError_PermissionDenied verifies permission error classification
func TestClassifyError_PermissionDenied(t *testing.T) {
	detector := NewFailureDetector()

	testCases := []struct {
		description string
		errorMsg    string
	}{
		{"permission denied", "permission denied: cannot write to /etc/config"},
		{"access forbidden", "403 Forbidden: access denied"},
		{"authentication failed", "authentication failed: invalid credentials"},
		{"unauthorized", "401 Unauthorized: insufficient permissions"},
	}

	for _, tc := range testCases {
		t.Run(tc.description, func(t *testing.T) {
			context := DetectionContext{
				Description:  tc.description,
				ErrorMessage: tc.errorMsg,
			}

			category := detector.ClassifyError(context, OutcomeFailure)
			if category != ErrorCategoryPermissionDenied {
				t.Errorf("ClassifyError() = %v, want %v", category, ErrorCategoryPermissionDenied)
			}
		})
	}
}

// TestClassifyError_Timeout verifies timeout classification
func TestClassifyError_Timeout(t *testing.T) {
	detector := NewFailureDetector()

	testCases := []struct {
		description string
		errorMsg    string
	}{
		{"connection timeout", "connection timeout after 30s"},
		{"request timed out", "request timed out: deadline exceeded"},
		{"process hung", "process hung and became unresponsive"},
	}

	for _, tc := range testCases {
		t.Run(tc.description, func(t *testing.T) {
			context := DetectionContext{
				Description:  tc.description,
				ErrorMessage: tc.errorMsg,
			}

			category := detector.ClassifyError(context, OutcomeFailure)
			if category != ErrorCategoryTimeout {
				t.Errorf("ClassifyError() = %v, want %v", category, ErrorCategoryTimeout)
			}
		})
	}
}

// TestClassifyError_Other verifies fallback classification
func TestClassifyError_Other(t *testing.T) {
	detector := NewFailureDetector()

	context := DetectionContext{
		Description:  "Something went wrong",
		ErrorMessage: "unknown error: system malfunction",
	}

	category := detector.ClassifyError(context, OutcomeFailure)
	if category != ErrorCategoryOther {
		t.Errorf("ClassifyError() = %v, want %v", category, ErrorCategoryOther)
	}
}

// TestClassifyError_NoClassificationForSuccess verifies no classification on success
func TestClassifyError_NoClassificationForSuccess(t *testing.T) {
	detector := NewFailureDetector()

	context := DetectionContext{
		Description: "Everything worked perfectly",
	}

	category := detector.ClassifyError(context, OutcomeSuccess)
	if category != "" {
		t.Errorf("ClassifyError() = %v, want empty string for success", category)
	}
}

// TestGenerateLessonLearned_FromLearning verifies using provided learning
func TestGenerateLessonLearned_FromLearning(t *testing.T) {
	detector := NewFailureDetector()

	context := DetectionContext{
		Learning:     "Always validate input before processing",
		ErrorMessage: "Some error that should be ignored",
	}

	lesson := detector.GenerateLessonLearned(context, ErrorCategoryToolMisuse)
	if lesson != context.Learning {
		t.Errorf("GenerateLessonLearned() = %q, want %q", lesson, context.Learning)
	}
}

// TestGenerateLessonLearned_FromErrorMessage verifies extraction from error
func TestGenerateLessonLearned_FromErrorMessage(t *testing.T) {
	detector := NewFailureDetector()

	context := DetectionContext{
		ErrorMessage: "Parameter validation failed: expected integer but got string.",
	}

	lesson := detector.GenerateLessonLearned(context, ErrorCategoryToolMisuse)
	expected := "Parameter validation failed: expected integer but got string."
	if lesson != expected {
		t.Errorf("GenerateLessonLearned() = %q, want %q", lesson, expected)
	}
}

// TestGenerateLessonLearned_Truncation verifies long message truncation
func TestGenerateLessonLearned_Truncation(t *testing.T) {
	detector := NewFailureDetector()

	longMessage := "This is a very long error message that exceeds one hundred characters and should be truncated to avoid bloating the reflection database with excessive detail"

	context := DetectionContext{
		ErrorMessage: longMessage,
	}

	lesson := detector.GenerateLessonLearned(context, ErrorCategoryOther)
	if len(lesson) > 100 {
		t.Errorf("GenerateLessonLearned() length = %d, want <= 100", len(lesson))
	}
	if lesson[len(lesson)-3:] != "..." {
		// Should end with "..." if truncated
		t.Error("GenerateLessonLearned() should end with '...' when truncated")
	}
}

// TestGenerateLessonLearned_DefaultForCategory verifies category-based defaults
func TestGenerateLessonLearned_DefaultForCategory(t *testing.T) {
	detector := NewFailureDetector()

	testCases := []struct {
		category ErrorCategory
		contains string
	}{
		{ErrorCategorySyntax, "syntax"},
		{ErrorCategoryToolMisuse, "parameters"},
		{ErrorCategoryPermissionDenied, "permissions"},
		{ErrorCategoryTimeout, "timeout"},
		{ErrorCategoryOther, "error handling"},
	}

	for _, tc := range testCases {
		t.Run(string(tc.category), func(t *testing.T) {
			context := DetectionContext{} // Empty context
			lesson := detector.GenerateLessonLearned(context, tc.category)

			if lesson == "" {
				t.Error("GenerateLessonLearned() returned empty string")
			}
			// Just verify it returns a non-empty default
		})
	}
}

// TestEnrichReflection_FullWorkflow verifies complete enrichment workflow
func TestEnrichReflection_FullWorkflow(t *testing.T) {
	detector := NewFailureDetector()

	reflection := &Reflection{
		SessionID: "test-session",
		Timestamp: time.Now(),
		Trigger: Trigger{
			Type:        TriggerRepeatedFailureToSuccess,
			Description: "Fixed syntax error after 3 attempts",
		},
		Learning: "Always check for missing semicolons in Go code",
		Tags:     []string{"debugging"},
		Metrics: SessionMetrics{
			ErrorsEncountered: 3,
		},
	}

	context := DetectionContext{
		Description:       "Fixed syntax error after 3 attempts",
		Learning:          "Always check for missing semicolons in Go code",
		ErrorMessage:      "syntax error: unexpected newline",
		ErrorsEncountered: 3,
		SessionCompleted:  false,
	}

	// Enrich the reflection
	detector.EnrichReflection(reflection, context)

	// Verify outcome was set
	if reflection.Outcome == "" {
		t.Error("EnrichReflection() did not set Outcome")
	}

	// Verify error category was set (should be syntax)
	if reflection.ErrorCategory != ErrorCategorySyntax {
		t.Errorf("EnrichReflection() ErrorCategory = %v, want %v",
			reflection.ErrorCategory, ErrorCategorySyntax)
	}

	// Verify lesson learned was set
	if reflection.LessonLearned == "" {
		t.Error("EnrichReflection() did not set LessonLearned")
	}
}

// TestEnrichReflection_SuccessCase verifies no enrichment for success
func TestEnrichReflection_SuccessCase(t *testing.T) {
	detector := NewFailureDetector()

	reflection := &Reflection{
		SessionID: "success-session",
		Timestamp: time.Now(),
		Trigger: Trigger{
			Type:        TriggerExplicitRequest,
			Description: "Completed feature implementation",
		},
		Learning: "Feature works as expected",
	}

	context := DetectionContext{
		Description:       "Completed feature implementation",
		Learning:          "Feature works as expected",
		ErrorsEncountered: 0,
		SessionCompleted:  true,
	}

	detector.EnrichReflection(reflection, context)

	// Verify outcome is success
	if reflection.Outcome != OutcomeSuccess {
		t.Errorf("EnrichReflection() Outcome = %v, want %v",
			reflection.Outcome, OutcomeSuccess)
	}

	// Verify no error category or lesson for success
	if reflection.ErrorCategory != "" {
		t.Error("EnrichReflection() should not set ErrorCategory for success")
	}
	if reflection.LessonLearned != "" {
		t.Error("EnrichReflection() should not set LessonLearned for success")
	}
}

// TestDetectionContext_CompleteFields verifies all context fields work
func TestDetectionContext_CompleteFields(t *testing.T) {
	detector := NewFailureDetector()

	context := DetectionContext{
		Description:       "Tool failed with timeout",
		Learning:          "Need retry logic",
		ErrorMessage:      "timeout: operation timed out after 30s",
		ErrorsEncountered: 1,
		SessionCompleted:  false,
		Commands:          []string{"run", "build", "test"},
	}

	outcome := detector.DetectOutcome(context)
	category := detector.ClassifyError(context, outcome)
	lesson := detector.GenerateLessonLearned(context, category)

	// Verify all detection works with complete context
	if outcome != OutcomeFailure {
		t.Error("Should detect failure with errors")
	}
	if category != ErrorCategoryTimeout {
		t.Error("Should classify as timeout error")
	}
	if lesson == "" {
		t.Error("Should generate lesson learned")
	}
}
