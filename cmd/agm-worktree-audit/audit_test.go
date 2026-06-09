package main

import (
	"testing"
	"time"
)

// ref is a fixed "now" so age math is deterministic.
var ref = time.Date(2026, 6, 3, 12, 0, 0, 0, time.UTC)

func daysAgo(n int) time.Time { return ref.Add(-time.Duration(n) * 24 * time.Hour) }

func hasFinding(fs []Finding, kind FindingKind, branch string) bool {
	for _, f := range fs {
		if f.Kind == kind && f.Branch == branch {
			return true
		}
	}
	return false
}

func TestAbandonedWorktree(t *testing.T) {
	repos := []RepoData{{
		Name: "r", Path: "/r", BaseRef: "origin/main",
		Worktrees: []Worktree{
			{Path: "/wt/old", Branch: "old", LastCommit: daysAgo(10), HasRemote: true},
			{Path: "/wt/fresh", Branch: "fresh", LastCommit: daysAgo(2), HasRemote: true},
		},
	}}
	fs := Categorize(repos, ref, DefaultThresholds())
	if !hasFinding(fs, KindAbandonedWorktree, "old") {
		t.Error("expected 10-day-old worktree to be abandoned")
	}
	if hasFinding(fs, KindAbandonedWorktree, "fresh") {
		t.Error("2-day-old worktree should not be abandoned")
	}
}

func TestAbandonedExactlyAtThreshold(t *testing.T) {
	// 7 days exactly should trigger (>=).
	repos := []RepoData{{
		Name: "r", BaseRef: "origin/main",
		Worktrees: []Worktree{{Path: "/wt/edge", Branch: "edge", LastCommit: daysAgo(7), HasRemote: true}},
	}}
	fs := Categorize(repos, ref, DefaultThresholds())
	if !hasFinding(fs, KindAbandonedWorktree, "edge") {
		t.Error("worktree at exactly 7 days should be abandoned")
	}
}

func TestMainWorktreeNeverAbandoned(t *testing.T) {
	repos := []RepoData{{
		Name: "r", BaseRef: "origin/main",
		Worktrees: []Worktree{{Path: "/r", Branch: "main", IsMain: true, LastCommit: daysAgo(100), HasRemote: true}},
	}}
	fs := Categorize(repos, ref, DefaultThresholds())
	if len(fs) != 0 {
		t.Errorf("main worktree should never be flagged, got %+v", fs)
	}
}

func TestWorktreeNoRemote(t *testing.T) {
	repos := []RepoData{{
		Name: "r", BaseRef: "origin/main",
		Worktrees: []Worktree{
			{Path: "/wt/local", Branch: "local", LastCommit: daysAgo(1), HasRemote: false},
			{Path: "/wt/pushed", Branch: "pushed", LastCommit: daysAgo(1), HasRemote: true},
		},
	}}
	fs := Categorize(repos, ref, DefaultThresholds())
	if !hasFinding(fs, KindWorktreeNoRemote, "local") {
		t.Error("local-only worktree should be flagged")
	}
	if hasFinding(fs, KindWorktreeNoRemote, "pushed") {
		t.Error("pushed worktree should not be flagged for no-remote")
	}
}

func TestDetachedWorktreeNotFlaggedForRemote(t *testing.T) {
	repos := []RepoData{{
		Name: "r", BaseRef: "origin/main",
		Worktrees: []Worktree{{Path: "/wt/det", Branch: "", LastCommit: daysAgo(1), HasRemote: false}},
	}}
	fs := Categorize(repos, ref, DefaultThresholds())
	if hasFinding(fs, KindWorktreeNoRemote, "") {
		t.Error("detached worktree has no branch — should not be flagged for no-remote")
	}
}

func TestMergedNotDeleted(t *testing.T) {
	repos := []RepoData{{
		Name: "r", BaseRef: "origin/main",
		Branches: []Branch{
			{Name: "main", IsBase: true, Merged: false, LastCommit: daysAgo(1)},
			{Name: "done", Merged: true, Ahead: 0, Behind: 3, LastCommit: daysAgo(1)},
		},
	}}
	fs := Categorize(repos, ref, DefaultThresholds())
	if !hasFinding(fs, KindMergedNotDeleted, "done") {
		t.Error("merged branch should be flagged")
	}
	if hasFinding(fs, KindMergedNotDeleted, "main") {
		t.Error("base branch must never be flagged")
	}
}

func TestStaleUnmerged(t *testing.T) {
	repos := []RepoData{{
		Name: "r", BaseRef: "origin/main",
		Branches: []Branch{
			{Name: "stale", Merged: false, Ahead: 2, Behind: 5, LastCommit: daysAgo(20)},
			{Name: "recent", Merged: false, Ahead: 1, Behind: 0, LastCommit: daysAgo(3)},
		},
	}}
	fs := Categorize(repos, ref, DefaultThresholds())
	if !hasFinding(fs, KindStaleUnmerged, "stale") {
		t.Error("20-day unmerged branch should be stale")
	}
	if hasFinding(fs, KindStaleUnmerged, "recent") {
		t.Error("3-day unmerged branch should not be stale")
	}
}

func TestMergedTakesPrecedenceOverStale(t *testing.T) {
	// A branch that is both merged and old should report merged, not stale.
	repos := []RepoData{{
		Name: "r", BaseRef: "origin/main",
		Branches: []Branch{{Name: "b", Merged: true, LastCommit: daysAgo(40)}},
	}}
	fs := Categorize(repos, ref, DefaultThresholds())
	if !hasFinding(fs, KindMergedNotDeleted, "b") {
		t.Error("expected merged finding")
	}
	if hasFinding(fs, KindStaleUnmerged, "b") {
		t.Error("merged branch should not also be reported stale")
	}
}

func TestDeterministicSort(t *testing.T) {
	repos := []RepoData{
		{Name: "zeta", BaseRef: "origin/main", Branches: []Branch{{Name: "b", Merged: true, LastCommit: daysAgo(1)}}},
		{Name: "alpha", BaseRef: "origin/main", Branches: []Branch{{Name: "a", Merged: true, LastCommit: daysAgo(1)}}},
	}
	fs := Categorize(repos, ref, DefaultThresholds())
	if len(fs) != 2 || fs[0].Repo != "alpha" || fs[1].Repo != "zeta" {
		t.Errorf("findings not sorted by repo: %+v", fs)
	}
}

func TestZeroTimeNotFlagged(t *testing.T) {
	// Unknown commit time (zero) must not be treated as infinitely old.
	repos := []RepoData{{
		Name: "r", BaseRef: "origin/main",
		Worktrees: []Worktree{{Path: "/wt/x", Branch: "x", LastCommit: time.Time{}, HasRemote: true}},
		Branches:  []Branch{{Name: "x", Merged: false, LastCommit: time.Time{}}},
	}}
	fs := Categorize(repos, ref, DefaultThresholds())
	if hasFinding(fs, KindAbandonedWorktree, "x") {
		t.Error("zero-time worktree should not be flagged abandoned")
	}
	if hasFinding(fs, KindStaleUnmerged, "x") {
		t.Error("zero-time branch should not be flagged stale")
	}
}

func TestWorktreeFindingEnrichedFromBranch(t *testing.T) {
	// An abandoned worktree should borrow ahead/behind/merged from the
	// matching local branch entry.
	repos := []RepoData{{
		Name: "r", BaseRef: "origin/main",
		Worktrees: []Worktree{{Path: "/wt/x", Branch: "x", LastCommit: daysAgo(10), HasRemote: true}},
		Branches:  []Branch{{Name: "x", Ahead: 4, Behind: 9, Merged: false, LastCommit: daysAgo(10)}},
	}}
	fs := Categorize(repos, ref, DefaultThresholds())
	var found *Finding
	for i := range fs {
		if fs[i].Kind == KindAbandonedWorktree {
			found = &fs[i]
		}
	}
	if found == nil {
		t.Fatal("expected abandoned-worktree finding")
	}
	if found.Ahead != 4 || found.Behind != 9 {
		t.Errorf("worktree finding not enriched: got +%d/-%d, want +4/-9", found.Ahead, found.Behind)
	}
}

func TestWorktreeFindingUnknownAheadBehindWhenNoBranch(t *testing.T) {
	// No matching branch entry -> ahead/behind reported as unknown (-1).
	repos := []RepoData{{
		Name: "r", BaseRef: "origin/main",
		Worktrees: []Worktree{{Path: "/wt/x", Branch: "x", LastCommit: daysAgo(10), HasRemote: false}},
	}}
	fs := Categorize(repos, ref, DefaultThresholds())
	for _, f := range fs {
		if f.Ahead != -1 || f.Behind != -1 {
			t.Errorf("expected unknown ahead/behind, got +%d/-%d", f.Ahead, f.Behind)
		}
	}
}

func TestCustomThresholds(t *testing.T) {
	th := Thresholds{WorktreeStale: 1 * 24 * time.Hour, BranchStale: 2 * 24 * time.Hour}
	repos := []RepoData{{
		Name: "r", BaseRef: "origin/main",
		Worktrees: []Worktree{{Path: "/wt/x", Branch: "x", LastCommit: daysAgo(2), HasRemote: true}},
		Branches:  []Branch{{Name: "y", Merged: false, LastCommit: daysAgo(3)}},
	}}
	fs := Categorize(repos, ref, th)
	if !hasFinding(fs, KindAbandonedWorktree, "x") {
		t.Error("2-day worktree should be abandoned under 1-day threshold")
	}
	if !hasFinding(fs, KindStaleUnmerged, "y") {
		t.Error("3-day branch should be stale under 2-day threshold")
	}
}
