package safegit

import (
	"strings"
	"testing"
)

// TestReviewThreadError_CarriesThreadIDs pins that safe-merge's rejection names
// each thread's node ID. The remediation it prints takes a thread ID, so an
// error without one leaves the operator with an unrunnable command.
func TestReviewThreadError_CarriesThreadIDs(t *testing.T) {
	err := reviewThreadError([]ReviewThread{
		{ID: "PRRT_resolved", IsResolved: true},
		{ID: "PRRT_live", IsResolved: false, Author: "gemini-code-assist", Body: "fix this"},
	})
	if err == nil {
		t.Fatal("want an error for an unresolved thread")
	}
	if !strings.Contains(err.Error(), "PRRT_live") {
		t.Errorf("unresolved thread ID missing from error: %s", err)
	}
	if strings.Contains(err.Error(), "PRRT_resolved") {
		t.Errorf("resolved thread should not appear: %s", err)
	}
}

// TestThreadRemediationGuidance_IsRunnable pins that the printed guidance gives
// a way to obtain thread IDs rather than leaving a bare <threadId> placeholder.
func TestThreadRemediationGuidance_IsRunnable(t *testing.T) {
	got := threadRemediationGuidance("owner/repo", 42)
	for _, want := range []string{
		"resolve-review-threads list owner repo 42",
		"reply-resolve",
		"resolve-all owner repo 42",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("guidance missing %q, got:\n%s", want, got)
		}
	}
}
