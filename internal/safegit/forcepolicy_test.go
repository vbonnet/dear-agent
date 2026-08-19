package safegit

import (
	"os/exec"
	"path/filepath"
	"testing"
)

// newPolicyRepo builds a repo with an origin remote and a resolvable
// origin/HEAD, so ProtectedTargets reports the default branch as known.
func newPolicyRepo(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	origin := filepath.Join(root, "origin.git")
	repo := filepath.Join(root, "repo")
	run := func(dir string, args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		cmd.Env = append(cmd.Environ(),
			"GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_SYSTEM=/dev/null",
			"GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@example.com",
			"GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@example.com")
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	run(root, "init", "--bare", "--initial-branch=main", origin)
	run(root, "init", "--initial-branch=main", repo)
	run(repo, "commit", "--allow-empty", "-m", "base")
	run(repo, "remote", "add", "origin", origin)
	run(repo, "push", "-u", "origin", "main")
	run(repo, "remote", "set-head", "origin", "main")
	return repo
}

func checkoutBranch(t *testing.T, repo, branch string) {
	t.Helper()
	cmd := exec.Command("git", "checkout", "-b", branch)
	cmd.Dir = repo
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("checkout %s: %v\n%s", branch, err, out)
	}
}

func setConfig(t *testing.T, repo, key, value string) {
	t.Helper()
	cmd := exec.Command("git", "config", key, value)
	cmd.Dir = repo
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("config %s: %v\n%s", key, err, out)
	}
}

// TestForcePushViolation_FailsClosedOnUnresolvableDestination covers the
// bypasses that all shared one root cause: an unparsed or unresolvable
// destination fell back to "the current branch", which then looked safe.
func TestForcePushViolation_FailsClosedOnUnresolvableDestination(t *testing.T) {
	repo := newPolicyRepo(t)
	checkoutBranch(t, repo, "feature")

	cases := []struct {
		name        string
		args        []string
		wantBlocked bool
	}{
		{"wildcard refspec", []string{"--force", "origin", "refs/heads/*:refs/heads/*"}, true},
		{"mirror", []string{"--force", "--mirror", "origin"}, true},
		{"all", []string{"--force", "--all", "origin"}, true},
		{"tags", []string{"--force", "--tags", "origin"}, true},
		{"set-upstream does not swallow the refspec", []string{"-f", "origin", "-u", "main"}, true},
		{"recurse-submodules value is not a refspec", []string{"--force", "--recurse-submodules", "check", "origin"}, false},
		{"plain force to a PR branch is allowed", []string{"--force-with-lease", "origin", "feature"}, false},
		{"explicit refs/heads PR branch is allowed", []string{"-f", "origin", "refs/heads/feature"}, false},
		{"custom remote name is not read as a refspec", []string{"-f", "production", "feature"}, false},
		{"custom remote with a protected refspec is blocked", []string{"-f", "production", "main"}, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			target, blocked := ForcePushViolation(repo, "", tc.args)
			if blocked != tc.wantBlocked {
				t.Fatalf("ForcePushViolation(%v) = (%q, %v), want blocked=%v",
					tc.args, target, blocked, tc.wantBlocked)
			}
		})
	}
}

// TestForcePushViolation_ValueOptionsDoNotHideTheCurrentBranch covers the
// option-parsing bugs whose effect was to make a value look like a refspec:
// the fabricated refspec suppressed the current-branch fallback, so a force
// push from main was never examined against main.
func TestForcePushViolation_ValueOptionsDoNotHideTheCurrentBranch(t *testing.T) {
	repo := newPolicyRepo(t) // still on main

	cases := []struct {
		name string
		args []string
	}{
		{"recurse-submodules value", []string{"--force", "--recurse-submodules", "check", "origin"}},
		{"push-option value", []string{"--force", "-o", "ci.skip", "origin"}},
		{"receive-pack value", []string{"--force", "--receive-pack", "git-receive-pack", "origin"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, blocked := ForcePushViolation(repo, "", tc.args); !blocked {
				t.Fatalf("ForcePushViolation(%v) from main was allowed: the option value was read as a refspec", tc.args)
			}
		})
	}
}

// TestForcePushViolation_SetUpstreamKeepsItsRefspec covers `-f origin -u main`
// from a feature branch: `-u` is boolean, so `main` is the refspec and the
// destination is main, not the feature branch.
func TestForcePushViolation_SetUpstreamKeepsItsRefspec(t *testing.T) {
	repo := newPolicyRepo(t)
	checkoutBranch(t, repo, "feature")
	if _, blocked := ForcePushViolation(repo, "", []string{"-f", "origin", "-u", "main"}); !blocked {
		t.Fatal("-u swallowed the refspec and the push to main was allowed")
	}
	if _, blocked := ForcePushViolation(repo, "", []string{"-f", "origin", "-u", "feature"}); blocked {
		t.Error("-u with a PR-branch refspec should be allowed")
	}
}

// TestForcePushViolation_ResolvesHEADRefspec covers `safe-push -f origin HEAD`
// on main, which git expands to HEAD:refs/heads/main.
func TestForcePushViolation_ResolvesHEADRefspec(t *testing.T) {
	repo := newPolicyRepo(t)
	for _, spec := range []string{"HEAD", "@"} {
		t.Run(spec, func(t *testing.T) {
			if _, blocked := ForcePushViolation(repo, "", []string{"-f", "origin", spec}); !blocked {
				t.Fatalf("force push of %s from main was allowed", spec)
			}
		})
	}
	checkoutBranch(t, repo, "feature")
	if _, blocked := ForcePushViolation(repo, "", []string{"-f", "origin", "HEAD"}); blocked {
		t.Error("force push of HEAD from a PR branch should be allowed")
	}
}

// TestForcePushViolation_ResolvesImplicitDestination covers a feature branch
// tracking origin/main under push.default=upstream, where `-f origin` updates
// main rather than the feature branch.
func TestForcePushViolation_ResolvesImplicitDestination(t *testing.T) {
	repo := newPolicyRepo(t)
	checkoutBranch(t, repo, "feature")
	setConfig(t, repo, "push.default", "upstream")
	setConfig(t, repo, "branch.feature.merge", "refs/heads/main")
	setConfig(t, repo, "branch.feature.remote", "origin")

	if _, blocked := ForcePushViolation(repo, "", []string{"-f", "origin"}); !blocked {
		t.Fatal("force push resolving to main via push.default=upstream was allowed")
	}

	setConfig(t, repo, "push.default", "matching")
	if _, blocked := ForcePushViolation(repo, "", []string{"-f", "origin"}); !blocked {
		t.Fatal("push.default=matching pushes every matching branch and must fail closed")
	}

	setConfig(t, repo, "push.default", "current")
	if _, blocked := ForcePushViolation(repo, "", []string{"-f", "origin"}); blocked {
		t.Error("push.default=current on a PR branch should be allowed")
	}
}

// TestForcePushViolation_RefusesWhenDefaultBranchUnknown covers a repository
// whose default is neither main nor master and whose origin/HEAD is absent.
func TestForcePushViolation_RefusesWhenDefaultBranchUnknown(t *testing.T) {
	repo := newPolicyRepo(t)
	cmd := exec.Command("git", "symbolic-ref", "-d", "refs/remotes/origin/HEAD")
	cmd.Dir = repo
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("removing origin/HEAD: %v\n%s", err, out)
	}
	checkoutBranch(t, repo, "feature")
	target, blocked := ForcePushViolation(repo, "", []string{"-f", "origin", "feature"})
	if !blocked {
		t.Fatal("force push was allowed while the default branch was unknown")
	}
	if target == "" {
		t.Error("the refusal should name the unknown default branch as the reason")
	}
}

// TestForcePushViolation_HonoursConfiguredProtectedBranches covers the shared
// integration branches that are not the default: they are not protected by
// name, and a repository opts in.
func TestForcePushViolation_HonoursConfiguredProtectedBranches(t *testing.T) {
	repo := newPolicyRepo(t)
	checkoutBranch(t, repo, "feature")

	if _, blocked := ForcePushViolation(repo, "", []string{"-f", "origin", "develop"}); blocked {
		t.Fatal("develop is not protected by default; the policy allows non-default branches")
	}
	setConfig(t, repo, "safegit.protectedbranch", "develop")
	if _, blocked := ForcePushViolation(repo, "", []string{"-f", "origin", "develop"}); !blocked {
		t.Fatal("a configured protected branch was force-pushed")
	}
}

// TestForcePushViolation_LeaseRefIsADestination covers
// `--force-with-lease=main`, where the lease names the ref being forced even
// though no operand repeats it.
func TestForcePushViolation_LeaseRefIsADestination(t *testing.T) {
	repo := newPolicyRepo(t)
	checkoutBranch(t, repo, "feature")
	if _, blocked := ForcePushViolation(repo, "", []string{"--force-with-lease=main", "origin", "feature"}); !blocked {
		t.Fatal("a lease naming main was allowed")
	}
	if _, blocked := ForcePushViolation(repo, "", []string{"--force-with-lease=feature", "origin", "feature"}); blocked {
		t.Error("a lease naming the PR branch should be allowed")
	}
}
