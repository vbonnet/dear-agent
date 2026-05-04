package gateway_test

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/vbonnet/dear-agent/pkg/gateway"
)

// echoHandler is the smallest useful handler: it copies args into body.
// We use it across most dispatch tests; any handler-internal logic is
// exercised in handlers_test.go.
func echoHandler(ctx context.Context, cmd gateway.Command) gateway.Response {
	return gateway.Response{CommandID: cmd.ID, Body: map[string]any{"echo": cmd.Args}}
}

func TestDispatch_RoutesByCommandType(t *testing.T) {
	var ranRun, ranStatus atomic.Int32
	gw := gateway.New(gateway.HandlerSet{
		Run: func(_ context.Context, cmd gateway.Command) gateway.Response {
			ranRun.Add(1)
			return gateway.Response{CommandID: cmd.ID, Body: map[string]any{"ok": true}}
		},
		Status: func(_ context.Context, cmd gateway.Command) gateway.Response {
			ranStatus.Add(1)
			return gateway.Response{CommandID: cmd.ID, Body: map[string]any{"ok": true}}
		},
	})

	resp := gw.Dispatch(context.Background(), gateway.Command{ID: "1", Type: gateway.CmdRun})
	if resp.Err != nil {
		t.Fatalf("Run: unexpected err: %v", resp.Err)
	}
	if resp.CommandID != "1" {
		t.Errorf("CommandID: got %q want %q", resp.CommandID, "1")
	}

	resp = gw.Dispatch(context.Background(), gateway.Command{ID: "2", Type: gateway.CmdStatus})
	if resp.Err != nil {
		t.Fatalf("Status: unexpected err: %v", resp.Err)
	}

	if ranRun.Load() != 1 || ranStatus.Load() != 1 {
		t.Errorf("counts: run=%d status=%d (want 1,1)", ranRun.Load(), ranStatus.Load())
	}
}

func TestDispatch_UnknownCommand(t *testing.T) {
	gw := gateway.New(gateway.HandlerSet{})
	resp := gw.Dispatch(context.Background(), gateway.Command{
		ID:   "abc",
		Type: gateway.CmdRun,
	})
	if resp.Err == nil {
		t.Fatal("want error, got none")
	}
	if resp.Err.Code != gateway.CodeUnknownCommand {
		t.Errorf("code: got %q want %q", resp.Err.Code, gateway.CodeUnknownCommand)
	}
	if resp.CommandID != "abc" {
		t.Errorf("CommandID echo: got %q want %q", resp.CommandID, "abc")
	}
}

func TestDispatch_PropagatesContext(t *testing.T) {
	var seenDeadline bool
	gw := gateway.New(gateway.HandlerSet{
		Run: func(ctx context.Context, _ gateway.Command) gateway.Response {
			_, seenDeadline = ctx.Deadline()
			return gateway.Response{}
		},
	})
	ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(time.Second))
	defer cancel()
	gw.Dispatch(ctx, gateway.Command{Type: gateway.CmdRun})
	if !seenDeadline {
		t.Error("handler did not see propagated deadline")
	}
}

func TestSubscribe_DeliversEventsToAllSubscribers(t *testing.T) {
	gw := gateway.New(gateway.HandlerSet{Run: echoHandler})

	subs := make([]<-chan gateway.Event, 3)
	unsubs := make([]func(), 3)
	for i := range subs {
		subs[i], unsubs[i] = gw.Subscribe(stubAdapter{name: "stub"})
	}
	defer func() {
		for _, u := range unsubs {
			u()
		}
	}()

	if got := gw.SubscriberCount(); got != 3 {
		t.Fatalf("SubscriberCount: got %d want 3", got)
	}

	ev := gateway.Event{Type: gateway.EventRunFinished, Subject: "run-1"}
	gw.Publish(ev)

	for i, ch := range subs {
		select {
		case got := <-ch:
			if got.Type != gateway.EventRunFinished || got.Subject != "run-1" {
				t.Errorf("sub %d: got %+v", i, got)
			}
		case <-time.After(time.Second):
			t.Errorf("sub %d: timed out waiting for event", i)
		}
	}
}

func TestSubscribe_UnsubStopsDelivery(t *testing.T) {
	gw := gateway.New(gateway.HandlerSet{})
	ch, unsub := gw.Subscribe(stubAdapter{name: "stub"})

	gw.Publish(gateway.Event{Type: gateway.EventHITLOpened, Subject: "a"})
	select {
	case got := <-ch:
		if got.Subject != "a" {
			t.Errorf("first event: got subject %q", got.Subject)
		}
	case <-time.After(time.Second):
		t.Fatal("first event: timed out")
	}

	unsub()

	// Second unsub must be safe.
	unsub()

	if got := gw.SubscriberCount(); got != 0 {
		t.Errorf("SubscriberCount after unsub: got %d want 0", got)
	}

	gw.Publish(gateway.Event{Type: gateway.EventHITLOpened, Subject: "b"})
	// Channel is closed; the only thing we can drain is the zero
	// value. Any non-zero subject means publish snuck through.
	select {
	case got, ok := <-ch:
		if ok {
			t.Errorf("event after unsub: got %+v", got)
		}
	case <-time.After(50 * time.Millisecond):
		// Channel is closed, but if the receive is blocking forever
		// the select will fire this branch instead. Either way is
		// fine: we wanted "no event delivered."
	}
}

func TestPublish_DropsOnFullBuffer(t *testing.T) {
	gw := gateway.New(gateway.HandlerSet{})
	_, unsub := gw.Subscribe(stubAdapter{name: "slow"})
	defer unsub()

	// Buffer is 16; publish 1000 without draining. Publish must not
	// block — the test passes if this line returns within the deadline.
	done := make(chan struct{})
	go func() {
		for i := 0; i < 1000; i++ {
			gw.Publish(gateway.Event{Type: gateway.EventRunFinished, Subject: "x"})
		}
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Publish blocked on a full subscriber buffer")
	}
}

func TestPublish_ConcurrentSubscribeAndPublish(t *testing.T) {
	// Race-detector smoke test: many goroutines subscribing,
	// unsubscribing, and publishing in parallel must not data-race.
	gw := gateway.New(gateway.HandlerSet{})

	var wg sync.WaitGroup
	stop := make(chan struct{})
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-stop:
					return
				default:
					_, unsub := gw.Subscribe(stubAdapter{name: "stub"})
					unsub()
				}
			}
		}()
	}
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-stop:
					return
				default:
					gw.Publish(gateway.Event{Type: gateway.EventRunFinished, Subject: "x"})
				}
			}
		}()
	}
	time.Sleep(100 * time.Millisecond)
	close(stop)
	wg.Wait()
}

func TestError_Unwrap(t *testing.T) {
	cause := errors.New("boom")
	e := gateway.WrapError(gateway.CodeInternal, "wrapped", cause)
	if !errors.Is(e, cause) {
		t.Errorf("errors.Is should reach cause: got false")
	}
	if e.Error() == "" {
		t.Errorf("Error() returned empty string")
	}
}

// stubAdapter is the minimum Adapter implementation needed to call
// Subscribe in tests. Run is never invoked.
type stubAdapter struct{ name string }

func (s stubAdapter) Name() string                                    { return s.name }
func (s stubAdapter) Run(_ context.Context, _ *gateway.Gateway) error { return nil }
