package safesrc

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// newRepo creates a minimal repo with a `.git` directory under a temp home and
// returns the repo path. It does not init a real git repo: gitDir() resolves the
// lock path from the filesystem alone, so a `.git` directory is enough.
func newRepo(t *testing.T) string {
	t.Helper()
	repo := filepath.Join(t.TempDir(), "repo")
	mustMkdir(t, filepath.Join(repo, ".git"))
	return repo
}

func writeLock(t *testing.T, repo string, modAge time.Duration) (lockPath string, now time.Time) {
	t.Helper()
	lockPath = filepath.Join(repo, ".git", "index.lock")
	if err := os.WriteFile(lockPath, nil, 0o644); err != nil {
		t.Fatalf("write lock: %v", err)
	}
	// Anchor a fixed "now" and back-date the lock's mtime by modAge so age is
	// deterministic regardless of wall clock.
	now = time.Unix(1_700_000_000, 0)
	mod := now.Add(-modAge)
	if err := os.Chtimes(lockPath, mod, mod); err != nil {
		t.Fatalf("chtimes: %v", err)
	}
	return lockPath, now
}

func TestUnlock_NoLockIsIdempotent(t *testing.T) {
	repo := newRepo(t)
	u := &Unlocker{Repo: repo, Now: time.Unix(1_700_000_000, 0), Log: &strings.Builder{}}
	res, err := u.Unlock()
	if err != nil {
		t.Fatalf("Unlock with no lock should succeed, got: %v", err)
	}
	if res.Existed || res.Removed {
		t.Fatalf("no lock present: Existed=%v Removed=%v, want both false", res.Existed, res.Removed)
	}
}

func TestUnlock_RemovesStaleLock(t *testing.T) {
	repo := newRepo(t)
	lockPath, now := writeLock(t, repo, 10*time.Minute) // well past the 60s default
	u := &Unlocker{Repo: repo, Now: now, Log: &strings.Builder{}}

	res, err := u.Unlock()
	if err != nil {
		t.Fatalf("Unlock of a stale lock should succeed, got: %v", err)
	}
	if !res.Existed || !res.Removed {
		t.Fatalf("stale lock: Existed=%v Removed=%v, want both true", res.Existed, res.Removed)
	}
	if _, err := os.Stat(lockPath); !os.IsNotExist(err) {
		t.Fatalf("lock should be gone, stat err = %v", err)
	}
}

func TestUnlock_RefusesYoungLock(t *testing.T) {
	repo := newRepo(t)
	lockPath, now := writeLock(t, repo, 5*time.Second) // younger than 60s default
	u := &Unlocker{Repo: repo, Now: now, Log: &strings.Builder{}}

	res, err := u.Unlock()
	if err == nil {
		t.Fatal("Unlock should refuse a lock younger than MinAge")
	}
	if !strings.Contains(err.Error(), "old") {
		t.Fatalf("error should explain the age guard, got: %v", err)
	}
	if !res.Existed || res.Removed {
		t.Fatalf("young lock: Existed=%v Removed=%v, want Existed=true Removed=false", res.Existed, res.Removed)
	}
	if _, err := os.Stat(lockPath); err != nil {
		t.Fatalf("young lock must NOT be removed, stat err = %v", err)
	}
}

func TestUnlock_DryRunLeavesLock(t *testing.T) {
	repo := newRepo(t)
	lockPath, now := writeLock(t, repo, 10*time.Minute)
	u := &Unlocker{Repo: repo, DryRun: true, Now: now, Log: &strings.Builder{}}

	res, err := u.Unlock()
	if err != nil {
		t.Fatalf("dry-run Unlock should succeed, got: %v", err)
	}
	if !res.Existed || res.Removed {
		t.Fatalf("dry-run: Existed=%v Removed=%v, want Existed=true Removed=false", res.Existed, res.Removed)
	}
	if _, err := os.Stat(lockPath); err != nil {
		t.Fatalf("dry-run must NOT remove the lock, stat err = %v", err)
	}
}

func TestUnlock_CustomMinAge(t *testing.T) {
	repo := newRepo(t)
	// Lock is 2m old; a 5m floor makes it too young, so it must be refused.
	_, now := writeLock(t, repo, 2*time.Minute)
	u := &Unlocker{Repo: repo, MinAge: 5 * time.Minute, Now: now, Log: &strings.Builder{}}

	if _, err := u.Unlock(); err == nil {
		t.Fatal("a 2m-old lock under a 5m --min-age floor should be refused")
	}
}

func TestGitDir_PlainCheckout(t *testing.T) {
	repo := newRepo(t)
	gd, err := gitDir(repo)
	if err != nil {
		t.Fatalf("gitDir on a plain checkout: %v", err)
	}
	if want := filepath.Join(repo, ".git"); gd != want {
		t.Fatalf("gitDir = %q, want %q", gd, want)
	}
}

func TestGitDir_WorktreeFile(t *testing.T) {
	dir := t.TempDir()
	repo := filepath.Join(dir, "wt")
	mustMkdir(t, repo)
	realGit := filepath.Join(dir, "gitcommon", "worktrees", "wt")
	mustMkdir(t, realGit)
	// A linked worktree's .git is a file pointing at the per-worktree git dir.
	if err := os.WriteFile(filepath.Join(repo, ".git"), []byte("gitdir: "+realGit+"\n"), 0o644); err != nil {
		t.Fatalf("write .git file: %v", err)
	}
	gd, err := gitDir(repo)
	if err != nil {
		t.Fatalf("gitDir on a worktree: %v", err)
	}
	if gd != realGit {
		t.Fatalf("gitDir = %q, want %q", gd, realGit)
	}
}

func TestGitDir_NotARepo(t *testing.T) {
	if _, err := gitDir(t.TempDir()); err == nil {
		t.Fatal("gitDir should fail when there is no .git")
	}
}
