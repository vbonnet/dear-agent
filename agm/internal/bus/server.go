package bus

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// DefaultSocketPath is the canonical location of the broker's unix socket.
// Clients use this to connect; the server creates it during Start and
// removes it on shutdown. Set AGM_BUS_SOCKET to override (useful in tests).
const DefaultSocketPath = "~/.agm/bus.sock"

// SocketPath returns the effective socket path, honoring AGM_BUS_SOCKET and
// expanding a leading ~/ to $HOME. Returns the expanded absolute path.
func SocketPath() (string, error) {
	if env := os.Getenv("AGM_BUS_SOCKET"); env != "" {
		return expandHome(env)
	}
	return expandHome(DefaultSocketPath)
}

func expandHome(path string) (string, error) {
	if len(path) >= 2 && path[:2] == "~/" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("expand home: %w", err)
		}
		return filepath.Join(home, path[2:]), nil
	}
	return path, nil
}

// Server is the agm-bus daemon. Construct with NewServer and start with
// Start(ctx). Cancelling the context gracefully drains connections and
// removes the socket file.
type Server struct {
	SocketPath string
	Logger     *slog.Logger

	// Registry is the active-connection table. Exposed so tests and callers
	// can inspect state; in normal use the server owns it.
	Registry *Registry

	// Queue persists frames for offline sessions and replays them on
	// reconnect. May be nil in tests or minimal deploys, in which case
	// frames to offline targets error immediately instead of queueing.
	Queue *Queue

	// ACL enforces who-can-send-to-whom. Nil ACL or nil *ACL inside the
	// ReloadableACL means allow-all; see ACL.Check. Keeping this optional
	// lets single-user dev setups skip configuration entirely.
	ACL interface {
		Check(sender, target string) ACLDecision
	}

	listener net.Listener
	wg       sync.WaitGroup
}

// NewServer returns a Server configured with the given socket path (empty
// string means SocketPath()). If logger is nil a discard logger is used.
// The returned server has no Queue set; callers who want offline delivery
// should construct one via NewQueue and assign it before Start.
func NewServer(socketPath string, logger *slog.Logger) (*Server, error) {
	if socketPath == "" {
		p, err := SocketPath()
		if err != nil {
			return nil, err
		}
		socketPath = p
	}
	if logger == nil {
		logger = slog.New(slog.NewTextHandler(io.Discard, nil))
	}
	return &Server{
		SocketPath: socketPath,
		Logger:     logger,
		Registry:   NewRegistry(),
	}, nil
}

// Start binds the unix socket and serves connections until ctx is cancelled
// or the listener fails. Returns the reason the server stopped. Remove the
// socket file on exit regardless of outcome — leaving a stale socket behind
// breaks subsequent starts.
func (s *Server) Start(ctx context.Context) error {
	if err := os.MkdirAll(filepath.Dir(s.SocketPath), 0o755); err != nil {
		return fmt.Errorf("create socket dir: %w", err)
	}
	// Remove any stale socket from a prior run. Unix sockets do not clean up
	// automatically; a leftover file makes Listen fail with "address in use".
	_ = os.Remove(s.SocketPath)

	var lc net.ListenConfig
	l, err := lc.Listen(ctx, "unix", s.SocketPath)
	if err != nil {
		return fmt.Errorf("listen %s: %w", s.SocketPath, err)
	}
	// Restrict to owner; the broker is a local-only primitive.
	if err := os.Chmod(s.SocketPath, 0o600); err != nil {
		_ = l.Close()
		return fmt.Errorf("chmod socket: %w", err)
	}
	s.listener = l
	s.Logger.Info("bus listening", "socket", s.SocketPath)

	// Close the listener when ctx is cancelled so Accept returns.
	go func() {
		<-ctx.Done()
		_ = l.Close()
	}()

	for {
		conn, err := l.Accept()
		if err != nil {
			// ctx cancellation closes the listener, which we treat as a
			// clean stop; any other Accept error is real.
			if errors.Is(ctx.Err(), context.Canceled) || errors.Is(ctx.Err(), context.DeadlineExceeded) {
				s.drain()
				_ = os.Remove(s.SocketPath)
				return nil
			}
			s.drain()
			_ = os.Remove(s.SocketPath)
			return fmt.Errorf("accept: %w", err)
		}
		s.wg.Add(1)
		go func(c net.Conn) {
			defer s.wg.Done()
			s.handleConn(ctx, c)
		}(conn)
	}
}

// drain waits for in-flight connection handlers to finish.
func (s *Server) drain() {
	done := make(chan struct{})
	go func() {
		s.wg.Wait()
		close(done)
	}()
	// Hard cap so a stuck handler doesn't hold shutdown indefinitely.
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		s.Logger.Warn("bus shutdown: handlers timed out, exiting anyway")
	}
}

// handleConn reads frames from a single client until EOF or protocol error.
// The connection lifecycle:
//  1. First frame must be Hello. Server replies Welcome and registers.
//  2. Subsequent Sends are routed to the target's Delivery.
//  3. On disconnect (EOF, error, Bye), the session is unregistered.
func (s *Server) handleConn(ctx context.Context, conn net.Conn) {
	defer func() { _ = conn.Close() }()

	br := bufio.NewReader(conn)

	// Phase 1: wait for Hello.
	helloFrame, err := ReadFrame(br)
	if err != nil {
		s.Logger.Debug("conn: read hello failed", "err", err)
		return
	}
	if helloFrame.Type != FrameHello {
		_ = WriteFrame(conn, &Frame{
			Type: FrameError, Code: ErrBadFrame,
			Message: fmt.Sprintf("first frame must be %q, got %q", FrameHello, helloFrame.Type),
		})
		return
	}
	if err := helloFrame.Validate(); err != nil {
		_ = WriteFrame(conn, &Frame{Type: FrameError, Code: ErrBadFrame, Message: err.Error()})
		return
	}
	sessionID := helloFrame.From
	delivery := &connDelivery{conn: conn}
	if err := s.Registry.Register(sessionID, delivery); err != nil {
		_ = WriteFrame(conn, &Frame{Type: FrameError, Code: ErrBadFrame, Message: err.Error()})
		return
	}
	defer func() {
		if d := s.Registry.Unregister(sessionID); d != nil {
			_ = d.Close()
		}
	}()

	if err := WriteFrame(conn, &Frame{Type: FrameWelcome, To: sessionID}); err != nil {
		s.Logger.Debug("conn: write welcome failed", "session", sessionID, "err", err)
		return
	}
	s.Logger.Info("session connected", "session", sessionID)

	// Replay any queued frames from while this session was offline.
	// Drain-then-deliver; if a deliver fails the session is probably
	// half-closed, so we bail and the next reconnect will re-drain (empty
	// since Drain truncated) while the frame is lost. Acceptable: the
	// sender got an Ack when they queued it; at-most-once under crash
	// between queue-drain and deliver is the documented tradeoff.
	if s.Queue != nil {
		queued, qerr := s.Queue.Drain(sessionID)
		if qerr != nil {
			s.Logger.Warn("queue drain had errors", "session", sessionID, "err", qerr)
		}
		for _, f := range queued {
			if err := delivery.Deliver(f); err != nil {
				s.Logger.Debug("replay deliver failed", "session", sessionID, "err", err)
				return
			}
		}
	}

	// Phase 2: route frames.
	for {
		if ctx.Err() != nil {
			return
		}
		frame, err := ReadFrame(br)
		if err != nil {
			if !errors.Is(err, io.EOF) {
				s.Logger.Debug("conn: read failed", "session", sessionID, "err", err)
			}
			return
		}
		s.dispatch(sessionID, conn, frame)
	}
}

// dispatch routes one frame from a client according to its Type. The server
// stamps From authoritatively (from the authenticated session id) before
// validation so clients never need to — and cannot successfully — set it.
func (s *Server) dispatch(sessionID string, writer io.Writer, f *Frame) {
	// Server-authoritative From: overwrite whatever the client sent (if any).
	// Done before Validate so the frame passes the "From is required" check
	// without burdening clients to set their own id on every send.
	if f.Type == FrameSend || f.Type == FramePermissionRequest {
		f.From = sessionID
	}
	if err := f.Validate(); err != nil {
		_ = WriteFrame(writer, &Frame{
			Type: FrameError, ID: f.ID, Code: ErrBadFrame, Message: err.Error(),
		})
		return
	}
	switch f.Type { //nolint:exhaustive // inbound-from-client types only; default rejects server-only shapes
	case FrameSend:
		s.routeSend(writer, f)
	case FramePermissionRequest:
		s.routeSend(writer, f) // Same routing shape; different meaning to receiver.
	case FramePermissionVerdict:
		s.routeVerdict(writer, f)
	case FrameBye:
		// Logged by caller via defer on Unregister.
	default:
		_ = WriteFrame(writer, &Frame{
			Type: FrameError, ID: f.ID, Code: ErrBadFrame,
			Message: fmt.Sprintf("unexpected frame from client: %s", f.Type),
		})
	}
}

// routeSend looks up the target and delivers the frame (converting Send to
// Deliver on the wire so the recipient can distinguish its own sends from
// incoming traffic). Acks the sender; if the target is offline, the frame
// is queued for redelivery (Phase 1: drop with error; queue support lands
// in queue.go).
func (s *Server) routeSend(senderWriter io.Writer, f *Frame) {
	target := f.To

	// ACL check (if configured) runs before routing so unauthorized sends
	// don't even consume queue space. Self-sends always pass (ACL.Check
	// handles this), so sessions can heartbeat themselves.
	if s.ACL != nil {
		d := s.ACL.Check(f.From, target)
		if !d.Allowed {
			_ = WriteFrame(senderWriter, &Frame{
				Type: FrameError, ID: f.ID, Code: ErrNotAllowed,
				Message: fmt.Sprintf("%s -> %s denied by ACL (%s)", f.From, target, d.Reason),
			})
			return
		}
	}

	// Deliver as FrameDeliver (or keep FramePermissionRequest — those pass
	// through unchanged so the receiver sees the right intent).
	if f.Type == FrameSend {
		f.Type = FrameDeliver
	}

	d, err := s.Registry.Route(target)
	if err != nil {
		// Target offline — queue if possible, else return an error.
		if s.Queue == nil {
			_ = WriteFrame(senderWriter, &Frame{
				Type: FrameError, ID: f.ID, Code: ErrUnknownTarget,
				Message: fmt.Sprintf("target %q is offline", target),
			})
			return
		}
		if qerr := s.Queue.Append(target, f); qerr != nil {
			_ = WriteFrame(senderWriter, &Frame{
				Type: FrameError, ID: f.ID, Code: ErrInternal,
				Message: fmt.Sprintf("queue append failed: %v", qerr),
			})
			return
		}
		_ = WriteFrame(senderWriter, &Frame{Type: FrameAck, ID: f.ID, Message: "queued"})
		return
	}
	if err := d.Deliver(f); err != nil {
		_ = WriteFrame(senderWriter, &Frame{
			Type: FrameError, ID: f.ID, Code: ErrInternal,
			Message: fmt.Sprintf("deliver to %q failed: %v", target, err),
		})
		return
	}
	_ = WriteFrame(senderWriter, &Frame{Type: FrameAck, ID: f.ID, Message: "delivered"})
}

// routeVerdict routes a permission_verdict back to the original worker by
// relying on f.To being set to the worker's session id. The worker's channel
// adapter correlates the ID to an open permission request.
func (s *Server) routeVerdict(senderWriter io.Writer, f *Frame) {
	d, err := s.Registry.Route(f.To)
	if err != nil {
		_ = WriteFrame(senderWriter, &Frame{
			Type: FrameError, ID: f.ID, Code: ErrUnknownTarget,
			Message: fmt.Sprintf("worker %q is offline; verdict dropped", f.To),
		})
		return
	}
	if err := d.Deliver(f); err != nil {
		_ = WriteFrame(senderWriter, &Frame{
			Type: FrameError, ID: f.ID, Code: ErrInternal, Message: err.Error(),
		})
		return
	}
	_ = WriteFrame(senderWriter, &Frame{Type: FrameAck, ID: f.ID, Message: "verdict delivered"})
}

// connDelivery is the Delivery impl that wraps a net.Conn. All writes are
// serialized by an internal mutex so concurrent deliveries from different
// goroutines don't interleave bytes mid-frame on the socket.
type connDelivery struct {
	mu   sync.Mutex
	conn net.Conn
}

func (c *connDelivery) Deliver(f *Frame) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return WriteFrame(c.conn, f)
}

func (c *connDelivery) Close() error {
	return c.conn.Close()
}
