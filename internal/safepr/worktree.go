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
	"slices"
	"strconv"
	"strings"
	"syscall"
	"time"
)

const (
	worktreeCommandTimeout      = 10 * time.Second
	worktreeLockAttempts        = 3
	safePROwnedLockPrefix       = "safe-pr-owned:"
	worktreeTransactionLockFile = "safe-pr-transaction.guard"
	worktreeLockRetryInterval   = 10 * time.Millisecond
)

type worktreeLock struct {
	locked bool
	reason string
}

type linkedWorktree struct {
	root   string
	gitDir string
}

// WorktreeTransaction holds the operating-system lock that serializes safe-pr
// ownership. Commands protected with ProtectCommand inherit the open lock file,
// so the kernel keeps the transaction live if the safe-pr parent is killed
// while a preflight or GitHub child is still running.
type WorktreeTransaction struct {
	lockFile *os.File
}

// ProtectCommand makes cmd inherit the transaction lock. It is safe to call on
// a nil transaction for command paths, such as `safe-pr close`, that do not own
// a linked-worktree transaction.
func (t *WorktreeTransaction) ProtectCommand(cmd *exec.Cmd) error {
	if cmd == nil {
		return fmt.Errorf("protect safe-pr child command: command is required")
	}
	if t == nil {
		return nil
	}
	if t.lockFile == nil {
		return fmt.Errorf("protect safe-pr child command: transaction lock is unavailable")
	}
	cmd.ExtraFiles = append(cmd.ExtraFiles, t.lockFile)
	return nil
}

// WithWorktreeLock runs action while dir's linked worktree is protected from
// cleanup. A pre-existing lock is preserved exactly. Otherwise, the function
// acquires a uniquely owned lock and releases it on both success and failure.
func WithWorktreeLock(dir, reason string, action func() error) error {
	if action == nil {
		return fmt.Errorf("protect safe-pr worktree: action is required")
	}
	return WithWorktreeTransaction(dir, reason, func(_ *WorktreeTransaction) error {
		return action()
	})
}

// WithWorktreeTransaction runs action while dir's linked worktree is protected
// and exposes the transaction to child commands that must outlive a killed
// parent without releasing worktree ownership prematurely.
func WithWorktreeTransaction(dir, reason string, action func(*WorktreeTransaction) error) error {
	if action == nil {
		return fmt.Errorf("protect safe-pr worktree: transaction action is required")
	}
	worktree, err := resolveLinkedWorktree(dir)
	if err != nil {
		return err
	}
	// Acquisition, the caller's action, ownership verification, and release all
	// execute inside this operating-system critical section. A second safe-pr
	// owner cannot replace the Git lock between release inspection and unlock.
	return withWorktreeTransactionLock(worktree.gitDir, worktreeCommandTimeout, func(transaction *WorktreeTransaction) error {
		return withGitWorktreeLock(worktree.root, reason, transaction, action)
	})
}

func withGitWorktreeLock(root, reason string, transaction *WorktreeTransaction, action func(*WorktreeTransaction) error) error {
	var ownedReason string
	var lastErr error
	for range worktreeLockAttempts {
		current, inspectErr := inspectWorktreeLockProtected(transaction, root)
		if inspectErr != nil {
			return errors.Join(lastErr, inspectErr)
		}
		if current.locked {
			if !isSafePROwnedLockReason(current.reason) {
				return runWithPreservedLock(root, current, transaction, func() error { return action(transaction) })
			}
			if _, reclaimErr := runWorktreeGitProtected(transaction, root, "worktree", "unlock", root); reclaimErr != nil {
				lastErr = errors.Join(lastErr, fmt.Errorf("protect safe-pr worktree: reclaim stale owned lock: %w", reclaimErr))
			}
			continue
		}

		if ownedReason == "" {
			generated, reasonErr := worktreeLockReason(reason)
			if reasonErr != nil {
				return fmt.Errorf("protect safe-pr worktree: generate lock owner: %w", reasonErr)
			}
			ownedReason = generated
		}
		if _, lockErr := runWorktreeGitProtected(transaction, root, "worktree", "lock", "--reason", ownedReason, root); lockErr != nil {
			lastErr = errors.Join(lastErr, fmt.Errorf("protect safe-pr worktree: acquire lock: %w", lockErr))
			continue
		}
		return runWithOwnedLock(root, ownedReason, transaction, action)
	}
	return errors.Join(lastErr, fmt.Errorf("protect safe-pr worktree: lock ownership did not stabilize after %d attempts", worktreeLockAttempts))
}

func isSafePROwnedLockReason(reason string) bool {
	if !strings.HasPrefix(reason, safePROwnedLockPrefix) {
		return false
	}
	parts := strings.SplitN(strings.TrimPrefix(reason, safePROwnedLockPrefix), ":", 3)
	if len(parts) != 3 {
		return false
	}
	pid, err := strconv.Atoi(parts[0])
	if err != nil || pid <= 0 {
		return false
	}
	owner, err := hex.DecodeString(parts[1])
	if err != nil || len(owner) != 8 {
		return false
	}
	return true
}

func resolveLinkedWorktree(dir string) (linkedWorktree, error) {
	out, err := runWorktreeGit(dir, "rev-parse", "--path-format=absolute", "--show-toplevel", "--git-dir", "--git-common-dir")
	if err != nil {
		return linkedWorktree{}, fmt.Errorf("protect safe-pr worktree: resolve repository: %w", err)
	}
	return parseLinkedWorktree(out)
}

func parseLinkedWorktree(out string) (linkedWorktree, error) {
	out = strings.ReplaceAll(out, "\r", "")
	lines := slices.Collect(strings.SplitSeq(strings.TrimSpace(out), "\n"))
	if len(lines) != 3 {
		return linkedWorktree{}, fmt.Errorf("protect safe-pr worktree: unexpected git directory output %q", out)
	}
	root := canonicalWorktreePath(lines[0])
	gitDir := canonicalWorktreePath(lines[1])
	if gitDir == canonicalWorktreePath(lines[2]) {
		return linkedWorktree{}, fmt.Errorf("safe-pr create requires a linked worktree that can be locked; %s is the primary checkout", root)
	}
	return linkedWorktree{root: root, gitDir: gitDir}, nil
}

func withWorktreeTransactionLock(gitDir string, timeout time.Duration, action func(*WorktreeTransaction) error) (err error) {
	path := filepath.Join(gitDir, worktreeTransactionLockFile)
	// #nosec G703 -- gitDir is the canonical linked-worktree administrative directory returned by Git.
	file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return fmt.Errorf("protect safe-pr worktree: open transaction lock: %w", err)
	}
	defer func() {
		if closeErr := file.Close(); closeErr != nil {
			err = errors.Join(err, fmt.Errorf("protect safe-pr worktree: close transaction lock: %w", closeErr))
		}
	}()

	timer := time.NewTimer(timeout)
	defer timer.Stop()
	ticker := time.NewTicker(worktreeLockRetryInterval)
	defer ticker.Stop()
	for {
		lockErr := syscall.Flock(int(file.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
		if lockErr == nil {
			break
		}
		if !errors.Is(lockErr, syscall.EWOULDBLOCK) && !errors.Is(lockErr, syscall.EAGAIN) {
			return fmt.Errorf("protect safe-pr worktree: acquire transaction lock: %w", lockErr)
		}
		select {
		case <-timer.C:
			return fmt.Errorf("protect safe-pr worktree: another safe-pr transaction remained active after %s", timeout)
		case <-ticker.C:
		}
	}
	// Do not call LOCK_UN explicitly. Protected children inherit this open-file
	// description, so closing the parent descriptor releases the flock only
	// after the last surviving child descriptor closes as well.
	return action(&WorktreeTransaction{lockFile: file})
}

func inspectWorktreeLock(root string) (worktreeLock, error) {
	return inspectWorktreeLockProtected(nil, root)
}

func inspectWorktreeLockProtected(transaction *WorktreeTransaction, root string) (worktreeLock, error) {
	out, err := runWorktreeGitProtected(transaction, root, "worktree", "list", "--porcelain")
	if err != nil {
		return worktreeLock{}, fmt.Errorf("protect safe-pr worktree: inspect lock: %w", err)
	}
	return parseWorktreeLock(root, out)
}

func parseWorktreeLock(root, out string) (worktreeLock, error) {
	out = strings.ReplaceAll(out, "\r", "")
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

func runWithPreservedLock(root string, initial worktreeLock, transaction *WorktreeTransaction, action func() error) error {
	actionErr := action()
	current, err := inspectWorktreeLockProtected(transaction, root)
	if err != nil {
		return errors.Join(actionErr, err)
	}
	if !current.locked || current.reason != initial.reason {
		err = fmt.Errorf("protect safe-pr worktree: pre-existing lock changed during transaction; original reason %q, current reason %q", initial.reason, current.reason)
	}
	return errors.Join(actionErr, err)
}

func runWithOwnedLock(root, ownedReason string, transaction *WorktreeTransaction, action func(*WorktreeTransaction) error) (err error) {
	defer func() {
		err = errors.Join(err, releaseOwnedWorktreeLock(transaction, root, ownedReason))
	}()
	return action(transaction)
}

func releaseOwnedWorktreeLock(transaction *WorktreeTransaction, root, ownedReason string) error {
	current, err := inspectWorktreeLockProtected(transaction, root)
	if err != nil {
		return err
	}
	if !current.locked {
		return fmt.Errorf("protect safe-pr worktree: owned lock disappeared before release")
	}
	if current.reason != ownedReason {
		return fmt.Errorf("protect safe-pr worktree: lock ownership changed to %q; preserving it", current.reason)
	}
	if _, err := runWorktreeGitProtected(transaction, root, "worktree", "unlock", root); err != nil {
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
	return fmt.Sprintf("%s%d:%s:%s", safePROwnedLockPrefix, os.Getpid(), hex.EncodeToString(owner), reason), nil
}

func runWorktreeGit(dir string, args ...string) (string, error) {
	return runWorktreeGitProtected(nil, dir, args...)
}

func runWorktreeGitProtected(transaction *WorktreeTransaction, dir string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), worktreeCommandTimeout)
	defer cancel()
	cmd := newWorktreeGitCommand(ctx, dir, args...)
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
	if err := transaction.ProtectCommand(cmd); err != nil {
		return "", err
	}
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
	// #nosec G702 -- callers supply fixed internal Git subcommands and exec does not invoke a shell.
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
