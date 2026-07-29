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
