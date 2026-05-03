package workflow

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sync"
	"time"
)

// JSONLAuditSink writes one JSON object per audit event, newline-delimited
// (NDJSON). Suitable for piping into jq, ingesting into log aggregators,
// or replaying through a downstream tool. Each line is a self-describing
// record so the format is forward-compatible with new payload fields.
//
// Concurrency: Emit is goroutine-safe — the mutex serializes encoder
// writes so partial lines never interleave.
type JSONLAuditSink struct {
	W io.Writer

	mu  sync.Mutex
	enc *json.Encoder
}

// NewJSONLAuditSinkFile opens (or creates and truncates) path and returns
// a sink writing to it. The caller must Close the underlying file when
// done; sinks don't own writer lifecycle (matching StdoutAuditSink).
func NewJSONLAuditSinkFile(path string) (*JSONLAuditSink, *os.File, error) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return nil, nil, fmt.Errorf("audit_jsonl: open %s: %w", path, err)
	}
	return &JSONLAuditSink{W: f}, f, nil
}

// auditEventJSON is the public record shape — independent of AuditEvent's
// Go layout so future Go-side renames don't change the on-disk format.
type auditEventJSON struct {
	EventID    string         `json:"event_id"`
	RunID      string         `json:"run_id"`
	NodeID     string         `json:"node_id,omitempty"`
	AttemptNo  int            `json:"attempt_no,omitempty"`
	FromState  string         `json:"from_state,omitempty"`
	ToState    string         `json:"to_state"`
	Reason     string         `json:"reason,omitempty"`
	Actor      string         `json:"actor"`
	OccurredAt time.Time      `json:"occurred_at"`
	Payload    map[string]any `json:"payload,omitempty"`
}

// Emit appends one NDJSON record. A failing write returns the error so
// MultiAuditSink can route it through its OnError hook.
func (s *JSONLAuditSink) Emit(_ context.Context, ev AuditEvent) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.W == nil {
		return fmt.Errorf("audit_jsonl: writer not configured")
	}
	if s.enc == nil {
		s.enc = json.NewEncoder(s.W)
	}
	occurred := ev.OccurredAt
	if occurred.IsZero() {
		occurred = time.Now()
	}
	rec := auditEventJSON{
		EventID:    ev.EventID,
		RunID:      ev.RunID,
		NodeID:     ev.NodeID,
		AttemptNo:  ev.AttemptNo,
		FromState:  ev.FromState,
		ToState:    ev.ToState,
		Reason:     ev.Reason,
		Actor:      ev.Actor,
		OccurredAt: occurred,
		Payload:    ev.Payload,
	}
	return s.enc.Encode(rec)
}
