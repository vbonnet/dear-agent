package workflowbus

import (
	"bufio"
	"context"
	"fmt"
	"log/slog"
	"net"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/vbonnet/dear-agent/agm/internal/bus"
)

// fakeSignaler captures each Signal(name) call so tests can assert on them.
// Thread-safe; the bridge reads from a single goroutine but tests read
// from the main goroutine.
type fakeSignaler struct {
	mu    sync.Mutex
	calls []string
}

func (f *fakeSignaler) Signal(name string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, name)
}

func (f *fakeSignaler) snapshot() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]string, len(f.calls))
	copy(out, f.calls)
	return out
}

// startTestBroker spins up a real bus.Server on a short unix socket path.
// The macOS 104-byte limit means t.TempDir() is too long; use /tmp.
func startTestBroker(t *testing.T) *bus.Server {
	t.Helper()
	sockDir, err := os.MkdirTemp("/tmp", "wfbus-*") //nolint:usetesting // socket path-length constraint
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(sockDir) })
	s, err := bus.NewServer(filepath.Join(sockDir, "s"), nil)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		_ = s.Start(ctx)
		close(done)
	}()
	t.Cleanup(func() {
		cancel()
		select {
		case <-done:
		case <-time.After(3 * time.Second):
		}
	})
	return s
}

// senderClient connects to the broker as a simple client so tests can
// dispatch frames to the bridge. Returns the connection + a function to
// send frames (server stamps From; we supply To + Type + Text/Extra).
func senderClient(t *testing.T, sockPath, sessionID string) (net.Conn, *bufio.Reader) {
	t.Helper()
	conn, err := net.Dial("unix", sockPath)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	if err := bus.WriteFrame(conn, &bus.Frame{
		Type: bus.FrameHello, From: sessionID,
	}); err != nil {
		t.Fatalf("hello: %v", err)
	}
	r := bufio.NewReader(conn)
	welcome, err := bus.ReadFrame(r)
	if err != nil {
		t.Fatalf("welcome: %v", err)
	}
	if welcome.Type != bus.FrameWelcome {
		t.Fatalf("expected welcome, got %+v", welcome)
	}
	return conn, r
}

// startBridge wires a Bridge to the test broker + a fakeSignaler.
// Returns the signaler and a cancel func; cleanup is registered.
func startBridge(t *testing.T, sockPath, sessionID string) *fakeSignaler {
	t.Helper()
	fs := &fakeSignaler{}
	b := &Bridge{
		SessionID:      sessionID,
		SocketPath:     sockPath,
		Signaler:       fs,
		Logger:         slog.New(slog.NewTextHandler(nullWriter{}, nil)),
		ReconnectDelay: 100 * time.Millisecond,
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		_ = b.Start(ctx)
		close(done)
	}()
	t.Cleanup(func() {
		cancel()
		select {
		case <-done:
		case <-time.After(3 * time.Second):
		}
	})
	// Wait briefly for the bridge to register.
	time.Sleep(100 * time.Millisecond)
	return fs
}

// nullWriter drops all writes — keeps test output clean when slog is
// configured to write somewhere.
type nullWriter struct{}

func (nullWriter) Write(p []byte) (int, error) { return len(p), nil }

func TestBridgeSignalsOnGatePrefix(t *testing.T) {
	s := startTestBroker(t)
	fs := startBridge(t, s.SocketPath, "wf-test")

	// Another session sends "gate:approve" to wf-test.
	sender, _ := senderClient(t, s.SocketPath, "human")
	if err := bus.WriteFrame(sender, &bus.Frame{
		Type: bus.FrameSend, ID: "m1", To: "wf-test", Text: "gate:approve",
	}); err != nil {
		t.Fatal(err)
	}

	// Wait for the signal to propagate.
	waitForSignals(t, fs, 1, time.Second)
	if got := fs.snapshot(); len(got) != 1 || got[0] != "approve" {
		t.Errorf("calls = %v, want [approve]", got)
	}
}

func TestBridgeSignalsOnExtraKind(t *testing.T) {
	s := startTestBroker(t)
	fs := startBridge(t, s.SocketPath, "wf-test-2")

	sender, _ := senderClient(t, s.SocketPath, "human")
	if err := bus.WriteFrame(sender, &bus.Frame{
		Type: bus.FrameSend, ID: "m2", To: "wf-test-2",
		Text:  "anything — text is informational when extras carry the gate",
		Extra: map[string]string{"kind": "gate", "gate": "review-done"},
	}); err != nil {
		t.Fatal(err)
	}
	waitForSignals(t, fs, 1, time.Second)
	if got := fs.snapshot(); len(got) != 1 || got[0] != "review-done" {
		t.Errorf("calls = %v, want [review-done]", got)
	}
}

func TestBridgeIgnoresNonGateMessages(t *testing.T) {
	s := startTestBroker(t)
	fs := startBridge(t, s.SocketPath, "wf-test-3")

	sender, _ := senderClient(t, s.SocketPath, "human")
	// Regular chat-style message — not a gate signal.
	if err := bus.WriteFrame(sender, &bus.Frame{
		Type: bus.FrameSend, ID: "m3", To: "wf-test-3",
		Text: "hello, how's the workflow going?",
	}); err != nil {
		t.Fatal(err)
	}
	time.Sleep(250 * time.Millisecond)
	if got := fs.snapshot(); len(got) != 0 {
		t.Errorf("non-gate message should not signal; got %v", got)
	}
}

func TestBridgeHandlesMultipleSignalsInSequence(t *testing.T) {
	s := startTestBroker(t)
	fs := startBridge(t, s.SocketPath, "wf-test-4")

	sender, _ := senderClient(t, s.SocketPath, "human")
	expected := []string{"step-1", "step-2", "step-3"}
	for _, name := range expected {
		if err := bus.WriteFrame(sender, &bus.Frame{
			Type: bus.FrameSend, ID: "m-" + name, To: "wf-test-4",
			Text: "gate:" + name,
		}); err != nil {
			t.Fatal(err)
		}
	}
	waitForSignals(t, fs, len(expected), time.Second)
	got := fs.snapshot()
	if len(got) != len(expected) {
		t.Fatalf("got %d signals, want %d", len(got), len(expected))
	}
	// Order should be preserved (single connection, FIFO delivery).
	for i, name := range expected {
		if got[i] != name {
			t.Errorf("signal[%d] = %q, want %q", i, got[i], name)
		}
	}
}

func TestBridgeReconnectsAfterBrokerDrop(t *testing.T) {
	// This test verifies the reconnect loop by dropping the bridge's
	// connection via unregister; the broker treats it as a disconnect
	// and the bridge's read loop errors, triggering reconnect.
	s := startTestBroker(t)
	fs := startBridge(t, s.SocketPath, "wf-reconnect")

	// Force-unregister the bridge's session from the broker — mimics a
	// half-closed socket. The broker Close()s the delivery, which the
	// bridge sees as EOF.
	d := s.Registry.Unregister("wf-reconnect")
	if d != nil {
		_ = d.Close()
	}

	// Wait for the bridge to reconnect and re-register.
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := s.Registry.Route("wf-reconnect"); err == nil {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if _, err := s.Registry.Route("wf-reconnect"); err != nil {
		t.Fatalf("bridge did not reconnect: %v", err)
	}

	// After reconnect, a new gate signal still reaches the signaler.
	sender, _ := senderClient(t, s.SocketPath, "human2")
	if err := bus.WriteFrame(sender, &bus.Frame{
		Type: bus.FrameSend, ID: "post-reconnect", To: "wf-reconnect",
		Text: "gate:post-reconnect",
	}); err != nil {
		t.Fatal(err)
	}
	waitForSignals(t, fs, 1, time.Second)
	calls := fs.snapshot()
	found := false
	for _, c := range calls {
		if c == "post-reconnect" {
			found = true
		}
	}
	if !found {
		t.Errorf("post-reconnect signal missing; calls = %v", calls)
	}
}

func TestBridgeRejectsEmptyConfig(t *testing.T) {
	b := &Bridge{}
	if err := b.Start(context.Background()); err == nil {
		t.Error("expected error for missing Signaler+SessionID")
	}
	b2 := &Bridge{Signaler: &fakeSignaler{}}
	if err := b2.Start(context.Background()); err == nil {
		t.Error("expected error for missing SessionID")
	}
}

func TestExtractGateNameForms(t *testing.T) {
	cases := []struct {
		name  string
		frame bus.Frame
		want  string
		ok    bool
	}{
		{
			name:  "text prefix",
			frame: bus.Frame{Text: "gate:approve"},
			want:  "approve",
			ok:    true,
		},
		{
			name:  "text prefix with trailing chat",
			frame: bus.Frame{Text: "gate:approve please"},
			want:  "approve",
			ok:    true,
		},
		{
			name: "extra kind+gate",
			frame: bus.Frame{
				Text:  "",
				Extra: map[string]string{"kind": "gate", "gate": "x"},
			},
			want: "x",
			ok:   true,
		},
		{
			name: "extra kind but no gate name",
			frame: bus.Frame{
				Text:  "",
				Extra: map[string]string{"kind": "gate"},
			},
			want: "",
			ok:   false,
		},
		{
			name:  "plain chat",
			frame: bus.Frame{Text: "hello there"},
			want:  "",
			ok:    false,
		},
		{
			name:  "empty",
			frame: bus.Frame{},
			want:  "",
			ok:    false,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := extractGateName(&tc.frame)
			if ok != tc.ok {
				t.Errorf("ok = %v, want %v", ok, tc.ok)
			}
			if got != tc.want {
				t.Errorf("name = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestBridgeRejectsDuplicateSessionOnReconnect(t *testing.T) {
	// If the session id is still registered when the bridge reconnects,
	// the broker rejects with ErrAlreadyRegistered. The bridge handles
	// this by backing off; over time the old registration drops and
	// reconnect succeeds. Exercised here by registering a bogus delivery
	// under the id first and freeing it after the bridge has tried once.
	s := startTestBroker(t)
	// Pre-register a placeholder so the bridge's first Hello fails.
	placeholder := &counterDelivery{}
	if err := s.Registry.Register("wf-dup", placeholder); err != nil {
		t.Fatal(err)
	}

	fs := &fakeSignaler{}
	b := &Bridge{
		SessionID:      "wf-dup",
		SocketPath:     s.SocketPath,
		Signaler:       fs,
		Logger:         slog.New(slog.NewTextHandler(nullWriter{}, nil)),
		ReconnectDelay: 50 * time.Millisecond,
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		_ = b.Start(ctx)
		close(done)
	}()
	defer func() {
		cancel()
		<-done
	}()

	// Give the bridge a chance to try and fail at least once.
	time.Sleep(200 * time.Millisecond)

	// Free the slot; next reconnect attempt should succeed.
	if d := s.Registry.Unregister("wf-dup"); d != nil {
		_ = d.Close()
	}

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := s.Registry.Route("wf-dup"); err == nil {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Error("bridge did not successfully reconnect after duplicate-session freed")
}

// counterDelivery is a minimal Delivery for tests that need to register
// a placeholder under a specific session id.
type counterDelivery struct {
	count atomic.Int64
}

func (c *counterDelivery) Deliver(_ *bus.Frame) error {
	c.count.Add(1)
	return nil
}
func (c *counterDelivery) Close() error { return nil }

// waitForSignals polls until the fakeSignaler has accumulated n calls or
// the deadline is hit. Fails the test on timeout.
func waitForSignals(t *testing.T, fs *fakeSignaler, n int, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if len(fs.snapshot()) >= n {
			return
		}
		time.Sleep(25 * time.Millisecond)
	}
	t.Fatalf("expected %d signals within %s; got %d: %v",
		n, timeout, len(fs.snapshot()), fs.snapshot())
}

// sanity compile
var _ = fmt.Sprintf
