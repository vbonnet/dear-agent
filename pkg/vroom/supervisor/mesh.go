package supervisor

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"
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

// maxRestartBackoff caps the exponential backoff for loop restarts.
const maxRestartBackoff = 30 * time.Second

// Run starts all three loops and blocks until ctx is cancelled or a loop
// panics. On return, all loops have stopped.
//
// A loop returning context.Canceled or context.DeadlineExceeded is treated
// as graceful shutdown. A panic is fatal and cancels the entire mesh.
// Any other error from a loop triggers a selective restart of that loop
// with exponential backoff — the remaining loops continue running.
func (m *Mesh) Run(ctx context.Context) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	loops := m.Loops()
	errs := make(chan loopError, len(loops))

	for _, l := range loops {
		l := l
		go m.runLoop(ctx, l, errs)
	}

	var firstErr error
	stopped := 0
	for stopped < len(loops) {
		le := <-errs
		if le.fatal {
			if firstErr == nil {
				firstErr = le.err
			}
			cancel()
		}
		if le.stopped {
			stopped++
			if firstErr == nil {
				firstErr = le.err
			}
			continue
		}
	}
	return firstErr
}

// runLoop runs a single supervisor loop with automatic restart on non-fatal
// errors. It sends a final loopError{stopped: true} when it will not restart.
func (m *Mesh) runLoop(ctx context.Context, l *Loop, errs chan<- loopError) {
	backoff := 1 * time.Second
	for {
		panicked, err := m.runLoopOnce(ctx, l)
		if panicked {
			errs <- loopError{role: l.Role(), err: err, fatal: true}
			errs <- loopError{role: l.Role(), err: err, stopped: true}
			return
		}
		if ctx.Err() != nil {
			errs <- loopError{role: l.Role(), err: ctx.Err(), stopped: true}
			return
		}
		errs <- loopError{role: l.Role(), err: err}

		select {
		case <-ctx.Done():
			errs <- loopError{role: l.Role(), err: ctx.Err(), stopped: true}
			return
		case <-time.After(backoff):
		}
		backoff *= 2
		if backoff > maxRestartBackoff {
			backoff = maxRestartBackoff
		}
	}
}

// runLoopOnce calls l.Run and catches panics. Returns (true, err) on panic.
func (m *Mesh) runLoopOnce(ctx context.Context, l *Loop) (panicked bool, err error) {
	defer func() {
		if r := recover(); r != nil {
			panicked = true
			err = fmt.Errorf("supervisor loop %s panicked: %v", l.Role(), r)
		}
	}()
	err = l.Run(ctx)
	return false, err
}

type loopError struct {
	role    Role
	err     error
	fatal   bool
	stopped bool
}
