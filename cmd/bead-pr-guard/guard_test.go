package main

import (
	"testing"
)

func TestContainsToken_Exact(t *testing.T) {
	t.Parallel()
	if !containsToken("fix ce-abc123 cleanup", "ce-abc123") {
		t.Error("exact match should return true")
	}
}

func TestContainsToken_AtStart(t *testing.T) {
	t.Parallel()
	if !containsToken("ce-abc123: some title", "ce-abc123") {
		t.Error("token at start should return true")
	}
}

func TestContainsToken_AtEnd(t *testing.T) {
	t.Parallel()
	if !containsToken("fix for ce-abc123", "ce-abc123") {
		t.Error("token at end should return true")
	}
}

func TestContainsToken_Substring_NoMatch(t *testing.T) {
	t.Parallel()
	// "ce-abc" must not match inside "ce-abcdef"
	if containsToken("fix ce-abcdef cleanup", "ce-abc") {
		t.Error("substring match should return false")
	}
}

func TestContainsToken_Empty(t *testing.T) {
	t.Parallel()
	if containsToken("", "ce-abc") {
		t.Error("empty haystack should return false")
	}
}

func TestContainsBeadID_InTitle(t *testing.T) {
	t.Parallel()
	pr := prSummary{Number: 1, Title: "fix(ce-abc): foo", HeadRefName: "main", Body: ""}
	if !containsBeadID(pr, "ce-abc") {
		t.Error("bead ID in title should match")
	}
}

func TestContainsBeadID_InBranch(t *testing.T) {
	t.Parallel()
	pr := prSummary{Number: 2, Title: "unrelated", HeadRefName: "feat/ce-abc-some-feature", Body: ""}
	if !containsBeadID(pr, "ce-abc") {
		t.Error("bead ID in branch name should match")
	}
}

func TestContainsBeadID_InBody(t *testing.T) {
	t.Parallel()
	pr := prSummary{Number: 3, Title: "unrelated", HeadRefName: "branch", Body: "Closes ce-abc"}
	if !containsBeadID(pr, "ce-abc") {
		t.Error("bead ID in body should match")
	}
}

func TestContainsBeadID_NoMatch(t *testing.T) {
	t.Parallel()
	pr := prSummary{Number: 4, Title: "fix ce-xyz", HeadRefName: "fix/ce-xyz", Body: "Closes ce-xyz"}
	if containsBeadID(pr, "ce-abc") {
		t.Error("different bead ID should not match")
	}
}

func TestExtractRepoFromURL(t *testing.T) {
	t.Parallel()
	cases := []struct {
		url  string
		want string
	}{
		{"https://github.com/owner/repo.git", "owner/repo"},
		{"https://github.com/owner/repo", "owner/repo"},
		{"git@github.com:owner/repo.git", "owner/repo"},
		{"https://gitlab.com/owner/repo", ""},
	}
	for _, tc := range cases {
		got := extractRepoFromURL(tc.url)
		if got != tc.want {
			t.Errorf("extractRepoFromURL(%q) = %q, want %q", tc.url, got, tc.want)
		}
	}
}
