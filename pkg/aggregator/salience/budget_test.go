package salience

import (
	"sync"
	"testing"
	"time"
)

func newClock(start time.Time) (func() time.Time, func(d time.Duration)) {
	cur := start
	var mu sync.Mutex
	now := func() time.Time {
		mu.Lock()
		defer mu.Unlock()
		return cur
	}
	advance := func(d time.Duration) {
		mu.Lock()
		cur = cur.Add(d)
		mu.Unlock()
	}
	return now, advance
}

func TestBudgetAllowsBypassTier(t *testing.T) {
	b := NewNotificationBudget(time.Hour, 3)
	for i := 0; i < 10; i++ {
		if !b.Allow(TierCritical) {
			t.Fatalf("critical signal %d should bypass capacity", i)
		}
	}
	if got := b.Used(); got != 0 {
		t.Errorf("bypassed signals should not consume budget, used=%d", got)
	}
}

func TestBudgetSuppressesBelowBypass(t *testing.T) {
	b := NewNotificationBudget(time.Hour, 2)
	if !b.Allow(TierLow) {
		t.Fatal("first low should pass")
	}
	if !b.Allow(TierLow) {
		t.Fatal("second low should pass")
	}
	if b.Allow(TierLow) {
		t.Error("third low should be suppressed (capacity exhausted)")
	}
	// High still bypasses regardless.
	if !b.Allow(TierHigh) {
		t.Error("high should bypass even when budget is full")
	}
}

func TestBudgetSlidingWindow(t *testing.T) {
	now, advance := newClock(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	b := NewNotificationBudget(time.Hour, 2)
	b.Now = now

	if !b.Allow(TierLow) {
		t.Fatal("first allow")
	}
	advance(30 * time.Minute)
	if !b.Allow(TierLow) {
		t.Fatal("second allow inside window")
	}
	if b.Allow(TierLow) {
		t.Fatal("third allow inside window should suppress")
	}

	// After enough time the first event falls out of the window and a
	// new low signal can fit.
	advance(31 * time.Minute) // total 61 min from t0; first event evicted
	if !b.Allow(TierLow) {
		t.Error("should allow after window slides past first event")
	}
}

func TestBudgetCapacityZeroDisablesSuppression(t *testing.T) {
	b := NewNotificationBudget(time.Hour, 0)
	for i := 0; i < 100; i++ {
		if !b.Allow(TierLow) {
			t.Fatalf("zero capacity should allow everything; failed at %d", i)
		}
	}
}

func TestBudgetNilReceiverAllowsAll(t *testing.T) {
	var b *NotificationBudget
	if !b.Allow(TierLow) {
		t.Error("nil budget should allow everything")
	}
	if got := b.Used(); got != 0 {
		t.Errorf("nil budget Used = %d", got)
	}
	b.Reset() // must not panic
}

func TestBudgetReset(t *testing.T) {
	b := NewNotificationBudget(time.Hour, 1)
	if !b.Allow(TierLow) {
		t.Fatal("first allow")
	}
	if b.Allow(TierLow) {
		t.Fatal("second should suppress")
	}
	b.Reset()
	if !b.Allow(TierLow) {
		t.Error("after reset, should allow again")
	}
}

func TestBudgetUsedTracksWindow(t *testing.T) {
	now, advance := newClock(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	b := NewNotificationBudget(time.Hour, 5)
	b.Now = now

	for i := 0; i < 3; i++ {
		_ = b.Allow(TierLow)
	}
	if got := b.Used(); got != 3 {
		t.Errorf("used = %d, want 3", got)
	}
	advance(2 * time.Hour)
	if got := b.Used(); got != 0 {
		t.Errorf("after window, used = %d, want 0", got)
	}
}

func TestBudgetCustomBypass(t *testing.T) {
	b := NewNotificationBudget(time.Hour, 1)
	b.BypassTier = TierMedium // medium and above bypass

	if !b.Allow(TierLow) {
		t.Fatal("low slot 1")
	}
	if b.Allow(TierLow) {
		t.Error("low slot 2 should suppress")
	}
	if !b.Allow(TierMedium) {
		t.Error("medium should bypass under custom config")
	}
}

func TestBudgetConcurrent(t *testing.T) {
	// Confirm Allow is safe under concurrent ingest. We can't assert
	// exact counts (timing-dependent eviction) but the data race
	// detector should stay clean and Used should not exceed capacity.
	b := NewNotificationBudget(time.Hour, 50)
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 20; j++ {
				_ = b.Allow(TierLow)
			}
		}()
	}
	wg.Wait()
	if got := b.Used(); got > 50 {
		t.Errorf("used = %d, exceeds capacity", got)
	}
}
