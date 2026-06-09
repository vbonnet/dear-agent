package a2a

import (
	"context"
	"sync"
	"testing"
	"time"
)

func TestInMemoryBus_PubSub(t *testing.T) {
	bus := NewInMemoryBus()
	t.Cleanup(func() { _ = bus.Close() })

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ch, err := bus.Subscribe(ctx, "topic-1")
	if err != nil {
		t.Fatalf("subscribe: %v", err)
	}

	if err := bus.Publish(context.Background(), Message{Topic: "topic-1", Body: []byte("hello")}); err != nil {
		t.Fatalf("publish: %v", err)
	}

	select {
	case m := <-ch:
		if string(m.Body) != "hello" {
			t.Errorf("got %q, want %q", m.Body, "hello")
		}
	case <-time.After(time.Second):
		t.Fatal("subscriber did not receive message")
	}
}

func TestInMemoryBus_TopicIsolation(t *testing.T) {
	bus := NewInMemoryBus()
	t.Cleanup(func() { _ = bus.Close() })
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	a, _ := bus.Subscribe(ctx, "A")
	b, _ := bus.Subscribe(ctx, "B")

	_ = bus.Publish(ctx, Message{Topic: "A", Body: []byte("only-A")})

	select {
	case m := <-a:
		if string(m.Body) != "only-A" {
			t.Errorf("A got %q", m.Body)
		}
	case <-time.After(time.Second):
		t.Fatal("A did not receive")
	}

	select {
	case m := <-b:
		t.Fatalf("B should not have received: %q", m.Body)
	case <-time.After(50 * time.Millisecond):
	}
}

func TestInMemoryBus_CloseDrainsSubscribers(t *testing.T) {
	bus := NewInMemoryBus()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ch, _ := bus.Subscribe(ctx, "x")
	_ = bus.Close()
	// Channel must close so receivers wake up.
	select {
	case _, ok := <-ch:
		if ok {
			t.Fatal("expected closed channel")
		}
	case <-time.After(time.Second):
		t.Fatal("close did not propagate")
	}
}

func TestInMemoryBus_ConcurrentPublishers(t *testing.T) {
	bus := NewInMemoryBus()
	t.Cleanup(func() { _ = bus.Close() })
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ch, _ := bus.Subscribe(ctx, "burst")

	var wg sync.WaitGroup
	for i := 0; i < 32; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = bus.Publish(ctx, Message{Topic: "burst", Body: []byte("x")})
		}()
	}
	wg.Wait()

	// Drain — buffer is 64, so all 32 should arrive.
	got := 0
	deadline := time.After(time.Second)
	for got < 32 {
		select {
		case <-ch:
			got++
		case <-deadline:
			t.Fatalf("only received %d/32", got)
		}
	}
}
