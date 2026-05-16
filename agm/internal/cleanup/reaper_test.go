package cleanup

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	gitpkg "github.com/vbonnet/dear-agent/agm/internal/git"
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

	removed []string
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

	run(t, repo, "git", "worktree", "add", "-b", "claude/clean", cleanWT)
	run(t, repo, "git", "worktree", "add", "-b", "claude/dirty", dirtyWT)
	run(t, repo, "git", "worktree", "add", "-b", "claude/ahead", aheadWT)
	run(t, repo, "git", "worktree", "add", "-b", "feature/human", humanWT)

	// dirty: an untracked file.
	if err := os.WriteFile(filepath.Join(dirtyWT, "scratch"), []byte("x"), 0600); err != nil {
		t.Fatal(err)
	}
	// ahead: a real commit on the branch.
	if err := os.WriteFile(filepath.Join(aheadWT, "a.txt"), []byte("a"), 0600); err != nil {
		t.Fatal(err)
	}
	run(t, aheadWT, "git", "add", "a.txt")
	run(t, aheadWT, "git", "commit", "-m", "work")

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
	mustMkdir(t, repo)
	run(t, "", "git", "init", "-b", "main", repo)
	run(t, repo, "git", "config", "user.name", "Test")
	run(t, repo, "git", "config", "user.email", "test@test.com")
	if err := os.WriteFile(filepath.Join(repo, "README.md"), []byte("# t\n"), 0600); err != nil {
		t.Fatal(err)
	}
	run(t, repo, "git", "add", "README.md")
	run(t, repo, "git", "commit", "-m", "init")
}

func run(t *testing.T, dir, name string, args ...string) {
	t.Helper()
	cmd := exec.Command(name, args...)
	if dir != "" {
		cmd.Dir = dir
	}
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("%s %v failed: %v\n%s", name, args, err, out)
	}
}
