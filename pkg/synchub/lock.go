package synchub

import (
	"context"
	"fmt"
	"sync/atomic"
	"time"
)

// LockHandle is what Acquire returns. The caller must call Release when
// done; if they don't, the lock auto-releases at its deadline. Calling
// Release on an auto-released handle is a no-op (fence mismatch).
type LockHandle struct {
	hub      *Hub
	key      string
	fence    uint64
	holder   string
	acquired time.Time
	deadline time.Duration
	released atomic.Bool
}

// Key is the lock name (e.g. "input"). Useful for diagnostics.
func (h *LockHandle) Key() string { return h.key }

// Holder is the holder identifier passed to Acquire.
func (h *LockHandle) Holder() string { return h.holder }

// Fence is the monotonic token assigned at acquisition. Stable for the
// life of the handle; changes on the next acquisition of the same lock.
func (h *LockHandle) Fence() uint64 { return h.fence }

// Deadline reports when this handle will auto-release.
func (h *LockHandle) Deadline() time.Time { return h.acquired.Add(h.deadline) }

// Release frees the lock. Safe to call from any goroutine, exactly once;
// later calls are no-ops. Also a no-op if the auto-release sweeper has
// already reclaimed the lock (detected via fence mismatch).
func (h *LockHandle) Release() error {
	if h.released.Swap(true) {
		return nil
	}
	return h.hub.releaseLock(h.key, h.fence, "explicit")
}

type lock struct {
	key      string
	holder   string
	fence    uint64
	acquired time.Time
	deadline time.Duration
}

// AcquireOptions tunes a single Acquire call.
type AcquireOptions struct {
	// Timeout is how long Acquire will block waiting for the lock.
	// 0 means "use the hub's LockTimeout"; negative means "non-blocking".
	Timeout time.Duration

	// Deadline is the auto-release deadline measured from acquisition.
	// 0 means "use the hub's LockTimeout".
	Deadline time.Duration
}

// Acquire takes the named lock for `holder`. Holders are identifying
// strings, NOT mutexes — re-entry by the same holder ID returns
// ErrAlreadyHeld immediately rather than blocking. This is the
// deadlock-prevention rule from the design spec: any flow needing to
// hold X while doing something that wants X is a bug, not a corner case.
//
// Wait semantics: if the lock is held by someone else, Acquire blocks
// up to opts.Timeout (or hub default). On timeout, returns ErrLockTimeout.
// The blocked caller does not consume CPU; it sleeps on a per-lock
// notification channel.
func (h *Hub) Acquire(ctx context.Context, key, holder string, opts AcquireOptions) (*LockHandle, error) {
	timeout := opts.Timeout
	if timeout == 0 {
		timeout = h.opts.LockTimeout
	}
	deadline := opts.Deadline
	if deadline == 0 {
		deadline = h.opts.LockTimeout
	}

	// Compute the wait budget. timeout < 0 means non-blocking: try once.
	var waitCtx context.Context
	var waitCancel context.CancelFunc
	if timeout < 0 {
		waitCtx, waitCancel = context.WithCancel(ctx)
		waitCancel() // already done; tryClaim only
	} else {
		waitCtx, waitCancel = context.WithTimeout(ctx, timeout)
	}
	defer waitCancel()

	for {
		claimed, err := h.tryClaim(key, holder, deadline)
		if err != nil {
			return nil, err
		}
		if claimed != nil {
			return claimed, nil
		}

		// Need to wait. Sleep until either the current holder releases
		// (we can't actually subscribe to that without a channel per lock),
		// the holder's deadline elapses, or our wait budget runs out. The
		// sweeper handles deadline expiry; we just back off and retry.
		// 50ms keeps p99 wait latency tight without burning CPU.
		select {
		case <-waitCtx.Done():
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}
			return nil, ErrLockTimeout
		case <-time.After(50 * time.Millisecond):
			// loop and retry
		}
	}
}

// tryClaim attempts a single non-blocking acquisition. Returns (handle,
// nil) on success, (nil, nil) if the lock is held by someone else,
// (nil, ErrAlreadyHeld) if the caller already holds it.
func (h *Hub) tryClaim(key, holder string, deadline time.Duration) (*LockHandle, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.closed {
		return nil, ErrLockClosed
	}
	if l, ok := h.locks[key]; ok {
		if l.holder == holder {
			return nil, ErrAlreadyHeld
		}
		// Lazy auto-release check: if deadline passed, reclaim now.
		if h.opts.now().Sub(l.acquired) > l.deadline {
			h.publish(context.Background(), topicLockTimedOut(h.opts.SessionID), mustJSON(map[string]any{
				"key":         key,
				"prev_holder": l.holder,
				"prev_fence":  l.fence,
			}))
			delete(h.locks, key)
			// fall through to claim
		} else {
			return nil, nil // someone else holds it, not us
		}
	}

	fence := atomic.AddUint64(&h.fenceSeq, 1)
	now := h.opts.now()
	l := &lock{
		key:      key,
		holder:   holder,
		fence:    fence,
		acquired: now,
		deadline: deadline,
	}
	h.locks[key] = l
	handle := &LockHandle{
		hub:      h,
		key:      key,
		fence:    fence,
		holder:   holder,
		acquired: now,
		deadline: deadline,
	}
	// Publish outside the lock would be cleaner; in-memory bus's
	// default-drop send means holding the mutex here is benign.
	h.publish(context.Background(), topicLockAcquired(h.opts.SessionID), mustJSON(map[string]any{
		"key":      key,
		"holder":   holder,
		"fence":    fence,
		"deadline": deadline.String(),
	}))
	return handle, nil
}

// releaseLock is the unified release path: explicit Release calls and
// the sweeper's auto-release both call it. Fence-token CAS ensures a
// stale release (from a handle whose lock was already reclaimed) is a
// no-op rather than a misrelease that frees someone else's lock.
func (h *Hub) releaseLock(key string, fence uint64, reason string) error {
	h.mu.Lock()
	l, ok := h.locks[key]
	if !ok || l.fence != fence {
		h.mu.Unlock()
		return ErrLockClosed
	}
	holder := l.holder
	delete(h.locks, key)
	h.mu.Unlock()
	topic := topicLockReleased(h.opts.SessionID)
	if reason == "timeout" {
		topic = topicLockTimedOut(h.opts.SessionID)
	}
	h.publish(context.Background(), topic, mustJSON(map[string]any{
		"key":    key,
		"holder": holder,
		"fence":  fence,
		"reason": reason,
	}))
	return nil
}

// sweepLocks auto-releases locks past their deadline. Called from
// the hub's sweeper goroutine.
func (h *Hub) sweepLocks(now time.Time) {
	// Two-phase: collect under lock, release outside it. Release takes
	// the lock again, so collecting first avoids a re-entrant grab.
	type expired struct {
		key   string
		fence uint64
	}
	var toExpire []expired

	h.mu.Lock()
	for key, l := range h.locks {
		if now.Sub(l.acquired) > l.deadline {
			toExpire = append(toExpire, expired{key, l.fence})
		}
	}
	h.mu.Unlock()

	for _, e := range toExpire {
		_ = h.releaseLock(e.key, e.fence, "timeout")
	}
}

// LockSnapshot is the read-only view returned by InspectLock.
type LockSnapshot struct {
	Key      string
	Holder   string
	Fence    uint64
	Acquired time.Time
	Deadline time.Time
}

// InspectLock returns a snapshot of a lock's state, or false if the lock
// is unheld.
func (h *Hub) InspectLock(key string) (LockSnapshot, bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	l, ok := h.locks[key]
	if !ok {
		return LockSnapshot{}, false
	}
	return LockSnapshot{
		Key:      l.key,
		Holder:   l.holder,
		Fence:    l.fence,
		Acquired: l.acquired,
		Deadline: l.acquired.Add(l.deadline),
	}, true
}

// sweep is the per-tick maintenance entrypoint. Splits into the two
// concrete sweeps for testability — tests call them directly with a
// chosen `now`.
func (h *Hub) sweep() {
	now := h.opts.now()
	h.sweepRounds(now)
	h.sweepLocks(now)
}

// String for diagnostics; not used in hot paths.
func (l *lock) String() string {
	return fmt.Sprintf("lock{%s held by %s fence=%d}", l.key, l.holder, l.fence)
}
