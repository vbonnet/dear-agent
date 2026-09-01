package main

import (
	"fmt"
	"strings"
	"testing"
)

// ---- ce-00u1: the rebase call site ----
//
// mergeloop invoked `safe-rebase --pr <n> --repo <owner/name>`. safe-rebase has
// no such flags: it takes -C, --base, --auto, --dry-run, --timeout, and it
// rebases THE CURRENT BRANCH in a repo directory. mergeloop runs from $HOME
// with no per-PR checkout, so the tool was structurally wrong for this call
// site, not merely mis-flagged.
//
// Every rebase therefore failed instantly with `unknown flag "--pr"`, and the
// BEHIND queue could never drain. Confirmed live on 2026-09-01: all 10 open
// dependabot PRs failed inside one tick, seconds apart.

func TestRebaseCommandUsesGitHubSideUpdateBranch(t *testing.T) {
	name, args := rebaseCommand("vbonnet/dear-agent", 1423)

	if name != "gh" {
		t.Fatalf("rebase command = %q, want gh: safe-rebase needs a checkout mergeloop does not have", name)
	}
	joined := strings.Join(args, " ")
	for _, want := range []string{"pr", "update-branch", "--rebase", "1423", "--repo", "vbonnet/dear-agent"} {
		if !strings.Contains(joined, want) {
			t.Errorf("args %v missing %q", args, want)
		}
	}
}

func TestRebaseCommandNeverInvokesSafeRebase(t *testing.T) {
	// A regression guard with teeth: the old code preferred safe-rebase
	// whenever exec.LookPath found it, so the broken path was taken
	// unconditionally on any host where that binary exists, which is this one.
	name, args := rebaseCommand("vbonnet/dear-agent", 7)
	if strings.Contains(name, "safe-rebase") {
		t.Fatalf("rebase command = %q, want no safe-rebase", name)
	}
	for _, a := range args {
		if strings.Contains(a, "safe-rebase") {
			t.Fatalf("args %v reference safe-rebase", args)
		}
	}
}

func TestRebaseCommandOmitsEmptyRepo(t *testing.T) {
	// gh resolves the repo from cwd when --repo is absent. Passing the flag
	// with an empty value would make gh fail on a malformed flag instead.
	_, args := rebaseCommand("", 9)
	for i, a := range args {
		if a == "--repo" {
			t.Fatalf("args %v pass --repo with no value (index %d)", args, i)
		}
	}
}

// TestCommandErrorNamesTheRealFailure pins the audit-legibility half. The old
// error was a bare "safe-rebase: exit status 1"; the actual message,
// `unknown flag "--pr"`, went only to stderr, so the audit record carried an
// exit code and nothing else. That is the same fail-opaque shape as the
// golangci exit-3 laundering, and it is why this defect survived so long.
func TestCommandErrorNamesTheRealFailure(t *testing.T) {
	err := commandError("gh", fmt.Errorf("exit status 1"),
		"gh: unknown flag \"--pr\"\nsee gh --help\n")
	if err == nil {
		t.Fatal("commandError returned nil")
	}
	if !strings.Contains(err.Error(), `unknown flag "--pr"`) {
		t.Errorf("error %q does not carry the real failure message", err)
	}
	if !strings.Contains(err.Error(), "exit status 1") {
		t.Errorf("error %q dropped the exit status", err)
	}
}

func TestCommandErrorSurvivesEmptyStderr(t *testing.T) {
	err := commandError("gh", fmt.Errorf("exit status 2"), "   \n  ")
	if err == nil {
		t.Fatal("commandError returned nil")
	}
	if !strings.Contains(err.Error(), "exit status 2") {
		t.Errorf("error %q lost the exit status when stderr was blank", err)
	}
}

func TestCommandErrorBoundsStderr(t *testing.T) {
	// A runaway subprocess must not write an unbounded line into the audit log.
	huge := strings.Repeat("x", 10000)
	err := commandError("gh", fmt.Errorf("exit status 1"), huge)
	if len(err.Error()) > 1200 {
		t.Errorf("error length %d, want it bounded", len(err.Error()))
	}
}
