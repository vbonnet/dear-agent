package main

import (
	"testing"
)

func TestExtractPRNumbers_Basic(t *testing.T) {
	t.Parallel()
	text := "PR #317 and also PR #327 and #400"
	nums := extractPRNumbers(text)
	want := []int{317, 327, 400}
	if len(nums) != len(want) {
		t.Fatalf("got %v, want %v", nums, want)
	}
	for i, n := range want {
		if nums[i] != n {
			t.Errorf("nums[%d] = %d, want %d", i, nums[i], n)
		}
	}
}

func TestExtractPRNumbers_Deduplicates(t *testing.T) {
	t.Parallel()
	text := "#100 appears twice #100"
	nums := extractPRNumbers(text)
	if len(nums) != 1 || nums[0] != 100 {
		t.Errorf("expected [100], got %v", nums)
	}
}

func TestExtractPRNumbers_None(t *testing.T) {
	t.Parallel()
	nums := extractPRNumbers("no PR references here")
	if len(nums) != 0 {
		t.Errorf("expected empty slice, got %v", nums)
	}
}

func TestExtractPRNumbers_SkipsCrossRepoRefs(t *testing.T) {
	t.Parallel()
	// "dotfiles#15" and "engram#27" are PRs in OTHER repos; resolving their
	// numbers against this repo is a false positive. Only the bare "#352"
	// (a local PR ref) should survive.
	text := "Fix per dotfiles#15 and engram#27; tracked as local PR #352."
	nums := extractPRNumbers(text)
	if len(nums) != 1 || nums[0] != 352 {
		t.Errorf("expected [352] (cross-repo refs skipped), got %v", nums)
	}
}

func TestExtractTitle_Typical(t *testing.T) {
	t.Parallel()
	text := "✓ ce-2uy [BUG] · bwrap self-test skips network isolation   [● P1 · CLOSED]\nOwner: vbonnet"
	got := extractTitle(text)
	if got == "" || got == "(unknown)" {
		t.Errorf("extractTitle produced empty/unknown result: %q", got)
	}
}

func TestExtractTitle_Empty(t *testing.T) {
	t.Parallel()
	got := extractTitle("")
	if got != "(unknown)" {
		t.Errorf("empty input: got %q, want (unknown)", got)
	}
}

func TestIsMaybBeadID(t *testing.T) {
	t.Parallel()
	cases := []struct {
		s    string
		want bool
	}{
		{"ce-abc123", true},
		{"ce-6as.80", true},
		{"ce-2uy", true},
		{"✓", false},
		{"x", false},
		{"", false},
		{"ce", false},
	}
	for _, tc := range cases {
		got := isMaybBeadID(tc.s)
		if got != tc.want {
			t.Errorf("isMaybBeadID(%q) = %v, want %v", tc.s, got, tc.want)
		}
	}
}

func TestDetectRepo_GitHubHTTPS(t *testing.T) {
	t.Parallel()
	cases := []struct {
		url  string
		want string
	}{
		{"https://github.com/owner/repo.git", "owner/repo"},
		{"https://github.com/owner/repo", "owner/repo"},
		{"git@github.com:owner/repo.git", "owner/repo"},
	}
	for _, tc := range cases {
		// Test the parsing logic directly by calling detectRepoFromURL.
		got := extractRepoFromURL(tc.url)
		if got != tc.want {
			t.Errorf("extractRepoFromURL(%q) = %q, want %q", tc.url, got, tc.want)
		}
	}
}
