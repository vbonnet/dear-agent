// Package synchub coordinates terminal, desktop/mobile, and message-bus
// surfaces that share a single harness-neutral agent session. Each session owns one Hub.
// Surfaces talk to the Hub instead of to each other; the Hub provides the
// two primitives they need to stay coherent:
//
//   - Q&A rounds with first-answer-wins arbitration and per-round IDs so
//     a follow-up question is a brand-new round, not a re-open of the old.
//   - Soft locks with monotonic-clock auto-release and no re-entry.
//
// Hub state is in-memory and per-session; cross-session and cross-machine
// coordination is out of scope (we federate via A2A topics instead, which
// is built in pkg/a2a).
//
// See SPEC.md for the observable contract.
package synchub

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/vbonnet/dear-agent/pkg/a2a"
)

// Surface names the three known surfaces. Strings, not an enum, so new
// surfaces (Slack, web preview, …) can be added without a type change.
type Surface string

// Known surface identifiers. Add new constants here as surfaces come online.
const (
	SurfaceTerminal Surface = "terminal"
	SurfaceDesktop  Surface = "desktop"
	SurfaceDiscord  Surface = "discord"
)

// Options tunes a Hub. Zero values use defaults appropriate for AGM.
type Options struct {
	// SessionID identifies the agent session this hub belongs to.
	// Required.
	SessionID string

	// Bus is the A2A bus used to publish lifecycle events. If nil, a
	// no-op publisher is used (events are silently dropped). For tests
	// and single-process use, a2a.NewInMemoryBus() is the easy default.
	Bus a2a.Bus

	// LockTimeout is the default auto-release deadline for locks
	// acquired without an explicit per-call deadline. 30s if zero.
	LockTimeout time.Duration

	// RoundTTL is how long a Q&A round stays open with no answer
	// before it expires. 5 minutes if zero.
	RoundTTL time.Duration

	// TombstoneTTL is how long an expired or answered round is kept
	// around so late answers get ErrClosed/ErrExpired rather than
	// ErrNotFound. 60s if zero.
	TombstoneTTL time.Duration

	// now is injectable for tests. Production code uses time.Now.
	// All durations are compared via time.Since which is monotonic-safe.
	now func() time.Time
}

func (o Options) withDefaults() Options {
	if o.LockTimeout == 0 {
		o.LockTimeout = 30 * time.Second
	}
	if o.RoundTTL == 0 {
		o.RoundTTL = 5 * time.Minute
	}
	if o.TombstoneTTL == 0 {
		o.TombstoneTTL = 60 * time.Second
	}
	if o.now == nil {
		o.now = time.Now
	}
	return o
}

// Hub is the per-session coordination point. Safe for concurrent use.
type Hub struct {
	opts Options

	mu       sync.Mutex
	qaSeq    uint64
	rounds   map[QuestionID]*round
	locks    map[string]*lock
	fenceSeq uint64

	stop   chan struct{}
	closed bool
	wg     sync.WaitGroup
}

// New builds a Hub. The Hub starts a background sweeper goroutine; call
// Close when the session ends to release it.
func New(opts Options) (*Hub, error) {
	if opts.SessionID == "" {
		return nil, errors.New("synchub: SessionID required")
	}
	h := &Hub{
		opts:   opts.withDefaults(),
		rounds: make(map[QuestionID]*round),
		locks:  make(map[string]*lock),
		stop:   make(chan struct{}),
	}
	h.wg.Add(1)
	go h.sweeperLoop()
	return h, nil
}

// Close stops the sweeper and frees per-hub resources. Safe to call
// multiple times.
func (h *Hub) Close() error {
	h.mu.Lock()
	if h.closed {
		h.mu.Unlock()
		return nil
	}
	h.closed = true
	close(h.stop)
	h.mu.Unlock()
	h.wg.Wait()
	return nil
}

// sweeperLoop runs hub-wide periodic maintenance: expire Q&A rounds,
// auto-release locks past their deadline, garbage-collect tombstones.
// One goroutine handles all of it so we don't multiply ticker overhead.
func (h *Hub) sweeperLoop() {
	defer h.wg.Done()
	// Pick a tick that's frequent enough to catch the tightest deadline
	// without being wasteful. min(LockTimeout, RoundTTL)/4 with a floor.
	tick := h.opts.LockTimeout / 4
	if h.opts.RoundTTL/4 < tick {
		tick = h.opts.RoundTTL / 4
	}
	if tick < 50*time.Millisecond {
		tick = 50 * time.Millisecond
	}
	t := time.NewTicker(tick)
	defer t.Stop()
	for {
		select {
		case <-h.stop:
			return
		case <-t.C:
			h.sweep()
		}
	}
}

// publish is a small helper that swallows nil-bus + errors. Hub callers
// don't care if a lifecycle event missed the bus.
func (h *Hub) publish(ctx context.Context, topic string, body []byte) {
	if h.opts.Bus == nil {
		return
	}
	_ = h.opts.Bus.Publish(ctx, a2a.Message{Topic: topic, Body: body})
}

// Errors returned by Hub mutators. Sentinel values so callers can use
// errors.Is for control flow.
var (
	ErrNotFound    = errors.New("synchub: round not found")
	ErrClosed      = errors.New("synchub: round already closed")
	ErrExpired     = errors.New("synchub: round expired")
	ErrAlreadyHeld = errors.New("synchub: lock already held by this holder")
	ErrLockTimeout = errors.New("synchub: lock acquire timed out")
	ErrLockClosed  = errors.New("synchub: lock no longer held by caller")
)
