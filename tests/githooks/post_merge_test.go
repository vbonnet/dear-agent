package githooks_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
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

// ── Stage 1 (binary rebuild) ────────────────────────────────────────────────

// stubGo writes a fake `go` that records each `go install <pkg>` invocation
// into recordFile as "<pkg>|<HEAD-of-cwd>" (one per line) and returns the dir
// holding it. Capturing the cwd's HEAD lets tests assert WHICH commit the hook
// built from — the whole point of the origin/main fix: the build must run in a
// checkout pinned to trunk, not in whatever ref the local working tree sits on.
func stubGo(t *testing.T, recordFile string) string {
	t.Helper()
	dir := t.TempDir()
	script := "#!/bin/sh\n" +
		"if [ \"$1\" = install ]; then echo \"$2|$(git rev-parse HEAD 2>/dev/null)\" >> \"" + recordFile + "\"; fi\n" +
		"exit 0\n"
	if err := os.WriteFile(filepath.Join(dir, "go"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	return dir
}

// installRecord is one captured `go install` invocation: the package and the
// HEAD commit of the directory the build actually ran in.
type installRecord struct {
	pkg    string
	commit string
}

// installRecords parses the stub's record into structured invocations.
func installRecords(t *testing.T, recordFile string) []installRecord {
	t.Helper()
	b, err := os.ReadFile(recordFile)
	if err != nil {
		return nil // no file => nothing installed
	}
	var out []installRecord
	for l := range strings.SplitSeq(strings.TrimSpace(string(b)), "\n") {
		if l == "" {
			continue
		}
		pkg, commit, _ := strings.Cut(l, "|")
		out = append(out, installRecord{pkg: pkg, commit: commit})
	}
	return out
}

// installed returns the set of pkgs the stub `go` was asked to install.
func installed(t *testing.T, recordFile string) []string {
	t.Helper()
	var out []string
	for _, r := range installRecords(t, recordFile) {
		out = append(out, r.pkg)
	}
	return out
}

// revParse returns the resolved commit for ref in dir.
func revParse(t *testing.T, dir, ref string) string {
	t.Helper()
	cmd := exec.Command("git", "rev-parse", ref)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git rev-parse %s in %s: %v\n%s", ref, dir, err, out)
	}
	return strings.TrimSpace(string(out))
}

// newRebuildRepo builds a repo that looks like dear-agent: it has the two
// binary package dirs and a baseline commit on main. It returns the repo path.
func newRebuildRepo(t *testing.T) string {
	t.Helper()
	repo := newRepo(t)
	for _, p := range []string{"agm/cmd/agm", "cmd/vroom-dispatch", "pkg/llm/auth", "internal/x", "docs"} {
		if err := os.MkdirAll(filepath.Join(repo, p), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	// Seed each package dir with a tracked file so later diffs are meaningful.
	for _, f := range []string{"agm/cmd/agm/main.go", "cmd/vroom-dispatch/main.go", "pkg/llm/auth/auth.go", "internal/x/x.go", "docs/readme.md"} {
		if err := os.WriteFile(filepath.Join(repo, f), []byte("package x\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	git(t, repo, "add", "-A")
	git(t, repo, "commit", "-qm", "scaffold")
	return repo
}

// mergeBranchChanging commits the given path-changes on a feature branch, then
// merges it back into main so ORIG_HEAD/HEAD@{1} reflect a real merge — exactly
// the state the hook inspects after `git pull`.
func mergeBranchChanging(t *testing.T, repo string, files map[string]string) {
	t.Helper()
	git(t, repo, "checkout", "-q", "-b", "feat")
	for f, content := range files {
		if err := os.MkdirAll(filepath.Dir(filepath.Join(repo, f)), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(repo, f), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	git(t, repo, "add", "-A")
	git(t, repo, "commit", "-qm", "change")
	git(t, repo, "checkout", "-q", "main")
	git(t, repo, "merge", "-q", "--no-ff", "-m", "merge feat", "feat")
}

// runRebuildRecord runs the hook with a stub `go` (and no agm) and returns the
// path to the record file capturing every `go install` invocation (pkg + the
// HEAD of the directory it built in).
func runRebuildRecord(t *testing.T, repo string, extraEnv ...string) string {
	t.Helper()
	if _, err := exec.LookPath("bash"); err != nil {
		t.Skip("bash not available")
	}
	home := t.TempDir()
	record := filepath.Join(t.TempDir(), "installed")
	goDir := stubGo(t, record)
	cmd := exec.Command("bash", hookPath(t))
	cmd.Dir = repo
	cmd.Env = append(os.Environ(),
		"HOME="+home,
		"PATH="+goDir+string(os.PathListSeparator)+os.Getenv("PATH"),
		"AGM_POST_MERGE_SWEEP=0", // isolate Stage 1: no sweep
	)
	cmd.Env = append(cmd.Env, extraEnv...)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("hook must exit 0, got %v\n%s", err, out)
	}
	return record
}

// runRebuild runs the hook with a stub `go` (and no agm) and returns the pkgs
// the stub was asked to install.
func runRebuild(t *testing.T, repo string, extraEnv ...string) []string {
	t.Helper()
	return installed(t, runRebuildRecord(t, repo, extraEnv...))
}

// A change under a shared source tree (pkg/) rebuilds BOTH binaries.
func TestRebuild_SharedSource_RebuildsBoth(t *testing.T) {
	repo := newRebuildRepo(t)
	mergeBranchChanging(t, repo, map[string]string{"pkg/llm/auth/auth.go": "package auth // v2\n"})
	got := runRebuild(t, repo)
	if !slices.Contains(got, "./agm/cmd/agm") || !slices.Contains(got, "./cmd/vroom-dispatch") {
		t.Fatalf("expected both binaries rebuilt on a pkg/ change, got %v", got)
	}
}

// A change confined to cmd/vroom-dispatch/ rebuilds ONLY vroom-dispatch.
func TestRebuild_ScopedSource_RebuildsOnlyAffected(t *testing.T) {
	repo := newRebuildRepo(t)
	mergeBranchChanging(t, repo, map[string]string{"cmd/vroom-dispatch/main.go": "package main // v2\n"})
	got := runRebuild(t, repo)
	if slices.Contains(got, "./agm/cmd/agm") {
		t.Fatalf("agm rebuilt for a vroom-dispatch-only change: %v", got)
	}
	if !slices.Contains(got, "./cmd/vroom-dispatch") {
		t.Fatalf("vroom-dispatch not rebuilt for its own change: %v", got)
	}
}

// A change confined to agm/ rebuilds ONLY agm.
func TestRebuild_AgmOnly(t *testing.T) {
	repo := newRebuildRepo(t)
	mergeBranchChanging(t, repo, map[string]string{"agm/cmd/agm/main.go": "package main // v2\n"})
	got := runRebuild(t, repo)
	if slices.Contains(got, "./cmd/vroom-dispatch") {
		t.Fatalf("vroom-dispatch rebuilt for an agm-only change: %v", got)
	}
	if !slices.Contains(got, "./agm/cmd/agm") {
		t.Fatalf("agm not rebuilt for its own change: %v", got)
	}
}

// Docs-, test-, and config-only merges rebuild NOTHING (fast path).
func TestRebuild_NonSourceChanges_SkipsAll(t *testing.T) {
	cases := []struct {
		name string
		file string
	}{
		{"markdown under pkg", "pkg/llm/auth/README.md"},
		{"test file under pkg", "pkg/llm/auth/auth_test.go"},
		{"docs tree", "docs/guide.md"},
		{"github workflows", ".github/workflows/ci.yml"},
		{"top-level config", ".dear-agent.yml"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			repo := newRebuildRepo(t)
			mergeBranchChanging(t, repo, map[string]string{tc.file: "noise\n"})
			got := runRebuild(t, repo)
			if len(got) != 0 {
				t.Fatalf("%s triggered a rebuild but should not: %v", tc.file, got)
			}
		})
	}
}

// DEAR_AGENT_POST_MERGE_REBUILD=0 disables Stage 1 even when source changed.
func TestRebuild_OptOut(t *testing.T) {
	repo := newRebuildRepo(t)
	mergeBranchChanging(t, repo, map[string]string{"pkg/llm/auth/auth.go": "package auth // v2\n"})
	got := runRebuild(t, repo, "DEAR_AGENT_POST_MERGE_REBUILD=0")
	if len(got) != 0 {
		t.Fatalf("rebuild ran despite DEAR_AGENT_POST_MERGE_REBUILD=0: %v", got)
	}
}

// A repo without the binary packages is a silent no-op (global-hook safety):
// the stub `go install` must never be invoked.
func TestRebuild_ForeignRepo_NoOp(t *testing.T) {
	repo := newRepo(t) // no agm/ or cmd/vroom-dispatch dirs
	mergeBranchChanging(t, repo, map[string]string{"main.go": "package main\n"})
	got := runRebuild(t, repo)
	if len(got) != 0 {
		t.Fatalf("rebuild ran in a non-dear-agent repo: %v", got)
	}
}

// On a feature branch, Stage 1 must not rebuild (mirrors the sweep gate).
func TestRebuild_FeatureBranch_Skips(t *testing.T) {
	repo := newRebuildRepo(t)
	mergeBranchChanging(t, repo, map[string]string{"pkg/llm/auth/auth.go": "package auth // v2\n"})
	git(t, repo, "checkout", "-q", "-b", "feature")
	got := runRebuild(t, repo)
	if len(got) != 0 {
		t.Fatalf("rebuild ran on a feature branch: %v", got)
	}
}

// ── Build-source invariant: always build from origin/<default_branch> ────────

// cloneRepo clones src into a fresh dir and returns the clone path. The clone's
// main tracks origin/main, exactly like a real working checkout.
func cloneRepo(t *testing.T, src string) string {
	t.Helper()
	parent := t.TempDir()
	dst := filepath.Join(parent, "clone")
	git(t, parent, "clone", "-q", src, dst)
	return dst
}

// The core of the Jun-18 fix (ce-11u3 / retro B2): when an origin remote exists,
// the rebuild must build from the freshly-fetched origin/<default_branch>, NOT
// from the local working tree's ref — even when local main has advanced past
// origin (an un-pushed merge). The stub records the HEAD of the dir it built in;
// it must equal origin/main, never the divergent local HEAD.
func TestRebuild_BuildsFromOriginMain_NotLocalRef(t *testing.T) {
	origin := newRebuildRepo(t)
	trunk := revParse(t, origin, "HEAD") // origin/main commit the binary must come from

	local := cloneRepo(t, origin)
	// Advance local main past origin via a merge that touches a shared source
	// tree — this both triggers the rebuild and makes local HEAD diverge from
	// origin/main (the un-pushed-merge scenario behind the incident).
	mergeBranchChanging(t, local, map[string]string{"pkg/llm/auth/auth.go": "package auth // local-only\n"})
	localHead := revParse(t, local, "HEAD")
	if localHead == trunk {
		t.Fatal("test setup: local HEAD did not diverge from origin/main")
	}

	recs := installRecords(t, runRebuildRecord(t, local))
	if len(recs) == 0 {
		t.Fatal("expected a rebuild, but the stub go install was never invoked")
	}
	for _, r := range recs {
		if r.commit != trunk {
			t.Errorf("built %s from %s; want origin/main %s (local HEAD was %s)",
				r.pkg, r.commit, trunk, localHead)
		}
	}
}

// Without an origin remote (local-only repos / most tests), there is no trunk
// ref to build from, so the hook falls back to building the working tree in
// place — building from local HEAD rather than skipping the rebuild entirely.
func TestRebuild_NoOrigin_FallsBackToWorkingTree(t *testing.T) {
	repo := newRebuildRepo(t)
	mergeBranchChanging(t, repo, map[string]string{"pkg/llm/auth/auth.go": "package auth // v2\n"})
	localHead := revParse(t, repo, "HEAD")

	recs := installRecords(t, runRebuildRecord(t, repo))
	if len(recs) == 0 {
		t.Fatal("expected a fallback rebuild, but the stub go install was never invoked")
	}
	for _, r := range recs {
		if r.commit != localHead {
			t.Errorf("fallback built %s from %s; want local HEAD %s", r.pkg, r.commit, localHead)
		}
	}
}
