package trigger

import (
	"sync"

	"github.com/vbonnet/dear-agent/pkg/engram"
)

// TriggerEntry associates an engram path with its trigger spec.
type TriggerEntry struct {
	EngramPath string
	Trigger    engram.TriggerSpec
}

// TriggerRegistry indexes triggered engrams by event type.
type TriggerRegistry struct {
	mu          sync.RWMutex
	byEventType map[string][]TriggerEntry
}

// NewTriggerRegistry creates a new empty TriggerRegistry.
func NewTriggerRegistry() *TriggerRegistry {
	return &TriggerRegistry{
		byEventType: make(map[string][]TriggerEntry),
	}
}

// Register adds a triggered engram to the registry.
func (r *TriggerRegistry) Register(path string, triggers []engram.TriggerSpec) {
	r.mu.Lock()
	defer r.mu.Unlock()

	for _, t := range triggers {
		entry := TriggerEntry{
			EngramPath: path,
			Trigger:    t,
		}
		r.byEventType[t.On] = append(r.byEventType[t.On], entry)
	}
}

// Lookup returns all trigger entries for a given event type.
func (r *TriggerRegistry) Lookup(eventType string) []TriggerEntry {
	r.mu.RLock()
	defer r.mu.RUnlock()

	entries := r.byEventType[eventType]
	if len(entries) == 0 {
		return nil
	}

	// Return a copy to avoid data races.
	result := make([]TriggerEntry, len(entries))
	copy(result, entries)
	return result
}

// Clear removes all entries.
func (r *TriggerRegistry) Clear() {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.byEventType = make(map[string][]TriggerEntry)
}
