package supervisor

import (
	"context"
	"errors"
	"fmt"
	"sync"
)

// Mesh holds the three Loops that make up a running VROOM supervisory
// mesh. It is the registry through which each Loop resolves its peers'
// LoopStatus, and the orchestrator that starts/stops them as a unit.
//
// A correctly-constructed Mesh contains exactly one Loop per Role
// (Meta-Orchestrator, Orchestrator, Overseer). NewMesh rejects any other
// configuration because the doctrine is structurally three-supervisor —
// running with two or four is not a partial deployment, it is a different
// (and undefined) system.
type Mesh struct {
	mu    sync.RWMutex
	loops map[Role]*Loop
}

// NewMesh builds a Mesh containing exactly the three canonical supervisors.
// The Loops may be constructed in any order; NewMesh verifies that every
// Role is represented exactly once.
func NewMesh(loops ...*Loop) (*Mesh, error) {
	m := &Mesh{loops: make(map[Role]*Loop, len(loops))}
	for _, l := range loops {
		if l == nil {
			return nil, errors.New("supervisor: NewMesh got a nil Loop")
		}
		r := l.Role()
		if !r.Valid() {
			return nil, fmt.Errorf("supervisor: NewMesh got Loop with invalid Role %q", r)
		}
		if _, dup := m.loops[r]; dup {
			return nil, fmt.Errorf("supervisor: NewMesh got duplicate Loop for role %q", r)
		}
		m.loops[r] = l
	}
	for _, r := range AllRoles() {
		if _, ok := m.loops[r]; !ok {
			return nil, fmt.Errorf("supervisor: NewMesh missing Loop for role %q", r)
		}
	}
	// Tie each Loop's peer lookup back to this Mesh. We do this after
	// validation so we don't leave half-wired Loops on error paths.
	for _, l := range m.loops {
		l.mesh = m
	}
	return m, nil
}

// Get implements PeerLookup. Returns the LoopStatus for the named role and
// whether it was found. The bool is `false` only if the Mesh is somehow
// missing a canonical role — which NewMesh prevents.
func (m *Mesh) Get(r Role) (LoopStatus, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	l, ok := m.loops[r]
	if !ok {
		return nil, false
	}
	return l, true
}

// Loop returns the underlying *Loop for the named role. Useful for
// inspection in tests; production code should prefer Get and the
// LoopStatus interface.
func (m *Mesh) Loop(r Role) (*Loop, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	l, ok := m.loops[r]
	return l, ok
}

// Loops returns all loops in canonical role order (matching AllRoles).
func (m *Mesh) Loops() []*Loop {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]*Loop, 0, len(m.loops))
	for _, r := range AllRoles() {
		if l, ok := m.loops[r]; ok {
			out = append(out, l)
		}
	}
	return out
}

// Run starts all three loops and blocks until ctx is cancelled or any loop
// returns a non-cancellation error. On return, all loops have stopped.
//
// A loop returning context.Canceled or context.DeadlineExceeded is treated
// as graceful shutdown — Run returns that error after waiting for siblings
// to stop. Any other error from any loop is treated as fatal and triggers
// cancellation of the shared context so siblings stop too.
func (m *Mesh) Run(ctx context.Context) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	loops := m.Loops()
	errs := make(chan loopError, len(loops))
	var wg sync.WaitGroup
	wg.Add(len(loops))

	for _, l := range loops {
		l := l
		go func() {
			defer wg.Done()
			err := l.Run(ctx)
			errs <- loopError{role: l.Role(), err: err}
		}()
	}

	// Wait for the first failure (or shutdown signal). Anything that
	// arrives, cancel the rest and wait for them to drain.
	first := <-errs
	cancel()
	wg.Wait()
	// Drain remaining results (we already cancelled — they'll all be
	// context.Canceled or context.DeadlineExceeded).
	for i := 0; i < len(loops)-1; i++ {
		<-errs
	}
	return first.err
}

type loopError struct {
	role Role
	err  error
}
