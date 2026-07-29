package git

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/vbonnet/dear-agent/internal/gittest"
)

func commitFile(t *testing.T, repo, name, content, msg string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(repo, name), []byte(content), 0600); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
	if err := gittest.Command(t, repo, "add", name).Run(); err != nil {
		t.Fatalf("git add: %v", err)
	}
	if err := gittest.Command(t, repo, "commit", "-m", msg).Run(); err != nil {
		t.Fatalf("git commit: %v", err)
	}
}

func TestIsAncestor(t *testing.T) {
	repo := t.TempDir()
	initTestRepo(t, repo) // leaves "main" at the initial commit

	// A branch that is exactly main is an ancestor of main.
	if err := gittest.Command(t, repo, "branch", "merged", "main").Run(); err != nil {
		t.Fatalf("branch merged: %v", err)
	}
	anc, err := IsAncestor(repo, "merged", "main")
	if err != nil || !anc {
		t.Fatalf("merged should be ancestor of main: anc=%v err=%v", anc, err)
	}

	// A branch with its own commit ahead of main is NOT an ancestor.
	if err := gittest.Command(t, repo, "checkout", "-b", "feature").Run(); err != nil {
		t.Fatalf("checkout feature: %v", err)
	}
	commitFile(t, repo, "f.txt", "x", "feature work")
	anc, err = IsAncestor(repo, "feature", "main")
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if anc {
		t.Fatal("feature has unmerged commits; must not be an ancestor of main")
	}

	// Probe failure (empty ref) must be (false, err) — fail-safe.
	if anc, err := IsAncestor(repo, "", "main"); anc || err == nil {
		t.Fatalf("empty ref must be (false,err), got anc=%v err=%v", anc, err)
	}
	if _, err := IsAncestor(t.TempDir(), "a", "b"); err == nil {
		t.Fatal("non-git path must error")
	}
}

func TestLastCommitInfo(t *testing.T) {
	repo := t.TempDir()
	initTestRepo(t, repo)
	tricky := "feat: weird\tsubject -- with dashes"
	commitFile(t, repo, "g.txt", "y", tricky)

	when, subj, err := LastCommitInfo(repo)
	if err != nil {
		t.Fatalf("LastCommitInfo: %v", err)
	}
	if subj != tricky {
		t.Fatalf("subject = %q, want %q", subj, tricky)
	}
	if time.Since(when) > time.Hour || when.IsZero() {
		t.Fatalf("commit time looks wrong: %v", when)
	}

	if _, _, err := LastCommitInfo(t.TempDir()); err == nil {
		t.Fatal("non-git path must error")
	}
}

func TestHasUnpushedCommits(t *testing.T) {
	origin := t.TempDir()
	if err := gittest.Command(t, origin, "init", "--bare", "-b", "main", origin).Run(); err != nil {
		t.Fatalf("init bare: %v", err)
	}
	repo := t.TempDir()
	initTestRepo(t, repo)
	if err := gittest.Command(t, repo, "remote", "add", "origin", origin).Run(); err != nil {
		t.Fatalf("remote add: %v", err)
	}
	if err := gittest.Command(t, repo, "push", "origin", "main").Run(); err != nil {
		t.Fatalf("push main: %v", err)
	}

	// Fully pushed branch ⇒ no unpushed commits.
	if err := gittest.Command(t, repo, "checkout", "-b", "pushed").Run(); err != nil {
		t.Fatalf("checkout pushed: %v", err)
	}
	commitFile(t, repo, "p.txt", "1", "pushed work")
	if err := gittest.Command(t, repo, "push", "origin", "pushed").Run(); err != nil {
		t.Fatalf("push pushed: %v", err)
	}
	if up, err := HasUnpushedCommits(repo, "pushed"); err != nil || up {
		t.Fatalf("fully-pushed branch: up=%v err=%v", up, err)
	}

	// Local-only commits ⇒ unpushed.
	commitFile(t, repo, "p.txt", "2", "local only")
	if up, err := HasUnpushedCommits(repo, "pushed"); err != nil || !up {
		t.Fatalf("local-only commit must be unpushed: up=%v err=%v", up, err)
	}

	// Empty branch is the fail-safe (true, err).
	if up, err := HasUnpushedCommits(repo, ""); !up || err == nil {
		t.Fatalf("empty branch must be (true,err), got up=%v err=%v", up, err)
	}
}

func TestMainWorktreePath(t *testing.T) {
	repo := t.TempDir()
	initTestRepo(t, repo)

	linked := filepath.Join(t.TempDir(), "linked")
	if err := gittest.Command(t, repo, "worktree", "add", "-b", "claude/x", linked).Run(); err != nil {
		t.Fatalf("worktree add: %v", err)
	}

	got, err := MainWorktreePath(linked)
	if err != nil {
		t.Fatalf("MainWorktreePath: %v", err)
	}
	want, _ := filepath.EvalSymlinks(repo)
	gotResolved, _ := filepath.EvalSymlinks(got)
	if gotResolved != want {
		t.Fatalf("main path = %q, want %q", gotResolved, want)
	}

	if _, err := MainWorktreePath(t.TempDir()); err == nil {
		t.Fatal("non-git path must error")
	}
}
