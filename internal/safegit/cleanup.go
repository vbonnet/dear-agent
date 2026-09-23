package safegit

import (
	"bytes"
	"context"
	"errors"
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

// indeterminateConfirmWindowEnv lets tests shorten the indeterminate-outcome
// poll. The helper that exercises the transaction runs as a subprocess, so a
// package variable cannot reach it.
const indeterminateConfirmWindowEnv = "SAFEGIT_INDETERMINATE_CONFIRM_WINDOW"

// indeterminateConfirmWindow bounds the poll that separates an accepted merge
// from a rejected one after a nonzero provider exit.
func indeterminateConfirmWindow(fallback time.Duration) time.Duration {
	if v := os.Getenv(indeterminateConfirmWindowEnv); v != "" {
		if d, err := time.ParseDuration(v); err == nil && d > 0 {
			return d
		}
	}
	return fallback
}

// indeterminateConfirmTimeout bounds the poll that distinguishes an accepted
// merge from a rejected one after the provider command exits nonzero. It is
// short because a genuinely rejected merge pays the whole window before its
// failure is reported.
const indeterminateConfirmTimeout = 45 * time.Second

// errNoIndeterminateProbe stands in when the caller supplied no probe, so the
// switch below has one shape for "the probe did not confirm" regardless of
// whether a probe existed.
var errNoIndeterminateProbe = errors.New("no indeterminate probe supplied")

// indeterminateProbe builds the bounded poll that separates an accepted merge
// from a rejected one. Polling rather than a single read, because an accepted
// async merge completes out of band: the first observation after a lost
// response is commonly still OPEN.
func indeterminateProbe(ctx context.Context, interval time.Duration, confirm func() error) func() error {
	// Detached from the caller's cancellation on purpose. Cancellation is one
	// of the ways a merge becomes indeterminate: the request can be accepted
	// and the caller canceled before the response lands. A probe that inherits
	// that cancellation reports failure on a merge that already happened, and
	// safe-merge then skips cleanup and lets watch mode retry it. The window is
	// what keeps this bounded, so a canceled caller waits at most
	// indeterminateConfirmTimeout rather than indefinitely.
	detached := context.WithoutCancel(ctx)
	return func() error {
		return waitForMergeCompletion(detached,
			indeterminateConfirmWindow(indeterminateConfirmTimeout), interval, confirm)
	}
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
	// Optional: a bounded exact-head read used only to tell an accepted merge
	// apart from a rejected one when the provider command exits nonzero.
	probeIndeterminate ...func() error,
) *providerMergeFailure {
	plan := prepareCleanupPlan(ctx, branch)
	if err := ctx.Err(); err != nil {
		return &providerMergeFailure{stage: providerMergeCommandStage, err: err}
	}
	mergeCmd := exec.CommandContext(ctx, mergeArgs[0], mergeArgs[1:]...)
	mergeCmd.Stdout = os.Stdout
	mergeCmd.Stderr = os.Stderr
	// A successful indeterminate probe is itself an exact-head confirmation, so
	// it stands in for the confirmation below rather than being repeated: a
	// second full poll can fail on a later provider read or an expiring caller
	// deadline and turn an already-proven merge into a reported failure.
	confirmed := false
	if err := mergeCmd.Run(); err != nil {
		// A nonzero exit does not prove the provider rejected the mutation. The
		// request can be accepted while the response is lost, which the async
		// route makes likelier because it completes out of band. Ask the
		// provider before calling the attempt failed: reporting failure on an
		// accepted merge skips cleanup and invites watch mode to retry it.
		//
		// Cancellation is not proof either, so the probe runs in that case too.
		// It is detached and bounded, which is what makes the distinction
		// possible at all: a probe carrying the caller's cancellation returns
		// on its first read and answers the question with the very condition
		// that made it indeterminate.
		probeErr := errNoIndeterminateProbe
		if len(probeIndeterminate) > 0 && probeIndeterminate[0] != nil {
			probeErr = probeIndeterminate[0]()
		}
		switch {
		case probeErr == nil:
			confirmed = true
			fmt.Fprintln(os.Stderr,
				"safe-merge: provider command failed but the merge is confirmed at the gated head; continuing")
		case errors.Is(probeErr, errNoIndeterminateProbe):
			// No rescue probe was supplied, so there is nothing to ask. A
			// clean nonzero exit is then a plain failure, while cancellation
			// still leaves the outcome indeterminate and falls through to the
			// caller's own confirmation below.
			if ctx.Err() == nil {
				return &providerMergeFailure{stage: providerMergeCommandStage, err: err}
			}
		case errors.Is(probeErr, errMergeHeadChanged):
			// Terminal, and it must stay terminal. The PR merged at a head
			// other than the one the gates cleared. Only an error still
			// carrying this sentinel stops watch mode, so returning the
			// command error here would downgrade an exact-head safety
			// violation into a retryable provider failure and let the wrapper
			// keep working on a PR that already merged as something else.
			return &providerMergeFailure{stage: providerMergeConfirmationStage, err: probeErr}
		default:
			return &providerMergeFailure{stage: providerMergeCommandStage, err: err}
		}
	}
	if !confirmed {
		if err := confirm(); err != nil {
			return &providerMergeFailure{stage: providerMergeConfirmationStage, err: err}
		}
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
