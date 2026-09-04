package githooks_test

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/vbonnet/dear-agent/internal/gittest"
)

// lockedBuffer safely captures concurrently copied stdout and stderr from a
// child hook while the test polls its output for a timeout diagnostic.
type lockedBuffer struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (b *lockedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.b.Write(p)
}

func (b *lockedBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.b.String()
}

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

// git runs a git command in dir and fails the test on error. The sandbox
// supplies a deterministic identity and, critically, an empty hooks path: these
// repositories are merged into, and an inherited host hook would run for real.
func git(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := gittest.Command(t, dir, args...)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

// newRepo creates a temp git repo whose default branch is main, with one
// commit, and returns its path.
func newRepo(t *testing.T) string {
	t.Helper()
	return newRepoAt(t, t.TempDir())
}

// newRepoAt is newRepo at a caller-chosen path, for tests that need the repo
// to sit at a specific place relative to a managed root.
func newRepoAt(t *testing.T, dir string) string {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
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
	runHookWith(t, repoDir, agmDir,
		append([]string{"DEAR_AGENT_MANAGED_REPO_ROOTS=" + repoDir}, extraEnv...)...)
}

// runHookUnscoped runs the hook WITHOUT opting the temp repo into the managed
// roots, so the managed-repository gate is the thing under test.
func runHookUnscoped(t *testing.T, repoDir, agmDir string, extraEnv ...string) {
	t.Helper()
	runHookWith(t, repoDir, agmDir, extraEnv...)
}

func runHookWith(t *testing.T, repoDir, agmDir string, extraEnv ...string) {
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
	// gittest.Env is the base so the git commands the hook itself runs are
	// sanitized too; the test's own overrides come last and win.
	cmd.Env = append(gittest.Env(t),
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

func waitForFile(t *testing.T, path string, timeout time.Duration, message string, output *lockedBuffer) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for {
		if _, err := os.Stat(path); err == nil {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("%s:\n%s", message, output.String())
		}
		time.Sleep(10 * time.Millisecond)
	}
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

// The hook is installed globally (core.hooksPath), so it fires after a merge
// in EVERY repository — including the throwaway ones a test suite creates. On
// 2026-07-10 that ran a repository-wide `agm worktree sweep --execute` from
// two temporary repositories and deleted two live worktrees (F-01, ce-3knl.1).
// Outside the managed roots the hook must do nothing at all.
//
// Note this is the one hook test that does NOT set
// DEAR_AGENT_MANAGED_REPO_ROOTS: every other test opts its temp repo in, so
// without this case the gate itself would never be exercised.
func TestPostMerge_UnmanagedRepo_NoOp(t *testing.T) {
	repo := newRepo(t)
	sentinel := sentinelPath(t)
	agmDir := stubAGM(t, sentinel)
	runHookUnscoped(t, repo, agmDir)
	if swept(t, sentinel) {
		t.Fatal("sweep ran in a repository outside the managed roots")
	}
}

func TestPostMerge_UnsetHome_UnmanagedRepo_NoOp(t *testing.T) {
	repo := newRepo(t)
	sentinel := sentinelPath(t)
	agmDir := stubAGM(t, sentinel)
	runHookUnscoped(t, repo, agmDir, "HOME=")
	if swept(t, sentinel) {
		t.Fatal("sweep ran when HOME was unavailable to establish managed roots")
	}
}

func TestPostMerge_UnsetHome_ExplicitManagedRoot_NoOp(t *testing.T) {
	repo := newRepo(t)
	sentinel := sentinelPath(t)
	agmDir := stubAGM(t, sentinel)
	runHookUnscoped(t, repo, agmDir,
		"HOME=", "DEAR_AGENT_MANAGED_REPO_ROOTS="+repo)
	if swept(t, sentinel) {
		t.Fatal("sweep ran when HOME was unavailable despite an explicit managed root")
	}
}

// A repository nested under a managed root is managed. ~/worktrees/<repo>/<wt>
// is the normal shape, so the gate must match by path prefix, not equality.
func TestPostMerge_NestedUnderManagedRoot_Sweeps(t *testing.T) {
	root := t.TempDir()
	repo := newRepoAt(t, filepath.Join(root, "some-repo", "some-worktree"))
	sentinel := sentinelPath(t)
	agmDir := stubAGM(t, sentinel)
	runHook(t, repo, agmDir, "DEAR_AGENT_MANAGED_REPO_ROOTS="+root)
	if !swept(t, sentinel) {
		t.Fatal("a repository nested under a managed root must be managed")
	}
}

func TestPostMerge_FilesystemRootManaged_Sweeps(t *testing.T) {
	repo := newRepo(t)
	sentinel := sentinelPath(t)
	agmDir := stubAGM(t, sentinel)
	runHookUnscoped(t, repo, agmDir, "DEAR_AGENT_MANAGED_REPO_ROOTS=/")
	if !swept(t, sentinel) {
		t.Fatal("a repository beneath the filesystem root must be managed when / is configured")
	}
}

// A root that merely shares a textual prefix with the repo path is not a
// parent of it: ~/src must not manage a repository in ~/src-scratch.
func TestPostMerge_SiblingPrefixRoot_NoOp(t *testing.T) {
	base := t.TempDir()
	repo := newRepoAt(t, filepath.Join(base, "src-scratch"))
	sentinel := sentinelPath(t)
	agmDir := stubAGM(t, sentinel)
	runHookUnscoped(t, repo, agmDir,
		"DEAR_AGENT_MANAGED_REPO_ROOTS="+filepath.Join(base, "src"))
	if swept(t, sentinel) {
		t.Fatal("a sibling sharing a path prefix was treated as managed")
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

// stubGo writes a fake `go` that records each `go build -o <tmp> <pkg>`
// invocation into recordFile as "<pkg>|<HEAD-of-cwd>|<ldflags>" (one per line)
// and returns the dir holding it. Capturing the cwd's HEAD and the single value
// following -ldflags lets tests assert which commit and provenance profile the
// hook built. It also writes a fake binary to the `-o` path so the hook's atomic
// rename into GOBIN succeeds.
func stubGo(t *testing.T, recordFile string) string {
	t.Helper()
	dir := t.TempDir()
	script := "#!/bin/sh\n" +
		"if [ \"$1\" = build ]; then\n" +
		"  out=\"\"; pkg=\"\"; ldflags=\"\"; prev=\"\"\n" +
		"  for a in \"$@\"; do [ \"$prev\" = -o ] && out=\"$a\"; [ \"$prev\" = -ldflags ] && ldflags=\"$a\"; pkg=\"$a\"; prev=\"$a\"; done\n" +
		"  [ -n \"$STUB_GO_FAIL_PKG\" ] && [ \"$pkg\" = \"$STUB_GO_FAIL_PKG\" ] && exit 1\n" +
		"  if [ -n \"$STUB_GO_BLOCK_PKG\" ] && [ \"$pkg\" = \"$STUB_GO_BLOCK_PKG\" ]; then\n" +
		"    : > \"$STUB_GO_BLOCK_READY\"\n" +
		"    while [ ! -e \"$STUB_GO_BLOCK_RELEASE\" ]; do sleep 0.01; done\n" +
		"  fi\n" +
		"  [ -n \"$out\" ] && printf 'fakebin\\n' > \"$out\"\n" +
		"  printf '%s|%s|%s\\n' \"$pkg\" \"$(git rev-parse HEAD 2>/dev/null)\" \"$ldflags\" >> \"" + recordFile + "\"\n" +
		"fi\n" +
		"exit 0\n"
	if err := os.WriteFile(filepath.Join(dir, "go"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	return dir
}

// stubLockf models stock macOS lockf's file-and-command CLI. It records the
// invocation and executes the command after the lock-file argument, rejecting
// the invalid open-file-descriptor shape that prompted the regression.
func stubLockf(t *testing.T, recordFile string) string {
	t.Helper()
	dir := t.TempDir()
	script := "#!/bin/sh\n" +
		"printf '%s\\n' \"$*\" > \"" + recordFile + "\"\n" +
		"[ \"$1\" = -t ] || exit 64\n" +
		"shift 2\n" +
		"[ -n \"$1\" ] || exit 64\n" +
		"shift\n" +
		"[ $# -gt 0 ] || exit 64\n" +
		"exec \"$@\"\n"
	if err := os.WriteFile(filepath.Join(dir, "lockf"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	return dir
}

// installRecord is one captured `go build` invocation: the package, the HEAD
// commit of the build directory, and the single value following -ldflags.
type installRecord struct {
	pkg     string
	commit  string
	ldflags string
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
		pkg, rest, _ := strings.Cut(l, "|")
		commit, ldflags, _ := strings.Cut(rest, "|")
		out = append(out, installRecord{pkg: pkg, commit: commit, ldflags: ldflags})
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
	cmd := gittest.Command(t, dir, "rev-parse", ref)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git rev-parse %s in %s: %v\n%s", ref, dir, err, out)
	}
	return strings.TrimSpace(string(out))
}

// newRebuildRepo builds a repo that looks like dear-agent: it has the managed
// binary package dirs and a baseline commit on main. It returns the repo path.
func newRebuildRepo(t *testing.T) string {
	t.Helper()
	repo := newRepo(t)
	for _, p := range []string{"agm/cmd/agm", "agm/cmd/agm-reaper", "agm/internal/tmux", "cmd/spec-contract-hook", "cmd/vroom-dispatch", "wayfinder/cmd/wayfinder", "pkg/llm/auth", "internal/x", "docs"} {
		if err := os.MkdirAll(filepath.Join(repo, p), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	// Seed each package dir with a tracked file so later diffs are meaningful.
	for _, f := range []string{"agm/cmd/agm/main.go", "agm/cmd/agm-reaper/main.go", "agm/internal/tmux/prompt.go", "cmd/spec-contract-hook/main.go", "cmd/vroom-dispatch/main.go", "wayfinder/cmd/wayfinder/main.go", "pkg/llm/auth/auth.go", "internal/x/x.go", "docs/readme.md"} {
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
	cmd.Env = append(gittest.Env(t),
		"HOME="+home,
		"PATH="+goDir+string(os.PathListSeparator)+os.Getenv("PATH"),
		"DEAR_AGENT_MANAGED_REPO_ROOTS="+repo, // scope the hook to this temp repo
		"AGM_POST_MERGE_SWEEP=0",              // isolate Stage 1: no sweep
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

// Re-installing must produce a NEW inode at the target — i.e. the hook builds
// to a temp file and rename()s it, never overwriting the target inode in place.
// An in-place overwrite is exactly what SIGKILLs a running binary with "Code
// Signature Invalid" (ce-w77v, the 2026-07-06 crash loop).
func TestRebuild_AtomicInstall_NewInode(t *testing.T) {
	if _, err := exec.LookPath("bash"); err != nil {
		t.Skip("bash not available")
	}
	repo := newRebuildRepo(t)
	mergeBranchChanging(t, repo, map[string]string{"agm/cmd/agm/main.go": "package main // v2\n"})
	gobin := t.TempDir()
	home := t.TempDir()

	install := func() uint64 {
		record := filepath.Join(t.TempDir(), "installed")
		goDir := stubGo(t, record)
		cmd := exec.Command("bash", hookPath(t))
		cmd.Dir = repo
		cmd.Env = append(gittest.Env(t),
			"HOME="+home, "GOBIN="+gobin,
			"PATH="+goDir+string(os.PathListSeparator)+os.Getenv("PATH"),
			"DEAR_AGENT_MANAGED_REPO_ROOTS="+repo,
			"AGM_POST_MERGE_SWEEP=0",
		)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("hook must exit 0: %v\n%s", err, out)
		}
		fi, err := os.Stat(filepath.Join(gobin, "agm"))
		if err != nil {
			t.Fatalf("agm was not atomically installed into GOBIN: %v", err)
		}
		st, ok := fi.Sys().(*syscall.Stat_t)
		if !ok {
			t.Skip("no inode info on this platform")
		}
		return uint64(st.Ino)
	}

	ino1 := install()
	ino2 := install()
	if ino1 == ino2 {
		t.Errorf("re-install reused inode %d — in-place overwrite, not atomic rename (would SIGKILL a running binary, ce-w77v)", ino1)
	}
}

// A change under a root shared source tree (pkg/) rebuilds every consumer.
func TestRebuild_SharedSource_RebuildsAllConsumers(t *testing.T) {
	repo := newRebuildRepo(t)
	mergeBranchChanging(t, repo, map[string]string{"pkg/llm/auth/auth.go": "package auth // v2\n"})
	got := runRebuild(t, repo)
	if !slices.Contains(got, "./agm/cmd/agm") || !slices.Contains(got, "./agm/cmd/agm-reaper") || !slices.Contains(got, "./cmd/vroom-dispatch") || !slices.Contains(got, "./wayfinder/cmd/wayfinder") {
		t.Fatalf("expected all consumers rebuilt on a pkg/ change, got %v", got)
	}
}

// The async archive companion consumes agm/internal just like the CLI. Both
// binaries must be built from one source revision so a merged prompt/lifecycle
// change cannot leave the detached reaper running stale code.
func TestRebuild_AGMSharedLifecycle_RebuildsCLIAndReaperFromSameRevision(t *testing.T) {
	repo := newRebuildRepo(t)
	mergeBranchChanging(t, repo, map[string]string{"agm/internal/tmux/prompt.go": "package tmux // v2\n"})
	wantCommit := revParse(t, repo, "HEAD")

	recs := installRecords(t, runRebuildRecord(t, repo))
	commits := make(map[string]string)
	for _, rec := range recs {
		commits[rec.pkg] = rec.commit
	}
	for _, pkg := range []string{"./agm/cmd/agm", "./agm/cmd/agm-reaper"} {
		if commits[pkg] != wantCommit {
			t.Errorf("%s built from %q; want shared source revision %s (records: %+v)", pkg, commits[pkg], wantCommit, recs)
		}
	}
	if _, ok := commits["./cmd/vroom-dispatch"]; ok {
		t.Fatal("AGM-only shared source unexpectedly rebuilt vroom-dispatch")
	}
}

// A partial pair build must not replace either installed binary. In
// particular, a reaper build failure cannot leave a newer CLI beside the old
// detached lifecycle implementation.
func TestRebuild_AGMPairBuildFailurePreservesInstalledPair(t *testing.T) {
	repo := newRebuildRepo(t)
	mergeBranchChanging(t, repo, map[string]string{"agm/internal/tmux/prompt.go": "package tmux // v2\n"})
	gobin := t.TempDir()
	for name, content := range map[string]string{"agm": "old-agm\n", "agm-reaper": "old-reaper\n"} {
		if err := os.WriteFile(filepath.Join(gobin, name), []byte(content), 0o755); err != nil {
			t.Fatal(err)
		}
	}

	record := filepath.Join(t.TempDir(), "installed")
	goDir := stubGo(t, record)
	cmd := exec.Command("bash", hookPath(t))
	cmd.Dir = repo
	cmd.Env = append(os.Environ(),
		"HOME="+t.TempDir(),
		"GOBIN="+gobin,
		"PATH="+goDir+string(os.PathListSeparator)+os.Getenv("PATH"),
		"DEAR_AGENT_MANAGED_REPO_ROOTS="+repo,
		"AGM_POST_MERGE_SWEEP=0",
		"STUB_GO_FAIL_PKG=./agm/cmd/agm-reaper",
	)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("hook must fail safe, got %v\n%s", err, out)
	}

	for name, want := range map[string]string{"agm": "old-agm\n", "agm-reaper": "old-reaper\n"} {
		got, err := os.ReadFile(filepath.Join(gobin, name))
		if err != nil {
			t.Fatal(err)
		}
		if string(got) != want {
			t.Fatalf("%s changed after pair build failure: got %q, want %q", name, got, want)
		}
	}
}

func TestRebuild_AGMPairActivationIsSerializedAcrossHookProcesses(t *testing.T) {
	repo := newRebuildRepo(t)
	mergeBranchChanging(t, repo, map[string]string{"agm/internal/tmux/prompt.go": "package tmux // v2\n"})
	gobin := t.TempDir()
	record := filepath.Join(t.TempDir(), "installed")
	goDir := stubGo(t, record)
	ready := filepath.Join(t.TempDir(), "first-build-ready")
	release := filepath.Join(t.TempDir(), "release-first-build")
	baseEnv := append(os.Environ(),
		"HOME="+t.TempDir(),
		"GOBIN="+gobin,
		"PATH="+goDir+string(os.PathListSeparator)+os.Getenv("PATH"),
		"DEAR_AGENT_MANAGED_REPO_ROOTS="+repo,
		"AGM_POST_MERGE_SWEEP=0",
		"STUB_GO_BLOCK_PKG=./agm/cmd/agm",
		"STUB_GO_BLOCK_READY="+ready,
		"STUB_GO_BLOCK_RELEASE="+release,
	)

	var firstOutput lockedBuffer
	first := exec.Command("bash", hookPath(t))
	first.Dir = repo
	first.Env = baseEnv
	first.Stdout = &firstOutput
	first.Stderr = &firstOutput
	if err := first.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if first.Process != nil {
			_ = first.Process.Kill()
		}
	})

	deadline := time.Now().Add(20 * time.Second)
	for {
		if _, err := os.Stat(ready); err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("first hook did not reach staged build:\n%s", firstOutput.String())
		}
		time.Sleep(10 * time.Millisecond)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	var secondOutput lockedBuffer
	second := exec.CommandContext(ctx, "bash", hookPath(t))
	second.Dir = repo
	second.Env = baseEnv
	second.Stdout = &secondOutput
	second.Stderr = &secondOutput
	if err := second.Start(); err != nil {
		t.Fatal(err)
	}
	secondDone := make(chan error, 1)
	go func() { secondDone <- second.Wait() }()
	select {
	case err := <-secondDone:
		t.Fatalf("contending hook did not wait for lock owner: %v\n%s", err, secondOutput.String())
	case <-time.After(200 * time.Millisecond):
	}

	if err := os.WriteFile(release, []byte("release\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := first.Wait(); err != nil {
		t.Fatalf("first hook failed: %v\n%s", err, firstOutput.String())
	}
	select {
	case err := <-secondDone:
		if err != nil {
			t.Fatalf("contending hook failed after lock release: %v\n%s", err, secondOutput.String())
		}
	case <-ctx.Done():
		t.Fatalf("contending hook did not reacquire deployment lock: %v\n%s", ctx.Err(), secondOutput.String())
	}
	requireCoherentAGMDeployments(t, installRecords(t, record), revParse(t, repo, "HEAD"), revParse(t, repo, "HEAD"))
}

func TestRebuild_AGMPairRefreshesTrunkAfterWaitingForLock(t *testing.T) {
	origin := newRebuildRepo(t)
	firstCheckout := cloneRepo(t, origin)
	secondCheckout := cloneRepo(t, origin)

	sharedSource := filepath.Join(origin, "agm", "internal", "tmux", "prompt.go")
	if err := os.WriteFile(sharedSource, []byte("package tmux // revision x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	git(t, origin, "add", "agm/internal/tmux/prompt.go")
	git(t, origin, "commit", "-qm", "revision x")
	firstRevision := revParse(t, origin, "HEAD")
	git(t, firstCheckout, "pull", "-q", "--ff-only")

	gobin := t.TempDir()
	record := filepath.Join(t.TempDir(), "installed")
	goDir := stubGo(t, record)
	ready := filepath.Join(t.TempDir(), "first-build-ready")
	release := filepath.Join(t.TempDir(), "release-first-build")
	baseEnv := append(os.Environ(),
		"HOME="+t.TempDir(),
		"GOBIN="+gobin,
		"PATH="+goDir+string(os.PathListSeparator)+os.Getenv("PATH"),
		"DEAR_AGENT_MANAGED_REPO_ROOTS="+firstCheckout+string(os.PathListSeparator)+secondCheckout,
		"AGM_POST_MERGE_SWEEP=0",
		"STUB_GO_BLOCK_PKG=./agm/cmd/agm",
		"STUB_GO_BLOCK_READY="+ready,
		"STUB_GO_BLOCK_RELEASE="+release,
	)

	var firstOutput lockedBuffer
	first := exec.Command("bash", hookPath(t))
	first.Dir = firstCheckout
	first.Env = baseEnv
	first.Stdout = &firstOutput
	first.Stderr = &firstOutput
	if err := first.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if first.Process != nil {
			_ = first.Process.Kill()
		}
	})
	waitForFile(t, ready, 20*time.Second, "first hook did not reach staged build", &firstOutput)

	if err := os.WriteFile(sharedSource, []byte("package tmux // revision y\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	git(t, origin, "add", "agm/internal/tmux/prompt.go")
	git(t, origin, "commit", "-qm", "revision y")
	secondRevision := revParse(t, origin, "HEAD")
	git(t, secondCheckout, "pull", "-q", "--ff-only")

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	var secondOutput lockedBuffer
	second := exec.CommandContext(ctx, "bash", hookPath(t))
	second.Dir = secondCheckout
	second.Env = baseEnv
	second.Stdout = &secondOutput
	second.Stderr = &secondOutput
	if err := second.Start(); err != nil {
		t.Fatal(err)
	}
	secondDone := make(chan error, 1)
	go func() { secondDone <- second.Wait() }()
	select {
	case err := <-secondDone:
		t.Fatalf("newer hook did not wait for the older deployment: %v\n%s", err, secondOutput.String())
	case <-time.After(200 * time.Millisecond):
	}

	if err := os.WriteFile(release, []byte("release\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := first.Wait(); err != nil {
		t.Fatalf("first hook failed: %v\n%s", err, firstOutput.String())
	}
	select {
	case err := <-secondDone:
		if err != nil {
			t.Fatalf("newer hook failed after lock release: %v\n%s", err, secondOutput.String())
		}
	case <-ctx.Done():
		t.Fatalf("newer hook did not deploy after lock release: %v\n%s", ctx.Err(), secondOutput.String())
	}

	requireCoherentAGMDeployments(t, installRecords(t, record), firstRevision, secondRevision)
}

func TestRebuild_AGMPairRecoversLegacyLockDirectories(t *testing.T) {
	tests := []struct {
		name       string
		pidContent string
	}{
		{name: "ownerless"},
		{name: "malformed owner", pidContent: "not-a-pid\n"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			repo := newRebuildRepo(t)
			mergeBranchChanging(t, repo, map[string]string{"agm/internal/tmux/prompt.go": "package tmux // v2\n"})
			wantRevision := revParse(t, repo, "HEAD")
			gobin := t.TempDir()
			lockDir := filepath.Join(gobin, ".agm-pair-install.lock")
			if err := os.Mkdir(lockDir, 0o700); err != nil {
				t.Fatal(err)
			}
			if tc.pidContent != "" {
				if err := os.WriteFile(filepath.Join(lockDir, "pid"), []byte(tc.pidContent), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			record := filepath.Join(t.TempDir(), "installed")
			goDir := stubGo(t, record)
			cmd := exec.Command("bash", hookPath(t))
			cmd.Dir = repo
			cmd.Env = append(gittest.Env(t),
				"HOME="+t.TempDir(),
				"GOBIN="+gobin,
				"PATH="+goDir+string(os.PathListSeparator)+os.Getenv("PATH"),
				"DEAR_AGENT_MANAGED_REPO_ROOTS="+repo,
				"AGM_POST_MERGE_SWEEP=0",
			)
			if output, err := cmd.CombinedOutput(); err != nil {
				t.Fatalf("hook failed with %s legacy lock: %v\n%s", tc.name, err, output)
			}
			requireCoherentAGMDeployments(t, installRecords(t, record), wantRevision)
		})
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
	if slices.Contains(got, "./wayfinder/cmd/wayfinder") {
		t.Fatalf("wayfinder rebuilt for a vroom-dispatch-only change: %v", got)
	}
}

// A Wayfinder runtime change rebuilds Wayfinder and no unrelated executable.
// This is the exact boundary missed when PR #1024 changed validator Go files
// but the installed binary remained at the prior revision.
func TestRebuild_WayfinderOnly(t *testing.T) {
	repo := newRebuildRepo(t)
	mergeBranchChanging(t, repo, map[string]string{"wayfinder/cmd/wayfinder/main.go": "package main // v2\n"})
	got := runRebuild(t, repo)
	if !slices.Contains(got, "./wayfinder/cmd/wayfinder") {
		t.Fatalf("wayfinder not rebuilt for its own runtime change: %v", got)
	}
	for _, unrelated := range []string{"./agm/cmd/agm", "./agm/cmd/agm-reaper", "./cmd/vroom-dispatch"} {
		if slices.Contains(got, unrelated) {
			t.Fatalf("%s rebuilt for a wayfinder-only change: %v", unrelated, got)
		}
	}
}

func TestRebuild_WayfinderUsesMacOSLockfFileAndCommandForm(t *testing.T) {
	repo := newRebuildRepo(t)
	mergeBranchChanging(t, repo, map[string]string{"wayfinder/cmd/wayfinder/main.go": "package main // v2\n"})
	gobin := t.TempDir()
	installRecord := filepath.Join(t.TempDir(), "installed")
	lockfRecord := filepath.Join(t.TempDir(), "lockf-args")
	goDir := stubGo(t, installRecord)
	lockfDir := stubLockf(t, lockfRecord)

	cmd := exec.Command("bash", hookPath(t))
	cmd.Dir = repo
	cmd.Env = append(gittest.Env(t),
		"HOME="+t.TempDir(),
		"GOBIN="+gobin,
		"PATH="+goDir+string(os.PathListSeparator)+lockfDir+string(os.PathListSeparator)+os.Getenv("PATH"),
		"DEAR_AGENT_MANAGED_REPO_ROOTS="+repo,
		"AGM_POST_MERGE_SWEEP=0",
		"DEAR_AGENT_INSTALL_LOCK_TOOL=lockf",
	)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("Wayfinder lockf-form rebuild failed: %v\n%s", err, output)
	}
	if got := installed(t, installRecord); !slices.Contains(got, "./wayfinder/cmd/wayfinder") {
		t.Fatalf("Wayfinder was not installed through lockf command form: %v", got)
	}
	args, err := os.ReadFile(lockfRecord)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"-t 300 " + filepath.Join(gobin, ".wayfinder-install.lock"),
		"env DEAR_AGENT_POST_MERGE_INTERNAL_MODE=wayfinder DEAR_AGENT_POST_MERGE_REBUILD=1 bash",
		hookPath(t),
	} {
		if !strings.Contains(string(args), want) {
			t.Fatalf("lockf invocation missing %q: %s", want, args)
		}
	}
}

// Two default-branch hooks can overlap from distinct checkouts that install to
// one GOBIN. The newer contender must wait, then refresh origin/main while it
// owns the lock so an older, slower build cannot become the final activation.
func TestRebuild_WayfinderRefreshesTrunkAfterWaitingForLock(t *testing.T) {
	origin := newRebuildRepo(t)
	firstCheckout := cloneRepo(t, origin)
	secondCheckout := cloneRepo(t, origin)

	sharedSource := filepath.Join(origin, "wayfinder", "cmd", "wayfinder", "main.go")
	if err := os.WriteFile(sharedSource, []byte("package main // revision x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	git(t, origin, "add", "wayfinder/cmd/wayfinder/main.go")
	git(t, origin, "commit", "-qm", "wayfinder revision x")
	firstRevision := revParse(t, origin, "HEAD")
	git(t, firstCheckout, "pull", "-q", "--ff-only")

	gobin := t.TempDir()
	record := filepath.Join(t.TempDir(), "installed")
	goDir := stubGo(t, record)
	ready := filepath.Join(t.TempDir(), "first-build-ready")
	release := filepath.Join(t.TempDir(), "release-first-build")
	baseEnv := append(os.Environ(),
		"HOME="+t.TempDir(),
		"GOBIN="+gobin,
		"PATH="+goDir+string(os.PathListSeparator)+os.Getenv("PATH"),
		"DEAR_AGENT_MANAGED_REPO_ROOTS="+firstCheckout+string(os.PathListSeparator)+secondCheckout,
		"AGM_POST_MERGE_SWEEP=0",
		"STUB_GO_BLOCK_PKG=./wayfinder/cmd/wayfinder",
		"STUB_GO_BLOCK_READY="+ready,
		"STUB_GO_BLOCK_RELEASE="+release,
	)

	var firstOutput lockedBuffer
	first := exec.Command("bash", hookPath(t))
	first.Dir = firstCheckout
	first.Env = baseEnv
	first.Stdout = &firstOutput
	first.Stderr = &firstOutput
	if err := first.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if first.Process != nil {
			_ = first.Process.Kill()
		}
	})
	waitForFile(t, ready, 20*time.Second, "first Wayfinder hook did not reach staged build", &firstOutput)

	if err := os.WriteFile(sharedSource, []byte("package main // revision y\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	git(t, origin, "add", "wayfinder/cmd/wayfinder/main.go")
	git(t, origin, "commit", "-qm", "wayfinder revision y")
	secondRevision := revParse(t, origin, "HEAD")
	git(t, secondCheckout, "pull", "-q", "--ff-only")

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	var secondOutput lockedBuffer
	second := exec.CommandContext(ctx, "bash", hookPath(t))
	second.Dir = secondCheckout
	second.Env = baseEnv
	second.Stdout = &secondOutput
	second.Stderr = &secondOutput
	if err := second.Start(); err != nil {
		t.Fatal(err)
	}
	secondDone := make(chan error, 1)
	go func() { secondDone <- second.Wait() }()
	select {
	case err := <-secondDone:
		t.Fatalf("newer Wayfinder hook did not wait for the older deployment: %v\n%s", err, secondOutput.String())
	case <-time.After(200 * time.Millisecond):
	}

	if err := os.WriteFile(release, []byte("release\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := first.Wait(); err != nil {
		t.Fatalf("first Wayfinder hook failed: %v\n%s", err, firstOutput.String())
	}
	select {
	case err := <-secondDone:
		if err != nil {
			t.Fatalf("newer Wayfinder hook failed after lock release: %v\n%s", err, secondOutput.String())
		}
	case <-ctx.Done():
		t.Fatalf("newer Wayfinder hook did not deploy after lock release: %v\n%s", ctx.Err(), secondOutput.String())
	}

	records := installRecords(t, record)
	if len(records) != 2 {
		t.Fatalf("serialized Wayfinder hooks staged %d builds, want two: %v", len(records), records)
	}
	for i, want := range []string{firstRevision, secondRevision} {
		if records[i].commit != want {
			t.Fatalf("Wayfinder build %d used revision %s, want %s (records: %+v)", i, records[i].commit, want, records)
		}
	}
}

// Any compiled AGM change rebuilds the coherent CLI/reaper pair. Even when a
// source change is CLI-local, exact revision handshaking means the detached
// companion must move to the same commit.
func TestRebuild_AgmOnly(t *testing.T) {
	repo := newRebuildRepo(t)
	mergeBranchChanging(t, repo, map[string]string{"agm/cmd/agm/main.go": "package main // v2\n"})
	got := runRebuild(t, repo)
	if slices.Contains(got, "./cmd/vroom-dispatch") {
		t.Fatalf("vroom-dispatch rebuilt for an agm-only change: %v", got)
	}
	if slices.Contains(got, "./wayfinder/cmd/wayfinder") {
		t.Fatalf("wayfinder rebuilt for an agm-only change: %v", got)
	}
	if !slices.Contains(got, "./agm/cmd/agm") {
		t.Fatalf("agm not rebuilt for its own change: %v", got)
	}
	if !slices.Contains(got, "./agm/cmd/agm-reaper") {
		t.Fatalf("agm-reaper not rebuilt with coherent AGM pair: %v", got)
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

// ── Stage 1.6 (host-artifact deploy via dear-deploy sync) ───────────────────

// stubMake writes a fake `make` that records each invocation's target(s) into
// recordFile (one line per run, space-joined args) and returns the dir holding
// it. It lets a test assert the hook drove the deploy through `make
// dear-deploy-sync` without running a real build.
func stubMake(t *testing.T, recordFile string) string {
	t.Helper()
	dir := t.TempDir()
	script := "#!/bin/sh\n" +
		"echo \"$*\" >> \"" + recordFile + "\"\n" +
		"exit 0\n"
	if err := os.WriteFile(filepath.Join(dir, "make"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	return dir
}

// newDeployRepo builds a repo that looks like dear-agent for the deploy stage:
// it carries deploy/manifest.yaml on main. It deliberately omits the binary
// package dirs so Stage 1 is an inert no-op and the test isolates Stage 1.6.
func newDeployRepo(t *testing.T) string {
	t.Helper()
	repo := newRepo(t)
	if err := os.MkdirAll(filepath.Join(repo, "deploy"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repo, "deploy", "manifest.yaml"), []byte("artifacts: []\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	git(t, repo, "add", "-A")
	git(t, repo, "commit", "-qm", "add deploy manifest")
	return repo
}

// runDeployRecord runs the hook with a stub `make` (and no agm), isolating Stage
// 1.6: Stage 1 (rebuild) and Stage 2 (sweep) are disabled via their opt-outs. It
// returns the path to the record file capturing every `make` invocation.
func runDeployRecord(t *testing.T, repo string, extraEnv ...string) string {
	t.Helper()
	if _, err := exec.LookPath("bash"); err != nil {
		t.Skip("bash not available")
	}
	home := t.TempDir()
	record := filepath.Join(t.TempDir(), "made")
	makeDir := stubMake(t, record)
	cmd := exec.Command("bash", hookPath(t))
	cmd.Dir = repo
	cmd.Env = append(gittest.Env(t),
		"HOME="+home,
		"PATH="+makeDir+string(os.PathListSeparator)+os.Getenv("PATH"),
		"DEAR_AGENT_MANAGED_REPO_ROOTS="+repo, // scope the hook to this temp repo
		"DEAR_AGENT_POST_MERGE_REBUILD=0",     // isolate Stage 1.6 from Stage 1
		"AGM_POST_MERGE_SWEEP=0",              // isolate Stage 1.6 from Stage 2
	)
	cmd.Env = append(cmd.Env, extraEnv...)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("hook must exit 0, got %v\n%s", err, out)
	}
	return record
}

// madeTargets parses the stub make's record into the list of target strings it
// was invoked with.
func madeTargets(t *testing.T, recordFile string) []string {
	t.Helper()
	b, err := os.ReadFile(recordFile)
	if err != nil {
		return nil // no file => make never invoked
	}
	var out []string
	for l := range strings.SplitSeq(strings.TrimSpace(string(b)), "\n") {
		if l != "" {
			out = append(out, l)
		}
	}
	return out
}

// On the default branch, a repo carrying deploy/manifest.yaml deploys host
// artifacts by shelling out to `make dear-deploy-sync`.
func TestDeploy_DefaultBranch_RunsSync(t *testing.T) {
	repo := newDeployRepo(t)
	got := madeTargets(t, runDeployRecord(t, repo))
	if !slices.Contains(got, "dear-deploy-sync") {
		t.Fatalf("expected `make dear-deploy-sync` on a default-branch merge, got %v", got)
	}
}

// On a feature branch, Stage 1.6 must not deploy (mirrors the rebuild/sweep gate).
func TestDeploy_FeatureBranch_Skips(t *testing.T) {
	repo := newDeployRepo(t)
	git(t, repo, "checkout", "-q", "-b", "feature")
	got := madeTargets(t, runDeployRecord(t, repo))
	if len(got) != 0 {
		t.Fatalf("deploy ran on a feature branch: %v", got)
	}
}

// DEAR_AGENT_POST_MERGE_DEPLOY=0 disables Stage 1.6 even on the default branch.
func TestDeploy_OptOut_Skips(t *testing.T) {
	repo := newDeployRepo(t)
	got := madeTargets(t, runDeployRecord(t, repo, "DEAR_AGENT_POST_MERGE_DEPLOY=0"))
	if len(got) != 0 {
		t.Fatalf("deploy ran despite DEAR_AGENT_POST_MERGE_DEPLOY=0: %v", got)
	}
}

// A repo without deploy/manifest.yaml is a silent no-op (global-hook safety):
// the stub `make` must never be invoked.
func TestDeploy_NoManifest_NoOp(t *testing.T) {
	repo := newRepo(t) // no deploy/manifest.yaml
	got := madeTargets(t, runDeployRecord(t, repo))
	if len(got) != 0 {
		t.Fatalf("deploy ran in a repo without a deploy manifest: %v", got)
	}
}

// ── Stage 3 (bead transition) ───────────────────────────────────────────────

// stubBd writes a fake `bd` modelling a tiny Beads store under the --db dir:
// each bead is a file "<db>/<id>" whose contents are its status. `show --json`
// emits {"id","status"} (exit 1 when the bead file is absent, i.e. not in this
// store); `close` records the id into closeLog and flips the bead to "closed".
func stubBd(t *testing.T, closeLog string) string {
	t.Helper()
	dir := t.TempDir()
	script := "#!/bin/sh\n" +
		"db=\"\"; cmd=\"\"; id=\"\"\n" +
		"while [ $# -gt 0 ]; do\n" +
		"  case \"$1\" in\n" +
		"    --db) db=\"$2\"; shift 2 ;;\n" +
		"    --json) shift ;;\n" +
		"    --reason) shift 2 ;;\n" +
		"    -*) shift ;;\n" +
		"    *) if [ -z \"$cmd\" ]; then cmd=\"$1\"; elif [ -z \"$id\" ]; then id=\"$1\"; fi; shift ;;\n" +
		"  esac\n" +
		"done\n" +
		"case \"$cmd\" in\n" +
		"  show) [ -f \"$db/$id\" ] || exit 1; printf '[{\"id\":\"%s\",\"status\":\"%s\"}]\\n' \"$id\" \"$(cat \"$db/$id\")\" ;;\n" +
		"  close) echo \"$id\" >> \"" + closeLog + "\"; echo closed > \"$db/$id\" ;;\n" +
		"esac\n" +
		"exit 0\n"
	if err := os.WriteFile(filepath.Join(dir, "bd"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	return dir
}

// seedBead creates a bead with the given status in the stub store at db.
func seedBead(t *testing.T, db, id, status string) {
	t.Helper()
	if err := os.MkdirAll(db, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(db, id), []byte(status), 0o644); err != nil {
		t.Fatal(err)
	}
}

// mergeWithMessage commits a change on a feature branch using featMsg, then
// merges it back into main with --no-ff so ORIG_HEAD/HEAD reflect a real merge
// and the merged range carries featMsg (where the Closes line lives).
func mergeWithMessage(t *testing.T, repo, featMsg string) {
	t.Helper()
	git(t, repo, "checkout", "-q", "-b", "feat-bead")
	if err := os.WriteFile(filepath.Join(repo, "merged.txt"), []byte("change\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	git(t, repo, "add", "-A")
	git(t, repo, "commit", "-qm", featMsg)
	git(t, repo, "checkout", "-q", "main")
	git(t, repo, "merge", "-q", "--no-ff", "-m", "Merge feat-bead", "feat-bead")
}

// runBeadTransition runs the hook with a stub `bd` and the other stages off,
// pointing Stage 3 at beadsDB. It returns the bead ids the stub was asked to
// close. The hook must always exit 0.
func runBeadTransition(t *testing.T, repo, beadsDB string, extraEnv ...string) []string {
	t.Helper()
	if _, err := exec.LookPath("bash"); err != nil {
		t.Skip("bash not available")
	}
	home := t.TempDir()
	closeLog := filepath.Join(t.TempDir(), "closed")
	bdDir := stubBd(t, closeLog)
	cmd := exec.Command("bash", hookPath(t))
	cmd.Dir = repo
	cmd.Env = append(gittest.Env(t),
		"HOME="+home,
		"PATH="+bdDir+string(os.PathListSeparator)+os.Getenv("PATH"),
		"DEAR_AGENT_MANAGED_REPO_ROOTS="+repo, // scope the hook to this temp repo
		"DEAR_AGENT_POST_MERGE_REBUILD=0",
		"DEAR_AGENT_POST_MERGE_VERIFY=0",
		"AGM_POST_MERGE_SWEEP=0",
		"DEAR_AGENT_BEADS_DB="+beadsDB,
	)
	cmd.Env = append(cmd.Env, extraEnv...)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("hook must exit 0, got %v\n%s", err, out)
	}
	return installed(t, closeLog)
}

// An explicit "Closes ce-xxx" in the merged range closes that bead.
func TestBeadTransition_ClosesOnClosesKeyword(t *testing.T) {
	repo := newRebuildRepo(t)
	db := t.TempDir()
	seedBead(t, db, "ce-aaaa1", "open")
	mergeWithMessage(t, repo, "feat: thing (ce-aaaa1)\n\nCloses ce-aaaa1")
	got := runBeadTransition(t, repo, db)
	if !slices.Contains(got, "ce-aaaa1") {
		t.Fatalf("expected ce-aaaa1 closed, got %v", got)
	}
}

// Fixes/Resolves, case-insensitivity, and sub-part ids (ce-6as.80) all work.
func TestBeadTransition_KeywordVariants(t *testing.T) {
	cases := []struct{ name, msg, id string }{
		{"Fixes", "fix: x\n\nFixes ce-bbbb1", "ce-bbbb1"},
		{"Resolves", "feat: x\n\nResolves ce-cccc1", "ce-cccc1"},
		{"lowercase", "feat: x\n\ncloses ce-dddd1", "ce-dddd1"},
		{"subpart", "feat: x\n\nCloses ce-6as.80", "ce-6as.80"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			repo := newRebuildRepo(t)
			db := t.TempDir()
			seedBead(t, db, tc.id, "open")
			mergeWithMessage(t, repo, tc.msg)
			got := runBeadTransition(t, repo, db)
			if !slices.Contains(got, tc.id) {
				t.Fatalf("expected %s closed, got %v", tc.id, got)
			}
		})
	}
}

// A bare "(ce-xxx)" subject mention WITHOUT a closing keyword must NOT close.
func TestBeadTransition_BareMention_NotClosed(t *testing.T) {
	repo := newRebuildRepo(t)
	db := t.TempDir()
	seedBead(t, db, "ce-eeee1", "open")
	mergeWithMessage(t, repo, "feat: thing (ce-eeee1)")
	got := runBeadTransition(t, repo, db)
	if len(got) != 0 {
		t.Fatalf("bare mention must not close, got %v", got)
	}
}

// A longer word that merely ENDS in a closing keyword ("prefixes ce-x",
// "discloses ce-x") must not false-close — the keyword needs a word boundary.
func TestBeadTransition_WordBoundary_NoFalseMatch(t *testing.T) {
	for _, msg := range []string{
		"refactor: tidy prefixes ce-kkkk1",
		"docs: it discloses ce-kkkk1 internals",
	} {
		t.Run(msg, func(t *testing.T) {
			repo := newRebuildRepo(t)
			db := t.TempDir()
			seedBead(t, db, "ce-kkkk1", "open")
			mergeWithMessage(t, repo, msg)
			got := runBeadTransition(t, repo, db)
			if len(got) != 0 {
				t.Fatalf("keyword-suffixed word must not close, got %v", got)
			}
		})
	}
}

// An already-closed bead is skipped (idempotent — no re-close).
func TestBeadTransition_AlreadyClosed_Skipped(t *testing.T) {
	repo := newRebuildRepo(t)
	db := t.TempDir()
	seedBead(t, db, "ce-ffff1", "closed")
	mergeWithMessage(t, repo, "feat: x\n\nCloses ce-ffff1")
	got := runBeadTransition(t, repo, db)
	if len(got) != 0 {
		t.Fatalf("already-closed bead must be skipped, got %v", got)
	}
}

// A bead absent from this store (e.g. a cross-store id) is a no-op, not error.
func TestBeadTransition_MissingBead_NoOp(t *testing.T) {
	repo := newRebuildRepo(t)
	db := t.TempDir()
	mergeWithMessage(t, repo, "feat: x\n\nCloses ce-9999z")
	got := runBeadTransition(t, repo, db)
	if len(got) != 0 {
		t.Fatalf("absent bead must be a no-op, got %v", got)
	}
}

// On a feature branch the whole hook is gated off — no bead transitions.
func TestBeadTransition_FeatureBranch_Skips(t *testing.T) {
	repo := newRebuildRepo(t)
	db := t.TempDir()
	seedBead(t, db, "ce-gggg1", "open")
	mergeWithMessage(t, repo, "feat: x\n\nCloses ce-gggg1")
	git(t, repo, "checkout", "-q", "-b", "feature")
	got := runBeadTransition(t, repo, db)
	if len(got) != 0 {
		t.Fatalf("must not transition beads on a feature branch, got %v", got)
	}
}

// DEAR_AGENT_POST_MERGE_BEAD_TRANSITION=0 disables Stage 3.
func TestBeadTransition_OptOut(t *testing.T) {
	repo := newRebuildRepo(t)
	db := t.TempDir()
	seedBead(t, db, "ce-hhhh1", "open")
	mergeWithMessage(t, repo, "feat: x\n\nCloses ce-hhhh1")
	got := runBeadTransition(t, repo, db, "DEAR_AGENT_POST_MERGE_BEAD_TRANSITION=0")
	if len(got) != 0 {
		t.Fatalf("opt-out must skip Stage 3, got %v", got)
	}
}

// A repo without agm/cmd/agm (not dear-agent) is a silent no-op.
func TestBeadTransition_ForeignRepo_NoOp(t *testing.T) {
	repo := newRepo(t)
	db := t.TempDir()
	seedBead(t, db, "ce-iiii1", "open")
	mergeWithMessage(t, repo, "feat: x\n\nCloses ce-iiii1")
	got := runBeadTransition(t, repo, db)
	if len(got) != 0 {
		t.Fatalf("non-dear-agent repo must be a no-op, got %v", got)
	}
}
