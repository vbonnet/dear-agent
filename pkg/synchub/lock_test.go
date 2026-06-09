package synchub

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/vbonnet/dear-agent/pkg/a2a"
)

// TestLock_BasicAcquireRelease: round-trip via the public API.
func TestLock_BasicAcquireRelease(t *testing.T) {
	h := newTestHub(t, Options{})
	ctx := context.Background()
	hdl, err := h.Acquire(ctx, "input", "terminal", AcquireOptions{})
	if err != nil {
		t.Fatalf("Acquire: %v", err)
	}
	if _, ok := h.InspectLock("input"); !ok {
		t.Fatal("InspectLock: lock should be held")
	}
	if err := hdl.Release(); err != nil {
		t.Fatalf("Release: %v", err)
	}
	if _, ok := h.InspectLock("input"); ok {
		t.Fatal("InspectLock: lock should be free")
	}
}

// TestLock_NoReentry: same holder ID acquiring twice is rejected.
// This is the deadlock-prevention rule from the spec.
func TestLock_NoReentry(t *testing.T) {
	h := newTestHub(t, Options{LockTimeout: time.Second})
	ctx := context.Background()
	hdl, err := h.Acquire(ctx, "input", "terminal", AcquireOptions{})
	if err != nil {
		t.Fatalf("first acquire: %v", err)
	}
	t.Cleanup(func() { _ = hdl.Release() })

	_, err = h.Acquire(ctx, "input", "terminal", AcquireOptions{Timeout: 100 * time.Millisecond})
	if !errors.Is(err, ErrAlreadyHeld) {
		t.Fatalf("re-entry err=%v, want ErrAlreadyHeld", err)
	}
}

// TestLock_DifferentHoldersBlock: a different holder waits, then
// times out if the first never releases.
func TestLock_DifferentHoldersBlock(t *testing.T) {
	h := newTestHub(t, Options{LockTimeout: time.Hour}) // big default; we explicitly bound the wait
	ctx := context.Background()
	hdl, _ := h.Acquire(ctx, "input", "terminal", AcquireOptions{})
	t.Cleanup(func() { _ = hdl.Release() })

	start := time.Now()
	_, err := h.Acquire(ctx, "input", "desktop", AcquireOptions{Timeout: 150 * time.Millisecond})
	elapsed := time.Since(start)
	if !errors.Is(err, ErrLockTimeout) {
		t.Fatalf("err=%v, want ErrLockTimeout", err)
	}
	if elapsed < 100*time.Millisecond {
		t.Fatalf("returned too fast: %v", elapsed)
	}
}

// TestLock_AutoRelease: a holder that never releases has its lock
// reclaimed at the deadline; the next waiter then acquires.
func TestLock_AutoRelease(t *testing.T) {
	h := newTestHub(t, Options{LockTimeout: 100 * time.Millisecond})
	ctx := context.Background()
	hdl, _ := h.Acquire(ctx, "input", "terminal", AcquireOptions{Deadline: 100 * time.Millisecond})

	got := make(chan *LockHandle, 1)
	go func() {
		got2, err := h.Acquire(ctx, "input", "desktop", AcquireOptions{Timeout: time.Second})
		if err != nil {
			t.Errorf("desktop Acquire: %v", err)
			return
		}
		got <- got2
	}()

	select {
	case h2 := <-got:
		_ = h2.Release()
	case <-time.After(2 * time.Second):
		t.Fatal("desktop never got the lock")
	}

	// Original holder's Release is now a no-op (fence mismatch).
	if err := hdl.Release(); err != nil {
		t.Fatalf("stale Release should be no-op, got %v", err)
	}
}

// TestLock_StaleReleaseNoop: after auto-release reclaims a lock, the
// original holder's Release must NOT free a later acquisition.
func TestLock_StaleReleaseNoop(t *testing.T) {
	h := newTestHub(t, Options{LockTimeout: 50 * time.Millisecond})
	ctx := context.Background()

	hdl1, _ := h.Acquire(ctx, "k", "h1", AcquireOptions{Deadline: 50 * time.Millisecond})
	// Wait for sweeper auto-release.
	time.Sleep(200 * time.Millisecond)

	hdl2, err := h.Acquire(ctx, "k", "h2", AcquireOptions{})
	if err != nil {
		t.Fatalf("h2 Acquire: %v", err)
	}
	// Stale release from h1 must not free h2.
	_ = hdl1.Release()
	snap, ok := h.InspectLock("k")
	if !ok {
		t.Fatal("h2's lock was freed by h1's stale Release")
	}
	if snap.Fence != hdl2.Fence() {
		t.Fatalf("fence drift: snap=%d, hdl2=%d", snap.Fence, hdl2.Fence())
	}
	_ = hdl2.Release()
}

// TestLock_HighConcurrency: N goroutines hammer Acquire/Release on one
// key. Mutual exclusion holds.
func TestLock_HighConcurrency(t *testing.T) {
	h := newTestHub(t, Options{LockTimeout: 5 * time.Second})
	ctx := context.Background()
	const N = 32
	const Iter = 50
	var inCS atomic.Int32
	var max atomic.Int32
	var wg sync.WaitGroup
	for i := 0; i < N; i++ {
		i := i
		wg.Add(1)
		go func() {
			defer wg.Done()
			holder := string(rune('a' + i))
			for j := 0; j < Iter; j++ {
				hdl, err := h.Acquire(ctx, "shared", holder, AcquireOptions{Timeout: 10 * time.Second})
				if err != nil {
					t.Errorf("Acquire i=%d j=%d: %v", i, j, err)
					return
				}
				v := inCS.Add(1)
				if v > max.Load() {
					max.Store(v)
				}
				inCS.Add(-1)
				_ = hdl.Release()
			}
		}()
	}
	wg.Wait()
	if got := max.Load(); got != 1 {
		t.Fatalf("max concurrent holders=%d, want 1", got)
	}
}

// TestLock_NonBlocking: Timeout<0 returns immediately if held.
func TestLock_NonBlocking(t *testing.T) {
	h := newTestHub(t, Options{})
	ctx := context.Background()
	hdl, _ := h.Acquire(ctx, "k", "h1", AcquireOptions{})
	t.Cleanup(func() { _ = hdl.Release() })

	start := time.Now()
	_, err := h.Acquire(ctx, "k", "h2", AcquireOptions{Timeout: -1})
	if !errors.Is(err, ErrLockTimeout) {
		t.Fatalf("err=%v, want ErrLockTimeout", err)
	}
	if elapsed := time.Since(start); elapsed > 100*time.Millisecond {
		t.Fatalf("non-blocking took %v", elapsed)
	}
}

// TestLock_CtxCancellation: cancelling the parent context unblocks
// Acquire promptly.
func TestLock_CtxCancellation(t *testing.T) {
	h := newTestHub(t, Options{LockTimeout: time.Hour})
	ctx, cancel := context.WithCancel(context.Background())
	hdl, _ := h.Acquire(ctx, "k", "h1", AcquireOptions{})
	t.Cleanup(func() { _ = hdl.Release() })

	done := make(chan error, 1)
	go func() {
		_, err := h.Acquire(ctx, "k", "h2", AcquireOptions{Timeout: time.Hour})
		done <- err
	}()
	time.Sleep(50 * time.Millisecond)
	cancel()

	select {
	case err := <-done:
		if err == nil {
			t.Fatal("expected error after cancel")
		}
	case <-time.After(time.Second):
		t.Fatal("Acquire did not unblock on cancel")
	}
}

// TestLock_MonotonicClockSafety: a wall-clock jump backward must NOT
// cause locks to auto-release prematurely. We simulate this with the
// fake clock: rewind below the lock's acquired time, then sweep.
//
// In production, time.Time carries a monotonic reading and .Sub uses
// it. The fakeClock injection in tests doesn't carry monotonic readings,
// so this test specifically pins down behavior for the *sweeper math*:
// negative now-acquired durations must not be treated as "expired".
func TestLock_MonotonicClockSafety(t *testing.T) {
	base := time.Now()
	clock := &fakeClock{t: base}
	h := newTestHub(t, Options{
		LockTimeout: time.Hour,
		now:         clock.Now,
	})
	ctx := context.Background()
	hdl, _ := h.Acquire(ctx, "k", "h1", AcquireOptions{Deadline: time.Hour})

	// Wall-clock jumps backward by a day. Lock must still be held.
	clock.Advance(-24 * time.Hour)
	h.sweep()

	if _, ok := h.InspectLock("k"); !ok {
		t.Fatal("lock auto-released on wall-clock rewind")
	}
	_ = hdl.Release()
}

// TestLock_AcquireRespectsContextDeadline: a context with shorter
// deadline than Timeout wins.
func TestLock_AcquireRespectsContextDeadline(t *testing.T) {
	h := newTestHub(t, Options{LockTimeout: time.Hour})
	hdl, _ := h.Acquire(context.Background(), "k", "h1", AcquireOptions{})
	t.Cleanup(func() { _ = hdl.Release() })

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	start := time.Now()
	_, err := h.Acquire(ctx, "k", "h2", AcquireOptions{Timeout: time.Hour})
	if err == nil {
		t.Fatal("expected timeout")
	}
	if d := time.Since(start); d > 500*time.Millisecond {
		t.Fatalf("context deadline ignored: %v", d)
	}
}

// TestLock_ReleaseIdempotent: double Release returns nil the second time.
func TestLock_ReleaseIdempotent(t *testing.T) {
	h := newTestHub(t, Options{})
	hdl, _ := h.Acquire(context.Background(), "k", "h1", AcquireOptions{})
	if err := hdl.Release(); err != nil {
		t.Fatalf("first Release: %v", err)
	}
	if err := hdl.Release(); err != nil {
		t.Fatalf("second Release should be nil, got %v", err)
	}
}

// TestLock_PublishesEvents wires an A2A subscriber and checks lifecycle
// events arrive in order.
func TestLock_PublishesEvents(t *testing.T) {
	bus := a2a.NewInMemoryBus()
	h := newTestHub(t, Options{Bus: bus, SessionID: "lk"})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	acq, _ := bus.Subscribe(ctx, "synchub.lk.lock.acquired")
	rel, _ := bus.Subscribe(ctx, "synchub.lk.lock.released")

	hdl, _ := h.Acquire(ctx, "input", "terminal", AcquireOptions{})
	select {
	case <-acq:
	case <-time.After(time.Second):
		t.Fatal("no acquired event")
	}
	_ = hdl.Release()
	select {
	case <-rel:
	case <-time.After(time.Second):
		t.Fatal("no released event")
	}
}

// TestHub_NewRequiresSessionID
func TestHub_NewRequiresSessionID(t *testing.T) {
	_, err := New(Options{})
	if err == nil {
		t.Fatal("expected error for empty SessionID")
	}
}

// TestHub_CloseIsIdempotent
func TestHub_CloseIsIdempotent(t *testing.T) {
	h, _ := New(Options{SessionID: "x"})
	if err := h.Close(); err != nil {
		t.Fatalf("first Close: %v", err)
	}
	if err := h.Close(); err != nil {
		t.Fatalf("second Close: %v", err)
	}
}
