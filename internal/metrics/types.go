// Package metrics provides benchmark measurement tooling for outcome quality
// and health metrics in the orchestrator ecosystem.
package metrics

import "time"

// Category classifies a metric.
type Category string

const (
	CategoryOutcomeQuality Category = "outcome_quality"
	CategoryHealth         Category = "health"
)

// MetricName identifies a specific metric.
type MetricName string

const (
	MetricTestPassRateDelta   MetricName = "test_pass_rate_delta"
	MetricFalseCompletionRate MetricName = "false_completion_rate"
	MetricHookBypassRate      MetricName = "hook_bypass_rate"
	MetricSessionSuccessRate  MetricName = "session_success_rate"
)

// Record is a single metric observation persisted to JSONL.
type Record struct {
	Timestamp   time.Time         `json:"timestamp"`
	Metric      MetricName        `json:"metric"`
	Category    Category          `json:"category"`
	Value       float64           `json:"value"`
	SessionID   string            `json:"session_id,omitempty"`
	SessionName string            `json:"session_name,omitempty"`
	Labels      map[string]string `json:"labels,omitempty"`
}

// TestPassRateEvent captures test results before and after a worker session.
type TestPassRateEvent struct {
	SessionID   string `json:"session_id"`
	SessionName string `json:"session_name,omitempty"`
	TestsBefore int    `json:"tests_before"` // passing tests before
	TotalBefore int    `json:"total_before"` // total tests before
	TestsAfter  int    `json:"tests_after"`  // passing tests after
	TotalAfter  int    `json:"total_after"`  // total tests after
}

// CompletionEvent captures whether a work item completion claim is valid.
type CompletionEvent struct {
	SessionID    string `json:"session_id"`
	SessionName  string `json:"session_name,omitempty"`
	WorkItemID   string `json:"work_item_id"`
	ClaimedDone  bool   `json:"claimed_done"`
	TestsPass    bool   `json:"tests_pass"`
	CommitsExist bool   `json:"commits_exist"`
}

// HookEvent captures a hook execution for bypass rate tracking.
type HookEvent struct {
	SessionID   string `json:"session_id"`
	SessionName string `json:"session_name,omitempty"`
	HookName    string `json:"hook_name"`
	PatternID   string `json:"pattern_id,omitempty"`
	Blocked     bool   `json:"blocked"`  // true = hook caught violation
	Bypassed    bool   `json:"bypassed"` // true = violation got through
}

// SessionOutcome captures the final state of a session.
type SessionOutcome struct {
	SessionID   string `json:"session_id"`
	SessionName string `json:"session_name,omitempty"`
	Outcome     string `json:"outcome"` // "completed", "failed", "abandoned"
}

// QueryFilter constrains metric queries.
type QueryFilter struct {
	Metric MetricName
	Since  time.Time
	Until  time.Time
}

// Summary holds aggregated metric results.
type Summary struct {
	Metric   MetricName `json:"metric"`
	Category Category   `json:"category"`
	Count    int        `json:"count"`
	Mean     float64    `json:"mean"`
	Latest   float64    `json:"latest"`
	Min      float64    `json:"min"`
	Max      float64    `json:"max"`
}
