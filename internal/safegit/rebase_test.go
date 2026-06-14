package safegit

import (
	"os/exec"
	"testing"
)

func TestIsProtectedBranch(t *testing.T) {
	cases := []struct {
		name string
		want bool
	}{
		{"main", true},
		{"master", true},
		{"develop", true},
		{"release", true},
		{"feature/add-login", false},
		{"fix-safety-regressions", false},
		{"hotfix-123", false},
		{"", false},
	}
	for _, tc := range cases {
		if got := IsProtectedBranch(tc.name); got != tc.want {
			t.Errorf("IsProtectedBranch(%q) = %v, want %v", tc.name, got, tc.want)
		}
	}
}

func TestSafeRebase_RefusesProtectedBranch(t *testing.T) {
	// SafeRebase should refuse to rebase protected branches even before
	// attempting any git operations. We test this by pointing at a non-repo
	// dir — the protected-branch check should fire before the git call.
	cfg := RebaseConfig{
		RepoDir: t.TempDir(),
	}
	// Create a minimal git repo on "main" to trigger the protection check.
	setupGitRepo(t, cfg.RepoDir, "main")

	_, err := SafeRebase(cfg)
	if err == nil {
		t.Fatal("expected error when rebasing main, got nil")
	}
	if got := err.Error(); !contains(got, "refusing to rebase") {
		t.Errorf("error = %q, want substring 'refusing to rebase'", got)
	}
}

func TestSafeRebase_DryRun(t *testing.T) {
	dir := t.TempDir()
	setupGitRepo(t, dir, "feature-x")
	cfg := RebaseConfig{
		RepoDir: dir,
		DryRun:  true,
	}
	// Dry run should succeed without network ops (fetch will fail but
	// the branch protection check passes first for non-protected branches).
	// We expect a fetch error since there's no remote.
	_, err := SafeRebase(cfg)
	if err == nil {
		// If no error, dry-run completed — that's fine too
		return
	}
	// Expected: fetch fails because no remote
	if !contains(err.Error(), "fetch") {
		t.Errorf("unexpected error: %v", err)
	}
}

func setupGitRepo(t *testing.T, dir, branch string) {
	t.Helper()
	cmds := [][]string{
		{"git", "-C", dir, "init", "-b", branch},
		{"git", "-C", dir, "config", "user.email", "test@test.com"},
		{"git", "-C", dir, "config", "user.name", "Test"},
		{"git", "-C", dir, "commit", "--allow-empty", "-m", "initial"},
	}
	for _, args := range cmds {
		cmd := exec.Command(args[0], args[1:]...)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("%v failed: %v\n%s", args, err, out)
		}
	}
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(sub) == 0 ||
		findSubstring(s, sub))
}

func findSubstring(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
