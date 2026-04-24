// Package bus implements the agm-bus broker: a long-lived local daemon that
// routes messages between AGM sessions and, via per-session channel adapters,
// between sessions and external platforms (Discord, Matrix).
//
// The broker is distinct from the in-process agm/internal/a2a/broker: that
// package is a single-process message bus scoped to one agm binary's lifetime;
// this package spans processes via a unix socket at ~/.agm/bus.sock.
//
// Wire protocol is newline-delimited JSON frames. Each frame has a Type tag
// that determines which payload fields are meaningful. Unknown types are
// dropped at the server; unknown fields on known types are ignored. This
// lets old clients co-exist with newer servers and vice versa during
// rolling restarts.
package bus

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"
)

// FrameType enumerates the kinds of wire messages clients and server exchange.
type FrameType string

const (
	// FrameHello is the first frame a client sends, announcing its session id.
	FrameHello FrameType = "hello"

	// FrameWelcome is the server's response to a successful Hello.
	FrameWelcome FrameType = "welcome"

	// FrameSend is a client-originated request to route a message to a target.
	FrameSend FrameType = "send"

	// FrameDeliver is a server-originated push of a routed message to a
	// client. The client sees its peer's Send as a Deliver.
	FrameDeliver FrameType = "deliver"

	// FrameAck acknowledges receipt of a Send. Carries the original frame id
	// and a status ("queued" for offline targets, "delivered" for live ones).
	FrameAck FrameType = "ack"

	// FrameError signals a routing or protocol failure. Carries the offending
	// frame id when known and a machine-readable code.
	FrameError FrameType = "error"

	// FramePermissionRequest is the server relaying a claude/channel permission
	// prompt from one session to a peer (supervisor) or human (Discord user).
	FramePermissionRequest FrameType = "permission_request"

	// FramePermissionVerdict is the peer's/human's allow|deny response to a
	// permission request.
	FramePermissionVerdict FrameType = "permission_verdict"

	// FrameBye is a graceful-shutdown signal from a client.
	FrameBye FrameType = "bye"
)

// ErrorCode is a stable identifier for failure reasons clients can branch on.
type ErrorCode string

// Standard error codes returned in FrameError.Code. Kept stable; clients
// key off these, so treat additions additively rather than renaming.
const (
	ErrUnknownTarget ErrorCode = "unknown_target"
	ErrNotAllowed    ErrorCode = "not_allowed"
	ErrBadFrame      ErrorCode = "bad_frame"
	ErrInternal      ErrorCode = "internal"
)

// Frame is a single wire message. Fields are tagged `omitempty` so the on-wire
// shape carries only the keys each type needs; serialization is compact and
// forward-compatible.
//
// Frames carry an ID (nanosecond-unique within a client) for request/response
// correlation: Acks and Errors echo the ID of the originating Send or
// PermissionRequest.
type Frame struct {
	Type    FrameType `json:"type"`
	ID      string    `json:"id,omitempty"`
	From    string    `json:"from,omitempty"`
	To      string    `json:"to,omitempty"`
	Text    string    `json:"text,omitempty"`
	TS      time.Time `json:"ts,omitempty"`

	// Permission-relay fields.
	ToolName     string    `json:"tool_name,omitempty"`
	Description  string    `json:"description,omitempty"`
	InputPreview string    `json:"input_preview,omitempty"`
	Verdict      string    `json:"verdict,omitempty"` // "allow" | "deny"

	// Error-frame fields.
	Code    ErrorCode `json:"code,omitempty"`
	Message string    `json:"message,omitempty"`

	// Extra allows channel adapters to attach routing metadata (e.g. Discord
	// chat_id) without the server interpreting it. Server forwards as-is.
	Extra map[string]string `json:"extra,omitempty"`
}

// Validate returns nil if the frame's required fields are set for its Type.
// The server runs this before accepting an inbound frame; client libraries
// can use it as a pre-flight check.
//
//nolint:gocyclo // one switch arm per frame type; splitting hurts readability more than it helps
func (f *Frame) Validate() error {
	if f.Type == "" {
		return errors.New("frame: missing type")
	}
	switch f.Type {
	case FrameHello:
		if f.From == "" {
			return errors.New("hello: missing from")
		}
	case FrameSend:
		if f.From == "" {
			return errors.New("send: missing from")
		}
		if f.To == "" {
			return errors.New("send: missing to")
		}
	case FrameDeliver:
		if f.From == "" || f.To == "" {
			return errors.New("deliver: missing from or to")
		}
	case FrameAck:
		if f.ID == "" {
			return errors.New("ack: missing id")
		}
	case FramePermissionRequest:
		if f.From == "" || f.To == "" || f.ToolName == "" {
			return errors.New("permission_request: missing from, to, or tool_name")
		}
	case FramePermissionVerdict:
		if f.ID == "" {
			return errors.New("permission_verdict: missing id")
		}
		if f.Verdict != "allow" && f.Verdict != "deny" {
			return fmt.Errorf("permission_verdict: verdict must be allow|deny, got %q", f.Verdict)
		}
	case FrameWelcome, FrameError, FrameBye:
		// No additional requirements.
	default:
		return fmt.Errorf("unknown frame type: %q", f.Type)
	}
	return nil
}

// WriteFrame serializes f as a single line of JSON plus a terminating newline.
// Callers should write frames sequentially on the same connection without
// interleaving bytes from other goroutines; the Reader/Writer wrappers in
// this package enforce that.
func WriteFrame(w io.Writer, f *Frame) error {
	if f.TS.IsZero() {
		f.TS = time.Now().UTC()
	}
	b, err := json.Marshal(f)
	if err != nil {
		return fmt.Errorf("encode frame: %w", err)
	}
	if _, err := w.Write(b); err != nil {
		return fmt.Errorf("write frame: %w", err)
	}
	if _, err := w.Write([]byte{'\n'}); err != nil {
		return fmt.Errorf("write frame terminator: %w", err)
	}
	return nil
}

// ReadFrame consumes a single newline-terminated JSON frame from r.
// Returns io.EOF at a clean end-of-stream (no partial line buffered).
// A blank line is skipped (some clients send trailing \n on close).
func ReadFrame(r *bufio.Reader) (*Frame, error) {
	for {
		line, err := r.ReadBytes('\n')
		if err != nil {
			if errors.Is(err, io.EOF) && len(line) == 0 {
				return nil, io.EOF
			}
			if !errors.Is(err, io.EOF) {
				return nil, fmt.Errorf("read frame: %w", err)
			}
			// EOF with trailing partial line — treat as truncated.
			if strings.TrimSpace(string(line)) == "" {
				return nil, io.EOF
			}
			return nil, fmt.Errorf("read frame: truncated line before EOF")
		}
		if strings.TrimSpace(string(line)) == "" {
			continue
		}
		var f Frame
		if err := json.Unmarshal(line, &f); err != nil {
			return nil, fmt.Errorf("decode frame: %w", err)
		}
		return &f, nil
	}
}
