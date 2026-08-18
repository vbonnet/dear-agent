package main

import (
	"testing"
	"time"
)

// now is a fixed clock so the window boundary is stated rather than observed.
var escapeNow = time.Date(2026, 8, 18, 12, 0, 0, 0, time.UTC)

func at(daysAgo float64) time.Time {
	return escapeNow.Add(-time.Duration(daysAgo * float64(24*time.Hour)))
}

func TestEscapeCommitsExcludesPostMergeDetections(t *testing.T) {
	// A workflow with both push and schedule triggers — CI, CodeQL, Language
	// Policy. The scheduled failures detect drift in the outside world, which
	// Classify calls post-merge-only, so they are not escapes and must not
	// price a pre-merge gate.
	runs := []escapeRun{
		{CreatedAt: at(1), HeadSHA: "aaa", Conclusion: "failure", Event: "push"},
		{CreatedAt: at(2), HeadSHA: "bbb", Conclusion: "failure", Event: "schedule"},
		{CreatedAt: at(3), HeadSHA: "ccc", Conclusion: "failure", Event: "workflow_dispatch"},
		{CreatedAt: at(4), HeadSHA: "ddd", Conclusion: "failure", Event: "repository_dispatch"},
	}
	if got := escapeCommits(runs, escapeNow, 30); got != 1 {
		t.Fatalf("escapeCommits = %v, want 1 (only the push failure is escape evidence)", got)
	}
}

func TestEscapeCommitsCountsMergeQueueAndPush(t *testing.T) {
	// merge_group evaluates a change on its way into main, so a failure there
	// is a commit that reached main's gate. Excluding it would undercount.
	runs := []escapeRun{
		{CreatedAt: at(1), HeadSHA: "aaa", Conclusion: "failure", Event: "push"},
		{CreatedAt: at(2), HeadSHA: "bbb", Conclusion: "failure", Event: "merge_group"},
	}
	if got := escapeCommits(runs, escapeNow, 30); got != 2 {
		t.Fatalf("escapeCommits = %v, want 2", got)
	}
}

func TestEscapeCommitsCountsDistinctCommits(t *testing.T) {
	// Re-runs and repeat failures against one unchanged SHA are one escape.
	runs := []escapeRun{
		{CreatedAt: at(1), HeadSHA: "aaa", Conclusion: "failure", Event: "push"},
		{CreatedAt: at(2), HeadSHA: "aaa", Conclusion: "failure", Event: "push"},
		{CreatedAt: at(3), HeadSHA: "aaa", Conclusion: "timed_out", Event: "push"},
	}
	if got := escapeCommits(runs, escapeNow, 30); got != 1 {
		t.Fatalf("escapeCommits = %v, want 1", got)
	}
}

func TestEscapeCommitsCountsEveryRedConclusion(t *testing.T) {
	// Detection treats a timed-out or unstartable workflow as red, so the
	// numerator has to agree with the thing that raised the alarm.
	runs := []escapeRun{
		{CreatedAt: at(1), HeadSHA: "aaa", Conclusion: "failure", Event: "push"},
		{CreatedAt: at(1), HeadSHA: "bbb", Conclusion: "timed_out", Event: "push"},
		{CreatedAt: at(1), HeadSHA: "ccc", Conclusion: "startup_failure", Event: "push"},
		{CreatedAt: at(1), HeadSHA: "ddd", Conclusion: "success", Event: "push"},
		{CreatedAt: at(1), HeadSHA: "eee", Conclusion: "skipped", Event: "push"},
	}
	if got := escapeCommits(runs, escapeNow, 30); got != 3 {
		t.Fatalf("escapeCommits = %v, want 3", got)
	}
}

func TestEscapeCommitsHonoursTheWindow(t *testing.T) {
	runs := []escapeRun{
		{CreatedAt: at(29), HeadSHA: "aaa", Conclusion: "failure", Event: "push"},
		{CreatedAt: at(31), HeadSHA: "bbb", Conclusion: "failure", Event: "push"},
	}
	if got := escapeCommits(runs, escapeNow, 30); got != 1 {
		t.Fatalf("escapeCommits = %v, want 1 (the 31-day-old failure is outside the window)", got)
	}
}

func TestEscapeCommitsSkipsRunsWithNoCommit(t *testing.T) {
	runs := []escapeRun{
		{CreatedAt: at(1), HeadSHA: "", Conclusion: "failure", Event: "push"},
	}
	if got := escapeCommits(runs, escapeNow, 30); got != 0 {
		t.Fatalf("escapeCommits = %v, want 0 (no commit to attribute the failure to)", got)
	}
}

// The escape count and the classifier have to agree on what a detection is: a
// run the classifier calls post-merge-only cannot be escape evidence.
func TestScheduledDetectionAgreesWithTheEscapeCount(t *testing.T) {
	for event, wantDetection := range map[string]bool{
		"schedule":            true,
		"workflow_dispatch":   true,
		"repository_dispatch": true,
		"push":                false,
		"merge_group":         false,
		"dynamic":             false,
	} {
		if got := (mainRun{Event: event}).scheduledDetection(); got != wantDetection {
			t.Errorf("scheduledDetection(%q) = %v, want %v", event, got, wantDetection)
		}
		counted := escapeCommits([]escapeRun{
			{CreatedAt: at(1), HeadSHA: "aaa", Conclusion: "failure", Event: event},
		}, escapeNow, 30)
		if (counted == 0) != wantDetection {
			t.Errorf("event %q: counted=%v but scheduledDetection=%v", event, counted, wantDetection)
		}
	}
}
