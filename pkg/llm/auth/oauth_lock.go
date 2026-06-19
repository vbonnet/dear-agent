//go:build unix

package auth

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"syscall"
	"time"
)

// defaultLockTimeout bounds how long a refresh waits for the credentials lock
// before giving up. A refresh exchange is a single sub-second HTTP round-trip,
// so a generous-but-finite wait lets a queued pane proceed while never hanging
// a session forever.
const defaultLockTimeout = 10 * time.Second

// lockPollInterval is how often a blocked waiter retries the non-blocking
// flock while waiting its turn.
const lockPollInterval = 50 * time.Millisecond

// withCredentialsLock acquires an exclusive, cross-process advisory lock on the
// lock file beside credPath, runs fn, then releases the lock.
//
// This is the fix for the refresh-token rotation race (ce-rnpt / ce-f3e3): the
// VROOM mesh runs three Claude Code sessions in separate tmux panes — separate
// OS processes — that share one credentials file. An in-process sync.Mutex
// cannot serialize them, so two panes could each spend the single-use refresh
// token: pane A rotates it, pane B then presents the now-invalidated token and
// gets 400 invalid_grant, poisoning the token family and cascading 401s across
// the whole mesh. A POSIX flock on a shared lock file is held against the open
// file description, so it serializes both separate processes AND separate
// goroutines (each opens its own descriptor), closing the race for good.
//
// ctx cancellation aborts the wait for the lock. The lock-file handle's Close
// error is surfaced (it shares the named return) so a failed close is never
// silently dropped.
func withCredentialsLock(ctx context.Context, credPath string, timeout time.Duration, fn func() error) (err error) {
	if timeout <= 0 {
		timeout = defaultLockTimeout
	}
	lockPath := filepath.Join(filepath.Dir(credPath), credentialsLockName)

	f, openErr := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0o600)
	if openErr != nil {
		return fmt.Errorf("open credentials lock %s: %w", lockPath, openErr)
	}
	defer func() {
		if closeErr := f.Close(); closeErr != nil && err == nil {
			err = fmt.Errorf("close credentials lock: %w", closeErr)
		}
	}()

	if lockErr := acquireFlock(ctx, f, timeout); lockErr != nil {
		return lockErr
	}
	// Best-effort unlock: closing the fd (deferred above) also releases the
	// flock, so an unlock error here is non-fatal.
	defer func() { _ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN) }()

	return fn()
}

// acquireFlock takes an exclusive flock on f, retrying the non-blocking variant
// until it succeeds, the timeout elapses, or ctx is canceled. A bounded poll
// loop (rather than a blocking LOCK_EX) keeps the wait observable, honors
// cancellation, and guarantees we never hang past the deadline.
func acquireFlock(ctx context.Context, f *os.File, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return fmt.Errorf("credentials lock wait canceled: %w", ctxErr)
		}
		err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
		if err == nil {
			return nil
		}
		if err != syscall.EWOULDBLOCK {
			return fmt.Errorf("acquire credentials lock: %w", err)
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("timed out after %s waiting for credentials lock (another process is refreshing)", timeout)
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("credentials lock wait canceled: %w", ctx.Err())
		case <-time.After(lockPollInterval):
		}
	}
}
