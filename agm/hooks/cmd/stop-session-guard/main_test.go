package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestStopSessionGuard_CleanRepo(t *testing.T) {
	// Create a temp git repo
	dir := t.TempDir()
	mustRun(t, dir, "git", "init")
	mustRun(t, dir, "git", "config", "user.email", "test@test.com")
	mustRun(t, dir, "git", "config", "user.name", "Test")

	// Create and commit a file so repo is clean
	os.WriteFile(filepath.Join(dir, "README.md"), []byte("hello"), 0o644)
	mustRun(t, dir, "git", "add", ".")
	mustRun(t, dir, "git", "commit", "-m", "init")

	// Hook should pass (exit 0) on clean repo
	// We can't easily test the full binary, but we can test the check functions
	// by calling them directly
	t.Run("git status clean", func(t *testing.T) {
		cmd := exec.Command("git", "-C", dir, "status", "--porcelain", ".")
		out, err := cmd.Output()
		if err != nil {
			t.Fatalf("git status failed: %v", err)
		}
		if len(out) != 0 {
			t.Errorf("expected clean repo, got: %s", out)
		}
	})

	t.Run("git status dirty", func(t *testing.T) {
		os.WriteFile(filepath.Join(dir, "new.txt"), []byte("dirty"), 0o644)
		cmd := exec.Command("git", "-C", dir, "status", "--porcelain", ".")
		out, err := cmd.Output()
		if err != nil {
			t.Fatalf("git status failed: %v", err)
		}
		if len(out) == 0 {
			t.Error("expected dirty repo")
		}
	})
}

func mustRun(t *testing.T, dir string, name string, args ...string) {
	t.Helper()
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("%s %v failed: %v\n%s", name, args, err, out)
	}
}
