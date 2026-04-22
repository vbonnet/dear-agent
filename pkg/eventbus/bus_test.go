package eventbus

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

func TestNewLocalBus(t *testing.T) {
	bus := NewLocalBus()
	if bus == nil {
		t.Fatal("NewLocalBus() returned nil")
	}
	if bus.handlers == nil {
		t.Error("handlers map is nil")
	}
	if bus.responses == nil {
		t.Error("responses map is nil")
	}
	bus.Close()
}

func TestNewEvent_ChannelDerivation(t *testing.T) {
	tests := []struct {
		eventType string
		wantCh    Channel
	}{
		{"telemetry.agent.launch", ChannelTelemetry},
		{"notification.phase.complete", ChannelNotification},
		{"audit.session.start", ChannelAudit},
		{"heartbeat.agent", ChannelHeartbeat},
	}
	for _, tt := range tests {
		e := NewEvent(tt.eventType, "test", nil)
		if e.Channel != tt.wantCh {
			t.Errorf("NewEvent(%q).Channel = %q, want %q", tt.eventType, e.Channel, tt.wantCh)
		}
		if e.ID == "" {
			t.Error("event ID is empty")
		}
		if e.Timestamp.IsZero() {
			t.Error("event timestamp is zero")
		}
	}
}

func TestChannelDurability(t *testing.T) {
	if !ChannelNotification.IsDurable() {
		t.Error("notification channel should be durable")
	}
	if !ChannelAudit.IsDurable() {
		t.Error("audit channel should be durable")
	}
	if ChannelTelemetry.IsDurable() {
		t.Error("telemetry channel should not be durable")
	}
	if ChannelHeartbeat.IsDurable() {
		t.Error("heartbeat channel should not be durable")
	}
}

func TestEmit_SingleSubscriber(t *testing.T) {
	bus := NewLocalBus()
	defer bus.Close()

	called := false
	bus.Subscribe("telemetry.test", "sub", func(ctx context.Context, event *Event) (*Response, error) {
		called = true
		return nil, nil
	})

	err := bus.Emit(context.Background(), NewEvent("telemetry.test", "pub", nil))
	if err != nil {
		t.Fatalf("Emit() failed: %v", err)
	}
	time.Sleep(50 * time.Millisecond)

	if !called {
		t.Error("handler was not called")
	}
}

func TestEmit_MultipleSubscribers(t *testing.T) {
	bus := NewLocalBus()
	defer bus.Close()

	var mu sync.Mutex
	var count int

	for i := 0; i < 3; i++ {
		bus.Subscribe("telemetry.test", "sub", func(ctx context.Context, event *Event) (*Response, error) {
			mu.Lock()
			count++
			mu.Unlock()
			return nil, nil
		})
	}

	bus.Emit(context.Background(), NewEvent("telemetry.test", "pub", nil))
	time.Sleep(50 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()
	if count != 3 {
		t.Errorf("expected 3 handlers called, got %d", count)
	}
}

func TestEmit_NoSubscribers(t *testing.T) {
	bus := NewLocalBus()
	defer bus.Close()

	err := bus.Emit(context.Background(), NewEvent("telemetry.test", "pub", nil))
	if err != nil {
		t.Errorf("Emit() with no subscribers should not error: %v", err)
	}
}

func TestEmit_HandlerError(t *testing.T) {
	bus := NewLocalBus()
	defer bus.Close()

	var handler2Called bool
	var mu sync.Mutex

	bus.Subscribe("telemetry.test", "sub1", func(ctx context.Context, event *Event) (*Response, error) {
		return nil, context.Canceled
	})
	bus.Subscribe("telemetry.test", "sub2", func(ctx context.Context, event *Event) (*Response, error) {
		mu.Lock()
		handler2Called = true
		mu.Unlock()
		return nil, nil
	})

	bus.Emit(context.Background(), NewEvent("telemetry.test", "pub", nil))
	time.Sleep(50 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()
	if !handler2Called {
		t.Error("handler2 should still be called despite handler1 error")
	}
}

func TestFilter_Channels(t *testing.T) {
	f := &Filter{Channels: []Channel{ChannelTelemetry}}
	if !f.Matches(NewEvent("telemetry.test", "s", nil)) {
		t.Error("filter should match telemetry event")
	}
	if f.Matches(NewEvent("audit.test", "s", nil)) {
		t.Error("filter should not match audit event")
	}
}

func TestFilter_Types(t *testing.T) {
	f := &Filter{Types: []string{"telemetry.*"}}
	if !f.Matches(NewEvent("telemetry.test", "s", nil)) {
		t.Error("filter should match telemetry.test")
	}
	if f.Matches(NewEvent("audit.test", "s", nil)) {
		t.Error("filter should not match audit.test")
	}
}

func TestFilter_MinLevel(t *testing.T) {
	f := &Filter{MinLevel: 4} // slog.LevelWarn
	info := NewEvent("telemetry.test", "s", nil) // default info level (0)
	if f.Matches(info) {
		t.Error("filter should not match info event with warn min level")
	}
	warn := NewEventWithLevel("telemetry.test", "s", 4, nil)
	if !f.Matches(warn) {
		t.Error("filter should match warn event with warn min level")
	}
}

func TestWildcardSubscriptions(t *testing.T) {
	tests := []struct {
		name        string
		pattern     string
		eventType   string
		shouldMatch bool
	}{
		{"exact match", "telemetry.test", "telemetry.test", true},
		{"prefix.* match", "telemetry.*", "telemetry.test", true},
		{"prefix.* no match", "telemetry.*", "audit.test", false},
		{"global wildcard", "*", "anything.here", true},
		{"prefix* match", "telemetry*", "telemetry.test", true},
		{"prefix* match no dot", "telemetry*", "telemetryXYZ", true},
		{"prefix* no match", "telemetry*", "audit.test", false},
		{"no match", "exact.match", "different.topic", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			bus := NewLocalBus()
			defer bus.Close()

			called := false
			bus.Subscribe(tt.pattern, "sub", func(ctx context.Context, event *Event) (*Response, error) {
				called = true
				return nil, nil
			})

			bus.Emit(context.Background(), NewEvent(tt.eventType, "pub", nil))
			time.Sleep(50 * time.Millisecond)

			if called != tt.shouldMatch {
				t.Errorf("handler called = %v, want %v", called, tt.shouldMatch)
			}
		})
	}
}

func TestSubscribe_ReturnsID(t *testing.T) {
	bus := NewLocalBus()
	defer bus.Close()

	id := bus.Subscribe("telemetry.test", "sub", func(ctx context.Context, event *Event) (*Response, error) {
		return nil, nil
	})
	if id == "" {
		t.Error("Subscribe should return non-empty ID")
	}
}

func TestUnsubscribe(t *testing.T) {
	bus := NewLocalBus()
	defer bus.Close()

	called := false
	id := bus.Subscribe("telemetry.test", "sub", func(ctx context.Context, event *Event) (*Response, error) {
		called = true
		return nil, nil
	})

	if err := bus.Unsubscribe("telemetry.test", id); err != nil {
		t.Fatalf("Unsubscribe() failed: %v", err)
	}

	bus.Emit(context.Background(), NewEvent("telemetry.test", "pub", nil))
	time.Sleep(50 * time.Millisecond)

	if called {
		t.Error("handler should not be called after unsubscribe")
	}
}

func TestUnsubscribe_EmptyID(t *testing.T) {
	bus := NewLocalBus()
	defer bus.Close()
	if err := bus.Unsubscribe("test", ""); err == nil {
		t.Error("Unsubscribe with empty ID should error")
	}
}

func TestUnsubscribe_InvalidPattern(t *testing.T) {
	bus := NewLocalBus()
	defer bus.Close()
	if err := bus.Unsubscribe("nonexistent", "some-id"); err == nil {
		t.Error("Unsubscribe for nonexistent pattern should error")
	}
}

func TestUnsubscribe_InvalidID(t *testing.T) {
	bus := NewLocalBus()
	defer bus.Close()
	bus.Subscribe("test", "sub", func(ctx context.Context, event *Event) (*Response, error) {
		return nil, nil
	})
	if err := bus.Unsubscribe("test", "nonexistent-id"); err == nil {
		t.Error("Unsubscribe for nonexistent ID should error")
	}
}

func TestContextCancellation(t *testing.T) {
	bus := NewLocalBus()
	defer bus.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	bus.Subscribe("telemetry.test", "sub", func(ctx context.Context, event *Event) (*Response, error) {
		time.Sleep(100 * time.Millisecond)
		return nil, nil
	})

	err := bus.Emit(ctx, NewEvent("telemetry.test", "pub", nil))
	if err == nil {
		t.Error("Emit should return error when context is cancelled")
	}
	if !errors.Is(err, context.Canceled) {
		t.Errorf("expected context.Canceled, got: %v", err)
	}
}

func TestPanicRecovery(t *testing.T) {
	bus := NewLocalBus()
	defer bus.Close()

	var handler2Called bool
	var mu sync.Mutex

	bus.Subscribe("telemetry.test", "sub1", func(ctx context.Context, event *Event) (*Response, error) {
		panic("test panic")
	})
	bus.Subscribe("telemetry.test", "sub2", func(ctx context.Context, event *Event) (*Response, error) {
		mu.Lock()
		handler2Called = true
		mu.Unlock()
		return nil, nil
	})

	err := bus.Emit(context.Background(), NewEvent("telemetry.test", "pub", nil))
	if err != nil {
		t.Errorf("Emit should not fail: %v", err)
	}

	time.Sleep(100 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()
	if !handler2Called {
		t.Error("handler2 should be called despite handler1 panic")
	}
}

func TestSubscriberLimit_PerTopic(t *testing.T) {
	bus := NewLocalBus(WithMaxSubscribersPerTopic(2))
	defer bus.Close()

	h := func(ctx context.Context, event *Event) (*Response, error) { return nil, nil }

	id1 := bus.Subscribe("test", "sub1", h)
	id2 := bus.Subscribe("test", "sub2", h)
	id3 := bus.Subscribe("test", "sub3", h)

	if id1 == "" || id2 == "" {
		t.Error("first two subscriptions should succeed")
	}
	if id3 != "" {
		t.Error("third subscription should fail (limit 2)")
	}
}

func TestSubscriberLimit_Global(t *testing.T) {
	bus := NewLocalBus(WithMaxTotalSubscribers(3))
	defer bus.Close()

	h := func(ctx context.Context, event *Event) (*Response, error) { return nil, nil }

	bus.Subscribe("a", "sub", h)
	bus.Subscribe("b", "sub", h)
	bus.Subscribe("c", "sub", h)
	id := bus.Subscribe("d", "sub", h)

	if id != "" {
		t.Error("4th subscription should fail (global limit 3)")
	}

	// Unsubscribe one, should allow new subscription
	bus.Unsubscribe("a", bus.handlers["a"][0].id)
	id = bus.Subscribe("d", "sub", h)
	if id == "" {
		t.Error("subscription should succeed after unsubscribe")
	}
}

func TestEventSizeValidation(t *testing.T) {
	bus := NewLocalBus(WithMaxEventSize(100))
	defer bus.Close()

	largeData := make(map[string]interface{})
	for i := 0; i < 50; i++ {
		largeData[string(rune('a'+i))] = "this is a long string value that will exceed the limit"
	}

	err := bus.Emit(context.Background(), NewEvent("telemetry.test", "pub", largeData))
	if err == nil {
		t.Error("Emit should fail for oversized event")
	}
}

func TestEventSizeValidation_SmallEvent(t *testing.T) {
	bus := NewLocalBus(WithMaxEventSize(1024 * 1024))
	defer bus.Close()

	err := bus.Emit(context.Background(), NewEvent("telemetry.test", "pub", map[string]interface{}{"k": "v"}))
	if err != nil {
		t.Errorf("Emit should succeed for small event: %v", err)
	}
}

func TestConcurrency(t *testing.T) {
	bus := NewLocalBus()
	defer bus.Close()

	for i := 0; i < 5; i++ {
		bus.Subscribe("telemetry.test", "sub", func(ctx context.Context, event *Event) (*Response, error) {
			return NewResponse(event.ID, "responder", nil), nil
		})
	}

	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			bus.Emit(context.Background(), NewEvent("telemetry.test", "pub", nil))
		}()
	}
	wg.Wait()
	// Main test: no panic under concurrent access
}

func TestClose(t *testing.T) {
	bus := NewLocalBus()
	bus.Subscribe("test", "sub", func(ctx context.Context, event *Event) (*Response, error) {
		return nil, nil
	})
	bus.Emit(context.Background(), NewEvent("telemetry.test", "pub", nil))

	if err := bus.Close(); err != nil {
		t.Errorf("Close() failed: %v", err)
	}

	bus.mu.RLock()
	hc := len(bus.handlers)
	rc := len(bus.responses)
	bus.mu.RUnlock()

	if hc != 0 {
		t.Errorf("handlers not cleared: %d", hc)
	}
	if rc != 0 {
		t.Errorf("responses not cleared: %d", rc)
	}
}

func TestAddSink(t *testing.T) {
	bus := NewLocalBus()
	defer bus.Close()

	var received []*Event
	var mu sync.Mutex

	sink := &testSink{
		name: "test",
		handleFunc: func(ctx context.Context, event *Event) error {
			mu.Lock()
			received = append(received, event)
			mu.Unlock()
			return nil
		},
	}

	bus.AddSink(sink, nil) // no filter = all events

	bus.Emit(context.Background(), NewEvent("telemetry.test", "pub", nil))
	bus.Emit(context.Background(), NewEvent("audit.test", "pub", nil))
	time.Sleep(50 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()
	if len(received) != 2 {
		t.Errorf("expected 2 events to sink, got %d", len(received))
	}
}

func TestAddSink_WithFilter(t *testing.T) {
	bus := NewLocalBus()
	defer bus.Close()

	var received []*Event
	var mu sync.Mutex

	sink := &testSink{
		name: "audit-only",
		handleFunc: func(ctx context.Context, event *Event) error {
			mu.Lock()
			received = append(received, event)
			mu.Unlock()
			return nil
		},
	}

	bus.AddSink(sink, &Filter{Channels: []Channel{ChannelAudit}})

	bus.Emit(context.Background(), NewEvent("telemetry.test", "pub", nil))
	bus.Emit(context.Background(), NewEvent("audit.test", "pub", nil))
	time.Sleep(50 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()
	if len(received) != 1 {
		t.Errorf("expected 1 event to sink, got %d", len(received))
	}
	if len(received) > 0 && received[0].Channel != ChannelAudit {
		t.Errorf("expected audit event, got %s", received[0].Channel)
	}
}

func TestNewResponse(t *testing.T) {
	r := NewResponse("event-1", "responder", map[string]interface{}{"ok": true})
	if r.ID == "" {
		t.Error("response ID is empty")
	}
	if r.EventID != "event-1" {
		t.Errorf("EventID = %q, want %q", r.EventID, "event-1")
	}
	if r.Responder != "responder" {
		t.Errorf("Responder = %q, want %q", r.Responder, "responder")
	}
}

// testSink is a test helper sink.
type testSink struct {
	name       string
	handleFunc func(ctx context.Context, event *Event) error
	closed     bool
}

func (s *testSink) Name() string { return s.name }
func (s *testSink) HandleEvent(ctx context.Context, event *Event) error {
	return s.handleFunc(ctx, event)
}
func (s *testSink) Close() error { s.closed = true; return nil }
