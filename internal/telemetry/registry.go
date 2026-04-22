package telemetry

import (
	"log/slog"
	"sync"
)

// ListenerRegistry manages event listeners with thread-safe access.
type ListenerRegistry struct {
	mu        sync.RWMutex
	listeners []EventListener
}

// NewListenerRegistry creates a new listener registry.
func NewListenerRegistry() *ListenerRegistry {
	return &ListenerRegistry{
		listeners: make([]EventListener, 0),
	}
}

// Register adds an event listener to the registry.
// Thread-safe (uses write lock).
func (r *ListenerRegistry) Register(listener EventListener) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.listeners = append(r.listeners, listener)
}

// Notify sends event to all registered listeners asynchronously.
// Thread-safe (uses read lock).
// Spawns goroutine per listener for async notification.
func (r *ListenerRegistry) Notify(event *Event) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	// Iterate listeners (concurrent readers allowed with RLock)
	for _, listener := range r.listeners {
		// Level filtering: skip listeners below event level
		if event.Level < listener.MinLevel() {
			continue
		}

		// Spawn async goroutine (pass listener as parameter to avoid closure race)
		go func(l EventListener) {
			defer func() {
				// Panic recovery: isolate panics per listener
				if r := recover(); r != nil {
					slog.Default().Error("listener panicked",
						"error", r,
						"event_type", event.Type,
						"min_level", l.MinLevel())
				}
			}()

			// Call listener
			if err := l.OnEvent(event); err != nil {
				slog.Default().Error("listener error",
					"error", err,
					"event_type", event.Type)
			}
		}(listener) // Capture as parameter (not closure)
	}
}
