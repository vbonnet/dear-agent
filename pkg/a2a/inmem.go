package a2a

import (
	"context"
	"sync"
)

// NewInMemoryBus returns a Bus backed by per-topic channel fan-out, good
// for tests and single-process operation. Subscribers get their own
// buffered channel; a slow subscriber blocks only itself once the buffer
// fills.
func NewInMemoryBus() Bus {
	return &inmemBus{
		subs: make(map[string][]*inmemSub),
	}
}

const inmemSubBuffer = 64

type inmemBus struct {
	mu     sync.RWMutex
	subs   map[string][]*inmemSub
	closed bool
}

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
			// stub we prefer liveness over delivery guarantees.
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
