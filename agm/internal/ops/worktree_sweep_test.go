package ops

import (
	"errors"
	"testing"
	"time"
)

// fakeSweepDeps drives the classification matrix with no real repo. Every
// probe is keyed by worktree path or branch so each test wires exactly the
// state it exercises; an unset key yields the zero value (the conservative
// answer for the bool/known probes).
type fakeSweepDeps struct {
	discovered    []DiscoveredWorktree
	discoverErr   error
	discoverCalls int

	dirty    map[string]bool
	dirtyErr map[string]error

	last    map[string]time.Time
	subject map[string]string
	lastErr map[string]error

	mainRepo    map[string]string
	mainRepoErr map[string]error

	base map[string]string // keyed by mainRepo

	ancestor    map[string]bool // keyed by branch
	ancestorErr map[string]error

	prState map[string]string // keyed by branch
	prKnown map[string]bool   // keyed by branch

	unpushed    map[string]bool // keyed by branch
	unpushedErr map[string]error

	commitsAbove    map[string]int   // keyed by branch
	commitsAboveErr map[string]error // keyed by branch

	awaiting       map[string]bool   // keyed by worktree path
	awaitingDetail map[string]string // keyed by worktree path

	removeErr  map[string]error
	deleteErr  map[string]error
	removed    []string
	delBranch  []string
	delForce   map[string]bool
	removeArgs map[string]bool // path -> force used
}

func (f *fakeSweepDeps) Discover(string) ([]DiscoveredWorktree, error) {
	f.discoverCalls++
	return f.discovered, f.discoverErr
}
func (f *fakeSweepDeps) IsDirty(p string) (bool, error) {
	if err := f.dirtyErr[p]; err != nil {
		return true, err
	}
	return f.dirty[p], nil
}
func (f *fakeSweepDeps) LastCommit(p string) (time.Time, string, error) {
	if err := f.lastErr[p]; err != nil {
		return time.Time{}, "", err
	}
	return f.last[p], f.subject[p], nil
}
func (f *fakeSweepDeps) MainRepo(p string) (string, error) {
	if err := f.mainRepoErr[p]; err != nil {
		return "", err
	}
	if v, ok := f.mainRepo[p]; ok {
		return v, nil
	}
	return "/repo", nil
}
func (f *fakeSweepDeps) BaseRef(repo string) string {
	if v, ok := f.base[repo]; ok {
		return v
	}
	return "origin/main"
}
func (f *fakeSweepDeps) IsAncestor(_, branch, _ string) (bool, error) {
	if err := f.ancestorErr[branch]; err != nil {
		return false, err
	}
	return f.ancestor[branch], nil
}
func (f *fakeSweepDeps) PRState(_, branch string) (string, bool) {
	return f.prState[branch], f.prKnown[branch]
}
func (f *fakeSweepDeps) HasUnpushedCommits(_, branch string) (bool, error) {
	if err := f.unpushedErr[branch]; err != nil {
		return true, err
	}
	return f.unpushed[branch], nil
}
func (f *fakeSweepDeps) CommitsAboveMergeBase(_, branch, _ string) (int, error) {
	if err := f.commitsAboveErr[branch]; err != nil {
		return -1, err
	}
	return f.commitsAbove[branch], nil
}
func (f *fakeSweepDeps) AwaitingInput(p string) (bool, string) {
	d := f.awaitingDetail[p]
	if d == "" {
		d = "test"
	}
	return f.awaiting[p], d
}
func (f *fakeSweepDeps) RemoveWorktree(_, p string, force bool) error {
	if err := f.removeErr[p]; err != nil {
		return err
	}
	if f.removeArgs == nil {
		f.removeArgs = map[string]bool{}
	}
	f.removeArgs[p] = force
	f.removed = append(f.removed, p)
	return nil
}
func (f *fakeSweepDeps) DeleteBranch(_, b string, force bool) error {
	if err := f.deleteErr[b]; err != nil {
		return err
	}
	if f.delForce == nil {
		f.delForce = map[string]bool{}
	}
	f.delForce[b] = force
	f.delBranch = append(f.delBranch, b)
	return nil
}

// classifyOne runs the pure classifier for a single discovered worktree.
func classifyOne(opts SweepOptions, d SweepDeps, dw DiscoveredWorktree) WorktreeStatus {
	return classify(opts, d, dw, cleanPath(opts.SelfPath))
}

func TestClassify_Matrix(t *testing.T) {
	const wt = "/wt/a"
	const br = "claude/a"

	tests := []struct {
		name       string
		opts       SweepOptions
		setup      func(*fakeSweepDeps)
		dw         DiscoveredWorktree
		wantClass  Classification
		wantReason string
	}{
		{
			name:       "self worktree is ACTIVE",
			opts:       SweepOptions{SelfPath: wt},
			dw:         DiscoveredWorktree{Path: wt, Branch: br},
			wantClass:  ClassActive,
			wantReason: "self-worktree",
		},
		{
			name:       "nested sandbox is ACTIVE",
			dw:         DiscoveredWorktree{Path: "/wt/x/.worktrees/uuid", Branch: br},
			wantClass:  ClassActive,
			wantReason: "nested-sandbox-worktree",
		},
		{
			name:       "live session by dir name is ACTIVE",
			opts:       SweepOptions{ActiveSessions: map[string]bool{"a": true}},
			dw:         DiscoveredWorktree{Path: wt, Branch: br},
			wantClass:  ClassActive,
			wantReason: "live-session",
		},
		{
			name:       "live session by branch is ACTIVE",
			opts:       SweepOptions{ActiveSessions: map[string]bool{br: true}},
			dw:         DiscoveredWorktree{Path: wt, Branch: br},
			wantClass:  ClassActive,
			wantReason: "live-session",
		},
		{
			name:       "detached HEAD is UNKNOWN never reaped",
			dw:         DiscoveredWorktree{Path: wt, Branch: ""},
			wantClass:  ClassUnknown,
			wantReason: "detached-head",
		},
		{
			name:       "status probe failure is UNKNOWN",
			setup:      func(f *fakeSweepDeps) { f.dirtyErr = map[string]error{wt: errors.New("x")} },
			dw:         DiscoveredWorktree{Path: wt, Branch: br},
			wantClass:  ClassUnknown,
			wantReason: "status-check-failed",
		},
		{
			name:       "dirty tree is DIRTY",
			setup:      func(f *fakeSweepDeps) { f.dirty = map[string]bool{wt: true} },
			dw:         DiscoveredWorktree{Path: wt, Branch: br},
			wantClass:  ClassDirty,
			wantReason: "uncommitted-changes",
		},
		{
			name:       "main repo unresolved is UNKNOWN",
			setup:      func(f *fakeSweepDeps) { f.mainRepoErr = map[string]error{wt: errors.New("x")} },
			dw:         DiscoveredWorktree{Path: wt, Branch: br},
			wantClass:  ClassUnknown,
			wantReason: "main-repo-unresolved",
		},
		{
			name:       "base ref unresolved is UNKNOWN",
			setup:      func(f *fakeSweepDeps) { f.base = map[string]string{"/repo": ""} },
			dw:         DiscoveredWorktree{Path: wt, Branch: br},
			wantClass:  ClassUnknown,
			wantReason: "base-ref-unresolved",
		},
		{
			name:       "ancestor probe failure is UNKNOWN",
			setup:      func(f *fakeSweepDeps) { f.ancestorErr = map[string]error{br: errors.New("x")} },
			dw:         DiscoveredWorktree{Path: wt, Branch: br},
			wantClass:  ClassUnknown,
			wantReason: "ancestor-check-failed",
		},
		{
			name:       "ancestor of base is MERGED",
			opts:       SweepOptions{CheckPR: true},
			setup:      func(f *fakeSweepDeps) { f.ancestor = map[string]bool{br: true} },
			dw:         DiscoveredWorktree{Path: wt, Branch: br},
			wantClass:  ClassMerged,
			wantReason: "ancestor-of-base",
		},
		{
			name: "squash-merged via PR MERGED is MERGED",
			opts: SweepOptions{CheckPR: true},
			setup: func(f *fakeSweepDeps) {
				f.prState = map[string]string{br: "MERGED"}
				f.prKnown = map[string]bool{br: true}
			},
			dw:         DiscoveredWorktree{Path: wt, Branch: br},
			wantClass:  ClassMerged,
			wantReason: "pr-merged",
		},
		{
			name: "open PR pushed not merged is ORPHANED",
			opts: SweepOptions{CheckPR: true},
			setup: func(f *fakeSweepDeps) {
				f.prState = map[string]string{br: "OPEN"}
				f.prKnown = map[string]bool{br: true}
			},
			dw:         DiscoveredWorktree{Path: wt, Branch: br},
			wantClass:  ClassOrphaned,
			wantReason: "open-pr-unmerged",
		},
		{
			name: "closed unmerged PR is ORPHANED",
			opts: SweepOptions{CheckPR: true},
			setup: func(f *fakeSweepDeps) {
				f.prState = map[string]string{br: "CLOSED"}
				f.prKnown = map[string]bool{br: true}
			},
			dw:         DiscoveredWorktree{Path: wt, Branch: br},
			wantClass:  ClassOrphaned,
			wantReason: "pr-closed-unmerged",
		},
		{
			name:       "no PR pushed is ORPHANED no-pr",
			opts:       SweepOptions{CheckPR: true},
			dw:         DiscoveredWorktree{Path: wt, Branch: br},
			wantClass:  ClassOrphaned,
			wantReason: "no-pr",
		},
		{
			name: "unpushed commits beat an open PR (data-loss risk)",
			opts: SweepOptions{CheckPR: true},
			setup: func(f *fakeSweepDeps) {
				f.prState = map[string]string{br: "OPEN"}
				f.prKnown = map[string]bool{br: true}
				f.unpushed = map[string]bool{br: true}
			},
			dw:         DiscoveredWorktree{Path: wt, Branch: br},
			wantClass:  ClassOrphaned,
			wantReason: "unpushed-commits",
		},
		{
			name:       "push probe failure is UNKNOWN",
			opts:       SweepOptions{CheckPR: true},
			setup:      func(f *fakeSweepDeps) { f.unpushedErr = map[string]error{br: errors.New("x")} },
			dw:         DiscoveredWorktree{Path: wt, Branch: br},
			wantClass:  ClassUnknown,
			wantReason: "push-check-failed",
		},
		{
			name:       "no-pr-check keeps squash-merged conservatively (not MERGED)",
			opts:       SweepOptions{CheckPR: false},
			dw:         DiscoveredWorktree{Path: wt, Branch: br},
			wantClass:  ClassOrphaned,
			wantReason: "no-pr",
		},
		{
			name: "awaiting input outranks a provably-merged tree (never reaped)",
			opts: SweepOptions{CheckPR: true},
			setup: func(f *fakeSweepDeps) {
				f.ancestor = map[string]bool{br: true} // would be MERGED
				f.awaiting = map[string]bool{wt: true}
				f.awaitingDetail = map[string]string{wt: "AskUserQuestion"}
			},
			dw:         DiscoveredWorktree{Path: wt, Branch: br},
			wantClass:  ClassAwaitingInput,
			wantReason: "awaiting-input:AskUserQuestion",
		},
		{
			name: "awaiting input outranks a dirty tree",
			setup: func(f *fakeSweepDeps) {
				f.dirty = map[string]bool{wt: true}
				f.awaiting = map[string]bool{wt: true}
			},
			dw:         DiscoveredWorktree{Path: wt, Branch: br},
			wantClass:  ClassAwaitingInput,
			wantReason: "awaiting-input:test",
		},
		{
			name: "awaiting input does NOT override a live session (ACTIVE wins)",
			opts: SweepOptions{ActiveSessions: map[string]bool{"a": true}},
			setup: func(f *fakeSweepDeps) {
				f.awaiting = map[string]bool{wt: true}
			},
			dw:         DiscoveredWorktree{Path: wt, Branch: br},
			wantClass:  ClassActive,
			wantReason: "live-session",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			f := &fakeSweepDeps{}
			if tc.setup != nil {
				tc.setup(f)
			}
			got := classifyOne(tc.opts, f, tc.dw)
			if got.Class != tc.wantClass || got.Reason != tc.wantReason {
				t.Fatalf("class=%s reason=%q, want class=%s reason=%q",
					got.Class, got.Reason, tc.wantClass, tc.wantReason)
			}
			if got.Class.Reapable() && got.Class != ClassMerged {
				t.Fatalf("only MERGED may be reapable; %s was", got.Class)
			}
		})
	}
}

func TestOrphanBranchDetection(t *testing.T) {
	const wt = "/wt/a"
	const br = "ce-task"

	t.Run("no-pr with commits above main sets IsOrphanBranch", func(t *testing.T) {
		f := &fakeSweepDeps{
			commitsAbove: map[string]int{br: 3},
		}
		got := classifyOne(SweepOptions{CheckPR: true}, f, DiscoveredWorktree{Path: wt, Branch: br})
		if got.Class != ClassOrphaned || got.Reason != "no-pr" {
			t.Fatalf("class=%s reason=%q, want ORPHANED/no-pr", got.Class, got.Reason)
		}
		if !got.IsOrphanBranch {
			t.Fatal("IsOrphanBranch should be true when commits above main > 0 and no PR")
		}
		if got.CommitsAboveMergeBase != 3 {
			t.Fatalf("CommitsAboveMergeBase = %d, want 3", got.CommitsAboveMergeBase)
		}
	})

	t.Run("no-pr with zero commits above main is not IsOrphanBranch", func(t *testing.T) {
		f := &fakeSweepDeps{
			commitsAbove: map[string]int{br: 0},
		}
		got := classifyOne(SweepOptions{CheckPR: true}, f, DiscoveredWorktree{Path: wt, Branch: br})
		if got.Class != ClassOrphaned || got.Reason != "no-pr" {
			t.Fatalf("class=%s reason=%q, want ORPHANED/no-pr", got.Class, got.Reason)
		}
		if got.IsOrphanBranch {
			t.Fatal("IsOrphanBranch should be false when branch has no commits above main")
		}
	})

	t.Run("CommitsAboveMergeBase error is fail-safe (no IsOrphanBranch)", func(t *testing.T) {
		f := &fakeSweepDeps{
			commitsAboveErr: map[string]error{br: errors.New("git error")},
		}
		got := classifyOne(SweepOptions{CheckPR: true}, f, DiscoveredWorktree{Path: wt, Branch: br})
		if got.Class != ClassOrphaned || got.Reason != "no-pr" {
			t.Fatalf("class=%s reason=%q, want ORPHANED/no-pr", got.Class, got.Reason)
		}
		if got.IsOrphanBranch {
			t.Fatal("IsOrphanBranch must be false on count error (fail-safe)")
		}
	})

	t.Run("open-pr orphan is never flagged as orphan-branch", func(t *testing.T) {
		f := &fakeSweepDeps{
			prState:      map[string]string{br: "OPEN"},
			prKnown:      map[string]bool{br: true},
			commitsAbove: map[string]int{br: 5},
		}
		got := classifyOne(SweepOptions{CheckPR: true}, f, DiscoveredWorktree{Path: wt, Branch: br})
		if got.Class != ClassOrphaned || got.Reason != "open-pr-unmerged" {
			t.Fatalf("class=%s reason=%q, want ORPHANED/open-pr-unmerged", got.Class, got.Reason)
		}
		if got.IsOrphanBranch {
			t.Fatal("IsOrphanBranch must only be set for no-pr reason")
		}
	})
}

func TestSweep_DryRunListsMergedButMutatesNothing(t *testing.T) {
	f := &fakeSweepDeps{
		discovered: []DiscoveredWorktree{
			{Path: "/wt/m", Repo: "r", Branch: "claude/m"},
			{Path: "/wt/d", Repo: "r", Branch: "claude/d"},
		},
		ancestor: map[string]bool{"claude/m": true},
		dirty:    map[string]bool{"/wt/d": true},
	}
	res, err := Sweep(SweepOptions{Execute: false, CheckPR: true}, f, nil)
	if err != nil {
		t.Fatalf("Sweep: %v", err)
	}
	if len(res.Removed) != 1 || res.Removed[0] != "/wt/m" {
		t.Fatalf("dry-run Removed = %v, want [/wt/m]", res.Removed)
	}
	if len(f.removed) != 0 || len(f.delBranch) != 0 {
		t.Fatalf("dry-run must not mutate: removed=%v delBranch=%v", f.removed, f.delBranch)
	}
	if c := res.Counts(); c[ClassMerged] != 1 || c[ClassDirty] != 1 {
		t.Fatalf("counts = %v", c)
	}
}

func TestSweep_ExecuteRemovesOnlyMergedAndForceDeletesBranch(t *testing.T) {
	f := &fakeSweepDeps{
		discovered: []DiscoveredWorktree{
			{Path: "/wt/m", Repo: "r", Branch: "claude/m"},
			{Path: "/wt/o", Repo: "r", Branch: "claude/o"}, // orphaned, must survive
		},
		ancestor: map[string]bool{"claude/m": true},
		mainRepo: map[string]string{"/wt/m": "/repo"},
	}
	res, err := Sweep(SweepOptions{Execute: true, CheckPR: true, ActiveSessionsKnown: true}, f, nil)
	if err != nil {
		t.Fatalf("Sweep: %v", err)
	}
	if len(f.removed) != 1 || f.removed[0] != "/wt/m" {
		t.Fatalf("executed removal = %v, want [/wt/m] only", f.removed)
	}
	if f.removeArgs["/wt/m"] != false {
		t.Fatalf("worktree removal must be non-force (git is the last guard)")
	}
	if !f.delForce["claude/m"] {
		t.Fatalf("orphaned local branch of a (squash-)merged worktree needs -D")
	}
	if len(res.Removed) != 1 || res.Removed[0] != "/wt/m" {
		t.Fatalf("res.Removed = %v", res.Removed)
	}
}

func TestSweep_RemoveFailureIsRecordedAndProcessingContinues(t *testing.T) {
	f := &fakeSweepDeps{
		discovered: []DiscoveredWorktree{
			{Path: "/wt/a", Repo: "r", Branch: "claude/a"},
			{Path: "/wt/b", Repo: "r", Branch: "claude/b"},
		},
		ancestor: map[string]bool{
			"claude/a": true,
			"claude/b": true,
		},
		removeErr: map[string]error{"/wt/a": errors.New("locked")},
	}
	res, err := Sweep(SweepOptions{Execute: true, CheckPR: true, ActiveSessionsKnown: true}, f, nil)
	if err != nil {
		t.Fatalf("Sweep must be non-fatal: %v", err)
	}
	if len(res.Removed) != 1 || res.Removed[0] != "/wt/b" {
		t.Fatalf("processing must continue after failure; res.Removed = %v, want [/wt/b]", res.Removed)
	}
	if res.Failed["/wt/a"] == "" {
		t.Fatalf("failure must be recorded in res.Failed")
	}
	if len(f.delBranch) != 1 || f.delBranch[0] != "claude/b" {
		t.Fatalf("failed worktree removal must preserve only its local branch: %v", f.delBranch)
	}
}

func TestSweep_BranchDeleteFailureStillCountsWorktreeRemoved(t *testing.T) {
	f := &fakeSweepDeps{
		discovered: []DiscoveredWorktree{{Path: "/wt/m", Repo: "r", Branch: "claude/m"}},
		ancestor:   map[string]bool{"claude/m": true},
		deleteErr:  map[string]error{"claude/m": errors.New("still referenced")},
	}
	res, err := Sweep(SweepOptions{Execute: true, CheckPR: true, ActiveSessionsKnown: true}, f, nil)
	if err != nil {
		t.Fatalf("Sweep: %v", err)
	}
	if len(res.Removed) != 1 {
		t.Fatalf("worktree removal must stand even if branch delete fails: %v", res.Removed)
	}
}

func TestSweep_DiscoverErrorIsFatal(t *testing.T) {
	f := &fakeSweepDeps{discoverErr: errors.New("boom")}
	if _, err := Sweep(SweepOptions{}, f, nil); err == nil {
		t.Fatal("a discovery error must surface (the sweep cannot proceed blind)")
	}
}

func TestAnnotateDuplicates(t *testing.T) {
	wts := []WorktreeStatus{
		{Path: "/a", Repo: "r1", Subject: "fix bug"},
		{Path: "/b", Repo: "r1", Subject: "fix bug"},
		{Path: "/c", Repo: "r1", Subject: "lone"},
		{Path: "/d", Repo: "r2", Subject: "fix bug"}, // different repo, not grouped
		{Path: "/e", Repo: "r1", Subject: ""},        // empty subject skipped
	}
	annotateDuplicates(wts)

	if wts[0].DupCount != 2 || wts[1].DupCount != 2 || wts[0].DupGroup != "fix bug" {
		t.Fatalf("r1 'fix bug' pair not grouped: %+v / %+v", wts[0], wts[1])
	}
	for _, i := range []int{2, 3, 4} {
		if wts[i].DupCount != 0 {
			t.Fatalf("index %d should not be a dup: %+v", i, wts[i])
		}
	}
}

// TestSweep_ExecuteRefusesWhenActiveSetIsUnknown is the ce-3knl.1 regression.
// Before this guard the caller downgraded a failed active-session lookup to a
// warning, on the reasoning that fewer ACTIVE matches is "more conservative".
// It is the opposite: a live worktree sitting clean at origin/main classifies
// as MERGED, so an unknown active set makes live work reapable. That is how
// two live worktrees were deleted during the 2026-07-10 audit.
func TestSweep_ExecuteRefusesWhenActiveSetIsUnknown(t *testing.T) {
	f := &fakeSweepDeps{
		discovered: []DiscoveredWorktree{
			{Path: "/wt/live", Repo: "r", Branch: "claude/live"},
		},
		ancestor: map[string]bool{"claude/live": true},
		mainRepo: map[string]string{"/wt/live": "/repo"},
	}

	res, err := Sweep(SweepOptions{Execute: true, CheckPR: true, ActiveSessionsKnown: false}, f, nil)
	if !errors.Is(err, ErrActiveSessionsUnknown) {
		t.Fatalf("Sweep err = %v, want ErrActiveSessionsUnknown", err)
	}
	if res != nil {
		t.Fatalf("a refused sweep must return no result, got %+v", res)
	}
	if len(f.removed) != 0 || len(f.delBranch) != 0 {
		t.Fatalf("a refused sweep must not mutate: removed=%v delBranch=%v", f.removed, f.delBranch)
	}
	if f.discoverCalls != 0 {
		t.Fatalf("a refused sweep must fail before discovery, got %d discover call(s)", f.discoverCalls)
	}
}

// TestSweep_DryRunStillClassifiesWhenActiveSetIsUnknown keeps the diagnostic
// path open: an operator whose Dolt and tmux lookups are both down can still
// see what the sweep would have done.
func TestSweep_DryRunStillClassifiesWhenActiveSetIsUnknown(t *testing.T) {
	f := &fakeSweepDeps{
		discovered: []DiscoveredWorktree{
			{Path: "/wt/m", Repo: "r", Branch: "claude/m"},
		},
		ancestor: map[string]bool{"claude/m": true},
	}

	res, err := Sweep(SweepOptions{Execute: false, CheckPR: true, ActiveSessionsKnown: false}, f, nil)
	if err != nil {
		t.Fatalf("dry run must still classify: %v", err)
	}
	if len(res.Removed) != 1 || res.Removed[0] != "/wt/m" {
		t.Fatalf("dry-run Removed = %v, want [/wt/m]", res.Removed)
	}
	if len(f.removed) != 0 || len(f.delBranch) != 0 {
		t.Fatalf("dry-run must not mutate: removed=%v delBranch=%v", f.removed, f.delBranch)
	}
}
