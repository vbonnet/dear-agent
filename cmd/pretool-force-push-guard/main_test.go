package main

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/vbonnet/dear-agent/internal/gittest"
)

// newRepo builds a real repository whose remote default branch is `main`, so
// the force-push policy resolves a genuine default rather than falling back to
// the conventional-name list. Returns the working clone's path.
func newRepo(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	remote := filepath.Join(root, "remote.git")
	work := filepath.Join(root, "work")
	gittest.Run(t, root, "init", "--bare", "--initial-branch=main", remote)
	gittest.Run(t, root, "clone", "--quiet", remote, work)
	// The clone was produced by git rather than by InitRepo, so it carries no
	// sandboxed hooks path of its own until this hardens it.
	gittest.HardenRepo(t, work)
	if err := os.WriteFile(filepath.Join(work, "f"), []byte("x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	gittest.Run(t, work, "add", "f")
	gittest.Run(t, work, "commit", "--quiet", "-m", "init")
	gittest.Run(t, work, "push", "--quiet", "-u", "origin", "main")
	gittest.Run(t, work, "remote", "set-head", "origin", "--auto")
	gittest.Run(t, work, "checkout", "--quiet", "-b", "feature/x")
	return work
}

func decide(t *testing.T, command, cwd string) (code int, stderr string) {
	t.Helper()
	env, err := json.Marshal(map[string]any{
		"tool_name":  "Bash",
		"cwd":        cwd,
		"tool_input": map[string]string{"command": command},
	})
	if err != nil {
		t.Fatal(err)
	}
	var out, errBuf bytes.Buffer
	code = run(context.Background(), bytes.NewReader(env), &out, &errBuf)
	return code, errBuf.String()
}

// The rule this guard exists to enforce, stated as its two halves. A rebase is
// only finishable if the first case is allowed, and main is only safe if the
// second is refused.
func TestForcePushAllowedOnFeatureBranchRefusedOnMain(t *testing.T) {
	repo := newRepo(t)
	allowed := []string{
		"git push --force-with-lease origin feature/x",
		"git push -f origin feature/x",
		"git push --force origin fix/bar",
		"git push -f origin stack/absence-alarm-cli-rebased",
		"safe-push --force-with-lease origin feature/x",
		"git push -f origin HEAD:refs/heads/feature/x",
		"git rebase origin/main && git push --force-with-lease origin feature/x",
		// The compound form that used to be refused because operands from
		// the two pushes were pooled into one argument list.
		"safe-push -u origin HEAD:main && safe-push -f origin feature/x",
	}
	for _, cmd := range allowed {
		if code, msg := decide(t, cmd, repo); code != 0 {
			t.Errorf("%q: exit %d, want 0 (allowed)\n%s", cmd, code, msg)
		}
	}

	refused := []string{
		"git push --force origin main",
		"git push -f origin main",
		"git push --force-with-lease origin main",
		"git push -f origin HEAD:main",
		"git push -f origin +main",
		"git push --force origin master",
		"git push --mirror origin",
		"git push -f origin 'refs/heads/*:refs/heads/*'",
	}
	for _, cmd := range refused {
		code, msg := decide(t, cmd, repo)
		if code != 2 {
			t.Errorf("%q: exit %d, want 2 (refused)", cmd, code)
			continue
		}
		if !strings.Contains(msg, "force-push") {
			t.Errorf("%q: message does not name the policy: %s", cmd, msg)
		}
	}
}

// A push on `main` itself is the case the default-branch check must catch even
// though no branch name appears on the command line.
func TestForcePushRefusedWhenDestinationIsImplicitMain(t *testing.T) {
	repo := newRepo(t)
	gittest.Run(t, repo, "checkout", "--quiet", "main")
	if code, _ := decide(t, "git push -f origin", repo); code != 2 {
		t.Errorf("implicit push to main: exit %d, want 2", code)
	}
}

// Reading about a force-push is not performing one. Text matching blocked
// these, which is how the guard came to look like a blanket ban.
func TestNonPushCommandsAreNotJudged(t *testing.T) {
	repo := newRepo(t)
	for _, cmd := range []string{
		`grep -rn "git push --force origin main" docs/`,
		`echo "never git push --force to main"`,
		"git push origin feature/x",
		"git log --oneline -3",
	} {
		if code, msg := decide(t, cmd, repo); code != 0 {
			t.Errorf("%q: exit %d, want 0\n%s", cmd, code, msg)
		}
	}
}

// The escape hatch is human-gated: present-and-substantial unlocks the local
// layer, absent or junk does not.
func TestProtectedForcePushApprovalGate(t *testing.T) {
	repo := newRepo(t)
	t.Setenv("OVERRIDE_AUDIT_DIR", t.TempDir())

	for _, reason := range []string{"", "x", "because", "....."} {
		t.Setenv(approvalEnv, reason)
		if code, _ := decide(t, "git push -f origin main", repo); code != 2 {
			t.Errorf("approval %q: exit %d, want 2 (still refused)", reason, code)
		}
	}

	t.Setenv(approvalEnv,
		"operator-approved history scrub of main to remove a leaked credential, ticket ce-0000")
	code, msg := decide(t, "git push -f origin main", repo)
	if code != 0 {
		t.Errorf("approved force-push to main: exit %d, want 0\n%s", code, msg)
	}
}

// The hook must never wedge the Bash tool for a reason unrelated to its policy.
func TestFailsOpenOnUnusableInput(t *testing.T) {
	repo := newRepo(t)
	if code, _ := decide(t, `git push -f origin "unterminated`, repo); code != 0 {
		t.Errorf("unparseable command: exit %d, want 0 (fail open)", code)
	}
	var out, errBuf bytes.Buffer
	if code := run(context.Background(), strings.NewReader("not json"), &out, &errBuf); code != 0 {
		t.Errorf("malformed envelope: exit %d, want 0 (fail open)", code)
	}
	env, _ := json.Marshal(map[string]any{
		"tool_name":  "Read",
		"tool_input": map[string]string{"command": "git push -f origin main"},
	})
	if code := run(context.Background(), bytes.NewReader(env), &out, &errBuf); code != 0 {
		t.Errorf("non-Bash tool: exit %d, want 0", code)
	}
}

// A refusal has to say what to do instead, not merely that nothing may happen.
func TestRefusalCarriesPositiveGuidance(t *testing.T) {
	repo := newRepo(t)
	_, msg := decide(t, "git push -f origin main", repo)
	for _, want := range []string{"main", "feature", approvalEnv} {
		if !strings.Contains(msg, want) {
			t.Errorf("refusal message lacks %q:\n%s", want, msg)
		}
	}
}
