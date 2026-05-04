// Package reflection implements the post-session learning system for capturing
// insights and lessons learned during AI agent sessions.
//
// Reflections are triggered automatically or manually to capture:
//   - What was learned during the session
//   - Why the learning occurred (trigger)
//   - Context and metadata for retrieval
//   - Session metrics and performance data
//
// Trigger types:
//   - Error: Learning from mistakes and failures
//   - Success: Reinforcing successful patterns
//   - Discovery: New patterns or approaches found
//   - Manual: Explicitly requested reflection
//
// Reflections are stored as engrams and become part of the retrievable
// knowledge base for future sessions. This creates a continuous learning
// loop where agents improve over time.
//
// Example usage:
//
//	recorder := reflection.NewRecorder("/path/to/engrams")
//	reflection := &Reflection{
//	    SessionID: "abc123",
//	    Timestamp: time.Now(),
//	    Trigger:   Trigger{Type: TriggerSuccess, Description: "Bug fixed"},
//	    Learning:  "Always check error returns in defer statements",
//	    Tags:      []string{"go", "errors"},
//	}
//	err := recorder.Record(ctx, reflection)
//
// Reflections feed into the ecphory retrieval system and can be queried
// like any other engram.
package reflection

import "time"

// Reflection represents a post-session learning reflection
type Reflection struct {
	// Session ID
	SessionID string

	// Timestamp
	Timestamp time.Time

	// Trigger that caused reflection
	Trigger Trigger

	// What was learned
	Learning string

	// Context/tags
	Tags []string

	// Session metrics (if available)
	Metrics SessionMetrics

	// Failure tracking fields (Task 1.1.1: Mistake Notebook)

	// Outcome of the session (success, failure, partial)
	Outcome OutcomeType

	// Error category for failures (only set if Outcome == OutcomeFailure)
	ErrorCategory ErrorCategory

	// Lesson learned from failure (concise summary for quick retrieval)
	LessonLearned string
}

// Trigger represents why a reflection was created
type Trigger struct {
	// Trigger type
	Type TriggerType

	// Description of what triggered reflection
	Description string
}

// TriggerType represents different reflection triggers
type TriggerType string

// Smart trigger values for TriggerType (see ADR-005).
const (
	// TriggerRepeatedFailureToSuccess fires when a sequence of failures finally succeeded.
	TriggerRepeatedFailureToSuccess TriggerType = "repeated_failure_to_success"
	// TriggerWorkDiscarded fires when completed work was thrown away.
	TriggerWorkDiscarded TriggerType = "work_discarded"
	// TriggerUnusualPattern fires when the session shows an anomalous interaction pattern.
	TriggerUnusualPattern TriggerType = "unusual_pattern"
	// TriggerExplicitRequest fires when the user explicitly asked for a reflection.
	TriggerExplicitRequest TriggerType = "explicit_request"
)

// OutcomeType represents the result of a session or operation
type OutcomeType string

// Outcome values for OutcomeType.
const (
	// OutcomeSuccess indicates the session achieved its goal.
	OutcomeSuccess OutcomeType = "success"

	// OutcomeFailure indicates the session failed to achieve its goal.
	OutcomeFailure OutcomeType = "failure"

	// OutcomePartial indicates the session partially achieved its goal.
	OutcomePartial OutcomeType = "partial"
)

// ErrorCategory classifies the type of error encountered
// (Task 1.1.1: Start with 5 categories, extensible for future)
type ErrorCategory string

// ErrorCategory values for ErrorCategory.
const (
	// ErrorCategorySyntax covers code syntax errors and parse failures.
	ErrorCategorySyntax ErrorCategory = "syntax_error"

	// ErrorCategoryToolMisuse covers incorrect tool usage, wrong parameters, and API misuse.
	ErrorCategoryToolMisuse ErrorCategory = "tool_misuse"

	// ErrorCategoryPermissionDenied covers permission-denied and authorization failures.
	ErrorCategoryPermissionDenied ErrorCategory = "permission_denied"

	// ErrorCategoryTimeout covers operation timeouts and hung processes.
	ErrorCategoryTimeout ErrorCategory = "timeout"

	// ErrorCategoryOther is the catchall for uncategorized errors.
	ErrorCategoryOther ErrorCategory = "other"
)

// SessionMetrics contains session statistics
type SessionMetrics struct {
	// Duration
	Duration time.Duration

	// Lines of code changed
	LinesChanged int

	// Files modified
	FilesModified int

	// Commands executed
	CommandsExecuted int

	// Errors encountered
	ErrorsEncountered int
}
