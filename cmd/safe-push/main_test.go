package main

import (
	"strings"
	"testing"

	"github.com/vbonnet/dear-agent/internal/gittest"
)

func TestRun_Help(t *testing.T) {
	err := run([]string{"-h"})
	if err != nil {
		t.Errorf("run(-h) = %v, want nil", err)
	}
}

func TestRun_MissingCArg(t *testing.T) {
	err := run([]string{"-C"})
	if err == nil {
		t.Fatal("run(-C) expected error for missing directory, got nil")
	}
	if !strings.Contains(err.Error(), "-C") {
		t.Errorf("error %q should mention -C flag", err)
	}
}

func TestRun_MissingTimeoutArg(t *testing.T) {
	err := run([]string{"--timeout"})
	if err == nil {
		t.Fatal("run(--timeout) expected error for missing duration, got nil")
	}
	if !strings.Contains(err.Error(), "--timeout") {
		t.Errorf("error %q should mention --timeout flag", err)
	}
}

func TestRun_InvalidTimeout(t *testing.T) {
	err := run([]string{"--timeout", "notaduration"})
	if err == nil {
		t.Fatal("run(--timeout notaduration) expected error, got nil")
	}
	if !strings.Contains(err.Error(), "--timeout") {
		t.Errorf("error %q should mention --timeout flag", err)
	}
}

func TestUsageContainsBinaryName(t *testing.T) {
	if !strings.Contains(usage, "safe-push") {
		t.Error("usage string should contain binary name 'safe-push'")
	}
}

// TestRun_RejectsProtectedForcePush drives the guard through the actual CLI entry point,
// which is where the bug was reported: `safe-push -uf origin main` reached git
// as a force push because the classifier compared whole argv tokens against
// "-f" and never expanded bundled short-option clusters. The force check runs
// before gh resolution, so these cases need no git, no gh, and no network.
func TestRun_RejectsProtectedForcePush(t *testing.T) {
	cases := []struct {
		name string
		argv []string
	}{
		{"standalone short force", []string{"-f", "origin", "main"}},
		{"long force", []string{"--force", "origin", "main"}},
		{"clustered -uf", []string{"-uf", "origin", "main"}},
		{"clustered -fu", []string{"-fu", "origin", "main"}},
		{"clustered -vfq", []string{"-vfq", "origin", "main"}},
		{"cluster after -C", []string{"-C", "/repo", "-uf", "origin", "main"}},
		{"force-with-lease", []string{"--force-with-lease", "origin", "main"}},
		{"force-with-lease with protected ref", []string{"--force-with-lease=refs/heads/main", "origin"}},
		{"force-if-includes", []string{"--force-if-includes", "origin", "main"}},
		{"mirror", []string{"--mirror", "origin"}},
		{"force refspec", []string{"origin", "+main"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := run(tc.argv)
			if err == nil {
				t.Fatalf("run(%q) = nil, want a force-push rejection", tc.argv)
			}
			if !strings.Contains(err.Error(), "force-push") {
				t.Fatalf("run(%q) error %q should name the force-push policy", tc.argv, err)
			}
		})
	}
}

func TestRun_CheckAllowsForceWithLeaseForNonProtectedBranch(t *testing.T) {
	repo := gittest.NewRepo(t)
	gittest.Run(t, repo, "remote", "add", "origin", t.TempDir())
	gittest.Run(t, repo, "update-ref", "refs/remotes/origin/main", "HEAD")
	gittest.Run(t, repo, "symbolic-ref", "refs/remotes/origin/HEAD", "refs/remotes/origin/main")
	gittest.Run(t, repo, "checkout", "-b", "feature")

	err := run([]string{"--check", "-C", repo, "--force-with-lease=refs/heads/x", "origin"})
	if err != nil {
		t.Fatalf("run(non-protected force-with-lease check) = %v, want nil", err)
	}
}
