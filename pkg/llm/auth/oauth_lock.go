package auth

import (
	"fmt"
	"os"
	"path/filepath"
	"syscall"
	"time"
)

// credentialsLockName is the advisory lock file that serializes credential
// refreshes. It lives beside the credentials file (i.e.
// ~/.claude/.credentials.lock) so every process that shares the credentials
// file contends on the same lock.
const credentialsLockName = ".credentials.lock"

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
func withCredentialsLock(credPath string, timeout time.Duration, fn func() error) error {
	if timeout <= 0 {
		timeout = defaultLockTimeout
	}
	lockPath := filepath.Join(filepath.Dir(credPath), credentialsLockName)

	f, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return fmt.Errorf("open credentials lock %s: %w", lockPath, err)
	}
	defer f.Close()

	if err := acquireFlock(f, timeout); err != nil {
		return err
	}
	// Best-effort unlock: closing the fd (deferred above) also releases the
	// flock, so an unlock error here is non-fatal.
	defer func() { _ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN) }()

	return fn()
}

// acquireFlock takes an exclusive flock on f, retrying the non-blocking variant
// until it succeeds or the timeout elapses. A bounded poll loop (rather than a
// blocking LOCK_EX) keeps the wait observable and guarantees we never hang past
// the deadline.
func acquireFlock(f *os.File, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for {
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
		time.Sleep(lockPollInterval)
	}
}
