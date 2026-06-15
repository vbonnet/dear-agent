package a2a

import (
	"context"
	"log/slog"
	"sync"
	"sync/atomic"
)

// NewInMemoryBus returns a Bus backed by per-topic channel fan-out, good
// for tests and single-process operation. Subscribers get their own
// buffered channel; a slow subscriber's buffer-full sends are DROPPED
// (lossy delivery — see Drops()) so the publisher never blocks.
//
// NOTE for stub users: this in-memory bus is for tests and short-lived
// processes. The cleanup goroutine spawned per Subscribe blocks until
// the subscriber's ctx is canceled or Close is called, so sustained
// subscribe/unsubscribe churn will accumulate goroutines until close
// (F-S3 from multi-persona review). The real pkg/a2a implementation
// will replace this; surfaces should not depend on its delivery
// semantics.
func NewInMemoryBus() Bus {
	return &inmemBus{
		subs: make(map[string][]*inmemSub),
	}
}

// Wider than 64 to reduce the drop rate at fan-out heavy steady state
// (F-Sc4: 3 surfaces × 10 Hz events × N sessions hits 64 quickly).
const inmemSubBuffer = 256

type inmemBus struct {
	mu     sync.RWMutex
	subs   map[string][]*inmemSub
	closed bool
	drops  atomic.Uint64
}

// Drops returns the number of messages dropped because a subscriber's
// buffer was full when Publish ran. Tests and operators can read this
// to detect when surfaces are falling behind. Non-resetting.
func (b *inmemBus) Drops() uint64 { return b.drops.Load() }

type inmemSub struct {
	topic string
	ch    chan Message
}

func (b *inmemBus) Publish(ctx context.Context, m Message) error {
	b.mu.RLock()
	if b.closed {
		b.mu.RUnlock()
		return ErrBusClosed
	}
	// Snapshot subscribers so we don't hold the lock during sends.
	var targets []*inmemSub
	if subs, ok := b.subs[m.Topic]; ok {
		targets = append(targets, subs...)
	}
	b.mu.RUnlock()

	for _, s := range targets {
		select {
		case s.ch <- m:
		case <-ctx.Done():
			return ctx.Err()
		default:
			// Subscriber buffer full: drop to keep the bus moving.
			// The real A2A will choose a backpressure policy; for the
			// stub we prefer liveness over delivery guarantees and
			// surface the count via Drops().
			b.drops.Add(1)
		}
	}
	return nil
}

func (b *inmemBus) Subscribe(ctx context.Context, topic string) (<-chan Message, error) {
	b.mu.Lock()
	if b.closed {
		b.mu.Unlock()
		return nil, ErrBusClosed
	}
	sub := &inmemSub{topic: topic, ch: make(chan Message, inmemSubBuffer)}
	b.subs[topic] = append(b.subs[topic], sub)
	b.mu.Unlock()

	go func() {
		defer func() {
			if r := recover(); r != nil {
				slog.Error("a2a: subscriber cleanup goroutine panic", "topic", topic, "panic", r)
			}
		}()
		<-ctx.Done()
		b.mu.Lock()
		defer b.mu.Unlock()
		// Remove subscriber and close its channel exactly once.
		list := b.subs[topic]
		for i, s := range list {
			if s == sub {
				b.subs[topic] = append(list[:i], list[i+1:]...)
				close(sub.ch)
				return
			}
		}
	}()

	return sub.ch, nil
}

func (b *inmemBus) Close() error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed {
		return nil
	}
	b.closed = true
	for _, list := range b.subs {
		for _, s := range list {
			close(s.ch)
		}
	}
	b.subs = nil
	return nil
}
