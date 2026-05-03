package workflow

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"time"
)

// StdoutAuditSink writes a one-line summary of every audit event to a
// writer (default os.Stdout). It is the simplest sink — useful for local
// runs and for debugging an unfamiliar workflow.
//
// Format:
//
//	2026-05-02T09:31:14Z run=abc123 node=stage1 attempt=1 pending→running   actor=system
//
// Concurrency: Emit is goroutine-safe. The mutex serializes writes so
// the output stays line-coherent even when multiple goroutines emit at
// once (parallel loops, fan-out hooks).
type StdoutAuditSink struct {
	W io.Writer
	// IncludePayload is false by default to keep the line terse. Set to
	// true to append the payload JSON; useful when debugging.
	IncludePayload bool

	mu sync.Mutex
}

// NewStdoutAuditSink returns a sink writing to os.Stdout. Tests usually
// construct StdoutAuditSink{W: &buf} directly.
func NewStdoutAuditSink() *StdoutAuditSink {
	return &StdoutAuditSink{W: os.Stdout}
}

// Emit writes one line per event. Errors from the writer surface to the
// runner; MultiAuditSink swallows them per-sink so a broken stdout doesn't
// block the SQLite sink.
func (s *StdoutAuditSink) Emit(_ context.Context, ev AuditEvent) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	w := s.W
	if w == nil {
		w = os.Stdout
	}
	occurred := ev.OccurredAt
	if occurred.IsZero() {
		occurred = time.Now()
	}
	from := ev.FromState
	if from == "" {
		from = "·"
	}
	var sb strings.Builder
	fmt.Fprintf(&sb, "%s run=%s", occurred.UTC().Format(time.RFC3339), ev.RunID)
	if ev.NodeID != "" {
		fmt.Fprintf(&sb, " node=%s", ev.NodeID)
	}
	if ev.AttemptNo > 0 {
		fmt.Fprintf(&sb, " attempt=%d", ev.AttemptNo)
	}
	fmt.Fprintf(&sb, " %s→%s actor=%s", from, ev.ToState, ev.Actor)
	if ev.Reason != "" {
		fmt.Fprintf(&sb, " reason=%q", ev.Reason)
	}
	if s.IncludePayload && len(ev.Payload) > 0 {
		fmt.Fprintf(&sb, " payload=%s", mustMarshalJSON(ev.Payload))
	}
	sb.WriteByte('\n')
	if _, err := io.WriteString(w, sb.String()); err != nil {
		return fmt.Errorf("stdout audit sink: %w", err)
	}
	return nil
}
