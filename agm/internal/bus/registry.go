package bus

import (
	"errors"
	"sync"
)

// ErrTargetOffline is returned by Registry.Route when no connection is
// registered for the target session. Callers can then queue the frame for
// later redelivery (see queue.go) instead of dropping it.
var ErrTargetOffline = errors.New("target offline")

// ErrAlreadyRegistered is returned by Registry.Register when a session id
// is already live. Callers should treat this as a protocol error — a new
// Hello from a session id that's already connected is almost always a bug
// (crashed-and-restarted clients should explicitly Unregister first).
var ErrAlreadyRegistered = errors.New("session already registered")

// Delivery is the pluggable write-side of a connection. The server
// implementation wraps a net.Conn; tests can substitute a channel or buffer.
type Delivery interface {
	// Deliver writes a single frame to the connected session. Delivery errors
	// are the caller's responsibility to diagnose (typically by unregistering
	// the connection). Implementations must be safe for concurrent use if
	// the server may call Deliver from multiple goroutines.
	Deliver(frame *Frame) error

	// Close closes the underlying transport. Called when a session unregisters
	// or the server is shutting down.
	Close() error
}

// Registry maps session ids to live Delivery endpoints. It's the authoritative
// routing table; all send-paths consult it. Concurrent reads and writes are
// safe — the sync.RWMutex allows many concurrent Route calls during bursts of
// traffic while protecting Register/Unregister from corrupting the map.
type Registry struct {
	mu    sync.RWMutex
	conns map[string]Delivery
}

// NewRegistry returns an empty Registry.
func NewRegistry() *Registry {
	return &Registry{conns: make(map[string]Delivery)}
}

// Register adds a live connection for sessionID. Returns ErrAlreadyRegistered
// if a connection for the same id is already present; callers must
// Unregister before re-Registering (no silent upsert — the old conn might
// still be receiving, and silently dropping it would lose pending deliveries).
func (r *Registry) Register(sessionID string, d Delivery) error {
	if sessionID == "" {
		return errors.New("registry: empty session id")
	}
	if d == nil {
		return errors.New("registry: nil delivery")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.conns[sessionID]; exists {
		return ErrAlreadyRegistered
	}
	r.conns[sessionID] = d
	return nil
}

// Unregister removes the connection for sessionID if present. It does NOT
// close the Delivery — the caller owns the connection lifecycle. Returns
// the removed Delivery so the caller can Close() it if desired; returns
// nil if no connection was registered.
func (r *Registry) Unregister(sessionID string) Delivery {
	r.mu.Lock()
	defer r.mu.Unlock()
	d := r.conns[sessionID]
	delete(r.conns, sessionID)
	return d
}

// Route looks up a live connection for targetID and returns it. Returns
// ErrTargetOffline if the target isn't registered. The returned Delivery is
// safe to call after Route returns even if the target unregisters in
// between: the Delivery may error on its next call, which the caller must
// handle.
func (r *Registry) Route(targetID string) (Delivery, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	d, ok := r.conns[targetID]
	if !ok {
		return nil, ErrTargetOffline
	}
	return d, nil
}

// Active returns a snapshot of session ids currently registered. The returned
// slice is safe to mutate; it's disconnected from the registry's internal state.
func (r *Registry) Active() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]string, 0, len(r.conns))
	for id := range r.conns {
		out = append(out, id)
	}
	return out
}

// Len returns the number of live connections.
func (r *Registry) Len() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.conns)
}
