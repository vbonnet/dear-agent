package main

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestClassifyBranch(t *testing.T) {
	tests := []struct {
		name    string
		tipDate string
		prs     []prRecord
		want    string
	}{
		{
			name:    "no PR at all",
			tipDate: "2026-06-10T12:00:00Z",
			prs:     nil,
			want:    bucketReviewNoPR,
		},
		{
			name:    "open PR",
			tipDate: "2026-06-10T12:00:00Z",
			prs:     []prRecord{{Number: 1, State: "OPEN"}},
			want:    bucketReviewOpenPR,
		},
		{
			name:    "open PR wins over a stale merged one in the same list",
			tipDate: "2026-06-10T12:00:00Z",
			prs: []prRecord{
				{Number: 1, State: "MERGED", MergedAt: "2026-05-01T00:00:00Z"},
				{Number: 2, State: "OPEN"},
			},
			want: bucketReviewOpenPR,
		},
		{
			name:    "merged, tip strictly before merge -> safe",
			tipDate: "2026-06-10T11:00:00Z",
			prs:     []prRecord{{Number: 1, State: "MERGED", MergedAt: "2026-06-10T12:00:00Z"}},
			want:    bucketSafeDelete,
		},
		{
			name:    "merged, tip exactly equal to merge -> safe (boundary)",
			tipDate: "2026-06-10T12:00:00Z",
			prs:     []prRecord{{Number: 1, State: "MERGED", MergedAt: "2026-06-10T12:00:00Z"}},
			want:    bucketSafeDelete,
		},
		{
			name:    "merged, tip after merge -> needs review",
			tipDate: "2026-06-10T13:00:00Z",
			prs:     []prRecord{{Number: 1, State: "MERGED", MergedAt: "2026-06-10T12:00:00Z"}},
			want:    bucketReviewNewCommitsAfterMerge,
		},
		{
			name:    "multiple merged PRs: classify against the most recent mergedAt",
			tipDate: "2026-07-01T00:00:00Z",
			prs: []prRecord{
				{Number: 1, State: "MERGED", MergedAt: "2026-05-01T00:00:00Z"},
				{Number: 2, State: "MERGED", MergedAt: "2026-07-05T00:00:00Z"},
			},
			// tip (07-01) is before the *latest* merge (07-05), so this is
			// safe even though it's after the earlier PR's merge.
			want: bucketSafeDelete,
		},
		{
			name:    "closed without merging, no merged PR",
			tipDate: "2026-06-10T12:00:00Z",
			prs:     []prRecord{{Number: 1, State: "CLOSED"}},
			want:    bucketReviewClosedUnmerged,
		},
		{
			name:    "closed and no-PR mix resolves to closed",
			tipDate: "2026-06-10T12:00:00Z",
			prs:     []prRecord{{Number: 1, State: "CLOSED"}, {Number: 2, State: "CLOSED"}},
			want:    bucketReviewClosedUnmerged,
		},
		{
			name:    "unparseable tip date with a merged PR defaults to needs-review",
			tipDate: "not-a-date",
			prs:     []prRecord{{Number: 1, State: "MERGED", MergedAt: "2026-06-10T12:00:00Z"}},
			want:    bucketReviewNewCommitsAfterMerge,
		},
		{
			name:    "unparseable mergedAt defaults to needs-review",
			tipDate: "2026-06-10T12:00:00Z",
			prs:     []prRecord{{Number: 1, State: "MERGED", MergedAt: "not-a-date"}},
			want:    bucketReviewNewCommitsAfterMerge,
		},
		{
			name:    "empty PR list",
			tipDate: "2026-06-10T12:00:00Z",
			prs:     []prRecord{},
			want:    bucketReviewNoPR,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := classifyBranch(tt.tipDate, tt.prs); got != tt.want {
				t.Errorf("classifyBranch(%q, %v) = %q, want %q", tt.tipDate, tt.prs, got, tt.want)
			}
		})
	}
}

func TestLastMerged(t *testing.T) {
	t.Run("no merged PRs", func(t *testing.T) {
		_, ok := lastMerged([]prRecord{{Number: 1, State: "OPEN"}, {Number: 2, State: "CLOSED"}})
		if ok {
			t.Fatal("expected ok=false with no MERGED prs")
		}
	})

	t.Run("single merged PR", func(t *testing.T) {
		got, ok := lastMerged([]prRecord{{Number: 5, State: "MERGED", MergedAt: "2026-01-01T00:00:00Z"}})
		if !ok || got.Number != 5 {
			t.Fatalf("got %v, %v", got, ok)
		}
	})

	t.Run("picks latest mergedAt among several", func(t *testing.T) {
		prs := []prRecord{
			{Number: 1, State: "MERGED", MergedAt: "2026-03-01T00:00:00Z"},
			{Number: 2, State: "MERGED", MergedAt: "2026-06-01T00:00:00Z"},
			{Number: 3, State: "MERGED", MergedAt: "2026-04-01T00:00:00Z"},
			{Number: 4, State: "OPEN"},
			{Number: 5, State: "CLOSED"},
		}
		got, ok := lastMerged(prs)
		if !ok || got.Number != 2 {
			t.Fatalf("got %v, %v, want PR #2", got, ok)
		}
	})
}

func TestIsProtected(t *testing.T) {
	tests := []struct {
		branch string
		want   bool
	}{
		{"main", true},
		{"master", true},
		{"HEAD", true},
		{"feat/branch-reaper-durable-fix", false},
		{"mainline", false},
		{"dependabot/go_modules/foo", false},
	}
	for _, tt := range tests {
		if got := isProtected(tt.branch); got != tt.want {
			t.Errorf("isProtected(%q) = %v, want %v", tt.branch, got, tt.want)
		}
	}
}

func TestParseTimestamp(t *testing.T) {
	valid := []string{
		"2026-06-10T12:00:00Z",      // gh mergedAt style
		"2026-06-10T12:00:00+00:00", // git committerdate:iso-strict style
		"2026-06-10T05:00:00-07:00", // non-UTC offset
	}
	for _, s := range valid {
		if _, err := parseTimestamp(s); err != nil {
			t.Errorf("parseTimestamp(%q) unexpected error: %v", s, err)
		}
	}

	invalid := []string{"", "not-a-date", "2026-06-10", "June 10 2026"}
	for _, s := range invalid {
		if _, err := parseTimestamp(s); err == nil {
			t.Errorf("parseTimestamp(%q) expected error, got nil", s)
		}
	}
}

func TestReviewTotal(t *testing.T) {
	tests := []struct {
		name string
		r    Report
		want int
	}{
		{"all empty", Report{}, 0},
		{"only safe delete does not count", Report{SafeDelete: []string{"a", "b"}}, 0},
		{
			"sums the three review buckets",
			Report{
				ReviewNoPR:                 []string{"a"},
				ReviewClosedUnmerged:       []string{"b", "c"},
				ReviewNewCommitsAfterMerge: []string{"d"},
			},
			4,
		},
	}
	for _, tt := range tests {
		if got := reviewTotal(tt.r); got != tt.want {
			t.Errorf("%s: reviewTotal = %d, want %d", tt.name, got, tt.want)
		}
	}
}

func TestParseBranchList(t *testing.T) {
	in := strings.Join([]string{
		"refs/remotes/origin/HEAD|2026-06-10T12:00:00+00:00",
		"refs/remotes/origin/main|2026-06-10T12:00:00+00:00",
		"refs/remotes/origin/feat/x|2026-06-09T08:00:00+00:00",
		"", // trailing blank line, as real `git for-each-ref` output ends with \n
	}, "\n")

	got := parseBranchList(in)
	if len(got) != 2 {
		t.Fatalf("expected 2 branches (HEAD symref dropped), got %d: %+v", len(got), got)
	}
	if got[0].Name != "main" || got[0].TipDate != "2026-06-10T12:00:00+00:00" {
		t.Errorf("branch[0] = %+v", got[0])
	}
	if got[1].Name != "feat/x" || got[1].TipDate != "2026-06-09T08:00:00+00:00" {
		t.Errorf("branch[1] = %+v", got[1])
	}
}

func TestParseBranchList_MalformedLinesSkipped(t *testing.T) {
	in := "no-pipe-here\nrefs/remotes/origin/ok|2026-01-01T00:00:00Z\n"
	got := parseBranchList(in)
	if len(got) != 1 || got[0].Name != "ok" {
		t.Fatalf("expected only the well-formed line to survive, got %+v", got)
	}
}

func TestReportJSON_EmptyBucketsAreArraysNotNull(t *testing.T) {
	var buf bytes.Buffer
	printJSON(&buf, Report{
		SafeDelete:                 []string{},
		ReviewNoPR:                 []string{},
		ReviewClosedUnmerged:       []string{},
		ReviewNewCommitsAfterMerge: []string{},
	})

	var decoded map[string]json.RawMessage
	if err := json.Unmarshal(buf.Bytes(), &decoded); err != nil {
		t.Fatalf("output is not valid JSON: %v\n%s", err, buf.String())
	}
	for _, key := range []string{"safe_delete", "review_no_pr", "review_closed_unmerged", "review_new_commits_after_merge"} {
		raw, ok := decoded[key]
		if !ok {
			t.Errorf("missing key %q", key)
			continue
		}
		if string(raw) != "[]" {
			t.Errorf("key %q = %s, want []", key, raw)
		}
	}
}

func TestReportJSON_RoundTrip(t *testing.T) {
	want := Report{
		SafeDelete:                 []string{"a", "b"},
		ReviewNoPR:                 []string{"c"},
		ReviewClosedUnmerged:       []string{"d"},
		ReviewNewCommitsAfterMerge: []string{"e", "f"},
	}
	var buf bytes.Buffer
	printJSON(&buf, want)

	var got Report
	if err := json.Unmarshal(buf.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(got.SafeDelete) != 2 || len(got.ReviewNoPR) != 1 || len(got.ReviewClosedUnmerged) != 1 || len(got.ReviewNewCommitsAfterMerge) != 2 {
		t.Errorf("round trip mismatch: %+v", got)
	}
}

func TestPrintHuman(t *testing.T) {
	var buf bytes.Buffer
	r := Report{
		SafeDelete: []string{"stale/one"},
		ReviewNoPR: []string{"orphan/two"},
	}
	printHuman(&buf, r, mustParseTime(t, "2026-08-17T00:00:00Z"))
	out := buf.String()

	for _, want := range []string{
		"## Branch reaper — 2026-08-17",
		"Safe to delete (merged, nothing pushed since): 1",
		"  - stale/one",
		"Review: no PR ever opened: 1",
		"  - orphan/two",
		"Review: PR closed without merging: 0",
		"Review: merged, but new commits pushed after: 0",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q; full output:\n%s", want, out)
		}
	}
}

func mustParseTime(t *testing.T, s string) time.Time {
	t.Helper()
	tm, err := parseTimestamp(s)
	if err != nil {
		t.Fatalf("parseTimestamp(%q): %v", s, err)
	}
	return tm
}
