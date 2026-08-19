package main

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/vbonnet/dear-agent/internal/gittest"
)

func TestParse(t *testing.T) {
	tests := []struct {
		name     string
		args     []string
		wantErr  bool
		wantCode int
		check    func(t *testing.T, cfg config)
	}{
		{
			name: "repo only defaults to dry-run and fourteen days",
			args: []string{"/repo"},
			check: func(t *testing.T, cfg config) {
				if cfg.repo != "/repo" {
					t.Errorf("repo = %q, want /repo", cfg.repo)
				}
				if cfg.fix {
					t.Error("fix = true, want dry-run by default")
				}
				if cfg.maxAge != 14 {
					t.Errorf("maxAge = %d, want 14", cfg.maxAge)
				}
			},
		},
		{
			name: "fix opts in to mutation",
			args: []string{"/repo", "--fix"},
			check: func(t *testing.T, cfg config) {
				if !cfg.fix {
					t.Error("fix = false, want true")
				}
			},
		},
		{
			name: "max-age overrides the default",
			args: []string{"/repo", "--max-age", "30"},
			check: func(t *testing.T, cfg config) {
				if cfg.maxAge != 30 {
					t.Errorf("maxAge = %d, want 30", cfg.maxAge)
				}
			},
		},
		{
			name: "preserve accumulates repeated values",
			args: []string{"/repo", "--preserve", "alpha", "--preserve", "beta"},
			check: func(t *testing.T, cfg config) {
				if !cfg.preserve["alpha"] || !cfg.preserve["beta"] {
					t.Errorf("preserve = %v, want alpha and beta", cfg.preserve)
				}
			},
		},
		{
			name: "flags may precede the repo path",
			args: []string{"--fix", "--max-age", "3", "/repo"},
			check: func(t *testing.T, cfg config) {
				if cfg.repo != "/repo" || !cfg.fix || cfg.maxAge != 3 {
					t.Errorf("cfg = %+v, want repo=/repo fix=true maxAge=3", cfg)
				}
			},
		},
		{name: "no repo path is rejected", args: nil, wantErr: true, wantCode: 1},
		{name: "second positional argument is rejected", args: []string{"/a", "/b"}, wantErr: true, wantCode: 1},
		{name: "unknown flag is rejected", args: []string{"/repo", "--wat"}, wantErr: true, wantCode: 1},
		{name: "max-age without a value is rejected", args: []string{"/repo", "--max-age"}, wantErr: true, wantCode: 1},
		{name: "non-integer max-age is rejected", args: []string{"/repo", "--max-age", "soon"}, wantErr: true, wantCode: 1},
		{name: "preserve without a value is rejected", args: []string{"/repo", "--preserve"}, wantErr: true, wantCode: 1},
		{name: "help exits zero", args: []string{"--help"}, wantErr: true, wantCode: 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg, err := parse(tt.args)
			if tt.wantErr {
				if err == nil {
					t.Fatal("parse() = nil error, want error")
				}
				var exit exitError
				if !errors.As(err, &exit) {
					t.Fatalf("parse() error = %v, want exitError", err)
				}
				if exit.code != tt.wantCode {
					t.Errorf("exit code = %d, want %d", exit.code, tt.wantCode)
				}
				return
			}
			if err != nil {
				t.Fatalf("parse() error = %v, want nil", err)
			}
			tt.check(t, cfg)
		})
	}
}

func TestSamePath(t *testing.T) {
	if !samePath("/tmp/a/../a", "/tmp/a") {
		t.Error("samePath did not clean equivalent paths")
	}
	if samePath("/tmp/a", "/tmp/b") {
		t.Error("samePath matched distinct paths")
	}
}

func TestGitEnvScrubsAmbientRepositorySelectors(t *testing.T) {
	t.Setenv("GIT_DIR", "/somewhere/else/.git")
	t.Setenv("GIT_WORK_TREE", "/somewhere/else")
	t.Setenv("GIT_TERMINAL_PROMPT", "1")

	var sawPrompt string
	for _, kv := range gitEnv() {
		name, value, _ := strings.Cut(kv, "=")
		if name == "GIT_DIR" || name == "GIT_WORK_TREE" {
			t.Errorf("gitEnv() leaked ambient selector %s", kv)
		}
		if name == "GIT_TERMINAL_PROMPT" {
			sawPrompt = value
		}
	}
	if sawPrompt != "0" {
		t.Errorf("GIT_TERMINAL_PROMPT = %q, want 0", sawPrompt)
	}
}

func TestGitIntReportsFailureInsteadOfZero(t *testing.T) {
	repo := newRepo(t)
	// A ref that does not resolve must surface as an error. Returning 0
	// here would read as "no commits ahead" and authorize a deletion.
	if got, err := gitInt(t.Context(), repo, "rev-list", "--count", "origin/main..deadbeefdeadbeefdeadbeefdeadbeefdeadbeef"); err == nil {
		t.Fatalf("gitInt() = %d, nil error; want an error for an unresolvable ref", got)
	}
	if got, err := gitInt(t.Context(), repo, "rev-parse", "HEAD"); err == nil {
		t.Fatalf("gitInt() = %d, nil error; want an error for non-integer output", got)
	}
}

// newRepo builds a repo with an origin/main ref and one commit.
//
// Every Git invocation goes through internal/gittest so no host hook can fire
// (internal/gittest guard, bead ce-3knl.1). The repository is additionally
// hardened, because the code under test shells out to plain `git` with the
// inherited environment; HardenRepo writes the empty hooks path into the
// repository's own config so those production calls are hermetic too.
func newRepo(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	origin := filepath.Join(root, "origin")
	gittest.Run(t, root, "init", "--bare", "--initial-branch=main", origin)
	gittest.HardenRepo(t, origin)

	repo := filepath.Join(root, "repo")
	gittest.Run(t, root, "init", "--initial-branch=main", repo)
	gittest.HardenRepo(t, repo)
	gittest.Run(t, repo, "commit", "--allow-empty", "-m", "base")
	gittest.Run(t, repo, "remote", "add", "origin", origin)
	gittest.Run(t, repo, "push", "-u", "origin", "main")
	return repo
}

// addWorktree creates a branch worktree and hardens it, so the production
// removal path cannot execute a host hook from the new checkout either.
func addWorktree(t *testing.T, repo, branch, path string) {
	t.Helper()
	gittest.Run(t, repo, "worktree", "add", "-b", branch, path)
	gittest.HardenRepo(t, path)
}

// testEnv is the scan context used by classification tests: no live
// sessions, probe healthy, only the main checkout protected.
func testEnv(repo string) scanEnv {
	return scanEnv{
		target:       "origin/main",
		targetBranch: "main",
		now:          time.Now(),
		active:       map[string]bool{},
		activeKnown:  true,
		protected:    map[string]bool{canonical(repo): true},
	}
}

// worktreeByName indexes the repo's worktrees by directory basename.
func worktreeByName(t *testing.T, repo string) map[string]worktree {
	t.Helper()
	all, err := listWorktrees(t.Context(), repo)
	if err != nil {
		t.Fatalf("listWorktrees() error = %v", err)
	}
	byName := map[string]worktree{}
	for _, wt := range all {
		byName[filepath.Base(wt.path)] = wt
	}
	return byName
}

func TestTargetRefPrefersOriginMain(t *testing.T) {
	repo := newRepo(t)
	got, err := targetRef(t.Context(), repo)
	if err != nil {
		t.Fatalf("targetRef() error = %v", err)
	}
	if got != "origin/main" && !strings.HasSuffix(got, "/main") {
		t.Errorf("targetRef() = %q, want a main ref", got)
	}
}

func TestTargetRefFailsWithoutTarget(t *testing.T) {
	root := t.TempDir()
	repo := filepath.Join(root, "solo")
	gittest.Run(t, root, "init", "--initial-branch=main", repo)
	gittest.Run(t, repo, "commit", "--allow-empty", "-m", "base")
	if _, err := targetRef(t.Context(), repo); err == nil {
		t.Fatal("targetRef() = nil error, want failure when no origin ref exists")
	}
}

func TestListWorktreesParsesPorcelain(t *testing.T) {
	repo := newRepo(t)
	wtPath := filepath.Join(t.TempDir(), "feature")
	addWorktree(t, repo, "feature", wtPath)

	got, err := listWorktrees(t.Context(), repo)
	if err != nil {
		t.Fatalf("listWorktrees() error = %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("listWorktrees() returned %d entries, want 2: %+v", len(got), got)
	}
	if !got[0].primary {
		t.Error("first record is not marked as the primary checkout")
	}
	var feature *worktree
	for i := range got {
		if strings.HasSuffix(got[i].branch, "/feature") {
			feature = &got[i]
		}
	}
	if feature == nil {
		t.Fatalf("no feature worktree in %+v", got)
	}
	if feature.head == "" {
		t.Error("feature worktree has no HEAD")
	}
	if feature.bare || feature.detached || feature.primary {
		t.Errorf("feature worktree bare=%v detached=%v primary=%v, want all false", feature.bare, feature.detached, feature.primary)
	}
}

func TestParseWorktreesKeepsNewlineBearingPaths(t *testing.T) {
	// A path containing a newline must survive intact. A line-based parser
	// truncates it to a prefix, and under --fix that prefix can name a
	// different registered checkout.
	record := "worktree /wt/od\nd\x00HEAD abc123\x00branch refs/heads/odd\x00\x00"
	got := parseWorktrees(record, "\x00")
	if len(got) != 1 {
		t.Fatalf("parseWorktrees() = %d records, want 1: %+v", len(got), got)
	}
	if got[0].path != "/wt/od\nd" {
		t.Errorf("path = %q, want the full newline-bearing path", got[0].path)
	}
}

func TestParseWorktreesMarksLocked(t *testing.T) {
	record := "worktree /wt/held\x00HEAD abc123\x00branch refs/heads/held\x00locked in use by an agent\x00\x00"
	got := parseWorktrees(record, "\x00")
	if len(got) != 1 || !got[0].locked {
		t.Fatalf("parseWorktrees() = %+v, want one locked record", got)
	}
}

func TestInspectClassification(t *testing.T) {
	repo := newRepo(t)
	mergedPath := filepath.Join(t.TempDir(), "merged")
	addWorktree(t, repo, "merged", mergedPath)
	aheadPath := filepath.Join(t.TempDir(), "ahead")
	addWorktree(t, repo, "ahead", aheadPath)
	gittest.Run(t, aheadPath, "commit", "--allow-empty", "-m", "work")

	byName := worktreeByName(t, repo)
	dryRun := config{repo: repo, maxAge: 14, preserve: preserveFlag{}}
	env := testEnv(repo)

	t.Run("no commits beyond the target is MERGED", func(t *testing.T) {
		if got := inspect(t.Context(), dryRun, env, byName["merged"]); got.class != classMerged {
			t.Errorf("inspect() = %+v, want %s", got, classMerged)
		}
	})

	t.Run("recent commits ahead are ORPHANED", func(t *testing.T) {
		if got := inspect(t.Context(), dryRun, env, byName["ahead"]); got.class != classOrphaned {
			t.Errorf("inspect() = %+v, want %s", got, classOrphaned)
		}
	})

	t.Run("age alone never makes unmerged work removable", func(t *testing.T) {
		aged := env
		aged.now = env.now.Add(365 * 24 * time.Hour)
		got := inspect(t.Context(), dryRun, aged, byName["ahead"])
		if got.class != classOrphaned {
			t.Fatalf("inspect() = %+v, want %s: age is not merge proof", got, classOrphaned)
		}
		if got.class.removable() {
			t.Error("an idle unmerged worktree was classified as removable")
		}
		if !strings.Contains(got.reason, "idle") {
			t.Errorf("reason = %q, want it to surface the idle age", got.reason)
		}
	})

	t.Run("preserve outranks the merged verdict", func(t *testing.T) {
		cfg := config{repo: repo, maxAge: 14, preserve: preserveFlag{"merged": true}}
		if got := inspect(t.Context(), cfg, env, byName["merged"]); got.class != classProtected {
			t.Errorf("inspect() = %+v, want %s", got, classProtected)
		}
	})

	t.Run("the primary checkout is protected", func(t *testing.T) {
		got := inspect(t.Context(), dryRun, env, worktree{path: repo, primary: true})
		if got.class != classProtected {
			t.Errorf("inspect(primary) = %+v, want %s", got, classProtected)
		}
	})

	t.Run("a locked checkout is protected", func(t *testing.T) {
		locked := byName["merged"]
		locked.locked = true
		if got := inspect(t.Context(), dryRun, env, locked); got.class != classProtected {
			t.Errorf("inspect(locked) = %+v, want %s", got, classProtected)
		}
	})

	t.Run("a worktree holding the target branch is protected", func(t *testing.T) {
		onMain := byName["merged"]
		onMain.branch = "refs/heads/main"
		if got := inspect(t.Context(), dryRun, env, onMain); got.class != classProtected {
			t.Errorf("inspect(main) = %+v, want %s: deleting the target branch is catastrophic", got, classProtected)
		}
	})

	t.Run("a detached checkout is UNKNOWN, not removable", func(t *testing.T) {
		got := inspect(t.Context(), dryRun, env, worktree{path: mergedPath, detached: true})
		if got.class != classUnknown || got.class.removable() {
			t.Errorf("inspect(detached) = %+v, want a non-removable %s", got, classUnknown)
		}
	})

	t.Run("dry run mutates nothing", func(t *testing.T) {
		if err := gitOK(t.Context(), repo, "rev-parse", "--verify", "--quiet", "refs/heads/merged"); err != nil {
			t.Fatalf("dry run removed branch merged: %v", err)
		}
		if list, err := listWorktrees(t.Context(), repo); err != nil || len(list) != 3 {
			t.Errorf("dry run changed the worktree set: %+v (err=%v)", list, err)
		}
	})
}

// TestInspectRefusesDirtyWorktree is the data-loss regression: a checkout
// with uncommitted or untracked work must never be reaped, even in fix mode,
// and even when it carries no commits beyond the target.
func TestInspectRefusesDirtyWorktree(t *testing.T) {
	cases := []struct {
		name  string
		dirty func(t *testing.T, path string)
	}{
		{
			name: "untracked file",
			dirty: func(t *testing.T, path string) {
				t.Helper()
				if err := os.WriteFile(filepath.Join(path, "scratch.txt"), []byte("unsaved work"), 0o600); err != nil {
					t.Fatalf("writing untracked file: %v", err)
				}
			},
		},
		{
			name: "modified tracked file",
			dirty: func(t *testing.T, path string) {
				t.Helper()
				tracked := filepath.Join(path, "tracked.txt")
				if err := os.WriteFile(tracked, []byte("v1"), 0o600); err != nil {
					t.Fatalf("writing tracked file: %v", err)
				}
				gittest.Run(t, path, "add", "tracked.txt")
				gittest.Run(t, path, "commit", "-m", "add tracked")
				gittest.Run(t, path, "push", "origin", "HEAD")
				if err := os.WriteFile(tracked, []byte("v2 uncommitted"), 0o600); err != nil {
					t.Fatalf("modifying tracked file: %v", err)
				}
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			repo := newRepo(t)
			wtPath := filepath.Join(t.TempDir(), "wip")
			addWorktree(t, repo, "wip", wtPath)
			tc.dirty(t, wtPath)

			byName := worktreeByName(t, repo)
			cfg := config{repo: repo, fix: true, maxAge: 14, preserve: preserveFlag{}}
			got := inspect(t.Context(), cfg, testEnv(repo), byName["wip"])

			if got.class != classDirty {
				t.Fatalf("inspect() = %+v, want %s", got, classDirty)
			}
			if got.removed {
				t.Fatal("fix mode removed a dirty worktree")
			}
			if _, err := os.Stat(wtPath); err != nil {
				t.Fatalf("dirty worktree was deleted from disk: %v", err)
			}
			if err := gitOK(t.Context(), repo, "rev-parse", "--verify", "--quiet", "refs/heads/wip"); err != nil {
				t.Errorf("branch wip was deleted for a dirty worktree: %v", err)
			}
		})
	}
}

// TestInspectRefusesActiveSessionWorktree is the second half of the same
// regression: a live session owning a clean, fully merged checkout must keep
// it. Reaping a live worktree at origin/main is the failure this guard exists
// for.
func TestInspectRefusesActiveSessionWorktree(t *testing.T) {
	repo := newRepo(t)
	wtPath := filepath.Join(t.TempDir(), "live-agent")
	addWorktree(t, repo, "live-agent", wtPath)
	byName := worktreeByName(t, repo)
	cfg := config{repo: repo, fix: true, maxAge: 14, preserve: preserveFlag{}}

	t.Run("session named for the directory", func(t *testing.T) {
		env := testEnv(repo)
		env.active = map[string]bool{"live-agent": true}
		got := inspect(t.Context(), cfg, env, byName["live-agent"])
		if got.class != classActive || got.removed {
			t.Fatalf("inspect() = %+v, want a non-removed %s", got, classActive)
		}
		if _, err := os.Stat(wtPath); err != nil {
			t.Fatalf("active worktree was deleted from disk: %v", err)
		}
	})

	t.Run("session named for the branch", func(t *testing.T) {
		env := testEnv(repo)
		env.active = map[string]bool{"live-agent": true}
		wt := byName["live-agent"]
		wt.path = filepath.Join(filepath.Dir(wt.path), "renamed-dir")
		if got := inspect(t.Context(), cfg, env, wt); got.class != classActive {
			t.Fatalf("inspect() = %+v, want %s matched on the branch name", got, classActive)
		}
	})

	t.Run("a failed session probe disables removal entirely", func(t *testing.T) {
		env := testEnv(repo)
		env.activeKnown = false
		got := inspect(t.Context(), cfg, env, byName["live-agent"])
		if got.class != classUnknown || got.removed {
			t.Fatalf("inspect() = %+v, want a non-removed %s when the probe failed", got, classUnknown)
		}
		if _, err := os.Stat(wtPath); err != nil {
			t.Fatalf("worktree was deleted despite an unusable session probe: %v", err)
		}
	})
}

// TestInspectFailsClosedOnProbeFailure proves a broken Git probe produces
// UNKNOWN rather than a deletion. A swallowed rev-list error used to read as
// "zero commits ahead".
func TestInspectFailsClosedOnProbeFailure(t *testing.T) {
	repo := newRepo(t)
	wtPath := filepath.Join(t.TempDir(), "probe")
	addWorktree(t, repo, "probe", wtPath)
	byName := worktreeByName(t, repo)
	cfg := config{repo: repo, fix: true, maxAge: 14, preserve: preserveFlag{}}

	t.Run("unresolvable HEAD", func(t *testing.T) {
		broken := byName["probe"]
		broken.head = "deadbeefdeadbeefdeadbeefdeadbeefdeadbeef"
		got := inspect(t.Context(), cfg, testEnv(repo), broken)
		if got.class != classUnknown || got.removed {
			t.Fatalf("inspect() = %+v, want a non-removed %s", got, classUnknown)
		}
		if _, err := os.Stat(wtPath); err != nil {
			t.Fatalf("worktree removed after a failed commits-ahead probe: %v", err)
		}
	})

	t.Run("unreadable checkout", func(t *testing.T) {
		missing := byName["probe"]
		missing.path = filepath.Join(t.TempDir(), "does-not-exist")
		got := inspect(t.Context(), cfg, testEnv(repo), missing)
		if got.class != classUnknown || got.removed {
			t.Fatalf("inspect() = %+v, want a non-removed %s", got, classUnknown)
		}
	})
}

// TestInspectFixRemovesStaleWorktreeAndBranch is the happy path: a clean,
// merged, unowned checkout is reclaimed, its local branch is deleted, and its
// remote ref survives.
func TestInspectFixRemovesStaleWorktreeAndBranch(t *testing.T) {
	repo := newRepo(t)
	stalePath := filepath.Join(t.TempDir(), "gone")
	addWorktree(t, repo, "gone", stalePath)
	gittest.Run(t, repo, "push", "origin", "gone")

	byName := worktreeByName(t, repo)
	target := byName["gone"]
	if target.path == "" {
		t.Fatal("stale worktree not listed")
	}

	cfg := config{repo: repo, fix: true, maxAge: 14, preserve: preserveFlag{}}
	got := inspect(t.Context(), cfg, testEnv(repo), target)
	if got.class != classMerged || !got.removed || got.failed {
		t.Fatalf("inspect() = %+v, want a removed %s", got, classMerged)
	}
	if err := gitOK(t.Context(), repo, "rev-parse", "--verify", "--quiet", "refs/heads/gone"); err == nil {
		t.Error("local branch gone still exists after --fix")
	}
	// The canonical sweep deletes the local branch and preserves the remote
	// ref; this tool must not diverge from that contract.
	if err := gitOK(t.Context(), repo, "rev-parse", "--verify", "--quiet", "refs/remotes/origin/gone"); err != nil {
		t.Errorf("remote ref origin/gone was deleted: %v", err)
	}
	for _, wt := range worktreeByName(t, repo) {
		if filepath.Base(wt.path) == "gone" {
			t.Error("worktree gone still registered after --fix")
		}
	}
}

func TestRunRejectsNonGitDirectory(t *testing.T) {
	err := run(t.Context(), []string{t.TempDir()})
	if err == nil {
		t.Fatal("run() = nil error, want failure for a non-git directory")
	}
	var exit exitError
	if !errors.As(err, &exit) || exit.code != 2 {
		t.Fatalf("run() error = %v, want exitError code 2", err)
	}
}

func TestNoTmuxServerIsNotAProbeFailure(t *testing.T) {
	if !noTmuxServer("no server running on /Users/x/.agm/agm.sock") {
		t.Error("an absent tmux server was treated as a probe failure")
	}
	if noTmuxServer("connection refused: bad socket permissions") {
		t.Error("a genuine tmux failure was treated as an empty session set")
	}
}
