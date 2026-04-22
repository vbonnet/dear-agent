package bus

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"net"
	"sync"
	"time"
)

// Emitter is a fire-and-forget client for publishing events to the
// agm-bus broker from system daemons (AGM delivery daemon, sentinel
// heartbeat monitor, etc.). It's intentionally degraded-but-working
// when the broker is down: all operations become no-ops, so wiring the
// emitter into a daemon doesn't gate that daemon on broker availability.
//
// Use this for side-channel observability events from infrastructure
// code — it's NOT the primary agent-to-agent message path. Worker and
// supervisor sessions use the TS/Bun channel MCP adapter for their A2A
// traffic; the Emitter is the SYSTEM path for events like
// "session_blocked" or "heartbeat_stale" that originate outside any
// running Claude session.
type Emitter struct {
	// SessionID is the sender id announced to the broker. Typical
	// values: "agm-daemon", "sentinel".
	SessionID string
	// SocketPath is where to reach the broker. Empty means resolve via
	// SocketPath() (honors AGM_BUS_SOCKET).
	SocketPath string
	// ReconnectDelay throttles reconnect attempts after a failure.
	// Zero defaults to 5s.
	ReconnectDelay time.Duration
	// DialTimeout caps each connection attempt. Zero defaults to 1s —
	// we don't want the emitter to block daemon startup when the broker
	// is down.
	DialTimeout time.Duration

	mu       sync.Mutex
	conn     net.Conn
	writer   *bufio.Writer
	stopped  bool
	lastErr  error
	lastTry  time.Time
}

// NewEmitter returns an Emitter ready to use. No network I/O until the
// first Emit call.
func NewEmitter(sessionID string) *Emitter {
	return &Emitter{
		SessionID:      sessionID,
		ReconnectDelay: 5 * time.Second,
		DialTimeout:    time.Second,
	}
}

// Emit sends a frame to the broker. The SessionID is stamped as From.
// Returns quickly on success; on failure, logs the error internally and
// returns nil — the caller should not gate on emitter success. If the
// broker is unreachable, future Emits retry after ReconnectDelay.
//
// Thread-safe. Frames are serialized onto the shared connection.
func (e *Emitter) Emit(ctx context.Context, frame *Frame) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	if e.stopped {
		return errors.New("emitter: closed")
	}
	frame.From = e.SessionID

	if err := e.ensureConn(ctx); err != nil {
		// Swallow — callers get nil and the event is dropped. Broker
		// consumers are expected to be resilient to missed events.
		e.lastErr = err
		return nil
	}

	if err := WriteFrame(e.writer, frame); err != nil {
		e.lastErr = err
		e.closeLocked()
		return nil
	}
	if err := e.writer.Flush(); err != nil {
		e.lastErr = err
		e.closeLocked()
		return nil
	}
	return nil
}

// EmitEvent is a convenience helper that packages a status/event message
// as a FrameSend-style frame targeting the broker itself (to=""). The
// broker treats empty-target sends as broadcast-to-subscribers; until
// subscription lands, these are visible only in broker logs + any
// adapter that pipes them to Discord/Matrix.
func (e *Emitter) EmitEvent(ctx context.Context, eventType, text string, meta map[string]string) error {
	return e.Emit(ctx, &Frame{
		Type:  FrameSend,
		Text:  fmt.Sprintf("[%s] %s", eventType, text),
		Extra: meta,
	})
}

// Close terminates the underlying connection and prevents future emits.
// Idempotent.
func (e *Emitter) Close() error {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.stopped = true
	e.closeLocked()
	return nil
}

// ensureConn opens a new broker connection if one isn't live. Returns
// an error if the broker is unreachable; the emitter throttles retries
// via ReconnectDelay so a wedged broker doesn't produce a dial storm.
func (e *Emitter) ensureConn(ctx context.Context) error {
	if e.conn != nil {
		return nil
	}
	if time.Since(e.lastTry) < e.ReconnectDelay && !e.lastTry.IsZero() {
		return fmt.Errorf("emitter: last dial failed; backing off (%v remaining)",
			e.ReconnectDelay-time.Since(e.lastTry))
	}
	e.lastTry = time.Now()

	path := e.SocketPath
	if path == "" {
		p, err := SocketPath()
		if err != nil {
			return fmt.Errorf("emitter: resolve socket: %w", err)
		}
		path = p
	}
	dialer := net.Dialer{Timeout: e.DialTimeout}
	dialCtx, cancel := context.WithTimeout(ctx, e.DialTimeout)
	defer cancel()
	conn, err := dialer.DialContext(dialCtx, "unix", path)
	if err != nil {
		return fmt.Errorf("emitter: dial %s: %w", path, err)
	}
	w := bufio.NewWriter(conn)

	// Send Hello so the broker registers us. Any error here means the
	// broker refused — close and report upstream.
	if err := WriteFrame(w, &Frame{Type: FrameHello, From: e.SessionID}); err != nil {
		_ = conn.Close()
		return fmt.Errorf("emitter: hello: %w", err)
	}
	if err := w.Flush(); err != nil {
		_ = conn.Close()
		return fmt.Errorf("emitter: hello flush: %w", err)
	}
	// Don't wait for the Welcome frame — the emitter is write-only and
	// we don't want to block on a half-duplex RTT. If the broker
	// rejects our Hello (duplicate session id) the next WriteFrame will
	// fail and trigger reconnect.
	e.conn = conn
	e.writer = w
	return nil
}

// closeLocked tears down the current conn. Caller must hold e.mu.
func (e *Emitter) closeLocked() {
	if e.conn != nil {
		_ = e.conn.Close()
		e.conn = nil
		e.writer = nil
	}
}

// LastErr exposes the most recent error for tests / diagnostics. Empty
// nil when the emitter has never hit an error or all errors have been
// cleared.
func (e *Emitter) LastErr() error {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.lastErr
}
