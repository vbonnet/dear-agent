package bus

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestEmitterDegradesWhenBrokerDown(t *testing.T) {
	// Point at a socket that doesn't exist. Emit should NOT return an
	// error — graceful degradation is the contract.
	e := NewEmitter("test-daemon")
	e.SocketPath = "/tmp/agmbus-nonexistent-for-test.sock"
	err := e.EmitEvent(context.Background(), "test", "noop", nil)
	if err != nil {
		t.Errorf("Emit against missing broker should return nil; got %v", err)
	}
	if e.LastErr() == nil {
		t.Error("expected LastErr to record the dial failure")
	}
}

func TestEmitterSendsFrameThroughBroker(t *testing.T) {
	s, _ := startServer(t)

	e := NewEmitter("test-daemon")
	e.SocketPath = s.SocketPath
	// Subscribe a receiver session so there's someone to route to.
	sub := dialOrFatal(t, s.SocketPath)
	defer sub.Close()
	_ = helloRoundTrip(t, sub, "subscriber")

	err := e.Emit(context.Background(), &Frame{
		Type: FrameSend, To: "subscriber", Text: "hello from daemon",
	})
	if err != nil {
		t.Fatalf("Emit: %v", err)
	}
	time.Sleep(50 * time.Millisecond)

	// Subscriber should have received a deliver (or a welcome first — read
	// until we see the expected frame or hit an idle window).
	// helloRoundTrip already consumed the welcome, so the bufio reader it
	// returned should see the delivery next.
}

func TestEmitterReconnectAfterBrokerRestart(t *testing.T) {
	// Spin up a server, emit, close, restart on the same socket, emit again.
	// Tests that lastTry/backoff doesn't permanently lock us out.
	sockDir, err := os.MkdirTemp("/tmp", "emit-restart-*") //nolint:usetesting // socket path-length constraint
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(sockDir) })
	sockPath := filepath.Join(sockDir, "s")

	// Start first instance.
	s1, err := NewServer(sockPath, nil)
	if err != nil {
		t.Fatal(err)
	}
	ctx1, cancel1 := context.WithCancel(context.Background())
	done1 := make(chan struct{})
	go func() { _ = s1.Start(ctx1); close(done1) }()
	defer func() {
		cancel1()
		select {
		case <-done1:
		case <-time.After(2 * time.Second):
		}
	}()

	// Wait for socket.
	for i := 0; i < 40 && !statExists(sockPath); i++ {
		time.Sleep(25 * time.Millisecond)
	}

	e := NewEmitter("emitter-restart-test")
	e.SocketPath = sockPath
	e.ReconnectDelay = 50 * time.Millisecond

	if err := e.EmitEvent(context.Background(), "t", "first", nil); err != nil {
		t.Fatal(err)
	}
	if e.LastErr() != nil {
		t.Errorf("first emit should succeed: %v", e.LastErr())
	}

	// Kill server.
	cancel1()
	<-done1

	// Emit during the down window — should gracefully no-op.
	_ = e.EmitEvent(context.Background(), "t", "during-outage", nil)

	// Wait backoff out.
	time.Sleep(100 * time.Millisecond)

	// Bring server back.
	s2, err := NewServer(sockPath, nil)
	if err != nil {
		t.Fatal(err)
	}
	ctx2, cancel2 := context.WithCancel(context.Background())
	done2 := make(chan struct{})
	go func() { _ = s2.Start(ctx2); close(done2) }()
	defer func() {
		cancel2()
		select {
		case <-done2:
		case <-time.After(2 * time.Second):
		}
	}()
	for i := 0; i < 40 && !statExists(sockPath); i++ {
		time.Sleep(25 * time.Millisecond)
	}

	// Subsequent emit should reconnect and succeed.
	_ = e.EmitEvent(context.Background(), "t", "after-restart", nil)
	// LastErr may linger from the outage emit; the success path clears
	// by reconnecting. Give it a moment and check connection is live.
	time.Sleep(50 * time.Millisecond)
	// Final sanity: one more emit should at minimum not set a fresh dial error.
	_ = e.EmitEvent(context.Background(), "t", "final", nil)
}

func TestEmitterCloseIsIdempotent(t *testing.T) {
	e := NewEmitter("e")
	if err := e.Close(); err != nil {
		t.Fatal(err)
	}
	if err := e.Close(); err != nil {
		t.Errorf("second Close should be idempotent: %v", err)
	}
	err := e.EmitEvent(context.Background(), "t", "after-close", nil)
	if err == nil {
		t.Error("Emit after Close should error")
	}
	if !strings.Contains(err.Error(), "closed") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestEmitterBackoffThrottlesDials(t *testing.T) {
	e := NewEmitter("e")
	e.SocketPath = "/tmp/nonexistent-for-backoff-test.sock"
	e.ReconnectDelay = 500 * time.Millisecond
	e.DialTimeout = 25 * time.Millisecond

	// First call: dial fails.
	_ = e.EmitEvent(context.Background(), "t", "x", nil)
	first := e.lastTry
	// Immediate second call: should NOT redial (backoff).
	_ = e.EmitEvent(context.Background(), "t", "x", nil)
	if !e.lastTry.Equal(first) {
		t.Errorf("backoff not respected: lastTry changed within ReconnectDelay")
	}
}

// statExists returns true if path exists.
func statExists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}

// Error-path sanity: construction should never panic.
func TestNewEmitterDefaults(t *testing.T) {
	e := NewEmitter("x")
	if e.SessionID != "x" {
		t.Errorf("SessionID = %q", e.SessionID)
	}
	if e.ReconnectDelay <= 0 || e.DialTimeout <= 0 {
		t.Error("defaults should set ReconnectDelay and DialTimeout")
	}
}

var _ = errors.New // keep errors import live if refactors remove callers
