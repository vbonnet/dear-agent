package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/vbonnet/dear-agent/internal/gittest"
)

const (
	tipSHA   = "1111111111111111111111111111111111111111"
	otherSHA = "2222222222222222222222222222222222222222"
)

func TestClassifyBranch(t *testing.T) {
	tests := []struct {
		name   string
		tipSHA string
		prs    []prRecord
		want   string
	}{
		{
			name:   "no PR at all",
			tipSHA: tipSHA,
			prs:    nil,
			want:   bucketReviewNoPR,
		},
		{
			name:   "open PR",
			tipSHA: tipSHA,
			prs:    []prRecord{{Number: 1, State: "OPEN"}},
			want:   bucketReviewOpenPR,
		},
		{
			name:   "open PR wins over a stale merged one in the same list",
			tipSHA: tipSHA,
			prs: []prRecord{
				{Number: 1, State: "MERGED", MergedAt: "2026-05-01T00:00:00Z", HeadRefOid: tipSHA},
				{Number: 2, State: "OPEN"},
			},
			want: bucketReviewOpenPR,
		},
		{
			name:   "merged, tip still the merged head -> safe",
			tipSHA: tipSHA,
			prs:    []prRecord{{Number: 1, State: "MERGED", MergedAt: "2026-06-10T12:00:00Z", HeadRefOid: tipSHA}},
			want:   bucketSafeDelete,
		},
		{
			name:   "merged head SHA compares case-insensitively",
			tipSHA: "ABCDEF1234567890abcdef1234567890ABCDEF12",
			prs: []prRecord{{
				Number: 1, State: "MERGED", MergedAt: "2026-06-10T12:00:00Z",
				HeadRefOid: "abcdef1234567890ABCDEF1234567890abcdef12",
			}},
			want: bucketSafeDelete,
		},
		{
			name:   "merged, tip moved off the merged head -> needs review",
			tipSHA: otherSHA,
			prs:    []prRecord{{Number: 1, State: "MERGED", MergedAt: "2026-06-10T12:00:00Z", HeadRefOid: tipSHA}},
			want:   bucketReviewNewCommitsAfterMerge,
		},
		{
			// The regression the SHA check exists for: a force-push after the
			// merge can leave a tip whose committer date predates mergedAt, so
			// a timestamp comparison would have called this safe and deleted
			// unmerged work.
			name:   "merged, tip force-pushed to an older commit -> needs review",
			tipSHA: otherSHA,
			prs: []prRecord{{
				Number: 1, State: "MERGED", MergedAt: "2026-06-10T12:00:00Z",
				HeadRefOid: tipSHA,
			}},
			want: bucketReviewNewCommitsAfterMerge,
		},
		{
			name:   "merged with no recorded head SHA -> needs review, never safe",
			tipSHA: tipSHA,
			prs:    []prRecord{{Number: 1, State: "MERGED", MergedAt: "2026-06-10T12:00:00Z"}},
			want:   bucketReviewNewCommitsAfterMerge,
		},
		{
			name:   "unknown tip SHA never matches an empty head SHA",
			tipSHA: "",
			prs:    []prRecord{{Number: 1, State: "MERGED", MergedAt: "2026-06-10T12:00:00Z"}},
			want:   bucketReviewNewCommitsAfterMerge,
		},
		{
			name:   "multiple merged PRs: classify against the most recent mergedAt",
			tipSHA: tipSHA,
			prs: []prRecord{
				{Number: 1, State: "MERGED", MergedAt: "2026-05-01T00:00:00Z", HeadRefOid: otherSHA},
				{Number: 2, State: "MERGED", MergedAt: "2026-07-05T00:00:00Z", HeadRefOid: tipSHA},
			},
			want: bucketSafeDelete,
		},
		{
			name:   "most recent merged PR's head is what counts, not an older match",
			tipSHA: tipSHA,
			prs: []prRecord{
				{Number: 1, State: "MERGED", MergedAt: "2026-05-01T00:00:00Z", HeadRefOid: tipSHA},
				{Number: 2, State: "MERGED", MergedAt: "2026-07-05T00:00:00Z", HeadRefOid: otherSHA},
			},
			want: bucketReviewNewCommitsAfterMerge,
		},
		{
			name:   "closed without merging, no merged PR",
			tipSHA: tipSHA,
			prs:    []prRecord{{Number: 1, State: "CLOSED"}},
			want:   bucketReviewClosedUnmerged,
		},
		{
			name:   "closed and no-PR mix resolves to closed",
			tipSHA: tipSHA,
			prs:    []prRecord{{Number: 1, State: "CLOSED"}, {Number: 2, State: "CLOSED"}},
			want:   bucketReviewClosedUnmerged,
		},
		{
			name:   "empty PR list",
			tipSHA: tipSHA,
			prs:    []prRecord{},
			want:   bucketReviewNoPR,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := classifyBranch(tt.tipSHA, tt.prs, 0); got != tt.want {
				t.Errorf("classifyBranch(%q, %v) = %q, want %q", tt.tipSHA, tt.prs, got, tt.want)
			}
		})
	}
}

func TestSameRepoPRs(t *testing.T) {
	in := []prRecord{
		{Number: 1, State: "MERGED", IsCrossRepository: true, HeadRefOid: tipSHA},
		{Number: 2, State: "CLOSED"},
	}
	got := sameRepoPRs(in)
	if len(got) != 1 || got[0].Number != 2 {
		t.Fatalf("fork PR should be dropped, got %+v", got)
	}

	// A fork PR that merged must not make this repo's same-named branch
	// look safe to delete.
	if b := classifyBranch(tipSHA, sameRepoPRs(in[:1]), 0); b != bucketReviewNoPR {
		t.Errorf("fork-only history classified as %q, want %q", b, bucketReviewNoPR)
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
	// As a workflow would express it: plain names plus a wildcard family.
	protected := []string{"main", "master", "develop", "HEAD", "release/**", "v*"}
	tests := []struct {
		branch string
		want   bool
	}{
		{"main", true},
		{"master", true},
		{"develop", true},
		{"HEAD", true},
		{"feat/branch-reaper-durable-fix", false},
		{"mainline", false},
		{"develop-x", false},
		{"dependabot/go_modules/foo", false},
		// A `release/**` trigger protects the whole family, not a branch
		// literally named "release/**".
		{"release/v2", true},
		{"release/2026/q3", true},
		{"release", false},
		{"v1", true},
		{"v1/2", false}, // single * stops at "/"
	}
	for _, tt := range tests {
		if got := isProtected(tt.branch, protected); got != tt.want {
			t.Errorf("isProtected(%q, %v) = %v, want %v", tt.branch, protected, got, tt.want)
		}
	}
}

func TestMatchBranchFilter(t *testing.T) {
	tests := []struct {
		pattern string
		name    string
		want    bool
	}{
		{"main", "main", true},
		{"main", "mainline", false},
		{"release/**", "release/v2/hotfix", true},
		{"release/*", "release/v2/hotfix", false},
		{"release/*", "release/v2", true},
		{"feat?", "feat1", true},
		{"feat?", "feat12", false},
		// Regex metacharacters in a branch name are literal, not syntax.
		{"v1.0", "v1.0", true},
		{"v1.0", "v1x0", false},
		// Character classes and `+` are part of GitHub's filter grammar.
		{"release/[0-9]", "release/2", true},
		{"release/[0-9]", "release/x", false},
		{"release/[!0-9]", "release/x", true},
		{"release/[!0-9]", "release/2", false},
		{"v1+", "v111", true},
		{"v1+", "v2", false},
		// A backslash escapes the next character.
		{`feat\*`, "feat*", true},
		{`feat\*`, "featx", false},
		// Untranslatable patterns fail CLOSED: they protect the branch
		// rather than leaving it eligible for deletion.
		{"release/[0-9", "anything", true},
		{`trailing\`, "anything", true},
		// Negated filters are exclusions in a trigger; they protect nothing.
		{"!main", "main", false},
		{"", "main", false},
	}
	for _, tt := range tests {
		if got := matchBranchFilter(tt.pattern, tt.name); got != tt.want {
			t.Errorf("matchBranchFilter(%q, %q) = %v, want %v", tt.pattern, tt.name, got, tt.want)
		}
	}
}

func TestBaseProtectedBranches(t *testing.T) {
	base := baseProtectedBranches()
	for _, b := range []string{"main", "master", "HEAD"} {
		if !isProtected(b, base) {
			t.Errorf("baseProtectedBranches() missing %q", b)
		}
	}
	if isProtected("develop", base) {
		t.Error("baseProtectedBranches() must not hardcode develop -- that's the bug being fixed, it must come from dynamic detection")
	}
}

func TestExtractTriggerBranches(t *testing.T) {
	tests := []struct {
		name string
		yaml string
		want []string
	}{
		{
			name: "push and pull_request branches",
			yaml: "on:\n  push:\n    branches: [main, develop]\n  pull_request:\n    branches: [main]\n",
			want: []string{"main", "develop", "main"},
		},
		{
			name: "no on key",
			yaml: "name: x\njobs:\n  build:\n    runs-on: ubuntu-latest\n",
			want: nil,
		},
		{
			name: "on with no branches filter (workflow_dispatch only)",
			yaml: "on:\n  workflow_dispatch:\n",
			want: nil,
		},
		{
			name: "malformed yaml yields nothing, not an error",
			yaml: "not: [valid: yaml: at: all",
			want: nil,
		},
		{
			name: "schedule-only trigger has no branches",
			yaml: "on:\n  schedule:\n    - cron: \"0 8 * * 5\"\n",
			want: nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractTriggerBranches([]byte(tt.yaml))
			if len(got) != len(tt.want) {
				t.Fatalf("extractTriggerBranches(%q) = %v, want %v", tt.yaml, got, tt.want)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("extractTriggerBranches(%q)[%d] = %q, want %q", tt.yaml, i, got[i], tt.want[i])
				}
			}
		})
	}
}

func TestWorkflowTriggerBranches(t *testing.T) {
	dir := t.TempDir()
	writeReaperFixture(t, dir, "ci.yml", "on:\n  push:\n    branches: [main, develop]\n")
	writeReaperFixture(t, dir, "release.yaml", "on:\n  push:\n    branches: [main, release]\n")
	writeReaperFixture(t, dir, "notes.txt", "on:\n  push:\n    branches: [ignored-not-yaml]\n")

	got := workflowTriggerBranches(dir)
	want := map[string]bool{"main": true, "develop": true, "release": true}
	seen := map[string]bool{}
	for _, b := range got {
		seen[b] = true
		if b == "ignored-not-yaml" {
			t.Errorf("workflowTriggerBranches must not read non-yaml files, got %q from notes.txt", b)
		}
	}
	for b := range want {
		if !seen[b] {
			t.Errorf("workflowTriggerBranches missing %q, got %v", b, got)
		}
	}
}

func TestWorkflowTriggerBranches_MissingDir(t *testing.T) {
	got := workflowTriggerBranches("/nonexistent/does-not-exist")
	if got != nil {
		t.Errorf("workflowTriggerBranches(missing dir) = %v, want nil", got)
	}
}

func writeReaperFixture(t *testing.T, dir, name, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o600); err != nil {
		t.Fatalf("write fixture %s: %v", name, err)
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
		"refs/remotes/origin/HEAD" + branchFieldSep + tipSHA,
		"refs/remotes/origin/main" + branchFieldSep + tipSHA,
		"refs/remotes/origin/feat/x" + branchFieldSep + otherSHA,
		"", // trailing blank line, as real `git for-each-ref` output ends with \n
	}, "\n")

	got := parseBranchList(in)
	if len(got) != 2 {
		t.Fatalf("expected 2 branches (HEAD symref dropped), got %d: %+v", len(got), got)
	}
	if got[0].Name != "main" || got[0].TipSHA != tipSHA {
		t.Errorf("branch[0] = %+v", got[0])
	}
	if got[1].Name != "feat/x" || got[1].TipSHA != otherSHA {
		t.Errorf("branch[1] = %+v", got[1])
	}
}

// A branch name may legally contain "|" (git check-ref-format accepts it),
// which is exactly why the record separator is NUL and not a pipe.
func TestParseBranchList_PipeInBranchNameSurvives(t *testing.T) {
	in := "refs/remotes/origin/foo|bar" + branchFieldSep + tipSHA + "\n"
	got := parseBranchList(in)
	if len(got) != 1 {
		t.Fatalf("expected 1 branch, got %+v", got)
	}
	if got[0].Name != "foo|bar" || got[0].TipSHA != tipSHA {
		t.Errorf("pipe-containing branch mangled: %+v", got[0])
	}
}

func TestParseBranchList_MalformedLinesSkipped(t *testing.T) {
	in := strings.Join([]string{
		"no-separator-here",
		"refs/remotes/origin/missing-sha" + branchFieldSep,
		"refs/heads/not-a-remote" + branchFieldSep + tipSHA,
		"refs/remotes/origin/ok" + branchFieldSep + tipSHA,
		"",
	}, "\n")
	got := parseBranchList(in)
	if len(got) != 1 || got[0].Name != "ok" {
		t.Fatalf("expected only the well-formed line to survive, got %+v", got)
	}
}

func TestReportJSON_EmptyBucketsAreArraysNotNull(t *testing.T) {
	var buf bytes.Buffer
	// LookupFailed deliberately left nil -- printJSON must normalize it to
	// [] just like the other buckets' zero values, never emit null.
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
	for _, key := range []string{"safe_delete", "review_no_pr", "review_closed_unmerged", "review_new_commits_after_merge", "lookup_failed", "deleted", "delete_failed"} {
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

// gh reports an unmerged PR's mergedAt as JSON null; that must decode to the
// empty string rather than failing the whole branch lookup.
func TestPRRecord_NullMergedAtDecodes(t *testing.T) {
	var prs []prRecord
	raw := `[{"number":1,"state":"OPEN","mergedAt":null,"headRefOid":"` + tipSHA + `","isCrossRepository":false}]`
	if err := json.Unmarshal([]byte(raw), &prs); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(prs) != 1 || prs[0].MergedAt != "" || prs[0].HeadRefOid != tipSHA {
		t.Fatalf("decoded %+v", prs)
	}
}

// A rejected delete must be counted, not swallowed — the workflow turns the
// non-zero exit into a hard failure instead of reporting the branch as
// auto-deleted.
func TestExecuteSafeDeletes_CountsFailuresAndKeepsGoing(t *testing.T) {
	targets := []branchInfo{
		{Name: "ok/one", TipSHA: tipSHA},
		{Name: "bad/two", TipSHA: otherSHA},
		{Name: "ok/three", TipSHA: tipSHA},
	}
	var attempted []string
	del := func(b branchInfo) error {
		attempted = append(attempted, b.Name)
		if b.Name == "bad/two" {
			return errors.New("stale info")
		}
		return nil
	}

	var stderr bytes.Buffer
	var rep Report
	failed := executeSafeDeletes(targets, del, &stderr, &rep)
	if failed != 1 {
		t.Errorf("failed = %d, want 1", failed)
	}
	if len(attempted) != 3 {
		t.Errorf("a failure aborted the remaining deletes: %v", attempted)
	}
	if !strings.Contains(stderr.String(), "delete bad/two") {
		t.Errorf("failure not reported; stderr:\n%s", stderr.String())
	}
	// The branch that is still on the remote must never appear as deleted.
	if strings.Join(rep.Deleted, ",") != "ok/one,ok/three" {
		t.Errorf("deleted = %v, want [ok/one ok/three]", rep.Deleted)
	}
	if strings.Join(rep.DeleteFailed, ",") != "bad/two" {
		t.Errorf("delete_failed = %v, want [bad/two]", rep.DeleteFailed)
	}
}

func TestExecuteSafeDeletes_LeasesEachDeleteToItsClassifiedSHA(t *testing.T) {
	targets := []branchInfo{{Name: "ok/one", TipSHA: tipSHA}}
	var seen branchInfo
	var rep Report
	executeSafeDeletes(targets, func(b branchInfo) error { seen = b; return nil }, io.Discard, &rep)
	if seen.TipSHA != tipSHA {
		t.Errorf("delete got SHA %q, want the classified %q", seen.TipSHA, tipSHA)
	}
}

func TestParseRemote(t *testing.T) {
	tests := []struct {
		url       string
		host      string
		ownerRepo string
	}{
		{"https://github.com/vbonnet/dear-agent.git", "github.com", "vbonnet/dear-agent"},
		{"https://github.com/vbonnet/dear-agent", "github.com", "vbonnet/dear-agent"},
		{"https://github.com/vbonnet/dear-agent/", "github.com", "vbonnet/dear-agent"},
		{"git@github.com:vbonnet/dear-agent.git", "github.com", "vbonnet/dear-agent"},
		{"ssh://git@github.com/vbonnet/dear-agent", "github.com", "vbonnet/dear-agent"},
		{"ssh://git@github.com:22/vbonnet/dear-agent", "github.com", "vbonnet/dear-agent"},
		// Another forge with the same path is a different repository.
		{"https://gitlab.com/vbonnet/dear-agent.git", "gitlab.com", "vbonnet/dear-agent"},
		// A filesystem remote has no host, so it can never be confirmed.
		{"/local/path/repo", "", "path/repo"},
		{"", "", ""},
		{"nonsense", "", ""},
	}
	for _, tt := range tests {
		host, ownerRepo := parseRemote(tt.url)
		if host != tt.host || ownerRepo != tt.ownerRepo {
			t.Errorf("parseRemote(%q) = (%q, %q), want (%q, %q)", tt.url, host, ownerRepo, tt.host, tt.ownerRepo)
		}
	}
}

func TestSameRepository(t *testing.T) {
	if !sameRepository("github.com", "vbonnet/dear-agent", "VBonnet/Dear-Agent") {
		t.Error("GitHub owner/repo comparison must be case-insensitive")
	}
	if !sameRepository("GitHub.com", "vbonnet/dear-agent", "vbonnet/dear-agent") {
		t.Error("host comparison must be case-insensitive too")
	}
	if sameRepository("github.com", "vbonnet/dear-agent", "someone-else/dear-agent") {
		t.Error("different owners must not compare equal")
	}
	// The whole point of carrying the host: an identical owner/repo path on
	// another forge is a different repository.
	if sameRepository("gitlab.com", "vbonnet/dear-agent", "vbonnet/dear-agent") {
		t.Error("a non-GitHub origin must never be accepted as the GitHub repo")
	}
	// A hostless (filesystem) remote can never prove a match, so --execute
	// stays refused.
	if sameRepository("", "vbonnet/dear-agent", "vbonnet/dear-agent") {
		t.Error("a hostless remote must never compare equal")
	}
	if sameRepository("github.com", "", "vbonnet/dear-agent") || sameRepository("github.com", "vbonnet/dear-agent", "") {
		t.Error("an empty identifier must never compare equal")
	}
}

func TestGHHost(t *testing.T) {
	t.Setenv("GH_HOST", "")
	if got := ghHost(); got != "github.com" {
		t.Errorf("ghHost() = %q, want github.com", got)
	}
	t.Setenv("GH_HOST", "github.example.com")
	if got := ghHost(); got != "github.example.com" {
		t.Errorf("ghHost() = %q, want the GH_HOST override", got)
	}
}

// A merged branch that is still the BASE of an open child PR must never be
// reaped: `gh pr list --head` cannot see the child, so the base count is a
// separate veto.
func TestClassifyBranch_OpenBasePRVetoesDeletion(t *testing.T) {
	merged := []prRecord{{Number: 1, State: "MERGED", MergedAt: "2026-06-10T12:00:00Z", HeadRefOid: tipSHA}}
	if got := classifyBranch(tipSHA, merged, 0); got != bucketSafeDelete {
		t.Fatalf("precondition: want %q with no child PRs, got %q", bucketSafeDelete, got)
	}
	if got := classifyBranch(tipSHA, merged, 1); got != bucketReviewOpenPR {
		t.Errorf("classifyBranch with an open child PR = %q, want %q", got, bucketReviewOpenPR)
	}
}

// A gh failure for one branch (auth, rate limit, transient API error) must
// not be silently reclassified as "no PR found", and must not abort
// classification of every other branch in the run -- this tool walks every
// branch in the repo on a schedule, so one flaky lookup should not deny a
// report on everything else.
func TestClassifyBranches_LookupFailureIsBucketedNotFatal(t *testing.T) {
	installReaperFakeGH(t, `
head=""
base=""
prev=""
for arg in "$@"; do
  if [ "$prev" = "--head" ]; then
    head="$arg"
  fi
  if [ "$prev" = "--base" ]; then
    base="$arg"
  fi
  prev="$arg"
done
# The base-PR veto query: no branch here has an open child PR.
if [ -n "$base" ]; then
  printf '%s\n' '[]'
  exit 0
fi
case "$head" in
  "bad/branch") echo "gh: rate limited" >&2; exit 1 ;;
  "good/branch") printf '%s\n' '[]' ;;
  *) echo "unexpected head: $head" >&2; exit 2 ;;
esac
`)
	branches := []branchInfo{
		{Name: "bad/branch", TipSHA: tipSHA},
		{Name: "good/branch", TipSHA: tipSHA},
	}
	var stderr bytes.Buffer
	rep, targets := classifyBranches(context.Background(), "owner/repo", nil, branches, &stderr)

	if len(targets) != 0 {
		t.Errorf("targets = %v, want none (neither branch is safe_delete)", targets)
	}
	if len(rep.LookupFailed) != 1 || rep.LookupFailed[0] != "bad/branch" {
		t.Errorf("LookupFailed = %v, want [bad/branch]", rep.LookupFailed)
	}
	if len(rep.ReviewNoPR) != 1 || rep.ReviewNoPR[0] != "good/branch" {
		t.Errorf("ReviewNoPR = %v, want [good/branch] -- a lookup failure for one branch must not stop classification of the next one", rep.ReviewNoPR)
	}
	if !strings.Contains(stderr.String(), "bad/branch") {
		t.Errorf("stderr does not report the failing branch: %s", stderr.String())
	}
}

// PR history truncated by the fetch limit is exactly as untrustworthy as an
// outright lookup failure: BRR-02 requires seeing every open PR to veto a
// deletion, so a saturated page must land in lookup_failed, never
// safe_delete.
func TestFetchPRs_TruncatedAtLimitIsAnError(t *testing.T) {
	installReaperFakeGH(t, `
n=`+strconv.Itoa(prFetchLimit)+`
out="["
i=0
while [ "$i" -lt "$n" ]; do
  [ "$i" -gt 0 ] && out="$out,"
  out="$out{\"number\":$i,\"state\":\"CLOSED\"}"
  i=$((i+1))
done
out="$out]"
printf '%s\n' "$out"
`)
	_, err := fetchPRs(context.Background(), "owner/repo", "reused/name")
	if err == nil {
		t.Fatal("fetchPRs at the truncation limit: want an error, got nil")
	}
}

func installReaperFakeGH(t *testing.T, body string) {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "gh")
	if err := os.WriteFile(path, []byte("#!/bin/sh\nset -eu\n"+body), 0o700); err != nil {
		t.Fatalf("write fake gh: %v", err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
}

// The deletion window: classification walked the whole repo, and someone
// opened a PR in the meantime. The SHA lease cannot see that -- opening a
// PR does not move the tip -- so the recheck must catch it.
func TestConfirmNoOpenPRs(t *testing.T) {
	t.Run("open head PR blocks the delete", func(t *testing.T) {
		installReaperFakeGH(t, `
for arg in "$@"; do if [ "$arg" = "--base" ]; then printf '%s\n' '[]'; exit 0; fi; done
printf '%s\n' '[{"number":7,"state":"OPEN"}]'
`)
		err := confirmNoOpenPRs(context.Background(), "owner/repo", "some/branch")
		if err == nil || !strings.Contains(err.Error(), "#7") {
			t.Fatalf("want an error naming PR #7, got %v", err)
		}
	})

	t.Run("open base PR blocks the delete", func(t *testing.T) {
		installReaperFakeGH(t, `
for arg in "$@"; do if [ "$arg" = "--base" ]; then printf '%s\n' '[{"number":9}]'; exit 0; fi; done
printf '%s\n' '[]'
`)
		err := confirmNoOpenPRs(context.Background(), "owner/repo", "some/branch")
		if err == nil || !strings.Contains(err.Error(), "base") {
			t.Fatalf("want a base-PR error, got %v", err)
		}
	})

	t.Run("a failed recheck fails closed", func(t *testing.T) {
		installReaperFakeGH(t, `
echo "gh: rate limited" >&2
exit 1
`)
		if err := confirmNoOpenPRs(context.Background(), "owner/repo", "some/branch"); err == nil {
			t.Fatal("a lookup failure must block the delete, not permit it")
		}
	})

	t.Run("nothing open permits the delete", func(t *testing.T) {
		installReaperFakeGH(t, `printf '%s\n' '[]'`)
		if err := confirmNoOpenPRs(context.Background(), "owner/repo", "some/branch"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})
}

// BRR-19: a delete that git rejects because the ref is already gone still
// achieved the requested end state, so it must not be reported as a failure
// that leaves the branch on the remote.
func TestDeleteBranch_AlreadyAbsentCountsAsDeleted(t *testing.T) {
	work := newReaperGitFixture(t)
	t.Chdir(work)
	// deleteBranch shells out to a plain `git push`, which would find the
	// developer's own hooks through core.hooksPath. Point the fixture's own
	// config at the sandbox's empty hooks path so it cannot.
	gittest.HardenRepo(t, work)

	// A branch that exists: a leased delete removes it.
	sha := strings.TrimSpace(gittest.Run(t, work, "rev-parse", "refs/remotes/origin/doomed"))
	if err := deleteBranch(t.Context(), branchInfo{Name: "doomed", TipSHA: sha}); err != nil {
		t.Fatalf("deleting an existing branch: %v", err)
	}
	if out := strings.TrimSpace(gittest.Run(t, work, "ls-remote", "--heads", "origin", "refs/heads/doomed")); out != "" {
		t.Fatalf("branch survived the delete: %q", out)
	}

	// The same delete replayed: git errors, but the end state already holds.
	if err := deleteBranch(t.Context(), branchInfo{Name: "doomed", TipSHA: sha}); err != nil {
		t.Errorf("deleting an already-absent branch reported failure: %v", err)
	}
}

// newReaperGitFixture builds a throwaway origin + checkout with one
// deletable branch, and returns the checkout path.
func newReaperGitFixture(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	bare := filepath.Join(root, "origin.git")
	work := filepath.Join(root, "work")
	gittest.Run(t, root, "init", "--quiet", "--bare", bare)
	gittest.Run(t, root, "clone", "--quiet", bare, work)
	gittest.Run(t, work, "config", "user.email", "reaper@test.invalid")
	gittest.Run(t, work, "config", "user.name", "reaper test")
	gittest.Run(t, work, "commit", "--quiet", "--allow-empty", "-m", "base")
	gittest.Run(t, work, "push", "--quiet", "origin", "HEAD:refs/heads/main")
	gittest.Run(t, work, "push", "--quiet", "origin", "HEAD:refs/heads/doomed")
	gittest.Run(t, work, "fetch", "--quiet", "origin", "+refs/heads/*:refs/remotes/origin/*")
	return work
}

func TestPrintHuman(t *testing.T) {
	var buf bytes.Buffer
	r := Report{
		SafeDelete: []string{"stale/one"},
		ReviewNoPR: []string{"orphan/two"},
	}
	printHuman(&buf, r, time.Date(2026, 8, 17, 0, 0, 0, 0, time.UTC))
	out := buf.String()

	for _, want := range []string{
		"## Branch reaper — 2026-08-17",
		"Safe to delete (merged, tip still the merged head): 1",
		"  - stale/one",
		"Review: no PR ever opened: 1",
		"  - orphan/two",
		"Review: PR closed without merging: 0",
		"Review: merged, but tip moved off the merged head: 0",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q; full output:\n%s", want, out)
		}
	}
}
