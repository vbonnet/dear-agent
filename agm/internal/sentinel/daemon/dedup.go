package daemon

import (
	"sync"
	"time"
)

// IncidentDeduplicator tracks (session, symptom) pairs to suppress duplicate incident logging.
// Only allows logging when a new symptom appears or the cooldown period expires.
type IncidentDeduplicator struct {
	mu       sync.Mutex
	lastSeen map[string]time.Time // key = "session::symptom"
	cooldown time.Duration
}

// NewIncidentDeduplicator creates a deduplicator with the given cooldown duration.
func NewIncidentDeduplicator(cooldown time.Duration) *IncidentDeduplicator {
	return &IncidentDeduplicator{
		lastSeen: make(map[string]time.Time),
		cooldown: cooldown,
	}
}

// ShouldLog returns true if this incident should be logged (new or cooldown expired).
func (d *IncidentDeduplicator) ShouldLog(sessionName, symptom string) bool {
	d.mu.Lock()
	defer d.mu.Unlock()

	key := sessionName + "::" + symptom
	lastTime, exists := d.lastSeen[key]
	now := time.Now()

	if !exists || now.Sub(lastTime) >= d.cooldown {
		d.lastSeen[key] = now
		return true
	}
	return false
}

// Cleanup removes entries older than 2x cooldown to prevent memory growth.
func (d *IncidentDeduplicator) Cleanup() {
	d.mu.Lock()
	defer d.mu.Unlock()

	threshold := time.Now().Add(-2 * d.cooldown)
	for key, t := range d.lastSeen {
		if t.Before(threshold) {
			delete(d.lastSeen, key)
		}
	}
}
