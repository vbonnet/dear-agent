package ops

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/vbonnet/dear-agent/internal/gittest"
)

func git(t *testing.T, dir string, args ...string) {
	t.Helper()
	if out, err := gittest.Command(t, dir, args...).CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

// TestRealSweepDeps_Discover lays out a real ~/worktrees/<repo>/<name> tree
// and asserts the FS+git discovery returns only the linked worktree (the
// main checkout and out-of-base trees excluded).
func TestRealSweepDeps_Discover(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	base := t.TempDir()
	repoDir := filepath.Join(base, "myrepo")
	mainCheckout := filepath.Join(repoDir, "main")
	if err := os.MkdirAll(mainCheckout, 0o755); err != nil {
		t.Fatal(err)
	}

	git(t, base, "init", "-b", "main", mainCheckout)
	if err := os.WriteFile(filepath.Join(mainCheckout, "f"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	git(t, mainCheckout, "add", "f")
	git(t, mainCheckout, "commit", "-m", "init")

	linked := filepath.Join(repoDir, "feat")
	git(t, mainCheckout, "worktree", "add", "-b", "claude/feat", linked)

	got, err := (RealSweepDeps{}).Discover(base)
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("expected exactly the linked worktree, got %d: %+v", len(got), got)
	}
	d := got[0]
	if d.Repo != "myrepo" || d.Branch != "claude/feat" {
		t.Fatalf("repo/branch wrong: %+v", d)
	}
	wantPath, _ := filepath.EvalSymlinks(linked)
	gotPath, _ := filepath.EvalSymlinks(d.Path)
	if gotPath != wantPath {
		t.Fatalf("path = %q, want %q", gotPath, wantPath)
	}

	// Missing base ⇒ empty, not an error (sweep is idempotent/no-op).
	none, err := (RealSweepDeps{}).Discover(filepath.Join(base, "does-not-exist"))
	if err != nil || len(none) != 0 {
		t.Fatalf("missing base must be ([],nil), got %v / %v", none, err)
	}
}

func TestRealSweepDeps_DeleteBranchPreservesRemoteRef(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	base := t.TempDir()
	remote := filepath.Join(base, "remote.git")
	repo := filepath.Join(base, "repo")

	git(t, base, "init", "--bare", remote)
	git(t, base, "init", "-b", "main", repo)
	if err := os.WriteFile(filepath.Join(repo, "f"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	git(t, repo, "add", "f")
	git(t, repo, "commit", "-m", "init")
	git(t, repo, "branch", "reclaim")
	git(t, repo, "remote", "add", "origin", remote)
	git(t, repo, "push", "origin", "main", "reclaim")

	if err := (RealSweepDeps{}).DeleteBranch(repo, "reclaim", true); err != nil {
		t.Fatalf("DeleteBranch: %v", err)
	}
	if err := gittest.Command(t, repo, "show-ref", "--verify", "--quiet", "refs/heads/reclaim").Run(); err == nil {
		t.Fatal("local reclaim branch still exists")
	}
	if err := gittest.Command(t, repo, "ls-remote", "--exit-code", "--heads", "origin", "refs/heads/reclaim").Run(); err != nil {
		t.Fatalf("remote reclaim ref was removed: %v", err)
	}
}
