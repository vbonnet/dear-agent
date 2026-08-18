package main

import (
	"errors"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
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

// gitT runs git in dir and fails the test on error.
func gitT(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	cmd.Env = append(cmd.Environ(),
		"GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@example.com",
		"GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@example.com",
		"GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_SYSTEM=/dev/null",
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s: %v: %s", strings.Join(args, " "), err, out)
	}
	return strings.TrimSpace(string(out))
}

// newRepo builds a repo with an origin/main ref and one commit.
func newRepo(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	origin := filepath.Join(root, "origin")
	gitT(t, root, "init", "--bare", "--initial-branch=main", origin)

	repo := filepath.Join(root, "repo")
	gitT(t, root, "init", "--initial-branch=main", repo)
	gitT(t, repo, "commit", "--allow-empty", "-m", "base")
	gitT(t, repo, "remote", "add", "origin", origin)
	gitT(t, repo, "push", "-u", "origin", "main")
	return repo
}

func TestTargetRefPrefersOriginMain(t *testing.T) {
	repo := newRepo(t)
	got, err := targetRef(repo)
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
	gitT(t, root, "init", "--initial-branch=main", repo)
	gitT(t, repo, "commit", "--allow-empty", "-m", "base")
	if _, err := targetRef(repo); err == nil {
		t.Fatal("targetRef() = nil error, want failure when no origin ref exists")
	}
}

func TestListWorktreesParsesPorcelain(t *testing.T) {
	repo := newRepo(t)
	wtPath := filepath.Join(t.TempDir(), "feature")
	gitT(t, repo, "worktree", "add", "-b", "feature", wtPath)

	got, err := listWorktrees(repo)
	if err != nil {
		t.Fatalf("listWorktrees() error = %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("listWorktrees() returned %d entries, want 2: %+v", len(got), got)
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
	if feature.bare || feature.detached {
		t.Errorf("feature worktree bare=%v detached=%v, want both false", feature.bare, feature.detached)
	}
}

func TestInspectClassification(t *testing.T) {
	repo := newRepo(t)
	target := "origin/main"
	now := time.Now()

	mergedPath := filepath.Join(t.TempDir(), "merged")
	gitT(t, repo, "worktree", "add", "-b", "merged", mergedPath)
	aheadPath := filepath.Join(t.TempDir(), "ahead")
	gitT(t, repo, "worktree", "add", "-b", "ahead", aheadPath)
	gitT(t, aheadPath, "commit", "--allow-empty", "-m", "work")

	all, err := listWorktrees(repo)
	if err != nil {
		t.Fatalf("listWorktrees() error = %v", err)
	}
	byName := map[string]worktree{}
	for _, wt := range all {
		byName[filepath.Base(wt.path)] = wt
	}

	dryRun := config{repo: repo, maxAge: 14, preserve: preserveFlag{}}

	t.Run("zero commits ahead is stale", func(t *testing.T) {
		got := inspect(dryRun, target, now, byName["merged"])
		if got.stale != 1 || got.kept != 0 {
			t.Errorf("inspect() = %+v, want stale=1", got)
		}
	})

	t.Run("recent commits ahead is kept", func(t *testing.T) {
		got := inspect(dryRun, target, now, byName["ahead"])
		if got.kept != 1 || got.stale != 0 {
			t.Errorf("inspect() = %+v, want kept=1", got)
		}
	})

	t.Run("old commits ahead are idle-stale", func(t *testing.T) {
		future := now.Add(30 * 24 * time.Hour)
		got := inspect(dryRun, target, future, byName["ahead"])
		if got.stale != 1 {
			t.Errorf("inspect() = %+v, want stale=1 for an idle worktree", got)
		}
	})

	t.Run("preserve wins over staleness", func(t *testing.T) {
		cfg := config{repo: repo, maxAge: 14, preserve: preserveFlag{"merged": true}}
		got := inspect(cfg, target, now, byName["merged"])
		if got.preserved != 1 || got.stale != 0 {
			t.Errorf("inspect() = %+v, want preserved=1", got)
		}
	})

	t.Run("bare and detached worktrees are skipped", func(t *testing.T) {
		if got := inspect(dryRun, target, now, worktree{path: "/x", bare: true}); got != (outcome{}) {
			t.Errorf("inspect(bare) = %+v, want zero outcome", got)
		}
		if got := inspect(dryRun, target, now, worktree{path: "/x", detached: true}); got != (outcome{}) {
			t.Errorf("inspect(detached) = %+v, want zero outcome", got)
		}
	})

	t.Run("dry run leaves the stale worktree on disk", func(t *testing.T) {
		if _, err := git(repo, "rev-parse", "--verify", "--quiet", "merged"); err != nil {
			t.Fatalf("dry run removed branch merged: %v", err)
		}
		list, err := listWorktrees(repo)
		if err != nil {
			t.Fatalf("listWorktrees() error = %v", err)
		}
		if len(list) != 3 {
			t.Errorf("dry run changed the worktree set: %+v", list)
		}
	})
}

func TestInspectFixRemovesStaleWorktreeAndBranch(t *testing.T) {
	repo := newRepo(t)
	stalePath := filepath.Join(t.TempDir(), "gone")
	gitT(t, repo, "worktree", "add", "-b", "gone", stalePath)

	all, err := listWorktrees(repo)
	if err != nil {
		t.Fatalf("listWorktrees() error = %v", err)
	}
	var target worktree
	for _, wt := range all {
		if filepath.Base(wt.path) == "gone" {
			target = wt
		}
	}
	if target.path == "" {
		t.Fatal("stale worktree not listed")
	}

	cfg := config{repo: repo, fix: true, maxAge: 14, preserve: preserveFlag{}}
	got := inspect(cfg, "origin/main", time.Now(), target)
	if got.stale != 1 || got.failed != 0 {
		t.Fatalf("inspect() = %+v, want stale=1 failed=0", got)
	}
	if _, err := git(repo, "rev-parse", "--verify", "--quiet", "gone"); err == nil {
		t.Error("branch gone still exists after --fix")
	}
	list, err := listWorktrees(repo)
	if err != nil {
		t.Fatalf("listWorktrees() error = %v", err)
	}
	for _, wt := range list {
		if filepath.Base(wt.path) == "gone" {
			t.Error("worktree gone still registered after --fix")
		}
	}
}

func TestRunRejectsNonGitDirectory(t *testing.T) {
	err := run([]string{t.TempDir()})
	if err == nil {
		t.Fatal("run() = nil error, want failure for a non-git directory")
	}
	var exit exitError
	if !errors.As(err, &exit) || exit.code != 2 {
		t.Fatalf("run() error = %v, want exitError code 2", err)
	}
}
