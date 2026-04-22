package bus

import (
	"bufio"
	"context"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// dialOrFatal opens a client connection to the server's socket.
func dialOrFatal(t *testing.T, socket string) net.Conn {
	t.Helper()
	var lastErr error
	for i := 0; i < 20; i++ { // up to ~1s waiting for the server to bind
		c, err := net.Dial("unix", socket)
		if err == nil {
			return c
		}
		lastErr = err
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("dial %s: %v", socket, lastErr)
	return nil
}

// helloRoundTrip sends a Hello on conn and reads the Welcome reply.
// Returns a *bufio.Reader caller can keep using for subsequent frames.
func helloRoundTrip(t *testing.T, conn net.Conn, sessionID string) *bufio.Reader {
	t.Helper()
	if err := WriteFrame(conn, &Frame{Type: FrameHello, From: sessionID}); err != nil {
		t.Fatalf("write hello: %v", err)
	}
	r := bufio.NewReader(conn)
	welcome, err := ReadFrame(r)
	if err != nil {
		t.Fatalf("read welcome: %v", err)
	}
	if welcome.Type != FrameWelcome {
		t.Fatalf("expected welcome, got %s", welcome.Type)
	}
	return r
}

func startServer(t *testing.T) (*Server, context.CancelFunc) {
	t.Helper()
	// macOS unix sockets are capped at ~104 bytes. t.TempDir() gives paths
	// over that limit for long test names, so create a short socket path
	// directly under /tmp.
	sockDir, err := os.MkdirTemp("/tmp", "bus-*") //nolint:usetesting // path-length constraint
	if err != nil {
		t.Fatalf("mkdtemp: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(sockDir) })
	socket := filepath.Join(sockDir, "s")
	s, err := NewServer(socket, nil)
	if err != nil {
		t.Fatalf("NewServer: %v", err)
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
			t.Log("server did not exit within 3s")
		}
	})
	return s, cancel
}

func TestServerRegistersOnHello(t *testing.T) {
	s, _ := startServer(t)
	conn := dialOrFatal(t, s.SocketPath)
	defer conn.Close()
	_ = helloRoundTrip(t, conn, "s1")
	// Give the server a moment to register.
	time.Sleep(50 * time.Millisecond)
	if _, err := s.Registry.Route("s1"); err != nil {
		t.Errorf("expected s1 registered: %v", err)
	}
}

func TestServerFirstFrameMustBeHello(t *testing.T) {
	s, _ := startServer(t)
	conn := dialOrFatal(t, s.SocketPath)
	defer conn.Close()

	// Send something other than Hello.
	_ = WriteFrame(conn, &Frame{Type: FrameSend, From: "s1", To: "s2"})
	r := bufio.NewReader(conn)
	frame, err := ReadFrame(r)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if frame.Type != FrameError || frame.Code != ErrBadFrame {
		t.Errorf("expected bad-frame error, got %+v", frame)
	}
}

func TestServerRoutesSend(t *testing.T) {
	s, _ := startServer(t)

	// Two peers connect, one sends to the other.
	conn1 := dialOrFatal(t, s.SocketPath)
	defer conn1.Close()
	r1 := helloRoundTrip(t, conn1, "sender")

	conn2 := dialOrFatal(t, s.SocketPath)
	defer conn2.Close()
	r2 := helloRoundTrip(t, conn2, "target")

	// Give registration a moment.
	time.Sleep(50 * time.Millisecond)

	// sender → target
	if err := WriteFrame(conn1, &Frame{
		Type: FrameSend, ID: "m1", To: "target", Text: "hello peer",
	}); err != nil {
		t.Fatalf("write send: %v", err)
	}

	// sender should get an Ack.
	ack, err := ReadFrame(r1)
	if err != nil {
		t.Fatalf("read ack: %v", err)
	}
	if ack.Type != FrameAck || ack.ID != "m1" {
		t.Errorf("expected ack for m1, got %+v", ack)
	}

	// target should get a Deliver.
	delivered, err := ReadFrame(r2)
	if err != nil {
		t.Fatalf("read deliver: %v", err)
	}
	if delivered.Type != FrameDeliver {
		t.Errorf("expected deliver, got %s", delivered.Type)
	}
	if delivered.From != "sender" {
		t.Errorf("From = %q, want sender (server should stamp authoritatively)", delivered.From)
	}
	if delivered.To != "target" || delivered.Text != "hello peer" {
		t.Errorf("unexpected deliver: %+v", delivered)
	}
}

func TestServerSpoofedFromOverriddenByServer(t *testing.T) {
	s, _ := startServer(t)
	conn1 := dialOrFatal(t, s.SocketPath)
	defer conn1.Close()
	r1 := helloRoundTrip(t, conn1, "alice")
	conn2 := dialOrFatal(t, s.SocketPath)
	defer conn2.Close()
	r2 := helloRoundTrip(t, conn2, "bob")
	time.Sleep(50 * time.Millisecond)

	// alice attempts to impersonate "eve" — server must overwrite From to
	// the authenticated session id.
	_ = WriteFrame(conn1, &Frame{Type: FrameSend, ID: "x", From: "eve", To: "bob", Text: "hi"})
	if _, err := ReadFrame(r1); err != nil {
		t.Fatalf("read ack: %v", err)
	}
	delivered, err := ReadFrame(r2)
	if err != nil {
		t.Fatalf("read deliver: %v", err)
	}
	if delivered.From != "alice" {
		t.Errorf("server did not overwrite From: got %q, want alice", delivered.From)
	}
}

func TestServerSendToOfflineTargetErrors(t *testing.T) {
	s, _ := startServer(t)
	conn := dialOrFatal(t, s.SocketPath)
	defer conn.Close()
	r := helloRoundTrip(t, conn, "sender")
	_ = WriteFrame(conn, &Frame{Type: FrameSend, ID: "m", To: "ghost", Text: "?"})
	reply, err := ReadFrame(r)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if reply.Type != FrameError || reply.Code != ErrUnknownTarget {
		t.Errorf("expected unknown_target error, got %+v", reply)
	}
}

func TestServerPermissionRelayRoundTrip(t *testing.T) {
	s, _ := startServer(t)

	// worker w1 asks supervisor s1 for permission; s1 allows; w1 gets verdict.
	worker := dialOrFatal(t, s.SocketPath)
	defer worker.Close()
	rw := helloRoundTrip(t, worker, "w1")

	sup := dialOrFatal(t, s.SocketPath)
	defer sup.Close()
	rs := helloRoundTrip(t, sup, "s1")
	time.Sleep(50 * time.Millisecond)

	// w1 → s1 (permission request)
	_ = WriteFrame(worker, &Frame{
		Type: FramePermissionRequest, ID: "p1", To: "s1",
		ToolName: "Bash", Description: "list dir",
	})
	// w1 should get an Ack.
	if ack, err := ReadFrame(rw); err != nil || ack.Type != FrameAck {
		t.Fatalf("w1 ack: %v, %+v", err, ack)
	}
	// s1 should receive the permission_request (passed through unchanged).
	req, err := ReadFrame(rs)
	if err != nil {
		t.Fatalf("read req: %v", err)
	}
	if req.Type != FramePermissionRequest || req.ToolName != "Bash" || req.From != "w1" {
		t.Errorf("unexpected req: %+v", req)
	}

	// s1 → w1 (verdict)
	_ = WriteFrame(sup, &Frame{
		Type: FramePermissionVerdict, ID: "p1", To: "w1", Verdict: "allow",
	})
	if ack, err := ReadFrame(rs); err != nil || ack.Type != FrameAck {
		t.Fatalf("s1 ack: %v, %+v", err, ack)
	}
	// w1 should receive the verdict.
	verdict, err := ReadFrame(rw)
	if err != nil {
		t.Fatalf("read verdict: %v", err)
	}
	if verdict.Type != FramePermissionVerdict || verdict.Verdict != "allow" || verdict.ID != "p1" {
		t.Errorf("unexpected verdict: %+v", verdict)
	}
}

func TestServerUnregistersOnDisconnect(t *testing.T) {
	s, _ := startServer(t)
	conn := dialOrFatal(t, s.SocketPath)
	_ = helloRoundTrip(t, conn, "transient")
	time.Sleep(50 * time.Millisecond)
	if s.Registry.Len() != 1 {
		t.Fatalf("pre-close registry size: %d", s.Registry.Len())
	}
	_ = conn.Close()

	// Registration should drop shortly after EOF.
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if s.Registry.Len() == 0 {
			return
		}
		time.Sleep(25 * time.Millisecond)
	}
	t.Errorf("session not unregistered within 2s")
}

func TestServerDuplicateSessionRejected(t *testing.T) {
	s, _ := startServer(t)
	c1 := dialOrFatal(t, s.SocketPath)
	defer c1.Close()
	_ = helloRoundTrip(t, c1, "dup")
	time.Sleep(50 * time.Millisecond)

	c2 := dialOrFatal(t, s.SocketPath)
	defer c2.Close()
	_ = WriteFrame(c2, &Frame{Type: FrameHello, From: "dup"})
	r2 := bufio.NewReader(c2)
	reply, err := ReadFrame(r2)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if reply.Type != FrameError {
		t.Errorf("expected error on duplicate session, got %+v", reply)
	}
	if !strings.Contains(reply.Message, "already registered") {
		t.Errorf("error message = %q, want mentions already registered", reply.Message)
	}
}

// startServerWithQueue is like startServer but also configures an on-disk
// offline-message queue so tests can exercise the queue-on-offline path.
func startServerWithQueue(t *testing.T) *Server {
	t.Helper()
	sockDir, err := os.MkdirTemp("/tmp", "bus-q-*") //nolint:usetesting // socket path-length constraint on macOS
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(sockDir) })
	qDir, err := os.MkdirTemp("/tmp", "busqd-*") //nolint:usetesting // paired with sockDir, keep both short
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(qDir) })
	s, err := NewServer(filepath.Join(sockDir, "s"), nil)
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}
	q, err := NewQueue(qDir)
	if err != nil {
		t.Fatalf("NewQueue: %v", err)
	}
	s.Queue = q

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
			t.Log("server did not exit within 3s")
		}
	})
	return s
}

func TestServerQueuesForOfflineTargetAndReplaysOnConnect(t *testing.T) {
	s := startServerWithQueue(t)

	// Sender connects and sends to an offline target.
	sender := dialOrFatal(t, s.SocketPath)
	defer sender.Close()
	rs := helloRoundTrip(t, sender, "sender")

	if err := WriteFrame(sender, &Frame{
		Type: FrameSend, ID: "q1", To: "later", Text: "stored",
	}); err != nil {
		t.Fatalf("write send: %v", err)
	}
	ack, err := ReadFrame(rs)
	if err != nil {
		t.Fatalf("read ack: %v", err)
	}
	if ack.Type != FrameAck || !strings.Contains(ack.Message, "queued") {
		t.Errorf("expected queued ack, got %+v", ack)
	}

	// Now "later" connects. It should receive the queued Deliver immediately
	// after Welcome.
	later := dialOrFatal(t, s.SocketPath)
	defer later.Close()
	rl := helloRoundTrip(t, later, "later")

	delivered, err := ReadFrame(rl)
	if err != nil {
		t.Fatalf("read queued deliver: %v", err)
	}
	if delivered.Type != FrameDeliver || delivered.Text != "stored" || delivered.From != "sender" {
		t.Errorf("unexpected replayed frame: %+v", delivered)
	}

	// Second connect should NOT replay again (queue drained).
	_ = later.Close()
	// Wait for the server to drop the registration.
	deadline := time.Now().Add(1 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := s.Registry.Route("later"); err != nil {
			break
		}
		time.Sleep(25 * time.Millisecond)
	}

	later2 := dialOrFatal(t, s.SocketPath)
	defer later2.Close()
	rl2 := helloRoundTrip(t, later2, "later")
	// No further frames should arrive in a short window.
	type res struct {
		f   *Frame
		err error
	}
	ch := make(chan res, 1)
	go func() {
		f, err := ReadFrame(rl2)
		ch <- res{f, err}
	}()
	select {
	case got := <-ch:
		t.Errorf("unexpected frame on second connect: %+v err=%v", got.f, got.err)
	case <-time.After(250 * time.Millisecond):
		// good — nothing arrived
	}
}

func TestServerEnforcesACL(t *testing.T) {
	s, _ := startServer(t)
	// Only allow alice -> bob; bob -> alice is implicitly denied.
	s.ACL = &ACL{Rules: []ACLRule{{From: "alice", To: "bob"}}}

	alice := dialOrFatal(t, s.SocketPath)
	defer alice.Close()
	ra := helloRoundTrip(t, alice, "alice")

	bob := dialOrFatal(t, s.SocketPath)
	defer bob.Close()
	rb := helloRoundTrip(t, bob, "bob")
	time.Sleep(50 * time.Millisecond)

	// alice -> bob: allowed by rule.
	_ = WriteFrame(alice, &Frame{Type: FrameSend, ID: "ok", To: "bob", Text: "hi"})
	ack, err := ReadFrame(ra)
	if err != nil {
		t.Fatalf("read alice ack: %v", err)
	}
	if ack.Type != FrameAck {
		t.Errorf("expected ack, got %+v", ack)
	}
	// bob receives deliver so inbox is clean for the next assertion.
	if _, err := ReadFrame(rb); err != nil {
		t.Fatalf("read bob deliver: %v", err)
	}

	// bob -> alice: denied.
	_ = WriteFrame(bob, &Frame{Type: FrameSend, ID: "nope", To: "alice", Text: "reverse"})
	reply, err := ReadFrame(rb)
	if err != nil {
		t.Fatalf("read bob reply: %v", err)
	}
	if reply.Type != FrameError || reply.Code != ErrNotAllowed {
		t.Errorf("expected not_allowed error, got %+v", reply)
	}
}

func TestSocketPathExpansion(t *testing.T) {
	// Set env and expect expansion.
	t.Setenv("AGM_BUS_SOCKET", "~/test-bus.sock")
	got, err := SocketPath()
	if err != nil {
		t.Fatal(err)
	}
	home, _ := osUserHomeDir()
	if got != filepath.Join(home, "test-bus.sock") {
		t.Errorf("SocketPath = %q, want home-expanded", got)
	}
}

// Tiny indirection so TestSocketPathExpansion uses the same expansion as
// production code without exposing expandHome publicly.
var osUserHomeDir = func() (string, error) {
	return expandHome("~/")
}

var _ = sync.Mutex{} // referenced to silence potential unused import if refactored
var _ io.Reader = (*bufio.Reader)(nil)
