package githooks_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
)

// hookPath resolves scripts/git-hooks/post-merge relative to this test file so
// the test is independent of the working directory `go test` chooses.
func hookPath(t *testing.T) string {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	// this file: <repo>/tests/githooks/post_merge_test.go
	repoRoot := filepath.Join(filepath.Dir(thisFile), "..", "..")
	p := filepath.Join(repoRoot, "scripts", "git-hooks", "post-merge")
	if _, err := os.Stat(p); err != nil {
		t.Fatalf("hook not found at %s: %v", p, err)
	}
	return p
}

// git runs a git command in dir and fails the test on error.
func git(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	// Deterministic identity; no global config dependence.
	cmd.Env = append(os.Environ(),
		"GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t",
		"GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t",
	)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

// newRepo creates a temp git repo whose default branch is main, with one
// commit, and returns its path.
func newRepo(t *testing.T) string {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	dir := t.TempDir()
	git(t, dir, "init", "-q", "-b", "main")
	if err := os.WriteFile(filepath.Join(dir, "f"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	git(t, dir, "add", "f")
	git(t, dir, "commit", "-qm", "init")
	return dir
}

// stubAGM writes a fake `agm` into a fresh dir and returns that dir. The stub
// touches sentinel when invoked as `agm worktree sweep ...`.
func stubAGM(t *testing.T, sentinel string) string {
	t.Helper()
	dir := t.TempDir()
	script := "#!/bin/sh\n" +
		"if [ \"$1\" = worktree ] && [ \"$2\" = sweep ]; then : > \"" + sentinel + "\"; fi\n" +
		"exit 0\n"
	if err := os.WriteFile(filepath.Join(dir, "agm"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	return dir
}

// runHook executes the post-merge hook in repoDir. agmDir (if non-empty) is
// prepended to PATH so the stub `agm` is found; extraEnv is appended. It
// returns the process exit; the hook must always exit 0.
func runHook(t *testing.T, repoDir, agmDir string, extraEnv ...string) {
	t.Helper()
	if _, err := exec.LookPath("bash"); err != nil {
		t.Skip("bash not available")
	}
	home := t.TempDir() // isolate ~/.claude/logs writes
	path := os.Getenv("PATH")
	if agmDir != "" {
		path = agmDir + string(os.PathListSeparator) + path
	}
	cmd := exec.Command("bash", hookPath(t))
	cmd.Dir = repoDir
	cmd.Env = append(os.Environ(),
		"HOME="+home,
		"PATH="+path,
		"AGM_POST_MERGE_SWEEP_SYNC=1", // deterministic: run sweep in foreground
	)
	cmd.Env = append(cmd.Env, extraEnv...)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("hook must exit 0, got %v\n%s", err, out)
	}
}

func sentinelPath(t *testing.T) string {
	return filepath.Join(t.TempDir(), "swept")
}

func swept(t *testing.T, sentinel string) bool {
	t.Helper()
	_, err := os.Stat(sentinel)
	return err == nil
}

// On the default branch, the hook triggers the sweep.
func TestPostMerge_DefaultBranch_Sweeps(t *testing.T) {
	repo := newRepo(t)
	sentinel := sentinelPath(t)
	agmDir := stubAGM(t, sentinel)
	runHook(t, repo, agmDir)
	if !swept(t, sentinel) {
		t.Fatal("expected sweep to run on default branch, but stub was not invoked")
	}
}

// On a feature branch, the hook must NOT sweep (a feature integration merge is
// not a PR landing on main).
func TestPostMerge_FeatureBranch_Skips(t *testing.T) {
	repo := newRepo(t)
	git(t, repo, "checkout", "-q", "-b", "feature")
	sentinel := sentinelPath(t)
	agmDir := stubAGM(t, sentinel)
	runHook(t, repo, agmDir)
	if swept(t, sentinel) {
		t.Fatal("sweep ran on a feature branch; it must only run on the default branch")
	}
}

// AGM_POST_MERGE_SWEEP=0 disables the hook entirely.
func TestPostMerge_OptOut_Skips(t *testing.T) {
	repo := newRepo(t)
	sentinel := sentinelPath(t)
	agmDir := stubAGM(t, sentinel)
	runHook(t, repo, agmDir, "AGM_POST_MERGE_SWEEP=0")
	if swept(t, sentinel) {
		t.Fatal("sweep ran despite AGM_POST_MERGE_SWEEP=0")
	}
}

// With no `agm` on PATH the hook is an inert no-op that still exits 0.
func TestPostMerge_NoAGM_NoOp(t *testing.T) {
	repo := newRepo(t)
	sentinel := sentinelPath(t)
	// agmDir empty => stub not on PATH. runHook fails the test if exit != 0.
	runHook(t, repo, "")
	if swept(t, sentinel) {
		t.Fatal("sweep sentinel appeared without an agm on PATH")
	}
}
