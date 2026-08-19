package main

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/vbonnet/dear-agent/internal/gittest"
)

// The shim, scripts/cleanup-worktrees.sh, is the entry point operators and
// automation actually invoke, so the guard is only as safe as what reaches it
// through that script. These tests drive the real script against a real
// repository and assert on what is left on disk afterwards.
//
// They live in Go rather than in tests/bats because the shim's fallback path
// compiles the command with `go build`, and the Bats matrix runs in
// bash/alpine/debian containers that install only git, bash, and coreutils.
// A Bats test therefore cannot reach the guard at all; it can only check the
// script's dispatch against a stub binary, which tests/bats/cleanup-worktrees.bats
// does. This file covers the half that needs a toolchain.

// shimPath returns the absolute path of the shim, and the module root it
// resolves its build from.
func shimPath(t *testing.T) (shim, root string) {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	root = filepath.Clean(filepath.Join(wd, "..", ".."))
	if _, err := os.Stat(filepath.Join(root, "go.mod")); err != nil {
		t.Fatalf("module root %s has no go.mod: %v", root, err)
	}
	shim = filepath.Join(root, "scripts", "cleanup-worktrees.sh")
	if _, err := os.Stat(shim); err != nil {
		t.Fatalf("shim not found at %s: %v", shim, err)
	}
	return shim, root
}

// runShim executes the shim from a working directory outside the repository,
// which is the case the module-root build fix exists for: `go build` must
// resolve this repository's go.mod no matter where the caller stands.
func runShim(t *testing.T, elsewhere string, args ...string) (stdout string, err error) {
	t.Helper()
	shim, _ := shimPath(t)
	cmd := exec.Command(shim, args...)
	cmd.Dir = elsewhere
	out, err := cmd.CombinedOutput()
	return string(out), err
}

// e2eRepo builds a repository whose linked worktrees cover one instance of
// every verdict the guard can reach without a live session: a clean merged
// checkout, an untracked-work checkout, an unmerged-commit checkout, and a
// git-locked checkout.
//
// Names are prefixed so they cannot collide with a real tmux session on a
// developer machine, which would otherwise classify a fixture as ACTIVE and
// make the reap assertion flap.
func e2eRepo(t *testing.T) (repo string, worktrees map[string]string) {
	t.Helper()
	root := t.TempDir()
	origin := filepath.Join(root, "origin")
	gittest.Run(t, root, "init", "--bare", "--initial-branch=main", origin)
	gittest.HardenRepo(t, origin)

	repo = filepath.Join(root, "repo")
	gittest.Run(t, root, "init", "--initial-branch=main", repo)
	gittest.HardenRepo(t, repo)
	gittest.Run(t, repo, "commit", "--allow-empty", "-m", "base")
	gittest.Run(t, repo, "remote", "add", "origin", origin)
	gittest.Run(t, repo, "push", "-u", "origin", "main")

	worktrees = map[string]string{}
	for _, name := range []string{"e2e-merged", "e2e-dirty", "e2e-ahead", "e2e-locked"} {
		path := filepath.Join(root, name)
		gittest.Run(t, repo, "worktree", "add", "-b", name, path)
		gittest.HardenRepo(t, path)
		worktrees[name] = path
	}

	// Uncommitted work that only exists in this checkout.
	if err := os.WriteFile(filepath.Join(worktrees["e2e-dirty"], "unsaved.txt"), []byte("work"), 0o600); err != nil {
		t.Fatalf("writing untracked file: %v", err)
	}
	// A commit that is on no other ref.
	gittest.Run(t, worktrees["e2e-ahead"], "commit", "--allow-empty", "-m", "unlanded work")
	// An operator-held lock, the house convention for "an agent is using this".
	// No unlock cleanup: the repository and every worktree live under
	// t.TempDir(), so the lock leaves nothing behind on the host.
	gittest.Run(t, repo, "worktree", "lock", worktrees["e2e-locked"], "--reason", "in use")

	return repo, worktrees
}

// TestShimDryRunMutatesNothing proves the default invocation is an audit.
func TestShimDryRunMutatesNothing(t *testing.T) {
	repo, worktrees := e2eRepo(t)
	elsewhere := t.TempDir()

	out, err := runShim(t, elsewhere, repo)
	if err != nil {
		t.Fatalf("shim dry run failed: %v\n%s", err, out)
	}
	for name, path := range worktrees {
		if _, statErr := os.Stat(path); statErr != nil {
			t.Errorf("dry run removed %s: %v", name, statErr)
		}
	}
	if !strings.Contains(out, "mode: DRY-RUN") {
		t.Errorf("output does not announce dry-run mode:\n%s", out)
	}
	if !strings.Contains(out, "would:") {
		t.Errorf("dry run did not preview any removal:\n%s", out)
	}
}

// TestShimFixReapsOnlyTheProvablyMergedWorktree is the end-to-end data-loss
// regression. Everything that is not provably reapable must still be on disk
// after a --fix run, and its branch must still exist.
func TestShimFixReapsOnlyTheProvablyMergedWorktree(t *testing.T) {
	repo, worktrees := e2eRepo(t)
	elsewhere := t.TempDir()

	out, err := runShim(t, elsewhere, repo, "--fix")
	if err != nil {
		t.Fatalf("shim --fix failed: %v\n%s", err, out)
	}

	if _, statErr := os.Stat(worktrees["e2e-merged"]); statErr == nil {
		t.Errorf("a clean, merged, unowned worktree was not reclaimed:\n%s", out)
	}
	survivors := []struct{ name, why string }{
		{"e2e-dirty", "it holds uncommitted work"},
		{"e2e-ahead", "it holds commits that are on no other ref"},
		{"e2e-locked", "an operator locked it"},
	}
	for _, s := range survivors {
		if _, statErr := os.Stat(worktrees[s.name]); statErr != nil {
			t.Errorf("%s was deleted even though %s: %v\n%s", s.name, s.why, statErr, out)
		}
		if branchErr := gitOK(t.Context(), repo, "rev-parse", "--verify", "--quiet", "refs/heads/"+s.name); branchErr != nil {
			t.Errorf("branch %s was deleted even though %s: %v", s.name, s.why, branchErr)
		}
	}
	// The remote ref of the reclaimed branch must survive: local hygiene is
	// not authorization to touch the remote.
	if err := gitOK(t.Context(), repo, "rev-parse", "--verify", "--quiet", "refs/remotes/origin/main"); err != nil {
		t.Errorf("remote ref origin/main disappeared during cleanup: %v", err)
	}
}

// TestShimRejectsANonRepositoryPath proves the shim propagates the command's
// exit status rather than swallowing it, which is what the removed `exec`
// used to obscure once a trap was involved.
func TestShimRejectsANonRepositoryPath(t *testing.T) {
	elsewhere := t.TempDir()
	out, err := runShim(t, elsewhere, t.TempDir())
	if err == nil {
		t.Fatalf("shim exited 0 for a non-git directory:\n%s", out)
	}
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) || exitErr.ExitCode() != 2 {
		t.Fatalf("shim exit = %v, want the command's own exit code 2:\n%s", err, out)
	}
}

// TestShimLeavesNoTemporaryBuildBehind covers the leak the `exec` created: it
// replaced the shell, so the EXIT trap never ran and every invocation left a
// compiled binary in TMPDIR.
func TestShimLeavesNoTemporaryBuildBehind(t *testing.T) {
	_, root := shimPath(t)
	if _, err := os.Stat(filepath.Join(root, "bin", "cleanup-worktrees")); err == nil {
		t.Skip("a prebuilt bin/cleanup-worktrees exists, so the shim takes the exec path and builds nothing")
	}
	tmp := t.TempDir()
	repo, _ := e2eRepo(t)

	shim, _ := shimPath(t)
	cmd := exec.Command(shim, repo)
	cmd.Dir = t.TempDir()
	cmd.Env = append(os.Environ(), "TMPDIR="+tmp)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("shim failed: %v\n%s", err, out)
	}

	leftovers, err := os.ReadDir(tmp)
	if err != nil {
		t.Fatalf("reading TMPDIR: %v", err)
	}
	if len(leftovers) != 0 {
		names := make([]string, 0, len(leftovers))
		for _, e := range leftovers {
			names = append(names, e.Name())
		}
		t.Errorf("shim leaked %d entr(ies) into TMPDIR: %v", len(leftovers), names)
	}
}
