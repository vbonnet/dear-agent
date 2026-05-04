package gateway

import (
	"context"
	"sync"
)

// Handler dispatches one Command and returns one Response. Handlers
// are pure functions of (ctx, cmd) — no hidden state — so the gateway
// can register, unregister, and replace them at construction time.
//
// A Handler MUST NOT call gw.Dispatch on a fresh command: that would
// create a same-process loop that the gateway does not detect. If a
// handler needs to compose another command, it should call the next
// handler directly.
type Handler func(ctx context.Context, cmd Command) Response

// HandlerSet is the registry passed to New. Each field maps to one
// CommandType. Nil entries are treated as "not implemented" — the
// gateway returns an unknown_command error rather than nil-deref.
//
// HandlerSet is a struct (not a map) so it's compile-time obvious which
// command types exist; adding a new CommandType means adding a field
// here, which forces every constructor to consider it.
type HandlerSet struct {
	Run     Handler
	Status  Handler
	List    Handler
	Logs    Handler
	Gates   Handler
	Approve Handler
	Reject  Handler
	Cancel  Handler
}

func (h HandlerSet) lookup(t CommandType) Handler {
	switch t {
	case CmdRun:
		return h.Run
	case CmdStatus:
		return h.Status
	case CmdList:
		return h.List
	case CmdLogs:
		return h.Logs
	case CmdGates:
		return h.Gates
	case CmdApprove:
		return h.Approve
	case CmdReject:
		return h.Reject
	case CmdCancel:
		return h.Cancel
	}
	return nil
}

// Gateway is the dispatcher. Use New to construct one with a
// HandlerSet, then hand it to one or more Adapter.Run goroutines.
//
// All methods are safe for concurrent use.
type Gateway struct {
	handlers HandlerSet

	mu          sync.RWMutex
	subscribers map[*subscription]struct{}
}

type subscription struct {
	adapter Adapter
	ch      chan Event
}

// New constructs a Gateway with the given HandlerSet. Nil handlers are
// allowed; Dispatch returns CodeUnknownCommand for those types.
func New(handlers HandlerSet) *Gateway {
	return &Gateway{
		handlers:    handlers,
		subscribers: make(map[*subscription]struct{}),
	}
}

// Dispatch routes cmd to the registered handler and returns its
// Response. Returns an unknown_command error if no handler is
// registered for cmd.Type.
//
// The context is propagated to the handler unchanged. Handlers that
// need a deadline should derive one from ctx.
func (g *Gateway) Dispatch(ctx context.Context, cmd Command) Response {
	h := g.handlers.lookup(cmd.Type)
	if h == nil {
		return errorResponse(cmd.ID, Errorf(CodeUnknownCommand,
			"no handler for command type %q", cmd.Type))
	}
	return h(ctx, cmd)
}

// Subscribe registers an adapter to receive Events. The returned
// channel is buffered (capacity 16) and closed when unsub is called or
// when the gateway shuts down. Adapters that don't drain the channel
// fast enough will drop events on overflow — Publish never blocks.
//
// The returned unsub function is idempotent and safe to call from any
// goroutine.
func (g *Gateway) Subscribe(adapter Adapter) (events <-chan Event, unsub func()) {
	sub := &subscription{
		adapter: adapter,
		ch:      make(chan Event, 16),
	}
	g.mu.Lock()
	g.subscribers[sub] = struct{}{}
	g.mu.Unlock()

	var once sync.Once
	unsub = func() {
		once.Do(func() {
			g.mu.Lock()
			if _, ok := g.subscribers[sub]; ok {
				delete(g.subscribers, sub)
				close(sub.ch)
			}
			g.mu.Unlock()
		})
	}
	return sub.ch, unsub
}

// Publish broadcasts ev to every current subscriber. Publish never
// blocks: if a subscriber's channel is full, the event is dropped for
// that subscriber. Publishers that need durability should write to
// their own log; the gateway is fire-and-forget by design (see ADR-017
// D5).
func (g *Gateway) Publish(ev Event) {
	g.mu.RLock()
	defer g.mu.RUnlock()
	for sub := range g.subscribers {
		select {
		case sub.ch <- ev:
		default:
			// Drop on overflow. Adapters that care about lag should
			// log or surface this themselves via their own channel
			// length monitoring.
		}
	}
}

// SubscriberCount returns the number of currently-subscribed adapters.
// Test-only helper; the production hot path does not need it.
func (g *Gateway) SubscriberCount() int {
	g.mu.RLock()
	defer g.mu.RUnlock()
	return len(g.subscribers)
}
