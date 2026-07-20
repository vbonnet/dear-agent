package safepr

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

const worktreeCommandTimeout = 10 * time.Second

type worktreeLock struct {
	locked bool
	reason string
}

// WithWorktreeLock runs action while dir's linked worktree is protected from
// cleanup. A pre-existing lock is preserved exactly. Otherwise, the function
// acquires a uniquely owned lock and releases it on both success and failure.
func WithWorktreeLock(dir, reason string, action func() error) error {
	if action == nil {
		return fmt.Errorf("protect safe-pr worktree: action is required")
	}
	root, err := linkedWorktreeRoot(dir)
	if err != nil {
		return err
	}
	initial, err := inspectWorktreeLock(root)
	if err != nil {
		return err
	}
	if initial.locked {
		return runWithPreservedLock(root, initial, action)
	}

	ownedReason, err := worktreeLockReason(reason)
	if err != nil {
		return fmt.Errorf("protect safe-pr worktree: generate lock owner: %w", err)
	}
	if _, err := runWorktreeGit(root, "worktree", "lock", "--reason", ownedReason, root); err != nil {
		// Another owner may have won the check-to-lock race. Preserve that lock
		// instead of treating it as ours or unlocking it after the transaction.
		current, inspectErr := inspectWorktreeLock(root)
		if inspectErr == nil && current.locked {
			return runWithPreservedLock(root, current, action)
		}
		return errors.Join(fmt.Errorf("protect safe-pr worktree: acquire lock: %w", err), inspectErr)
	}

	return runWithOwnedLock(root, ownedReason, action)
}

func linkedWorktreeRoot(dir string) (string, error) {
	out, err := runWorktreeGit(dir, "rev-parse", "--path-format=absolute", "--show-toplevel", "--git-dir", "--git-common-dir")
	if err != nil {
		return "", fmt.Errorf("protect safe-pr worktree: resolve repository: %w", err)
	}
	lines := strings.Split(strings.TrimSpace(out), "\n")
	if len(lines) != 3 {
		return "", fmt.Errorf("protect safe-pr worktree: unexpected git directory output %q", out)
	}
	root := canonicalWorktreePath(lines[0])
	if canonicalWorktreePath(lines[1]) == canonicalWorktreePath(lines[2]) {
		return "", fmt.Errorf("safe-pr create requires a linked worktree that can be locked; %s is the primary checkout", root)
	}
	return root, nil
}

func inspectWorktreeLock(root string) (worktreeLock, error) {
	out, err := runWorktreeGit(root, "worktree", "list", "--porcelain")
	if err != nil {
		return worktreeLock{}, fmt.Errorf("protect safe-pr worktree: inspect lock: %w", err)
	}
	want := canonicalWorktreePath(root)
	for record := range strings.SplitSeq(strings.TrimSpace(out), "\n\n") {
		var path string
		state := worktreeLock{}
		for line := range strings.SplitSeq(record, "\n") {
			switch {
			case strings.HasPrefix(line, "worktree "):
				path = canonicalWorktreePath(strings.TrimPrefix(line, "worktree "))
			case line == "locked":
				state.locked = true
			case strings.HasPrefix(line, "locked "):
				state.locked = true
				state.reason = strings.TrimPrefix(line, "locked ")
			}
		}
		if path == want {
			return state, nil
		}
	}
	return worktreeLock{}, fmt.Errorf("protect safe-pr worktree: %s is not registered", root)
}

func canonicalWorktreePath(path string) string {
	path = filepath.Clean(path)
	resolved, err := filepath.EvalSymlinks(path)
	if err == nil {
		return filepath.Clean(resolved)
	}
	return path
}

func runWithPreservedLock(root string, initial worktreeLock, action func() error) error {
	actionErr := action()
	current, err := inspectWorktreeLock(root)
	if err != nil {
		return errors.Join(actionErr, err)
	}
	if !current.locked || current.reason != initial.reason {
		err = fmt.Errorf("protect safe-pr worktree: pre-existing lock changed during transaction; original reason %q, current reason %q", initial.reason, current.reason)
	}
	return errors.Join(actionErr, err)
}

func runWithOwnedLock(root, ownedReason string, action func() error) (err error) {
	defer func() {
		err = errors.Join(err, releaseOwnedWorktreeLock(root, ownedReason))
	}()
	return action()
}

func releaseOwnedWorktreeLock(root, ownedReason string) error {
	current, err := inspectWorktreeLock(root)
	if err != nil {
		return err
	}
	if !current.locked {
		return fmt.Errorf("protect safe-pr worktree: owned lock disappeared before release")
	}
	if current.reason != ownedReason {
		return fmt.Errorf("protect safe-pr worktree: lock ownership changed to %q; preserving it", current.reason)
	}
	if _, err := runWorktreeGit(root, "worktree", "unlock", root); err != nil {
		return fmt.Errorf("protect safe-pr worktree: release owned lock: %w", err)
	}
	return nil
}

func worktreeLockReason(reason string) (string, error) {
	owner := make([]byte, 8)
	if _, err := rand.Read(owner); err != nil {
		return "", err
	}
	reason = strings.TrimSpace(reason)
	if reason == "" {
		reason = "safe-pr transaction"
	}
	return fmt.Sprintf("safe-pr-owned:%d:%s:%s", os.Getpid(), hex.EncodeToString(owner), reason), nil
}

func runWorktreeGit(dir string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), worktreeCommandTimeout)
	defer cancel()
	cmd := newWorktreeGitCommand(ctx, dir, args...)
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
	out, err := cmd.CombinedOutput()
	if err != nil {
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return string(out), fmt.Errorf("git %s timed out after %s", strings.Join(args, " "), worktreeCommandTimeout)
		}
		return string(out), fmt.Errorf("git %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(string(out)))
	}
	return string(out), nil
}

func newWorktreeGitCommand(ctx context.Context, dir string, args ...string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, "git", append([]string{"-C", dir}, args...)...)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		if cmd.Process == nil {
			return nil
		}
		return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	}
	cmd.WaitDelay = time.Second
	return cmd
}
