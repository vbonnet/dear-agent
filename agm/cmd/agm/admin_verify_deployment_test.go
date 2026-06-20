package main

import (
	"errors"
	"testing"

	"github.com/vbonnet/dear-agent/agm/internal/freshness"
)

func TestReportVerifyDeployment_ExitMapping(t *testing.T) {
	// Ensure non-interactive, non-exiting paths.
	verifyDeploymentJSON = false
	verifyDeploymentStrict = false

	cases := []struct {
		name      string
		result    freshness.AncestryResult
		wantError bool
	}{
		{"verified", freshness.AncestryResult{Status: freshness.AncestryVerified}, false},
		{"not_ancestor", freshness.AncestryResult{Status: freshness.AncestryNotAncestor}, true},
		{"commit_missing", freshness.AncestryResult{Status: freshness.AncestryCommitMissing}, true},
		{"unknown_commit", freshness.AncestryResult{Status: freshness.AncestryUnknownCommit}, true},
		{"indeterminate", freshness.AncestryResult{Status: freshness.AncestryIndeterminate, Err: errors.New("no repo")}, false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := reportVerifyDeployment(tc.result)
			if tc.wantError && err == nil {
				t.Errorf("%s: expected error (exit 1), got nil", tc.name)
			}
			if !tc.wantError && err != nil {
				t.Errorf("%s: expected nil error (exit 0), got %v", tc.name, err)
			}
		})
	}
}

func TestAncestryStatusString(t *testing.T) {
	want := map[freshness.AncestryStatus]string{
		freshness.AncestryVerified:      "verified",
		freshness.AncestryNotAncestor:   "not_ancestor",
		freshness.AncestryCommitMissing: "commit_missing",
		freshness.AncestryUnknownCommit: "unknown_commit",
		freshness.AncestryIndeterminate: "indeterminate",
	}
	for status, str := range want {
		if got := ancestryStatusString(status); got != str {
			t.Errorf("status %v: got %q want %q", status, got, str)
		}
	}
}
