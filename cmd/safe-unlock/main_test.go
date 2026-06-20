package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// makeRepoWithLock builds a checkout with a back-dated stale index.lock and an
// isolated audit dir, returning the checkout root.
func makeRepoWithLock(t *testing.T, age time.Duration) string {
	t.Helper()
	t.Setenv("SAFE_UNLOCK_AUDIT_DIR", filepath.Join(t.TempDir(), "audit"))
	repo := t.TempDir()
	gitDir := filepath.Join(repo, ".git")
	if err := os.MkdirAll(gitDir, 0o755); err != nil {
		t.Fatal(err)
	}
	lock := filepath.Join(gitDir, "index.lock")
	if err := os.WriteFile(lock, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	mt := time.Now().Add(-age)
	if err := os.Chtimes(lock, mt, mt); err != nil {
		t.Fatal(err)
	}
	return repo
}

func TestRun_StaleLockExitsZero(t *testing.T) {
	repo := makeRepoWithLock(t, 10*time.Minute)
	if code := run([]string{repo}); code != 0 {
		t.Fatalf("run exit = %d, want 0", code)
	}
	if _, err := os.Stat(filepath.Join(repo, ".git", "index.lock")); !os.IsNotExist(err) {
		t.Errorf("stale lock not removed")
	}
}

func TestRun_ActiveLockExitsTwo(t *testing.T) {
	repo := makeRepoWithLock(t, 5*time.Second) // too young → active
	if code := run([]string{repo}); code != 2 {
		t.Fatalf("run exit = %d, want 2 for an active lock", code)
	}
	if _, err := os.Stat(filepath.Join(repo, ".git", "index.lock")); err != nil {
		t.Errorf("active lock must be left in place: %v", err)
	}
}

func TestRun_DryRunKeepsLockExitsZero(t *testing.T) {
	repo := makeRepoWithLock(t, 10*time.Minute)
	if code := run([]string{"--dry-run", repo}); code != 0 {
		t.Fatalf("dry-run exit = %d, want 0", code)
	}
	if _, err := os.Stat(filepath.Join(repo, ".git", "index.lock")); err != nil {
		t.Errorf("dry-run must not remove the lock: %v", err)
	}
}

func TestRun_NotARepoExitsOne(t *testing.T) {
	if code := run([]string{t.TempDir()}); code != 1 {
		t.Fatalf("run exit = %d, want 1 for a non-repo path", code)
	}
}

func TestRun_TooManyArgsExitsOne(t *testing.T) {
	if code := run([]string{"a", "b"}); code != 1 {
		t.Fatalf("run exit = %d, want 1 for too many args", code)
	}
}

func TestRun_HelpExitsZero(t *testing.T) {
	if code := run([]string{"--help"}); code != 0 {
		t.Fatalf("--help exit = %d, want 0", code)
	}
}
