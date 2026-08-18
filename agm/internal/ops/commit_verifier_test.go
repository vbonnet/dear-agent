package ops

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/vbonnet/dear-agent/internal/gittest"
)

// initTestRepo creates a temporary git repo with an initial commit on main
// and returns the repo path. The caller does not need to clean up; t.TempDir
// handles that.
func initTestRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()

	gittest.Run(t, dir, "init", "-b", "main", dir)

	// Create an initial commit so main has history.
	f := filepath.Join(dir, "README.md")
	if err := os.WriteFile(f, []byte("init"), 0o644); err != nil {
		t.Fatal(err)
	}
	gittest.Run(t, dir, "add", ".")
	gittest.Run(t, dir, "commit", "-m", "initial")

	return dir
}

// headSHA returns the HEAD commit SHA for the repo at dir.
func headSHA(t *testing.T, dir string) string {
	t.Helper()
	out, err := gittest.Output(t, dir, "rev-parse", "HEAD")
	if err != nil {
		t.Fatalf("rev-parse HEAD failed: %v\n%s", err, out)
	}
	return out[:len(out)-1] // trim newline
}

func TestVerifyOnMain_CommitOnMain(t *testing.T) {
	repo := initTestRepo(t)
	sha := headSHA(t, repo)

	ok, err := VerifyOnMain(repo, sha)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !ok {
		t.Error("VerifyOnMain = false; want true for commit on main")
	}
}

func TestVerifyOnMain_CommitNotOnMain(t *testing.T) {
	repo := initTestRepo(t)

	// Create a feature branch with a commit not merged to main.
	gittest.Run(t, repo, "checkout", "-b", "feature")

	f := filepath.Join(repo, "feature.txt")
	if err := os.WriteFile(f, []byte("feature work"), 0o644); err != nil {
		t.Fatal(err)
	}
	gittest.Run(t, repo, "add", ".")
	gittest.Run(t, repo, "commit", "-m", "feature commit")

	featureSHA := headSHA(t, repo)

	ok, err := VerifyOnMain(repo, featureSHA)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ok {
		t.Error("VerifyOnMain = true; want false for unmerged feature commit")
	}
}

func TestVerifyOnMain_MergedFeature(t *testing.T) {
	repo := initTestRepo(t)

	// Create feature branch, commit, then merge to main.
	gittest.Run(t, repo, "checkout", "-b", "feature")

	f := filepath.Join(repo, "feature.txt")
	if err := os.WriteFile(f, []byte("merged work"), 0o644); err != nil {
		t.Fatal(err)
	}
	gittest.Run(t, repo, "add", ".")
	gittest.Run(t, repo, "commit", "-m", "feature commit")

	featureSHA := headSHA(t, repo)

	// Merge feature into main.
	gittest.Run(t, repo, "checkout", "main")
	gittest.Run(t, repo, "merge", "feature", "--no-ff", "-m", "merge feature")

	ok, err := VerifyOnMain(repo, featureSHA)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !ok {
		t.Error("VerifyOnMain = false; want true for merged feature commit")
	}
}

func TestVerifyOnMain_EmptyRepoPath(t *testing.T) {
	_, err := VerifyOnMain("", "abc123")
	if err == nil {
		t.Error("expected error for empty repoPath")
	}
}

func TestVerifyOnMain_EmptyCommitSHA(t *testing.T) {
	_, err := VerifyOnMain("/tmp", "")
	if err == nil {
		t.Error("expected error for empty commitSHA")
	}
}

func TestVerifyOnMain_InvalidRepo(t *testing.T) {
	dir := t.TempDir()
	_, err := VerifyOnMain(dir, "abc123")
	if err == nil {
		t.Error("expected error for non-git directory")
	}
}
