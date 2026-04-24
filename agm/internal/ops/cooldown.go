package ops

import (
	"sync"
	"time"

	"github.com/vbonnet/dear-agent/agm/internal/contracts"
)

// DefaultCooldown returns the minimum interval between repeated recovery actions.
func DefaultCooldown() time.Duration {
	return contracts.Load().SessionLifecycle.CooldownInterval.Duration
}

// CooldownTracker prevents rapid-fire recovery actions (kill, restart, etc.)
// by enforcing a minimum interval between consecutive invocations of the same action.
type CooldownTracker struct {
	mu      sync.Mutex
	actions map[string]time.Time
	now     func() time.Time // for testing
}

// NewCooldownTracker returns a ready-to-use CooldownTracker.
func NewCooldownTracker() *CooldownTracker {
	return &CooldownTracker{
		actions: make(map[string]time.Time),
		now:     time.Now,
	}
}

// CanAct reports whether the given action is allowed, i.e. the last
// invocation was more than cooldown ago (or has never been recorded).
func (ct *CooldownTracker) CanAct(action string, cooldown time.Duration) bool {
	ct.mu.Lock()
	defer ct.mu.Unlock()

	last, ok := ct.actions[action]
	if !ok {
		return true
	}
	return ct.now().Sub(last) >= cooldown
}

// RecordAction records the current time as the last invocation of action.
func (ct *CooldownTracker) RecordAction(action string) {
	ct.mu.Lock()
	defer ct.mu.Unlock()

	ct.actions[action] = ct.now()
}
