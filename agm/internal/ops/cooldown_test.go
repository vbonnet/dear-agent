package ops

import (
	"testing"
	"time"
)

func TestCooldownTracker_FirstActionAlwaysAllowed(t *testing.T) {
	ct := NewCooldownTracker()

	for _, action := range []string{"kill", "restart", "archive", "force-kill"} {
		if !ct.CanAct(action, DefaultCooldown()) {
			t.Errorf("CanAct(%q) = false on first call; want true", action)
		}
	}
}

func TestCooldownTracker_BlocksWithinCooldown(t *testing.T) {
	ct := NewCooldownTracker()
	frozen := time.Date(2026, 4, 10, 12, 0, 0, 0, time.UTC)
	ct.now = func() time.Time { return frozen }

	ct.RecordAction("force-kill")

	// Still within cooldown — advance only 1 minute.
	ct.now = func() time.Time { return frozen.Add(1 * time.Minute) }
	if ct.CanAct("force-kill", DefaultCooldown()) {
		t.Error("CanAct(force-kill) = true within cooldown; want false")
	}
}

func TestCooldownTracker_AllowsAfterCooldown(t *testing.T) {
	ct := NewCooldownTracker()
	frozen := time.Date(2026, 4, 10, 12, 0, 0, 0, time.UTC)
	ct.now = func() time.Time { return frozen }

	ct.RecordAction("kill")

	// Advance past cooldown.
	ct.now = func() time.Time { return frozen.Add(DefaultCooldown()) }
	if !ct.CanAct("kill", DefaultCooldown()) {
		t.Error("CanAct(kill) = false after cooldown elapsed; want true")
	}
}

func TestCooldownTracker_IndependentActions(t *testing.T) {
	ct := NewCooldownTracker()
	frozen := time.Date(2026, 4, 10, 12, 0, 0, 0, time.UTC)
	ct.now = func() time.Time { return frozen }

	ct.RecordAction("kill")

	// "restart" was never recorded — should be allowed even though "kill" is in cooldown.
	ct.now = func() time.Time { return frozen.Add(1 * time.Minute) }
	if !ct.CanAct("restart", DefaultCooldown()) {
		t.Error("CanAct(restart) = false; want true (independent of kill)")
	}
	if ct.CanAct("kill", DefaultCooldown()) {
		t.Error("CanAct(kill) = true within cooldown; want false")
	}
}

func TestCooldownTracker_RecordUpdatesTimestamp(t *testing.T) {
	ct := NewCooldownTracker()
	t0 := time.Date(2026, 4, 10, 12, 0, 0, 0, time.UTC)
	ct.now = func() time.Time { return t0 }
	ct.RecordAction("archive")

	// Advance past cooldown, record again.
	t1 := t0.Add(DefaultCooldown() + time.Second)
	ct.now = func() time.Time { return t1 }
	ct.RecordAction("archive")

	// 1 minute after re-record — still in cooldown.
	ct.now = func() time.Time { return t1.Add(1 * time.Minute) }
	if ct.CanAct("archive", DefaultCooldown()) {
		t.Error("CanAct(archive) = true; want false (cooldown reset by second RecordAction)")
	}
}
