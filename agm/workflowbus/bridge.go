// Package workflowbus connects a workflow.Runner to the agm-bus broker so
// Gate nodes in a running workflow can be signaled by external systems
// (Discord messages, Matrix replies, webhook-driven events, etc.) via the
// bus's A2A routing layer.
//
// The bridge is a client of the broker: it registers a pseudo-session and
// listens for FrameDeliver frames addressed to it. Any frame whose
// Extra["kind"] == "gate" (or whose Text starts with "gate:<name>") is
// translated to runner.Signal(name), unblocking any matching Gate node.
//
// Located under agm/ rather than pkg/ because agm/internal/bus is only
// importable from agm/. pkg/workflow stays independent of bus wire
// protocol, and agm/internal/bus stays ignorant of workflow semantics;
// both are dependencies of workflowbus so the coupling lives here.
package workflowbus

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/vbonnet/dear-agent/agm/internal/bus"
	"github.com/vbonnet/dear-agent/pkg/workflow"
)

// Signaler is the minimal workflow.Runner surface the bridge needs. Keeps
// tests free of Runner construction — a simple fake satisfies this.
type Signaler interface {
	Signal(name string)
}

// Bridge is the agm-bus ↔ workflow.Runner connector. Construct with New
// and call Start(ctx); it blocks until ctx cancels or the bus connection
// errors unrecoverably.
//
// Bridge automatically reconnects to the broker if the socket drops. Each
// reconnect re-sends the Hello frame with the configured SessionID; a
// stale registration from a crashed prior Bridge is unregistered by the
// broker when the socket closes, so new Hellos are accepted.
type Bridge struct {
	// SessionID is the pseudo-session name the bridge registers as.
	// Workflows signal external sources by sending to this id; external
	// sources (Discord, Matrix, human-driven agm bus send) send to this
	// id to unblock gates.
	SessionID string

	// SocketPath is the broker's unix socket. Empty uses bus.SocketPath()
	// which honors AGM_BUS_SOCKET.
	SocketPath string

	// Signaler receives the runner.Signal(name) call for each matched gate
	// frame. Typically a *workflow.Runner.
	Signaler Signaler

	// Logger is used for adapter-level events.
	Logger *slog.Logger

	// ReconnectDelay throttles reconnect attempts after an error. Zero
	// defaults to 2 seconds.
	ReconnectDelay time.Duration
}

// New returns a Bridge with sensible defaults. sessionID and signaler
// are required; the caller owns lifecycle (Start + ctx cancellation).
func New(sessionID string, signaler Signaler) *Bridge {
	return &Bridge{
		SessionID:      sessionID,
		Signaler:       signaler,
		Logger:         slog.Default(),
		ReconnectDelay: 2 * time.Second,
	}
}

// Start runs the connect-read loop until ctx is cancelled. Returns ctx.Err()
// on clean shutdown, or an unrecoverable error (nil signaler, empty
// session id) on startup.
func (b *Bridge) Start(ctx context.Context) error {
	if b.Signaler == nil {
		return errors.New("workflowbus: Signaler is required")
	}
	if b.SessionID == "" {
		return errors.New("workflowbus: SessionID is required")
	}
	if b.Logger == nil {
		b.Logger = slog.Default()
	}
	if b.ReconnectDelay <= 0 {
		b.ReconnectDelay = 2 * time.Second
	}

	for {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		err := b.connectAndRead(ctx)
		if ctx.Err() != nil {
			return ctx.Err()
		}
		b.Logger.Warn("workflowbus: reconnecting after error", "err", err,
			"delay", b.ReconnectDelay)
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(b.ReconnectDelay):
		}
	}
}

// connectAndRead opens a single broker connection, says Hello, and reads
// frames until an error or ctx cancellation. Errors are returned; the
// outer loop reconnects.
func (b *Bridge) connectAndRead(ctx context.Context) error {
	path := b.SocketPath
	if path == "" {
		p, err := bus.SocketPath()
		if err != nil {
			return fmt.Errorf("resolve socket: %w", err)
		}
		path = p
	}
	dialer := net.Dialer{Timeout: time.Second}
	dialCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	conn, err := dialer.DialContext(dialCtx, "unix", path)
	if err != nil {
		return fmt.Errorf("dial %s: %w", path, err)
	}
	defer func() { _ = conn.Close() }()

	// Close conn when ctx cancels so the reader unblocks.
	closeOnDone := make(chan struct{})
	defer close(closeOnDone)
	go func() {
		select {
		case <-ctx.Done():
			_ = conn.Close()
		case <-closeOnDone:
		}
	}()

	if err := bus.WriteFrame(conn, &bus.Frame{
		Type: bus.FrameHello, From: b.SessionID,
	}); err != nil {
		return fmt.Errorf("hello: %w", err)
	}

	br := bufio.NewReader(conn)
	// Read the Welcome so we know the broker accepted us. An error frame
	// here (duplicate session id, malformed hello) is fatal for this
	// connection — the outer loop retries after backoff.
	welcome, err := bus.ReadFrame(br)
	if err != nil {
		return fmt.Errorf("read welcome: %w", err)
	}
	if welcome.Type == bus.FrameError {
		return fmt.Errorf("broker rejected hello: %s (%s)",
			welcome.Message, welcome.Code)
	}
	b.Logger.Info("workflowbus: connected", "session", b.SessionID)

	for {
		f, err := bus.ReadFrame(br)
		if err != nil {
			if errors.Is(err, io.EOF) {
				return errors.New("broker disconnected")
			}
			if ctx.Err() != nil {
				return ctx.Err()
			}
			return fmt.Errorf("read: %w", err)
		}
		b.handleFrame(f)
	}
}

// handleFrame dispatches one incoming frame. Only FrameDeliver frames are
// relevant; other types are logged and dropped.
func (b *Bridge) handleFrame(f *bus.Frame) {
	if f.Type != bus.FrameDeliver {
		b.Logger.Debug("workflowbus: ignoring non-deliver frame", "type", f.Type)
		return
	}
	gateName, ok := extractGateName(f)
	if !ok {
		b.Logger.Debug("workflowbus: deliver not gate-shaped, ignoring",
			"from", f.From, "text", truncate(f.Text, 80))
		return
	}
	b.Logger.Info("workflowbus: signaling gate", "gate", gateName, "from", f.From)
	b.Signaler.Signal(gateName)
}

// extractGateName returns the gate name if f is a gate-signal frame.
// Recognized shapes:
//  1. f.Extra["kind"] == "gate" AND f.Extra["gate"] is set — preferred.
//     Carries the gate name in Extra so Text can be arbitrary human text.
//  2. f.Text starts with "gate:<name>" — simpler human-typed form ideal
//     for Discord/Matrix users. Trailing whitespace/text after the name
//     is ignored.
func extractGateName(f *bus.Frame) (string, bool) {
	if kind, ok := f.Extra["kind"]; ok && kind == "gate" {
		if name, ok := f.Extra["gate"]; ok && name != "" {
			return name, true
		}
	}
	if strings.HasPrefix(f.Text, "gate:") {
		rest := strings.TrimPrefix(f.Text, "gate:")
		// Take everything up to the first whitespace as the name.
		name := strings.TrimSpace(strings.SplitN(rest, " ", 2)[0])
		if name != "" {
			return name, true
		}
	}
	return "", false
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

// Compile-time check: *workflow.Runner satisfies Signaler. If the Runner's
// Signal signature ever changes, this will fail at build time instead of
// causing a confusing runtime mismatch.
var _ Signaler = (*workflow.Runner)(nil)

// mu is reserved for future bridge state; currently the bridge is
// single-connection and needs no internal locking. Declared here so
// later additions (metrics, handler map) don't break the file layout.
var _ = sync.Mutex{}
