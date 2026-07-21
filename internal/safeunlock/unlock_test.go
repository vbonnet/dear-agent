package safeunlock

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

// newRepo creates a throwaway checkout with a real `.git` directory and returns
// the checkout root and its git dir. Tests drop lock files into the git dir and
// run Clean against the root.
func newRepo(t *testing.T) (repo, gitDir string) {
	t.Helper()
	repo = t.TempDir()
	gitDir = filepath.Join(repo, ".git")
	for _, d := range []string{gitDir, filepath.Join(gitDir, "refs", "heads")} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", d, err)
		}
	}
	return repo, gitDir
}

// writeLock creates a lock file and back-dates its mtime by age.
func writeLock(t *testing.T, path string, now time.Time, age time.Duration) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir for %s: %v", path, err)
	}
	if err := os.WriteFile(path, nil, 0o600); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
	mt := now.Add(-age)
	if err := os.Chtimes(path, mt, mt); err != nil {
		t.Fatalf("chtimes %s: %v", path, err)
	}
}

// newCleaner builds a Cleaner with a fixed clock and an isolated audit dir.
func newCleaner(t *testing.T, repo string, now time.Time) (*Cleaner, *bytes.Buffer) {
	t.Helper()
	t.Setenv("SAFE_UNLOCK_AUDIT_DIR", filepath.Join(t.TempDir(), "audit"))
	var log bytes.Buffer
	return &Cleaner{
		Repo:             repo,
		MinAge:           DefaultMinLockAge,
		IncludeWorktrees: true,
		Now:              now,
		Log:              &log,
	}, &log
}

func TestClean_NoLocks(t *testing.T) {
	repo, _ := newRepo(t)
	c, _ := newCleaner(t, repo, time.Now())
	res, err := c.Clean()
	if err != nil {
		t.Fatalf("Clean: %v", err)
	}
	if len(res) != 0 {
		t.Fatalf("want 0 results on a clean repo, got %d: %+v", len(res), res)
	}
}

func TestClean_IgnoresNonGitTransactionGuard(t *testing.T) {
	repo, gitDir := newRepo(t)
	now := time.Now()
	guard := filepath.Join(gitDir, "safe-pr-transaction.guard")
	writeLock(t, guard, now, 10*time.Minute)

	c, _ := newCleaner(t, repo, now)
	res, err := c.Clean()
	if err != nil {
		t.Fatalf("Clean: %v", err)
	}
	if len(res) != 0 {
		t.Fatalf("non-Git transaction guard was collected: %+v", res)
	}
	if _, err := os.Stat(guard); err != nil {
		t.Fatalf("non-Git transaction guard was removed: %v", err)
	}
}

func TestClean_StaleIndexLockRemoved(t *testing.T) {
	repo, gitDir := newRepo(t)
	now := time.Now()
	lock := filepath.Join(gitDir, "index.lock")
	writeLock(t, lock, now, 10*time.Minute) // well past MinAge, no holder

	c, _ := newCleaner(t, repo, now)
	res, err := c.Clean()
	if err != nil {
		t.Fatalf("Clean: %v", err)
	}
	if len(res) != 1 || !res[0].Removed || res[0].Active {
		t.Fatalf("want one removed stale lock, got %+v", res)
	}
	if res[0].Kind != "index" {
		t.Errorf("kind = %q, want index", res[0].Kind)
	}
	if _, err := os.Stat(lock); !os.IsNotExist(err) {
		t.Errorf("lock still present after removal: stat err = %v", err)
	}
}

func TestClean_YoungLockIsActive(t *testing.T) {
	repo, gitDir := newRepo(t)
	now := time.Now()
	lock := filepath.Join(gitDir, "index.lock")
	writeLock(t, lock, now, 5*time.Second) // younger than MinAge

	c, _ := newCleaner(t, repo, now)
	res, err := c.Clean()
	if err != nil {
		t.Fatalf("Clean: %v", err)
	}
	if len(res) != 1 || res[0].Removed || !res[0].Active {
		t.Fatalf("want one active (too-young) lock, got %+v", res)
	}
	if _, err := os.Stat(lock); err != nil {
		t.Errorf("young lock must NOT be removed, stat err = %v", err)
	}
}

func TestClean_DryRunRemovesNothing(t *testing.T) {
	repo, gitDir := newRepo(t)
	now := time.Now()
	lock := filepath.Join(gitDir, "index.lock")
	writeLock(t, lock, now, 10*time.Minute)

	c, _ := newCleaner(t, repo, now)
	c.DryRun = true
	res, err := c.Clean()
	if err != nil {
		t.Fatalf("Clean: %v", err)
	}
	if len(res) != 1 || res[0].Removed {
		t.Fatalf("dry-run must not remove, got %+v", res)
	}
	if _, err := os.Stat(lock); err != nil {
		t.Errorf("dry-run deleted the lock: stat err = %v", err)
	}
}

func TestClean_RefAndConfigLocks(t *testing.T) {
	repo, gitDir := newRepo(t)
	now := time.Now()
	refLock := filepath.Join(gitDir, "refs", "heads", "main.lock")
	cfgLock := filepath.Join(gitDir, "config.lock")
	writeLock(t, refLock, now, 10*time.Minute)
	writeLock(t, cfgLock, now, 10*time.Minute)

	c, _ := newCleaner(t, repo, now)
	res, err := c.Clean()
	if err != nil {
		t.Fatalf("Clean: %v", err)
	}
	if len(res) != 2 {
		t.Fatalf("want 2 results, got %d: %+v", len(res), res)
	}
	kinds := map[string]bool{}
	for _, r := range res {
		if !r.Removed {
			t.Errorf("expected removal of %s", r.LockPath)
		}
		kinds[r.Kind] = true
	}
	if !kinds["ref:heads/main"] || !kinds["config"] {
		t.Errorf("kinds = %v, want ref:heads/main and config", kinds)
	}
}

func TestClean_WorktreeLockScanToggle(t *testing.T) {
	repo, gitDir := newRepo(t)
	now := time.Now()
	wtLock := filepath.Join(gitDir, "worktrees", "feat-x", "index.lock")
	writeLock(t, wtLock, now, 10*time.Minute)

	// Disabled: the worktree subtree is not scanned.
	c, _ := newCleaner(t, repo, now)
	c.IncludeWorktrees = false
	res, err := c.Clean()
	if err != nil {
		t.Fatalf("Clean: %v", err)
	}
	if len(res) != 0 {
		t.Fatalf("IncludeWorktrees=false should skip worktree locks, got %+v", res)
	}
	if _, err := os.Stat(wtLock); err != nil {
		t.Fatalf("worktree lock must be untouched when scanning is off: %v", err)
	}

	// Enabled: the worktree lock is found and removed.
	c2, _ := newCleaner(t, repo, now)
	res2, err := c2.Clean()
	if err != nil {
		t.Fatalf("Clean: %v", err)
	}
	if len(res2) != 1 || !res2[0].Removed {
		t.Fatalf("want worktree lock removed, got %+v", res2)
	}
	if res2[0].Kind != "worktree:feat-x/index" {
		t.Errorf("kind = %q, want worktree:feat-x/index", res2[0].Kind)
	}
}

func TestClean_HeldLockIsActive(t *testing.T) {
	if _, err := exec.LookPath("lsof"); err != nil {
		t.Skip("lsof unavailable; holder guard not exercisable")
	}
	repo, gitDir := newRepo(t)
	now := time.Now()
	lock := filepath.Join(gitDir, "index.lock")
	writeLock(t, lock, now, 10*time.Minute) // old enough to pass the age guard

	// Hold the lock open for the duration of the check: lsof must report this
	// test process as a holder, so the lock is judged ACTIVE despite its age.
	f, err := os.Open(lock)
	if err != nil {
		t.Fatalf("open lock: %v", err)
	}
	defer f.Close()

	c, _ := newCleaner(t, repo, now)
	res, err := c.Clean()
	if err != nil {
		t.Fatalf("Clean: %v", err)
	}
	if len(res) != 1 || res[0].Removed || !res[0].Active {
		t.Fatalf("a held lock must be active and not removed, got %+v", res)
	}
	if _, err := os.Stat(lock); err != nil {
		t.Errorf("held lock must NOT be removed: %v", err)
	}
}

func TestClean_LinkedWorktreeGitFile(t *testing.T) {
	// A linked worktree's .git is a FILE pointing at the real git dir.
	now := time.Now()
	commonGit := t.TempDir()
	realGitDir := filepath.Join(commonGit, "worktrees", "wt1")
	if err := os.MkdirAll(realGitDir, 0o755); err != nil {
		t.Fatal(err)
	}
	worktree := t.TempDir()
	if err := os.WriteFile(filepath.Join(worktree, ".git"),
		[]byte("gitdir: "+realGitDir+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	lock := filepath.Join(realGitDir, "index.lock")
	writeLock(t, lock, now, 10*time.Minute)

	c, _ := newCleaner(t, worktree, now)
	res, err := c.Clean()
	if err != nil {
		t.Fatalf("Clean: %v", err)
	}
	if len(res) != 1 || !res[0].Removed {
		t.Fatalf("want lock in linked worktree git dir removed, got %+v", res)
	}
}

func TestClean_ContinuesPastRemovalError(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root bypasses directory write permissions; cannot force a removal error")
	}
	repo, gitDir := newRepo(t)
	now := time.Now()

	// One stale lock in a normal ref dir, one in a dir we make read-only so its
	// os.Remove fails. The clean lock must still be removed and the error from
	// the unremovable one surfaced — not abort the whole scan.
	okLock := filepath.Join(gitDir, "refs", "heads", "ok.lock")
	roDir := filepath.Join(gitDir, "refs", "heads", "locked")
	badLock := filepath.Join(roDir, "stuck.lock")
	writeLock(t, okLock, now, 10*time.Minute)
	writeLock(t, badLock, now, 10*time.Minute)
	if err := os.Chmod(roDir, 0o500); err != nil {
		t.Fatalf("chmod: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(roDir, 0o700) }) // let TempDir cleanup succeed

	c, _ := newCleaner(t, repo, now)
	results, err := c.Clean()
	if err == nil {
		t.Skip("removal did not fail on this filesystem; cannot exercise error path")
	}
	if len(results) != 2 {
		t.Fatalf("want both locks reported despite one error, got %d: %+v", len(results), results)
	}
	if _, statErr := os.Stat(okLock); !os.IsNotExist(statErr) {
		t.Errorf("the removable lock must still be cleaned despite the other's error")
	}
}

func TestClean_NotAGitRepo(t *testing.T) {
	c, _ := newCleaner(t, t.TempDir(), time.Now())
	if _, err := c.Clean(); err == nil {
		t.Fatal("expected an error for a directory with no .git")
	}
}

func TestClean_WritesAuditTrail(t *testing.T) {
	repo, gitDir := newRepo(t)
	now := time.Now()
	auditDir := filepath.Join(t.TempDir(), "audit")
	t.Setenv("SAFE_UNLOCK_AUDIT_DIR", auditDir)
	writeLock(t, filepath.Join(gitDir, "index.lock"), now, 10*time.Minute)

	c := &Cleaner{Repo: repo, MinAge: DefaultMinLockAge, Now: now, Log: &bytes.Buffer{}}
	if _, err := c.Clean(); err != nil {
		t.Fatalf("Clean: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(auditDir, "safe-unlock-audit.jsonl"))
	if err != nil {
		t.Fatalf("read audit log: %v", err)
	}
	var entry AuditEntry
	if err := json.Unmarshal(bytes.TrimSpace(data), &entry); err != nil {
		t.Fatalf("audit line is not valid JSON: %v\n%s", err, data)
	}
	if entry.Outcome != "removed" || entry.Kind != "index" || entry.Repo != repo {
		t.Errorf("unexpected audit entry: %+v", entry)
	}
}

func TestClassify(t *testing.T) {
	gitDir := "/repo/.git"
	cases := map[string]string{
		"/repo/.git/index.lock":               "index",
		"/repo/.git/HEAD.lock":                "HEAD",
		"/repo/.git/config.lock":              "config",
		"/repo/.git/packed-refs.lock":         "packed-refs",
		"/repo/.git/refs/heads/main.lock":     "ref:heads/main",
		"/repo/.git/worktrees/wt1/index.lock": "worktree:wt1/index",
	}
	for path, want := range cases {
		if got := classify(gitDir, path); got != want {
			t.Errorf("classify(%q) = %q, want %q", path, got, want)
		}
	}
}
