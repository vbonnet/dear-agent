package main

import (
	"strings"
	"testing"
)

func TestExtractPRNumbers_Basic(t *testing.T) {
	t.Parallel()
	nums := extractPRNumbers("PR #449 merged, also #317")
	want := []int{449, 317}
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
	nums := extractPRNumbers("#449 and again #449")
	if len(nums) != 1 || nums[0] != 449 {
		t.Errorf("expected [449], got %v", nums)
	}
}

func TestExtractPRNumbers_None(t *testing.T) {
	t.Parallel()
	nums := extractPRNumbers("no PR references here")
	if len(nums) != 0 {
		t.Errorf("expected empty, got %v", nums)
	}
}

func TestExtractPRNumbers_SkipsCrossRepo(t *testing.T) {
	t.Parallel()
	nums := extractPRNumbers("dotfiles#15 engram#27 but #352 is local")
	if len(nums) != 1 || nums[0] != 352 {
		t.Errorf("expected [352], got %v", nums)
	}
}

func TestExtractPRNumbers_InBeadOutput(t *testing.T) {
	t.Parallel()
	text := `✓ ce-gilj · Enforce safe-pr wrapper for PR lifecycle   [● P1 · CLOSED]
Owner: vbonnet
Description: Build safe-pr CLI wrapper. PR #449 implements the enforcement.
Comments:
  2026-06-12: PR #449 created, ready for review
  2026-06-13: Marking as done — PR #449 merged`
	nums := extractPRNumbers(text)
	if len(nums) != 1 || nums[0] != 449 {
		t.Errorf("expected [449], got %v", nums)
	}
}

func TestExtractTitle_Typical(t *testing.T) {
	t.Parallel()
	text := "✓ ce-gilj · Enforce safe-pr wrapper   [● P1 · CLOSED]"
	got := extractTitle(text)
	if got == "" || got == "(unknown)" {
		t.Errorf("unexpected empty/unknown: %q", got)
	}
}

func TestExtractTitle_Empty(t *testing.T) {
	t.Parallel()
	got := extractTitle("")
	if got != "(unknown)" {
		t.Errorf("got %q, want (unknown)", got)
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

func TestFormatResult_NoPRs(t *testing.T) {
	t.Parallel()
	r := GuardResult{BeadID: "ce-test", PRs: nil, Passed: true}
	var buf strings.Builder
	FormatResult(r, &buf)
	if !strings.Contains(buf.String(), "no PR references") {
		t.Errorf("expected 'no PR references', got %q", buf.String())
	}
}

func TestFormatResult_AllMerged(t *testing.T) {
	t.Parallel()
	r := GuardResult{BeadID: "ce-test", PRs: []int{449}, Passed: true}
	var buf strings.Builder
	FormatResult(r, &buf)
	if !strings.Contains(buf.String(), "all") && !strings.Contains(buf.String(), "merged") {
		t.Errorf("expected merged message, got %q", buf.String())
	}
}

func TestFormatResult_Blocked(t *testing.T) {
	t.Parallel()
	r := GuardResult{
		BeadID:     "ce-test",
		PRs:        []int{449},
		UnmergedPR: []UnmergedPR{{Number: 449, State: "OPEN"}},
		Passed:     false,
	}
	var buf strings.Builder
	FormatResult(r, &buf)
	out := buf.String()
	if !strings.Contains(out, "BLOCKED") {
		t.Errorf("expected BLOCKED, got %q", out)
	}
	if !strings.Contains(out, "#449") {
		t.Errorf("expected PR #449, got %q", out)
	}
}

func TestFormatResult_Forced(t *testing.T) {
	t.Parallel()
	r := GuardResult{
		BeadID:     "ce-test",
		PRs:        []int{449},
		UnmergedPR: []UnmergedPR{{Number: 449, State: "OPEN"}},
		Passed:     true,
		Forced:     true,
	}
	var buf strings.Builder
	FormatResult(r, &buf)
	if !strings.Contains(buf.String(), "OVERRIDE") {
		t.Errorf("expected OVERRIDE, got %q", buf.String())
	}
}
