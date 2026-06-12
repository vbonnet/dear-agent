package main

import (
	"context"
	"testing"
	"time"
)

func TestClassifyPR_Clean(t *testing.T) {
	pr := PullRequest{Number: 1, Title: "fix: something", MergedAt: "2026-06-12T10:00:00Z"}
	mergedAt, _ := time.Parse(time.RFC3339, "2026-06-12T10:00:00Z")
	checks := []CheckRun{
		{Name: "Build & Test (ubuntu-latest)", Status: "completed", Conclusion: "success", CompletedAt: "2026-06-12T09:50:00Z"},
		{Name: "govulncheck", Status: "completed", Conclusion: "success", CompletedAt: "2026-06-12T09:55:00Z"},
	}
	required := []string{"Build & Test (ubuntu-latest)", "govulncheck"}
	f := classifyPR(pr, required, checks, 1, mergedAt)
	if f != nil {
		t.Errorf("expected clean PR, got finding: %v", f.Issues)
	}
}

func TestClassifyPR_NonSquash(t *testing.T) {
	pr := PullRequest{Number: 2, Title: "merge: something", MergeCommitSHA: "abcdef1234567890", MergedAt: "2026-06-12T10:00:00Z"}
	mergedAt, _ := time.Parse(time.RFC3339, "2026-06-12T10:00:00Z")
	checks := []CheckRun{
		{Name: "Build & Test", Status: "completed", Conclusion: "success", CompletedAt: "2026-06-12T09:50:00Z"},
	}
	f := classifyPR(pr, []string{"Build & Test"}, checks, 2, mergedAt)
	if f == nil {
		t.Fatal("expected finding for non-squash merge, got nil")
	}
	found := false
	for _, issue := range f.Issues {
		if containsStr(issue, "non-squash") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected non-squash issue, got: %v", f.Issues)
	}
}

func TestClassifyPR_FailedCheck(t *testing.T) {
	pr := PullRequest{Number: 3, Title: "feat: new thing", MergedAt: "2026-06-12T10:00:00Z"}
	mergedAt, _ := time.Parse(time.RFC3339, "2026-06-12T10:00:00Z")
	checks := []CheckRun{
		{Name: "Build & Test", Status: "completed", Conclusion: "failure", CompletedAt: "2026-06-12T09:50:00Z"},
	}
	f := classifyPR(pr, []string{"Build & Test"}, checks, 1, mergedAt)
	if f == nil {
		t.Fatal("expected finding for failed check, got nil")
	}
	found := false
	for _, issue := range f.Issues {
		if containsStr(issue, "non-success") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected non-success issue, got: %v", f.Issues)
	}
}

func TestClassifyPR_MissingCheck(t *testing.T) {
	pr := PullRequest{Number: 4, Title: "chore: update deps", MergedAt: "2026-06-12T10:00:00Z"}
	mergedAt, _ := time.Parse(time.RFC3339, "2026-06-12T10:00:00Z")
	checks := []CheckRun{
		{Name: "Other Check", Status: "completed", Conclusion: "success", CompletedAt: "2026-06-12T09:50:00Z"},
	}
	f := classifyPR(pr, []string{"Required Check"}, checks, 1, mergedAt)
	if f == nil {
		t.Fatal("expected finding for missing required check, got nil")
	}
	found := false
	for _, issue := range f.Issues {
		if containsStr(issue, "absent") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected absent issue, got: %v", f.Issues)
	}
}

func TestClassifyPR_CheckCompletedAfterMerge(t *testing.T) {
	pr := PullRequest{Number: 5, Title: "fix: something", MergedAt: "2026-06-12T10:00:00Z"}
	mergedAt, _ := time.Parse(time.RFC3339, "2026-06-12T10:00:00Z")
	// Check completed AFTER the merge — should be flagged.
	checks := []CheckRun{
		{Name: "Build & Test", Status: "completed", Conclusion: "success", CompletedAt: "2026-06-12T10:05:00Z"},
	}
	f := classifyPR(pr, []string{"Build & Test"}, checks, 1, mergedAt)
	if f == nil {
		t.Fatal("expected finding for check completed after merge, got nil")
	}
	found := false
	for _, issue := range f.Issues {
		if containsStr(issue, "completed after merge") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected 'completed after merge' issue, got: %v", f.Issues)
	}
}

func TestClassifyPR_NoRequiredChecks(t *testing.T) {
	pr := PullRequest{Number: 6, Title: "docs: update README", MergedAt: "2026-06-12T10:00:00Z"}
	mergedAt, _ := time.Parse(time.RFC3339, "2026-06-12T10:00:00Z")
	f := classifyPR(pr, nil, nil, 1, mergedAt)
	if f != nil {
		t.Errorf("expected clean PR with no required checks, got: %v", f.Issues)
	}
}

func TestIsSafeSHA(t *testing.T) {
	cases := []struct {
		sha  string
		want bool
	}{
		{"abc1234", true},
		{"8152a351e73159cd6b53270b59e93dc002057c93", true},
		{"", false},
		{"abc123!", false},
		{"abc", false}, // too short (< 7)
		{"../../etc", false},
	}
	for _, tc := range cases {
		got := isSafeSHA(tc.sha)
		if got != tc.want {
			t.Errorf("isSafeSHA(%q) = %v, want %v", tc.sha, got, tc.want)
		}
	}
}

func TestFetchRequiredChecks_RejectsMalformedInput(t *testing.T) {
	cases := []struct {
		repo   string
		branch string
	}{
		{"evil.com/x/repo", "main"},
		{"http://evil.com/o/r", "main"},
		{"owner", "main"},
		{"owner/repo", "main?x=1"},
	}
	for _, tc := range cases {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		_, err := fetchRequiredChecks(ctx, "token", tc.repo, tc.branch)
		if err == nil {
			t.Errorf("fetchRequiredChecks(%q, %q) expected error, got nil", tc.repo, tc.branch)
		}
	}
}

func TestFetchRequiredChecks_AcceptsValidInput(t *testing.T) {
	// Pre-cancel context so no real HTTP request is made.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := fetchRequiredChecks(ctx, "token", "owner/repo", "main")
	if err == nil {
		t.Error("expected error (cancelled context), got nil")
	}
	if containsStr(err.Error(), "invalid repo") || containsStr(err.Error(), "invalid branch") {
		t.Errorf("should not be a validation error, got: %v", err)
	}
}

func TestFetchCheckRuns_RejectsInvalidSHA(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := fetchCheckRuns(ctx, "token", "owner/repo", "../../etc/passwd")
	if err == nil || !containsStr(err.Error(), "invalid SHA") {
		t.Errorf("expected invalid SHA error, got: %v", err)
	}
}

func TestFetchSinglePR_RejectsInvalidRepo(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := fetchSinglePR(ctx, "token", "not-a-repo", 42)
	if err == nil || !containsStr(err.Error(), "invalid repo") {
		t.Errorf("expected invalid repo error, got: %v", err)
	}
}

func TestFetchSinglePR_AcceptsValidRepo(t *testing.T) {
	// Pre-cancel context so no real HTTP request is made.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := fetchSinglePR(ctx, "token", "owner/repo", 42)
	if err == nil {
		t.Error("expected error (cancelled context), got nil")
	}
	if containsStr(err.Error(), "invalid repo") {
		t.Errorf("should not be a validation error, got: %v", err)
	}
}

func containsStr(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(sub) == 0 ||
		func() bool {
			for i := 0; i+len(sub) <= len(s); i++ {
				if s[i:i+len(sub)] == sub {
					return true
				}
			}
			return false
		}())
}
