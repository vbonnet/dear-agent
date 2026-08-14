package safegit

import (
	"context"
	"errors"
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
