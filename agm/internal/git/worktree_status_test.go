package git

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/vbonnet/dear-agent/internal/gittest"
)

func TestHasUncommittedChanges_CleanRepo(t *testing.T) {
	tmpDir := t.TempDir()
	initTestRepo(t, tmpDir)

	dirty, err := HasUncommittedChanges(tmpDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if dirty {
		t.Fatal("expected clean repo to report no uncommitted changes")
	}
}

func TestHasUncommittedChanges_UntrackedFile(t *testing.T) {
	tmpDir := t.TempDir()
	initTestRepo(t, tmpDir)

	if err := os.WriteFile(filepath.Join(tmpDir, "scratch.txt"), []byte("x"), 0600); err != nil {
		t.Fatalf("write file: %v", err)
	}

	dirty, err := HasUncommittedChanges(tmpDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !dirty {
		t.Fatal("expected untracked file to count as uncommitted changes")
	}
}

func TestHasUncommittedChanges_ModifiedTrackedFile(t *testing.T) {
	tmpDir := t.TempDir()
	initTestRepo(t, tmpDir)

	if err := os.WriteFile(filepath.Join(tmpDir, "README.md"), []byte("# changed\n"), 0600); err != nil {
		t.Fatalf("write file: %v", err)
	}

	dirty, err := HasUncommittedChanges(tmpDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !dirty {
		t.Fatal("expected modified tracked file to count as uncommitted changes")
	}
}

func TestHasUncommittedChanges_NonRepoTreatedAsDirty(t *testing.T) {
	// A non-repo path must NOT be reported as clean — that would let a
	// caller delete it. Expect (true, err).
	dirty, err := HasUncommittedChanges(t.TempDir())
	if err == nil {
		t.Fatal("expected an error for a non-git path")
	}
	if !dirty {
		t.Fatal("a path whose status cannot be determined must be treated as dirty")
	}
}

func TestResolveBaseRef_PrefersOriginThenMain(t *testing.T) {
	tmpDir := t.TempDir()
	initTestRepo(t, tmpDir)

	// No remote: falls back to local main.
	if got := ResolveBaseRef(tmpDir); got != "main" {
		t.Fatalf("expected fallback to main, got %q", got)
	}
}

func TestResolveBaseRef_UnknownRepoReturnsEmpty(t *testing.T) {
	if got := ResolveBaseRef(t.TempDir()); got != "" {
		t.Fatalf("expected empty base ref for non-repo, got %q", got)
	}
}

func TestCommitsAhead_ZeroWhenBranchAtBase(t *testing.T) {
	tmpDir := t.TempDir()
	initTestRepo(t, tmpDir)

	mustGit(t, tmpDir, "branch", "claude/fresh")

	n, err := CommitsAhead(tmpDir, "claude/fresh", "main")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if n != 0 {
		t.Fatalf("expected 0 commits ahead, got %d", n)
	}
}

func TestCommitsAhead_CountsUnmergedCommits(t *testing.T) {
	tmpDir := t.TempDir()
	initTestRepo(t, tmpDir)

	mustGit(t, tmpDir, "checkout", "-b", "claude/work")
	if err := os.WriteFile(filepath.Join(tmpDir, "f.txt"), []byte("a"), 0600); err != nil {
		t.Fatalf("write: %v", err)
	}
	mustGit(t, tmpDir, "add", "f.txt")
	mustGit(t, tmpDir, "commit", "-m", "work commit")

	n, err := CommitsAhead(tmpDir, "claude/work", "main")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if n != 1 {
		t.Fatalf("expected 1 commit ahead, got %d", n)
	}
}

func TestCommitsAhead_NegativeOnError(t *testing.T) {
	tmpDir := t.TempDir()
	initTestRepo(t, tmpDir)

	n, err := CommitsAhead(tmpDir, "claude/work", "no-such-base")
	if err == nil {
		t.Fatal("expected an error for a missing base ref")
	}
	if n >= 0 {
		t.Fatalf("error result must be negative (treated as has-work), got %d", n)
	}
}

func mustGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	if out, err := gittest.Output(t, dir, args...); err != nil {
		t.Fatalf("git %v failed: %v\n%s", args, err, out)
	}
}
