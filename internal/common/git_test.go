package common

import (
	"os"
	"os/exec"
	"testing"
)

func TestGetCurrentCommit(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	_, cleanup := setupGitRepo(t)
	defer cleanup()

	// Create initial commit
	os.WriteFile("test.txt", []byte("test"), 0644)
	exec.Command("git", "add", "test.txt").Run()
	exec.Command("git", "commit", "-m", "initial commit").Run()

	commit, err := GetCurrentCommit()
	if err != nil {
		t.Fatalf("GetCurrentCommit() failed: %v", err)
	}

	if len(commit) != 40 {
		t.Errorf("commit SHA should be 40 characters, got %d: %s", len(commit), commit)
	}
}

func TestGetCurrentCommit_NoRepo(t *testing.T) {
	// Create temp directory without git
	tmpDir := t.TempDir()
	t.Chdir(tmpDir)

	_, err := GetCurrentCommit()
	if err == nil {
		t.Error("GetCurrentCommit() should fail outside git repo")
	}
}

func TestGetCurrentBranch(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	_, cleanup := setupGitRepo(t)
	defer cleanup()

	// Create initial commit (branch won't exist until first commit)
	os.WriteFile("test.txt", []byte("test"), 0644)
	exec.Command("git", "add", "test.txt").Run()
	exec.Command("git", "commit", "-m", "initial commit").Run()

	branch, err := GetCurrentBranch()
	if err != nil {
		t.Fatalf("GetCurrentBranch() failed: %v", err)
	}

	// Default branch is usually "master" or "main"
	if branch != "master" && branch != "main" {
		t.Logf("Branch is %q (expected master or main, but this may vary)", branch)
	}

	if branch == "" {
		t.Error("branch name should not be empty")
	}
}

func TestGetCurrentBranch_NoRepo(t *testing.T) {
	// Create temp directory without git
	tmpDir := t.TempDir()
	t.Chdir(tmpDir)

	_, err := GetCurrentBranch()
	if err == nil {
		t.Error("GetCurrentBranch() should fail outside git repo")
	}
}
