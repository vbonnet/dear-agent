package git

import (
	"fmt"
	"os/exec"
	"strings"
)

// BranchPreservation is the verdict on whether a local branch may be deleted
// by automated cleanup, and why.
type BranchPreservation struct {
	// Preserve is true when the branch must survive cleanup.
	Preserve bool
	// Reason is a short human-readable explanation, suitable for a cleanup
	// audit record. Empty when Preserve is false.
	Reason string
}

// PreserveLocalBranch reports whether automated cleanup may delete a local
// branch.
//
// The rule is an allowlist on deletion, in this order:
//
//  1. An unnamed branch cannot be reasoned about, so it is preserved.
//  2. A confirmed OPEN pull request preserves the branch. The work is in
//     flight, not abandoned, and stripping the local ref is needlessly
//     destructive.
//  3. A confirmed MERGED pull request authorizes deletion. This is the
//     squash-merge case and it must stay deletable: after a squash merge the
//     local commits exist on no remote, so the unpushed test below would
//     otherwise preserve every landed branch forever.
//  4. Otherwise, commits that exist on no remote preserve the branch. Those
//     commits are unrecoverable once the ref is gone.
//
// Rule 4 applies only when the repository actually has a remote. In a repo
// with none, every commit trivially exists on no remote, so the test carries
// no information and would preserve every branch forever; there is also no
// remote copy whose loss the rule is protecting against.
//
// Every probe is fail-closed. gh being missing, unauthenticated, or slow
// leaves the PR state unknown, and an unknown state never authorizes a
// deletion by itself; a failing rev-list is treated as "has unpushed work".
func PreserveLocalBranch(repoPath, branch string) BranchPreservation {
	if branch == "" {
		return BranchPreservation{Preserve: true, Reason: "branch name unknown"}
	}
	if num, open := OpenPRForBranch(repoPath, branch); open {
		return BranchPreservation{Preserve: true, Reason: fmt.Sprintf("open PR #%d", num)}
	}
	if merged, known := PRMergedState(repoPath, branch); known && merged {
		return BranchPreservation{}
	}
	if !hasRemotes(repoPath) {
		return BranchPreservation{}
	}
	unpushed, err := HasUnpushedCommits(repoPath, branch)
	if err != nil {
		return BranchPreservation{Preserve: true, Reason: fmt.Sprintf("could not verify pushed state: %v", err)}
	}
	if unpushed {
		return BranchPreservation{Preserve: true, Reason: "carries commits that exist on no remote"}
	}
	return BranchPreservation{}
}

// hasRemotes reports whether the repository has at least one configured
// remote. It fails closed: an unreadable repository is treated as having a
// remote, so the unpushed-commit rule still applies and cleanup stays
// conservative.
func hasRemotes(repoPath string) bool {
	out, err := exec.Command("git", "-C", repoPath, "remote").Output()
	if err != nil {
		return true
	}
	return strings.TrimSpace(string(out)) != ""
}
