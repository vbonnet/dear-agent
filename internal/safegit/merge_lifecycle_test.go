package safegit

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestMergeResultHeadChangeUsesTerminalSentinel(t *testing.T) {
	err := validateMergeResult(mergeResult{State: "MERGED", HeadRefOid: "changed"}, "abc123")
	if !errors.Is(err, errMergeHeadChanged) {
		t.Fatalf("validateMergeResult() error = %v, want errMergeHeadChanged", err)
	}
}

func TestConfirmMergedWithinSurvivesCallerCancellation(t *testing.T) {
	installMergeConfirmationFakeGH(t, `printf '%s\n' '{"state":"MERGED","headRefOid":"abc123"}'`)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if err := confirmMergedWithin(ctx, 10*time.Second, 42, "owner/repo", "abc123"); err != nil {
		t.Fatalf("confirmMergedWithin() error = %v", err)
	}
}

func TestConfirmMergedWithinBoundsBlockingProviderQuery(t *testing.T) {
	installMergeConfirmationFakeGH(t, `exec sleep 2`)
	err := confirmMergedWithin(context.Background(), 50*time.Millisecond,
		42, "owner/repo", "abc123")
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("confirmMergedWithin() error = %v, want context.DeadlineExceeded", err)
	}
}

func TestConfirmMergedWithinBoundsDescendantHeldPipe(t *testing.T) {
	installMergeConfirmationFakeGH(t, `sleep 3 &`)
	err := confirmMergedWithin(context.Background(), 30*time.Second,
		42, "owner/repo", "abc123")
	if !errors.Is(err, exec.ErrWaitDelay) {
		t.Fatalf("confirmMergedWithin() error = %v, want exec.ErrWaitDelay", err)
	}
}

type cancelBetweenPollsContext struct {
	context.Context
	done     chan struct{}
	errCalls int
}

func (c *cancelBetweenPollsContext) Done() <-chan struct{} { return c.done }

func (c *cancelBetweenPollsContext) Err() error {
	c.errCalls++
	if c.errCalls == 1 {
		// Model cancellation occurring immediately after the polling loop's
		// non-canceled observation and before it enters the wait select.
		close(c.done)
		return nil
	}
	return context.Canceled
}

func TestWaitForMergeCompletionChecksOnceAfterCancellationBetweenPolls(t *testing.T) {
	ctx := &cancelBetweenPollsContext{
		Context: context.Background(),
		done:    make(chan struct{}),
	}
	attempts := 0
	err := waitForMergeCompletion(ctx, time.Second, time.Hour, func() error {
		attempts++
		if attempts == 1 {
			return errMergePending
		}
		return nil
	})
	if err != nil {
		t.Fatalf("waitForMergeCompletion() error = %v", err)
	}
	if attempts != 2 {
		t.Fatalf("waitForMergeCompletion() attempts = %d, want final confirmation attempt", attempts)
	}
}

func installMergeConfirmationFakeGH(t *testing.T, body string) {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "gh")
	script := "#!/bin/sh\nset -eu\n" + body + "\n"
	if err := os.WriteFile(path, []byte(script), 0o700); err != nil {
		t.Fatalf("write fake gh: %v", err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
}

func TestWaitForMergeCompletionRetriesTransientConfirmationError(t *testing.T) {
	attempts := 0
	transient := errors.New("provider unavailable")
	err := waitForMergeCompletion(context.Background(), time.Second, time.Millisecond, func() error {
		attempts++
		if attempts < 3 {
			return transient
		}
		return nil
	})
	if err != nil {
		t.Fatalf("waitForMergeCompletion() error = %v", err)
	}
	if attempts != 3 {
		t.Fatalf("waitForMergeCompletion() attempts = %d, want 3", attempts)
	}
}

func TestWaitForMergeCompletionPrefersCallerCancellationAfterCheck(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	transient := errors.New("provider unavailable")
	err := waitForMergeCompletion(ctx, -time.Second, time.Hour, func() error {
		cancel()
		return transient
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("waitForMergeCompletion() error = %v, want context.Canceled", err)
	}
	if strings.Contains(err.Error(), "merge completion timeout") {
		t.Fatalf("caller cancellation was misreported as polling timeout: %v", err)
	}
}

func TestWatchMergeStopsOnCallerCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	attempts := 0
	err := watchMergeWithAttempt(ctx, MergeConfig{
		WatchTimeout:  time.Hour,
		WatchInterval: time.Hour,
	}, func(context.Context, MergeConfig) error {
		attempts++
		cancel()
		return errors.New("gate not ready")
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("watchMergeWithAttempt() error = %v, want context.Canceled", err)
	}
	if attempts != 1 {
		t.Fatalf("watchMergeWithAttempt() attempts = %d, want 1", attempts)
	}
}

func TestWatchMergePreservesEarlierParentDeadline(t *testing.T) {
	ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
	defer cancel()
	attempts := 0
	err := watchMergeWithAttempt(ctx, MergeConfig{
		WatchTimeout:  time.Hour,
		WatchInterval: time.Hour,
	}, func(context.Context, MergeConfig) error {
		attempts++
		return nil
	})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("watchMergeWithAttempt() error = %v, want context.DeadlineExceeded", err)
	}
	if strings.Contains(err.Error(), "watch timeout after 1h") {
		t.Fatalf("parent deadline was misreported as configured watch timeout: %v", err)
	}
	if attempts != 0 {
		t.Fatalf("watchMergeWithAttempt() attempts = %d, want 0", attempts)
	}
}
