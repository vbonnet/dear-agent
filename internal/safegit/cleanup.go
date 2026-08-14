package safegit

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
)

const (
	// cleanupCommandTimeout bounds each best-effort local Git cleanup operation.
	// Cleanup runs after provider merge success, so it must never hold the caller
	// indefinitely even when a local worktree or Git process is unhealthy.
	cleanupCommandTimeout = 30 * time.Second
	// cleanupCommandWaitDelay bounds pipe draining when a Git descendant keeps
	// stdout or stderr open after the direct Git process exits.
	cleanupCommandWaitDelay = time.Second
)

// cleanupPlan retains a local Git command root while the provider merge is
// allowed to remove the worktree from which safe-merge was invoked.
type cleanupPlan struct {
	branch          string
	primaryWorktree string
	prepareErr      error
}

// prepareCleanupPlan captures a stable Git command root before provider merge.
// Preparation failure is retained for a post-merge warning because local
// cleanup remains best-effort and must not change provider merge truth.
func prepareCleanupPlan(ctx context.Context, branch string) cleanupPlan {
	plan := cleanupPlan{branch: branch}
	if branch == "" {
		return plan
	}

	primary, err := listCleanupWorktrees(ctx, "")
	if err != nil {
		plan.prepareErr = fmt.Errorf("git worktree list: %w", err)
		return plan
	}
	if primary == "" {
		plan.prepareErr = fmt.Errorf("git worktree list returned no primary worktree")
		return plan
	}

	plan.primaryWorktree = primary
	return plan
}

type providerMergeStage uint8

const (
	providerMergeCommandStage providerMergeStage = iota + 1
	providerMergeConfirmationStage
)

type providerMergeFailure struct {
	stage providerMergeStage
	err   error
}

// runProviderMergeTransaction owns the provider-mutation lifetime: it captures
// local cleanup context, runs the provider, requires exact-head confirmation,
// records confirmation, and only then attempts best-effort local cleanup.
func runProviderMergeTransaction(
	ctx context.Context,
	branch string,
	mergeArgs []string,
	confirm func() error,
	onConfirmed func(),
) *providerMergeFailure {
	plan := prepareCleanupPlan(ctx, branch)
	if err := ctx.Err(); err != nil {
		return &providerMergeFailure{stage: providerMergeCommandStage, err: err}
	}
	mergeCmd := exec.Command(mergeArgs[0], mergeArgs[1:]...)
	mergeCmd.Stdout = os.Stdout
	mergeCmd.Stderr = os.Stderr
	if err := mergeCmd.Run(); err != nil {
		return &providerMergeFailure{stage: providerMergeCommandStage, err: err}
	}
	if err := confirm(); err != nil {
		return &providerMergeFailure{stage: providerMergeConfirmationStage, err: err}
	}
	if onConfirmed != nil {
		onConfirmed()
	}
	plan.run(ctx)
	return nil
}

func (plan cleanupPlan) run(ctx context.Context) {
	if plan.branch == "" {
		return
	}
	if plan.prepareErr != nil {
		fmt.Fprintf(os.Stderr, "safe-merge: cleanup: pre-merge context: %v\n", plan.prepareErr)
		return
	}
	cleanupWorktree(ctx, plan.primaryWorktree, plan.branch)
}

// cleanupWorktree removes local worktrees tracking branch and then deletes the
// local branch. All Git commands start from the pre-provider primary worktree.
// Failures are warnings because the provider merge is already confirmed.
func cleanupWorktree(ctx context.Context, commandRoot, branch string) {
	out, err := runCleanupGit(ctx, commandRoot, "worktree", "list", "--porcelain", "-z")
	if err != nil {
		fmt.Fprintf(os.Stderr, "safe-merge: cleanup: git worktree list: %v\n", err)
		return
	}

	// Parse NUL-delimited porcelain fields. Each block has "worktree <path>",
	// optionally "branch refs/heads/<branch>", followed by an empty field.
	// The very first worktree block is always the main worktree — never remove it.
	var toRemove []string
	var mainWorktree, currentPath string
	for field := range bytes.SplitSeq(out, []byte{0}) {
		if after, ok := bytes.CutPrefix(field, []byte("worktree ")); ok {
			currentPath = string(after)
			if mainWorktree == "" {
				mainWorktree = currentPath
			}
		} else if bytes.Equal(field, []byte("branch refs/heads/"+branch)) && currentPath != "" {
			if currentPath != mainWorktree {
				toRemove = append(toRemove, currentPath)
			}
			currentPath = ""
		}
	}
	if mainWorktree == "" {
		fmt.Fprintln(os.Stderr, "safe-merge: cleanup: git worktree list returned no primary worktree")
		return
	}

	for _, path := range toRemove {
		fmt.Fprintf(os.Stderr, "safe-merge: cleanup: removing worktree %s\n", path)
		if out, err := runCleanupGit(ctx, mainWorktree, "worktree", "remove", "--", path); err != nil {
			fmt.Fprintf(os.Stderr, "safe-merge: cleanup: worktree remove %s: %v: %s\n", path, err, out)
		}
	}

	// -d refuses to delete an unmerged branch. Suppress only branch-not-found,
	// which is expected when the provider already removed the local branch.
	if out, err := runCleanupGit(ctx, mainWorktree, "branch", "-d", "--", branch); err != nil {
		if !strings.Contains(string(out), "not found") {
			fmt.Fprintf(os.Stderr, "safe-merge: cleanup: branch -d %s: %s\n", branch, strings.TrimSpace(string(out)))
		}
	} else {
		fmt.Fprintf(os.Stderr, "safe-merge: cleanup: removed local branch %s\n", branch)
	}
}

func listCleanupWorktrees(ctx context.Context, dir string) (string, error) {
	out, err := runCleanupGit(ctx, dir, "worktree", "list", "--porcelain", "-z")
	if err != nil {
		return "", err
	}
	for field := range bytes.SplitSeq(out, []byte{0}) {
		if path, ok := bytes.CutPrefix(field, []byte("worktree ")); ok {
			return string(path), nil
		}
	}
	return "", nil
}

func runCleanupGit(ctx context.Context, dir string, args ...string) ([]byte, error) {
	commandCtx, cancel := context.WithTimeout(ctx, cleanupCommandTimeout)
	defer cancel()

	// #nosec G204,G702 -- executable name is fixed; internal Git argv uses
	// explicit option terminators before provider- or repository-derived values.
	cmd := exec.CommandContext(commandCtx, "git", args...)
	cmd.Dir = dir
	cmd.WaitDelay = cleanupCommandWaitDelay
	out, err := cmd.CombinedOutput()
	if err != nil && commandCtx.Err() != nil {
		return out, fmt.Errorf("git cleanup command: %w", commandCtx.Err())
	}
	return out, err
}
