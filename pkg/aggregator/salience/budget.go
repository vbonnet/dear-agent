package salience

import (
	"sync"
	"time"
)

// NotificationBudget caps how many notifications fire inside a sliding
// time window so a flood of low-salience drift can't drown out the
// high-salience signals a human actually needs to see.
//
// Signals at or above BypassTier are always allowed and never count
// against the budget — this is what lets a build_failure interrupt even
// when the window is full of cosmetic noise.
type NotificationBudget struct {
	// Window is the lookback duration for the sliding counter. Events
	// older than now-Window are evicted on each Allow call.
	Window time.Duration
	// Capacity is the maximum number of budget-counted notifications
	// allowed within Window. Capacity <= 0 disables suppression entirely
	// (every signal allowed); think of it as "off".
	Capacity int
	// BypassTier is the inclusive threshold at or above which a signal
	// is always allowed and does not consume a slot. Default zero value
	// (TierNoise) means *every* signal bypasses, which is rarely what
	// you want — NewNotificationBudget defaults this to TierHigh.
	BypassTier Tier

	// Now is overridable so tests can drive the sliding window with a
	// deterministic clock. Production leaves it nil; the budget uses
	// time.Now.
	Now func() time.Time

	mu     sync.Mutex
	events []time.Time
}

// NewNotificationBudget builds a budget with sensible defaults: any
// signal at TierHigh or above bypasses the budget. Pass capacity <= 0
// to disable suppression.
func NewNotificationBudget(window time.Duration, capacity int) *NotificationBudget {
	return &NotificationBudget{
		Window:     window,
		Capacity:   capacity,
		BypassTier: TierHigh,
	}
}

// Allow reports whether a signal of the given salience should fire a
// notification. When it returns true and the signal is below BypassTier,
// a slot is consumed. Concurrent callers are safe.
func (b *NotificationBudget) Allow(salience Tier) bool {
	if b == nil {
		return true
	}
	if b.Capacity <= 0 {
		return true
	}
	if salience >= b.BypassTier {
		return true
	}

	now := b.now()

	b.mu.Lock()
	defer b.mu.Unlock()

	b.evictLocked(now)
	if len(b.events) >= b.Capacity {
		return false
	}
	b.events = append(b.events, now)
	return true
}

// Used reports the number of slots currently consumed within the window.
// Useful for tests and for the CLI's "X of Y notifications used" line.
func (b *NotificationBudget) Used() int {
	if b == nil {
		return 0
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	b.evictLocked(b.now())
	return len(b.events)
}

// Reset clears the sliding-window state. Tests use this between cases;
// production rarely needs it because eviction handles the steady state.
func (b *NotificationBudget) Reset() {
	if b == nil {
		return
	}
	b.mu.Lock()
	b.events = b.events[:0]
	b.mu.Unlock()
}

func (b *NotificationBudget) now() time.Time {
	if b.Now != nil {
		return b.Now()
	}
	return time.Now()
}

// evictLocked drops events older than the sliding-window cutoff. Caller
// must hold b.mu.
func (b *NotificationBudget) evictLocked(now time.Time) {
	if b.Window <= 0 || len(b.events) == 0 {
		return
	}
	cutoff := now.Add(-b.Window)
	keep := 0
	for _, t := range b.events {
		if t.After(cutoff) {
			b.events[keep] = t
			keep++
		}
	}
	b.events = b.events[:keep]
}
