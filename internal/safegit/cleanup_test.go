package safegit

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/vbonnet/dear-agent/internal/gittest"
)

const (
	cleanupHelperEnv       = "SAFEGIT_CLEANUP_HELPER"
	cleanupHelperBranchEnv = "SAFEGIT_CLEANUP_BRANCH"
)

type cleanupFixture struct {
	primary string
	linked  string
	branch  string
}

func TestCleanupWorktreeHelper(t *testing.T) {
	if os.Getenv(cleanupHelperEnv) != "1" {
		return
	}
	branch := os.Getenv(cleanupHelperBranchEnv)
	if branch == "" {
		t.Fatal("cleanup helper requires a branch")
	}

	cleanupWorktree(context.Background(), branch)
	// Exit without asking the test runner to inspect the helper's removed cwd.
	os.Exit(0)
}

func TestRunCleanupGitHonorsCanceledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if _, err := runCleanupGit(ctx, "", "version"); !errors.Is(err, context.Canceled) {
		t.Fatalf("runCleanupGit error = %v, want context.Canceled", err)
	}
}

func TestRunCleanupGitBoundsDescendantHeldPipes(t *testing.T) {
	fakeGit := filepath.Join(t.TempDir(), "git")
	if err := os.WriteFile(fakeGit, []byte("#!/bin/sh\n(sleep 30) &\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", filepath.Dir(fakeGit)+string(os.PathListSeparator)+os.Getenv("PATH"))

	started := time.Now()
	_, err := runCleanupGit(context.Background(), "", "version")
	if !errors.Is(err, exec.ErrWaitDelay) {
		t.Fatalf("runCleanupGit error = %v, want exec.ErrWaitDelay", err)
	}
	if elapsed := time.Since(started); elapsed > 5*cleanupCommandWaitDelay {
		t.Fatalf("runCleanupGit waited %s for descendant-held pipes", elapsed)
	}
}

func TestCleanupWorktreeUsesPrimaryAfterRemovingCallerDirectory(t *testing.T) {
	fixture := newCleanupFixture(t)
	output := runCleanupHelper(t, fixture)

	if _, err := os.Stat(fixture.linked); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("linked worktree path still exists after cleanup: %v", err)
	}
	assertWorktreeRegistration(t, fixture, false)
	assertBranchRef(t, fixture, false)
	assertPrimaryUsable(t, fixture.primary)

	if !strings.Contains(output, "removed local branch "+fixture.branch) {
		t.Fatalf("cleanup did not report successful branch deletion:\n%s", output)
	}
	for _, diagnostic := range []string{
		"getcwd",
		"unable to read current working directory",
		"cannot chdir",
		"no such file or directory",
	} {
		if strings.Contains(strings.ToLower(output), diagnostic) {
			t.Fatalf("cleanup emitted stale-cwd diagnostic %q:\n%s", diagnostic, output)
		}
	}
}

func TestCleanupWorktreePreservesPrimaryPathWhitespace(t *testing.T) {
	fixture := newCleanupFixtureWithPrimaryName(t, "primary\ncheckout \t")
	output := runCleanupHelper(t, fixture)

	if _, err := os.Stat(fixture.linked); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("linked worktree path still exists after cleanup: %v", err)
	}
	assertWorktreeRegistration(t, fixture, false)
	assertBranchRef(t, fixture, false)
	assertPrimaryUsable(t, fixture.primary)
	if !strings.Contains(output, "removed local branch "+fixture.branch) {
		t.Fatalf("cleanup did not preserve the primary worktree path:\n%s", output)
	}
}

func TestCleanupWorktreeWarnsWhenDirtyWorktreeCannotBeRemoved(t *testing.T) {
	fixture := newCleanupFixture(t)
	readme := filepath.Join(fixture.linked, "README.md")
	if err := os.WriteFile(readme, []byte("# dirty linked worktree\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	output := runCleanupHelper(t, fixture)

	if !strings.Contains(output, "cleanup: worktree remove "+fixture.linked+":") {
		t.Fatalf("cleanup did not identify the failed worktree removal:\n%s", output)
	}
	if !strings.Contains(output, "cleanup: branch -d "+fixture.branch+":") {
		t.Fatalf("cleanup did not identify the conservative branch deletion failure:\n%s", output)
	}
	if strings.Contains(output, "removed local branch "+fixture.branch) {
		t.Fatalf("cleanup falsely reported complete branch cleanup:\n%s", output)
	}
	if _, err := os.Stat(fixture.linked); err != nil {
		t.Fatalf("dirty linked worktree path was not preserved: %v", err)
	}
	assertWorktreeRegistration(t, fixture, true)
	assertBranchRef(t, fixture, true)
	assertPrimaryUsable(t, fixture.primary)
}

func newCleanupFixture(t *testing.T) cleanupFixture {
	return newCleanupFixtureWithPrimaryName(t, "primary")
}

func newCleanupFixtureWithPrimaryName(t *testing.T, primaryName string) cleanupFixture {
	t.Helper()
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	fixture := cleanupFixture{
		primary: filepath.Join(root, primaryName),
		linked:  filepath.Join(root, "linked-topic"),
		branch:  "cleanup-topic",
	}
	gittest.InitRepo(t, fixture.primary)
	gittest.Run(t, fixture.primary, "branch", fixture.branch)
	gittest.Run(t, fixture.primary, "worktree", "add", "--quiet", fixture.linked, fixture.branch)
	return fixture
}

func runCleanupHelper(t *testing.T, fixture cleanupFixture) string {
	t.Helper()
	helper := exec.Command(os.Args[0], "-test.run=^TestCleanupWorktreeHelper$")
	helper.Dir = fixture.linked
	helper.Env = append(gittest.Env(t),
		cleanupHelperEnv+"=1",
		cleanupHelperBranchEnv+"="+fixture.branch,
	)
	out, err := helper.CombinedOutput()
	if err != nil {
		t.Fatalf("cleanup helper failed: %v\n%s", err, out)
	}
	return string(out)
}

func assertWorktreeRegistration(t *testing.T, fixture cleanupFixture, want bool) {
	t.Helper()
	list := gittest.Run(t, fixture.primary, "worktree", "list", "--porcelain")
	got := strings.Contains(list, "worktree "+fixture.linked+"\n")
	if got != want {
		t.Fatalf("linked worktree registration present = %t, want %t:\n%s", got, want, list)
	}
}

func assertBranchRef(t *testing.T, fixture cleanupFixture, want bool) {
	t.Helper()
	refs := strings.TrimSpace(gittest.Run(t, fixture.primary,
		"for-each-ref", "--format=%(refname)", "refs/heads/"+fixture.branch))
	got := refs == "refs/heads/"+fixture.branch
	if got != want {
		t.Fatalf("topic branch ref present = %t, want %t (refs %q)", got, want, refs)
	}
}

func assertPrimaryUsable(t *testing.T, primary string) {
	t.Helper()
	if head := strings.TrimSpace(gittest.Run(t, primary, "rev-parse", "--verify", "HEAD")); head == "" {
		t.Fatal("primary worktree has no usable HEAD after cleanup")
	}
	gittest.Run(t, primary, "status", "--porcelain")
}
