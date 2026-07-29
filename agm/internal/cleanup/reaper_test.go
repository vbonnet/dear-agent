package cleanup

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	gitpkg "github.com/vbonnet/dear-agent/agm/internal/git"
	"github.com/vbonnet/dear-agent/internal/gittest"
)

// fakeReaperGit lets the decision logic be tested without a real repo.
type fakeReaperGit struct {
	worktrees []gitpkg.Worktree
	listErr   error
	base      string
	dirty     map[string]bool
	dirtyErr  map[string]error
	ahead     map[string]int
	aheadErr  map[string]error
	removeErr map[string]error

	// PR state keyed by branch. prKnown[b] absent ⇒ unknown (false,false).
	prKnown         map[string]bool
	prMerged        map[string]bool
	deleteBranchErr map[string]error

	removed           []string
	branchDeleted     []string
	branchDeleteForce map[string]bool
}

func (f *fakeReaperGit) ListWorktrees(string) ([]gitpkg.Worktree, error) {
	return f.worktrees, f.listErr
}

func (f *fakeReaperGit) HasUncommittedChanges(p string) (bool, error) {
	if err := f.dirtyErr[p]; err != nil {
		return true, err
	}
	return f.dirty[p], nil
}

func (f *fakeReaperGit) ResolveBaseRef(string) string { return f.base }

func (f *fakeReaperGit) CommitsAhead(_, ref, _ string) (int, error) {
	if err := f.aheadErr[ref]; err != nil {
		return -1, err
	}
	return f.ahead[ref], nil
}

func (f *fakeReaperGit) RemoveWorktree(_, worktreePath string, _ bool) error {
	if err := f.removeErr[worktreePath]; err != nil {
		return err
	}
	f.removed = append(f.removed, worktreePath)
	return nil
}

func (f *fakeReaperGit) DeleteBranch(_, branch string, force bool) error {
	if err := f.deleteBranchErr[branch]; err != nil {
		return err
	}
	if f.branchDeleteForce == nil {
		f.branchDeleteForce = map[string]bool{}
	}
	f.branchDeleteForce[branch] = force
	f.branchDeleted = append(f.branchDeleted, branch)
	return nil
}

func (f *fakeReaperGit) PRMerged(_, branch string) (bool, bool) {
	known := f.prKnown[branch]
	if !known {
		return false, false
	}
	return f.prMerged[branch], true
}

func wt(path, branch string, main bool) gitpkg.Worktree {
	return gitpkg.Worktree{Path: path, Branch: branch, IsMain: main}
}

const base = "/home/u/worktrees"

func TestReap_RemovesCleanZeroAheadAgentWorktree(t *testing.T) {
	target := base + "/dear-agent/lucid-fermi-1a2b3c"
	g := &fakeReaperGit{
		worktrees: []gitpkg.Worktree{
			wt("/src/dear-agent", "main", true),
			wt(target, "claude/lucid-fermi-1a2b3c", false),
		},
		base:  "origin/main",
		dirty: map[string]bool{},
		ahead: map[string]int{"claude/lucid-fermi-1a2b3c": 0},
	}

	res := ReapStaleWorktrees(ReaperOptions{
		RepoPath:      "/src/dear-agent",
		WorktreesBase: base,
	}, g, nil)

	if len(res.Removed) != 1 || res.Removed[0] != target {
		t.Fatalf("expected %s removed, got removed=%v kept=%v", target, res.Removed, res.Kept)
	}
	if len(g.removed) != 1 || g.removed[0] != target {
		t.Fatalf("RemoveWorktree not called as expected: %v", g.removed)
	}
}

func TestReap_KeepsForEachIneligibilityReason(t *testing.T) {
	selfPath := base + "/dear-agent/self-running"
	cases := []struct {
		name       string
		w          gitpkg.Worktree
		wantReason string
	}{
		{"main checkout", wt("/src/dear-agent", "main", true), "main-checkout"},
		{"detached", wt(base+"/dear-agent/det", "", false), "detached-head"},
		{"self", wt(selfPath, "claude/self-running", false), "self-worktree"},
		{"nested sandbox", wt(base+"/dear-agent/p/.worktrees/uuid", "claude/x", false), "nested-sandbox-worktree"},
		{"non-agent branch", wt(base+"/dear-agent/manual", "feature/manual", false), "non-agent-branch"},
		{"outside base", wt("/tmp/elsewhere/foo", "claude/foo", false), "outside-worktrees-base"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			g := &fakeReaperGit{
				worktrees: []gitpkg.Worktree{c.w},
				base:      "origin/main",
				dirty:     map[string]bool{},
				ahead:     map[string]int{c.w.Branch: 0},
			}
			res := ReapStaleWorktrees(ReaperOptions{
				RepoPath:      "/src/dear-agent",
				WorktreesBase: base,
				SelfPath:      selfPath,
			}, g, nil)

			if len(res.Removed) != 0 {
				t.Fatalf("nothing should be removed, got %v", res.Removed)
			}
			if got := res.Kept[c.w.Path]; got != c.wantReason {
				t.Fatalf("kept reason = %q, want %q", got, c.wantReason)
			}
			if len(g.removed) != 0 {
				t.Fatalf("RemoveWorktree must not be called, got %v", g.removed)
			}
		})
	}
}

func TestReap_KeepsDirtyWorktree(t *testing.T) {
	p := base + "/dear-agent/dirty"
	g := &fakeReaperGit{
		worktrees: []gitpkg.Worktree{wt(p, "claude/dirty", false)},
		base:      "origin/main",
		dirty:     map[string]bool{p: true},
		ahead:     map[string]int{"claude/dirty": 0},
	}
	res := ReapStaleWorktrees(ReaperOptions{RepoPath: "/r", WorktreesBase: base}, g, nil)

	if len(res.Removed) != 0 || len(g.removed) != 0 {
		t.Fatalf("dirty worktree must not be removed: %v", res.Removed)
	}
	if res.Kept[p] != "uncommitted-changes" {
		t.Fatalf("kept reason = %q, want uncommitted-changes", res.Kept[p])
	}
}

func TestReap_KeepsWorktreeWithUnmergedCommits(t *testing.T) {
	p := base + "/dear-agent/ahead"
	g := &fakeReaperGit{
		worktrees: []gitpkg.Worktree{wt(p, "claude/ahead", false)},
		base:      "origin/main",
		dirty:     map[string]bool{},
		ahead:     map[string]int{"claude/ahead": 3},
	}
	res := ReapStaleWorktrees(ReaperOptions{RepoPath: "/r", WorktreesBase: base}, g, nil)

	if len(res.Removed) != 0 || len(g.removed) != 0 {
		t.Fatalf("ahead worktree must not be removed: %v", res.Removed)
	}
	if res.Kept[p] != "commits-ahead" {
		t.Fatalf("kept reason = %q, want commits-ahead", res.Kept[p])
	}
}

func TestReap_StatusOrAheadErrorKeepsWorktree(t *testing.T) {
	pStatus := base + "/dear-agent/serr"
	pAhead := base + "/dear-agent/aerr"
	g := &fakeReaperGit{
		worktrees: []gitpkg.Worktree{
			wt(pStatus, "claude/serr", false),
			wt(pAhead, "claude/aerr", false),
		},
		base:     "origin/main",
		dirty:    map[string]bool{},
		dirtyErr: map[string]error{pStatus: fmt.Errorf("status boom")},
		ahead:    map[string]int{"claude/aerr": 0},
		aheadErr: map[string]error{"claude/aerr": fmt.Errorf("ahead boom")},
	}
	res := ReapStaleWorktrees(ReaperOptions{RepoPath: "/r", WorktreesBase: base}, g, nil)

	if len(g.removed) != 0 {
		t.Fatalf("nothing should be removed on probe failure: %v", g.removed)
	}
	if res.Kept[pStatus] != "status-check-failed" {
		t.Fatalf("status err reason = %q", res.Kept[pStatus])
	}
	if res.Kept[pAhead] != "ahead-check-failed" {
		t.Fatalf("ahead err reason = %q", res.Kept[pAhead])
	}
}

func TestReap_RemoveFailureIsRecordedNotFatal(t *testing.T) {
	p := base + "/dear-agent/rmfail"
	g := &fakeReaperGit{
		worktrees: []gitpkg.Worktree{wt(p, "claude/rmfail", false)},
		base:      "origin/main",
		dirty:     map[string]bool{},
		ahead:     map[string]int{"claude/rmfail": 0},
		removeErr: map[string]error{p: fmt.Errorf("git refused")},
	}
	res := ReapStaleWorktrees(ReaperOptions{RepoPath: "/r", WorktreesBase: base}, g, nil)

	if len(res.Removed) != 0 {
		t.Fatalf("failed removal must not be reported as removed: %v", res.Removed)
	}
	if res.Kept[p] != "remove-failed" {
		t.Fatalf("kept reason = %q, want remove-failed", res.Kept[p])
	}
}

func TestReap_UnresolvedBaseRefKeepsEverything(t *testing.T) {
	p := base + "/dear-agent/x"
	g := &fakeReaperGit{
		worktrees: []gitpkg.Worktree{wt(p, "claude/x", false)},
		base:      "", // unresolved
		dirty:     map[string]bool{},
	}
	res := ReapStaleWorktrees(ReaperOptions{RepoPath: "/r", WorktreesBase: base}, g, nil)

	if len(res.Removed) != 0 || len(g.removed) != 0 {
		t.Fatalf("nothing removable without a base ref: %v", res.Removed)
	}
	if res.Kept[p] != "base-ref-unresolved" {
		t.Fatalf("kept reason = %q, want base-ref-unresolved", res.Kept[p])
	}
}

func TestReap_DryRunReportsButDoesNotRemove(t *testing.T) {
	p := base + "/dear-agent/dry"
	g := &fakeReaperGit{
		worktrees: []gitpkg.Worktree{wt(p, "claude/dry", false)},
		base:      "origin/main",
		dirty:     map[string]bool{},
		ahead:     map[string]int{"claude/dry": 0},
	}
	res := ReapStaleWorktrees(ReaperOptions{RepoPath: "/r", WorktreesBase: base, DryRun: true}, g, nil)

	if len(res.Removed) != 1 || res.Removed[0] != p {
		t.Fatalf("dry-run should report %s as removable, got %v", p, res.Removed)
	}
	if len(g.removed) != 0 {
		t.Fatalf("dry-run must not call RemoveWorktree, got %v", g.removed)
	}
}

func TestReap_ListErrorIsNonFatal(t *testing.T) {
	g := &fakeReaperGit{listErr: fmt.Errorf("git unavailable")}
	res := ReapStaleWorktrees(ReaperOptions{RepoPath: "/r", WorktreesBase: base}, g, nil)
	if len(res.Removed) != 0 || len(res.Kept) != 0 {
		t.Fatalf("list error should yield empty result, got %+v", res)
	}
}

// --- branch deletion + merged-PR escape (spec points 2 & 4) ---

func TestReap_CleanZeroAheadDeletesLocalBranchSafely(t *testing.T) {
	p := base + "/dear-agent/clean"
	g := &fakeReaperGit{
		worktrees: []gitpkg.Worktree{wt(p, "claude/clean", false)},
		base:      "origin/main",
		dirty:     map[string]bool{},
		ahead:     map[string]int{"claude/clean": 0},
	}
	res := ReapStaleWorktrees(ReaperOptions{RepoPath: "/r", WorktreesBase: base}, g, nil)

	if len(res.Removed) != 1 || res.Removed[0] != p {
		t.Fatalf("expected %s removed, got removed=%v kept=%v", p, res.Removed, res.Kept)
	}
	if len(g.branchDeleted) != 1 || g.branchDeleted[0] != "claude/clean" {
		t.Fatalf("expected local branch claude/clean deleted, got %v", g.branchDeleted)
	}
	if g.branchDeleteForce["claude/clean"] {
		t.Fatalf("zero-ahead branch must be deleted with safe -d, not -D")
	}
}

func TestReap_MergedPRReclaimsCommitsAheadWorktree(t *testing.T) {
	p := base + "/dear-agent/merged"
	g := &fakeReaperGit{
		worktrees: []gitpkg.Worktree{wt(p, "claude/merged", false)},
		base:      "origin/main",
		dirty:     map[string]bool{},
		ahead:     map[string]int{"claude/merged": 4}, // squash-merge fingerprint
		prKnown:   map[string]bool{"claude/merged": true},
		prMerged:  map[string]bool{"claude/merged": true},
	}
	res := ReapStaleWorktrees(ReaperOptions{
		RepoPath: "/r", WorktreesBase: base, CheckMergedPR: true,
	}, g, nil)

	if len(res.Removed) != 1 || res.Removed[0] != p {
		t.Fatalf("merged-PR worktree should be reclaimed, got removed=%v kept=%v", res.Removed, res.Kept)
	}
	if !g.branchDeleteForce["claude/merged"] {
		t.Fatalf("squash-merged branch must be force-deleted (-D), git still sees it unmerged")
	}
}

func TestReap_CommitsAheadKeptWhenPRStateUnknown(t *testing.T) {
	p := base + "/dear-agent/unknownpr"
	g := &fakeReaperGit{
		worktrees: []gitpkg.Worktree{wt(p, "claude/unknownpr", false)},
		base:      "origin/main",
		dirty:     map[string]bool{},
		ahead:     map[string]int{"claude/unknownpr": 2},
		// prKnown absent ⇒ PRMerged returns (false,false) = unknown
	}
	res := ReapStaleWorktrees(ReaperOptions{
		RepoPath: "/r", WorktreesBase: base, CheckMergedPR: true,
	}, g, nil)

	if len(res.Removed) != 0 || len(g.removed) != 0 {
		t.Fatalf("unknown PR state must be conservative (keep): %v", res.Removed)
	}
	if res.Kept[p] != "commits-ahead" {
		t.Fatalf("kept reason = %q, want commits-ahead", res.Kept[p])
	}
}

func TestReap_CommitsAheadKeptWhenPROpenNotMerged(t *testing.T) {
	p := base + "/dear-agent/openpr"
	g := &fakeReaperGit{
		worktrees: []gitpkg.Worktree{wt(p, "claude/openpr", false)},
		base:      "origin/main",
		dirty:     map[string]bool{},
		ahead:     map[string]int{"claude/openpr": 1},
		prKnown:   map[string]bool{"claude/openpr": true}, // known, but not merged
	}
	res := ReapStaleWorktrees(ReaperOptions{
		RepoPath: "/r", WorktreesBase: base, CheckMergedPR: true,
	}, g, nil)

	if len(res.Removed) != 0 || res.Kept[p] != "commits-ahead" {
		t.Fatalf("open (unmerged) PR must be kept, got removed=%v kept=%v", res.Removed, res.Kept)
	}
}

func TestReap_DirtyWorktreeKeptEvenWhenPRMerged(t *testing.T) {
	p := base + "/dear-agent/dirtymerged"
	g := &fakeReaperGit{
		worktrees: []gitpkg.Worktree{wt(p, "claude/dirtymerged", false)},
		base:      "origin/main",
		dirty:     map[string]bool{p: true},
		ahead:     map[string]int{"claude/dirtymerged": 3},
		prKnown:   map[string]bool{"claude/dirtymerged": true},
		prMerged:  map[string]bool{"claude/dirtymerged": true},
	}
	res := ReapStaleWorktrees(ReaperOptions{
		RepoPath: "/r", WorktreesBase: base, CheckMergedPR: true,
	}, g, nil)

	if len(res.Removed) != 0 || len(g.removed) != 0 {
		t.Fatalf("uncommitted work must never be reaped even if PR merged: %v", res.Removed)
	}
	if res.Kept[p] != "uncommitted-changes" {
		t.Fatalf("kept reason = %q, want uncommitted-changes", res.Kept[p])
	}
	if len(g.branchDeleted) != 0 {
		t.Fatalf("dirty worktree's branch must not be deleted: %v", g.branchDeleted)
	}
}

func TestReap_PRCheckBudgetExhaustedKeepsCommitsAhead(t *testing.T) {
	p := base + "/dear-agent/budget"
	g := &fakeReaperGit{
		worktrees: []gitpkg.Worktree{wt(p, "claude/budget", false)},
		base:      "origin/main",
		dirty:     map[string]bool{},
		ahead:     map[string]int{"claude/budget": 5},
		prKnown:   map[string]bool{"claude/budget": true},
		prMerged:  map[string]bool{"claude/budget": true},
	}
	// 1ns budget ⇒ deadline already in the past by the time the loop runs.
	res := ReapStaleWorktrees(ReaperOptions{
		RepoPath: "/r", WorktreesBase: base, CheckMergedPR: true, PRCheckBudget: 1,
	}, g, nil)

	if len(res.Removed) != 0 || res.Kept[p] != "commits-ahead" {
		t.Fatalf("budget exhaustion must keep (no PR lookup), got removed=%v kept=%v", res.Removed, res.Kept)
	}
}

func TestReap_BranchDeleteFailureStillCountsWorktreeRemoved(t *testing.T) {
	p := base + "/dear-agent/brfail"
	g := &fakeReaperGit{
		worktrees:       []gitpkg.Worktree{wt(p, "claude/brfail", false)},
		base:            "origin/main",
		dirty:           map[string]bool{},
		ahead:           map[string]int{"claude/brfail": 0},
		deleteBranchErr: map[string]error{"claude/brfail": fmt.Errorf("branch checked out elsewhere")},
	}
	res := ReapStaleWorktrees(ReaperOptions{RepoPath: "/r", WorktreesBase: base}, g, nil)

	// Worktree is the sprawl artifact: its removal stands even if the
	// best-effort branch cleanup fails.
	if len(res.Removed) != 1 || res.Removed[0] != p {
		t.Fatalf("worktree removal must stand despite branch-delete failure: %v", res.Removed)
	}
	if _, kept := res.Kept[p]; kept {
		t.Fatalf("worktree must not be in Kept after successful removal: %v", res.Kept)
	}
}

func TestReap_NoPRCheckKeepsSquashMergedConservatively(t *testing.T) {
	p := base + "/dear-agent/nocheck"
	g := &fakeReaperGit{
		worktrees: []gitpkg.Worktree{wt(p, "claude/nocheck", false)},
		base:      "origin/main",
		dirty:     map[string]bool{},
		ahead:     map[string]int{"claude/nocheck": 2},
		prKnown:   map[string]bool{"claude/nocheck": true},
		prMerged:  map[string]bool{"claude/nocheck": true},
	}
	// CheckMergedPR false ⇒ historic conservative behavior, no gh lookup.
	res := ReapStaleWorktrees(ReaperOptions{RepoPath: "/r", WorktreesBase: base}, g, nil)

	if len(res.Removed) != 0 || res.Kept[p] != "commits-ahead" {
		t.Fatalf("with PR check off, commits-ahead must be kept, got removed=%v kept=%v", res.Removed, res.Kept)
	}
}

// --- integration: real git, exercises RealReaperGitOps end to end ---

func TestReap_Integration_RealGit(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}

	root := evalDir(t, t.TempDir())
	repo := filepath.Join(root, "repo")
	wtBase := filepath.Join(root, "worktrees", "repo")
	mustMkdir(t, wtBase)

	gitInit(t, repo)

	cleanWT := filepath.Join(wtBase, "clean-zero-ahead")
	dirtyWT := filepath.Join(wtBase, "dirty")
	aheadWT := filepath.Join(wtBase, "ahead")
	humanWT := filepath.Join(wtBase, "human")

	gitRun(t, repo, "worktree", "add", "-b", "claude/clean", cleanWT)
	gitRun(t, repo, "worktree", "add", "-b", "claude/dirty", dirtyWT)
	gitRun(t, repo, "worktree", "add", "-b", "claude/ahead", aheadWT)
	gitRun(t, repo, "worktree", "add", "-b", "feature/human", humanWT)

	// dirty: an untracked file.
	if err := os.WriteFile(filepath.Join(dirtyWT, "scratch"), []byte("x"), 0600); err != nil {
		t.Fatal(err)
	}
	// ahead: a real commit on the branch.
	if err := os.WriteFile(filepath.Join(aheadWT, "a.txt"), []byte("a"), 0600); err != nil {
		t.Fatal(err)
	}
	gitRun(t, aheadWT, "add", "a.txt")
	gitRun(t, aheadWT, "commit", "-m", "work")

	res := ReapStaleWorktrees(ReaperOptions{
		RepoPath:      repo,
		WorktreesBase: filepath.Join(root, "worktrees"),
	}, RealReaperGitOps{}, nil)

	if len(res.Removed) != 1 || res.Removed[0] != cleanWT {
		t.Fatalf("expected only %s removed, got removed=%v kept=%v", cleanWT, res.Removed, res.Kept)
	}
	if _, err := os.Stat(cleanWT); !os.IsNotExist(err) {
		t.Fatalf("clean worktree dir should be gone, stat err=%v", err)
	}
	for _, keep := range []string{dirtyWT, aheadWT, humanWT} {
		if _, err := os.Stat(keep); err != nil {
			t.Fatalf("worktree %s must still exist: %v", keep, err)
		}
	}
}

func evalDir(t *testing.T, p string) string {
	t.Helper()
	resolved, err := filepath.EvalSymlinks(p)
	if err != nil {
		t.Fatalf("eval symlinks: %v", err)
	}
	return resolved
}

func mustMkdir(t *testing.T, p string) {
	t.Helper()
	if err := os.MkdirAll(p, 0750); err != nil {
		t.Fatalf("mkdir %s: %v", p, err)
	}
}

func gitInit(t *testing.T, repo string) {
	t.Helper()
	gittest.Run(t, "", "init", "-b", "main", repo)
	gittest.HardenRepo(t, repo)
	gitRun(t, repo, "config", "user.name", "Test")
	gitRun(t, repo, "config", "user.email", "test@test.com")
	if err := os.WriteFile(filepath.Join(repo, "README.md"), []byte("# t\n"), 0600); err != nil {
		t.Fatal(err)
	}
	gitRun(t, repo, "add", "README.md")
	gitRun(t, repo, "commit", "-m", "init")
}

func gitRun(t *testing.T, dir string, args ...string) {
	t.Helper()
	gittest.Run(t, dir, args...)
}
