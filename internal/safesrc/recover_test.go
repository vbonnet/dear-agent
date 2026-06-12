package safesrc

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestValidateRepo_AcceptsRepoUnderSrc(t *testing.T) {
	home := t.TempDir()
	repo := filepath.Join(home, "src", "dear-agent")
	mustMkdir(t, repo)

	got, err := ValidateRepo(home, repo)
	if err != nil {
		t.Fatalf("ValidateRepo(%q) returned error: %v", repo, err)
	}
	// Returned path is symlink-resolved; on macOS /var -> /private/var, so
	// compare against the resolved form rather than the literal input.
	want, _ := filepath.EvalSymlinks(repo)
	if got != want {
		t.Fatalf("ValidateRepo = %q, want %q", got, want)
	}
}

func TestValidateRepo_ExpandsTilde(t *testing.T) {
	home := t.TempDir()
	mustMkdir(t, filepath.Join(home, "src", "brain-v2"))

	got, err := ValidateRepo(home, "~/src/brain-v2")
	if err != nil {
		t.Fatalf("ValidateRepo(~/src/brain-v2) error: %v", err)
	}
	want, _ := filepath.EvalSymlinks(filepath.Join(home, "src", "brain-v2"))
	if got != want {
		t.Fatalf("ValidateRepo tilde = %q, want %q", got, want)
	}
}

func TestValidateRepo_RejectsSrcRootItself(t *testing.T) {
	home := t.TempDir()
	mustMkdir(t, filepath.Join(home, "src"))

	_, err := ValidateRepo(home, filepath.Join(home, "src"))
	if err == nil {
		t.Fatal("ValidateRepo(~/src) should reject the src root itself")
	}
	if !strings.Contains(err.Error(), "root itself") {
		t.Fatalf("error should explain it is the src root, got: %v", err)
	}
}

func TestValidateRepo_RejectsOutsideSrc(t *testing.T) {
	home := t.TempDir()
	mustMkdir(t, filepath.Join(home, "src"))
	outside := filepath.Join(home, "worktrees", "dear-agent")
	mustMkdir(t, outside)

	_, err := ValidateRepo(home, outside)
	if err == nil {
		t.Fatal("ValidateRepo should reject a path under ~/worktrees")
	}
	if !strings.Contains(err.Error(), "under") {
		t.Fatalf("error should explain the ~/src boundary, got: %v", err)
	}
}

func TestValidateRepo_RejectsDotDotEscape(t *testing.T) {
	home := t.TempDir()
	mustMkdir(t, filepath.Join(home, "src"))
	mustMkdir(t, filepath.Join(home, "evil"))

	// A textual path that starts under src but climbs out must be rejected
	// after resolution.
	sneaky := filepath.Join(home, "src", "..", "evil")
	if _, err := ValidateRepo(home, sneaky); err == nil {
		t.Fatal("ValidateRepo should reject a ../-escape out of ~/src")
	}
}

func TestValidateRepo_RejectsNonexistent(t *testing.T) {
	home := t.TempDir()
	mustMkdir(t, filepath.Join(home, "src"))
	if _, err := ValidateRepo(home, filepath.Join(home, "src", "ghost")); err == nil {
		t.Fatal("ValidateRepo should reject a path that does not exist")
	}
}

func TestValidateRepo_RejectsEmpty(t *testing.T) {
	home := t.TempDir()
	if _, err := ValidateRepo(home, "   "); err == nil {
		t.Fatal("ValidateRepo should reject an empty path")
	}
}

func TestValidateRepo_RejectsSymlinkEscape(t *testing.T) {
	home := t.TempDir()
	mustMkdir(t, filepath.Join(home, "src"))
	target := filepath.Join(home, "outside-repo")
	mustMkdir(t, target)
	link := filepath.Join(home, "src", "linked")
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("symlink unsupported: %v", err)
	}
	// The link lives under ~/src but resolves outside it; validation must
	// reject it on the resolved path, not the literal one.
	if _, err := ValidateRepo(home, link); err == nil {
		t.Fatal("ValidateRepo should reject a symlink under ~/src that points outside")
	}
}

func TestAllowedVerbs_OnlySafeSet(t *testing.T) {
	// The recovery + inspection verbs must be present...
	for _, v := range []string{"rev-parse", "symbolic-ref", "status", "diff", "stash", "checkout", "pull"} {
		if !allowedVerbs[v] {
			t.Errorf("verb %q must be allowed for recovery to work", v)
		}
	}
	// ...and every mutating verb that could damage a golden tree must NOT be.
	for _, v := range []string{"commit", "reset", "push", "branch", "merge", "rebase", "clean", "rm", "tag", "fetch"} {
		if allowedVerbs[v] {
			t.Errorf("verb %q must NOT be in the src-recovery allowlist", v)
		}
	}
}

func TestRunGit_RejectsForbiddenVerb(t *testing.T) {
	r := &Recoverer{Repo: t.TempDir(), Log: &strings.Builder{}}
	if _, err := r.runGit(context.Background(), "reset", "--hard"); err == nil {
		t.Fatal("runGit must refuse a verb outside the allowlist")
	}
}

func TestStashStamp_UsesInjectedClock(t *testing.T) {
	ctx := WithStamp(context.Background(), "2026-06-11T23:00:00Z")
	if got := stashStamp(ctx); got != "src-recovery 2026-06-11T23:00:00Z" {
		t.Fatalf("stashStamp = %q", got)
	}
	if got := stashStamp(context.Background()); got != "src-recovery" {
		t.Fatalf("stashStamp without stamp = %q", got)
	}
}

func TestUnderDir(t *testing.T) {
	root := filepath.FromSlash("/home/u/src")
	cases := []struct {
		path string
		want bool
	}{
		{filepath.FromSlash("/home/u/src/repo"), true},
		{filepath.FromSlash("/home/u/src/repo/sub"), true},
		{filepath.FromSlash("/home/u/src"), false}, // dir itself
		{filepath.FromSlash("/home/u/worktrees/x"), false},
		{filepath.FromSlash("/home/u/srcfoo"), false}, // prefix-but-not-under
	}
	for _, c := range cases {
		if got := underDir(c.path, root); got != c.want {
			t.Errorf("underDir(%q,%q)=%v want %v", c.path, root, got, c.want)
		}
	}
}

func mustMkdir(t *testing.T, p string) {
	t.Helper()
	if err := os.MkdirAll(p, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", p, err)
	}
}
