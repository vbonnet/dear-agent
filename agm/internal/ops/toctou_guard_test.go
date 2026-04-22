package ops

import (
	"errors"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"syscall"
	"testing"
	"time"
)

func TestWithSessionLock_ExecutesFn(t *testing.T) {
	called := false
	err := WithSessionLock("test-exec-"+t.Name(), func() error {
		called = true
		return nil
	})
	if err != nil {
		t.Fatalf("WithSessionLock returned error: %v", err)
	}
	if !called {
		t.Error("fn was not called")
	}
}

func TestWithSessionLock_PropagatesFnError(t *testing.T) {
	sentinel := errors.New("boom")
	err := WithSessionLock("test-err-"+t.Name(), func() error {
		return sentinel
	})
	if !errors.Is(err, sentinel) {
		t.Fatalf("expected sentinel error, got: %v", err)
	}
}

func TestWithSessionLock_CreatesLockFile(t *testing.T) {
	session := "test-lockfile-" + t.Name()
	err := WithSessionLock(session, func() error {
		// Lock file should exist while fn is running.
		lockPath := filepath.Join(lockDir(), session+".lock")
		if _, err := os.Stat(lockPath); err != nil {
			t.Errorf("lock file does not exist during fn: %v", err)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("WithSessionLock returned error: %v", err)
	}
}

func TestWithSessionLock_MutualExclusion(t *testing.T) {
	session := "test-mutex-" + t.Name()

	var (
		mu        sync.Mutex
		maxConc   int32
		curConc   int32
		wg        sync.WaitGroup
		goroutines = 5
	)

	wg.Add(goroutines)
	for range goroutines {
		go func() {
			defer wg.Done()
			err := WithSessionLock(session, func() error {
				c := atomic.AddInt32(&curConc, 1)
				mu.Lock()
				if c > maxConc {
					maxConc = c
				}
				mu.Unlock()

				// Hold lock briefly to allow overlap attempts.
				time.Sleep(10 * time.Millisecond)

				atomic.AddInt32(&curConc, -1)
				return nil
			})
			if err != nil {
				t.Errorf("WithSessionLock returned error: %v", err)
			}
		}()
	}

	wg.Wait()

	if maxConc != 1 {
		t.Errorf("max concurrent executions = %d; want 1", maxConc)
	}
}

func TestWithSessionLockTimeout_TimesOut(t *testing.T) {
	session := "test-timeout-" + t.Name()

	// Acquire lock externally to block WithSessionLockTimeout.
	dir := lockDir()
	_ = os.MkdirAll(dir, 0o700)
	lockPath := filepath.Join(dir, session+".lock")

	f, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		t.Fatalf("failed to open lock file: %v", err)
	}
	defer f.Close()

	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX); err != nil {
		t.Fatalf("failed to acquire external lock: %v", err)
	}
	defer func() {
		_ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
	}()

	start := time.Now()
	err = WithSessionLockTimeout(session, 200*time.Millisecond, func() error {
		t.Error("fn should not have been called")
		return nil
	})
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("expected timeout error, got nil")
	}

	var opErr *OpError
	if !errors.As(err, &opErr) {
		t.Fatalf("expected *OpError, got %T: %v", err, err)
	}
	if opErr.Code != ErrCodeLockTimeout {
		t.Errorf("error code = %q; want %q", opErr.Code, ErrCodeLockTimeout)
	}

	// Should have waited at least the timeout duration.
	if elapsed < 150*time.Millisecond {
		t.Errorf("elapsed = %v; expected >= 150ms", elapsed)
	}
}

func TestWithSessionLock_ReleasesLockAfterFn(t *testing.T) {
	session := "test-release-" + t.Name()

	// First call acquires and releases.
	err := WithSessionLock(session, func() error { return nil })
	if err != nil {
		t.Fatalf("first call failed: %v", err)
	}

	// Second call should succeed immediately if lock was released.
	done := make(chan error, 1)
	go func() {
		done <- WithSessionLockTimeout(session, 500*time.Millisecond, func() error {
			return nil
		})
	}()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("second call failed: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("second call timed out — lock was not released")
	}
}

func TestWithSessionLock_IndependentSessions(t *testing.T) {
	// Two different sessions should not block each other.
	var wg sync.WaitGroup
	errs := make([]error, 2)

	wg.Add(2)
	for i, name := range []string{"session-a-" + t.Name(), "session-b-" + t.Name()} {
		go func() {
			defer wg.Done()
			errs[i] = WithSessionLockTimeout(name, 500*time.Millisecond, func() error {
				time.Sleep(50 * time.Millisecond)
				return nil
			})
		}()
	}

	wg.Wait()

	for i, err := range errs {
		if err != nil {
			t.Errorf("session %d failed: %v", i, err)
		}
	}
}
