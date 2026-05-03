package workflow

import (
	"context"
	"fmt"
	"sync"
)

// EngramPublisher is the narrow interface the EngramAuditSink uses to push
// rows into the engram-research substrate. Phase 3 (FetchSource/AddSource)
// ships the concrete implementation; Phase 2 only needs a seam so workflows
// can declare engram_indexed durability and have audit rows surface in
// engram queries once the bridge is live.
//
// Implementations should be non-blocking — the runner can fan thousands of
// audit events through this sink during a single run. A buffered channel
// or async-write strategy is the expected pattern.
type EngramPublisher interface {
	Publish(ctx context.Context, kind string, payload map[string]any) error
}

// EngramAuditSink forwards every audit event to an EngramPublisher. The
// substrate goal is "every state transition is queryable", which the
// SQLite sink already satisfies for the workflow engine; this sink
// exposes the same events to the broader knowledge graph so a single
// `dear-agent search "topic"` can surface the runs that produced or
// referenced that topic.
//
// Failures from the publisher do not halt the run — see ADR-010 §D3
// "failure of one sink doesn't break the run". The sink stores the last
// error for tests/diagnostics via LastErr.
type EngramAuditSink struct {
	Publisher EngramPublisher

	mu      sync.Mutex
	lastErr error
}

// Emit forwards the event to the publisher. Errors are stored in
// LastErr but not returned to the runner — fan-out semantics are the
// caller's responsibility (use MultiAuditSink to route through SQLite +
// stdout + engram simultaneously).
func (s *EngramAuditSink) Emit(ctx context.Context, ev AuditEvent) error {
	if s == nil || s.Publisher == nil {
		return nil
	}
	payload := map[string]any{
		"event_id":    ev.EventID,
		"run_id":      ev.RunID,
		"node_id":     ev.NodeID,
		"attempt_no":  ev.AttemptNo,
		"from_state":  ev.FromState,
		"to_state":    ev.ToState,
		"reason":      ev.Reason,
		"actor":       ev.Actor,
		"occurred_at": ev.OccurredAt,
	}
	for k, v := range ev.Payload {
		payload[k] = v
	}
	if err := s.Publisher.Publish(ctx, "workflow.audit_event", payload); err != nil {
		s.mu.Lock()
		s.lastErr = err
		s.mu.Unlock()
		return fmt.Errorf("engram audit: %w", err)
	}
	return nil
}

// LastErr returns the last error reported by the publisher. Useful in
// tests to assert a failure was observed without aborting the run.
func (s *EngramAuditSink) LastErr() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.lastErr
}
